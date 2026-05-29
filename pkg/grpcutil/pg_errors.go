package grpcutil

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pgInvalidArgCodes — коды PostgreSQL которые означают «клиент прислал невалидные данные»,
// а не внутреннюю ошибку сервера. Маппируются в codes.InvalidArgument.
//
//   - 22P02  invalid_text_representation  — "invalid input syntax for type uuid: ..."
//   - 22023  invalid_parameter_value      — тоже возникает при плохих uuid через ANY($1::uuid[])
//   - 22007  invalid_datetime_format      — плохой формат даты/времени
//   - 22008  datetime_field_overflow      — дата вне допустимого диапазона
var pgInvalidArgCodes = map[string]bool{
	"22P02": true, // invalid_text_representation
	"22023": true, // invalid_parameter_value
	"22007": true, // invalid_datetime_format
	"22008": true, // datetime_field_overflow
}

// pgAlreadyExistsCodes — коды PostgreSQL которые означают конфликт уникальности.
// Маппируются в codes.AlreadyExists.
//
//   - 23505  unique_violation  — INSERT/UPDATE нарушил UNIQUE-ограничение
var pgAlreadyExistsCodes = map[string]bool{
	"23505": true, // unique_violation
}

// pgErrToStatus конвертирует pgconn.PgError в gRPC status если код входит в
// один из известных наборов; иначе возвращает nil (вызывающий должен обработать сам).
func pgErrToStatus(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	if pgInvalidArgCodes[pgErr.Code] {
		return status.Errorf(codes.InvalidArgument, "invalid argument: %s", pgErr.Message)
	}
	if pgAlreadyExistsCodes[pgErr.Code] {
		// Do NOT include pgErr.Detail in the user-facing message. PostgreSQL
		// formats Detail as "Key (column)=(value) already exists." which
		// leaks the column name AND the conflicting value — which is often
		// PII (email, phone) or otherwise sensitive (booking slot, user id).
		// Services that want to expose a human-friendly conflict message
		// should map their own domain-level sentinel error in the usecase /
		// delivery layer before this interceptor sees the raw pg error.
		return status.Error(codes.AlreadyExists, "already exists")
	}
	return nil
}

// PgErrorUnaryInterceptor — серверный unary-interceptor, который конвертирует
// PostgreSQL ошибки типа "неверный формат UUID/даты" из Internal в InvalidArgument.
//
// Порядок проверки:
//  1. Если в цепочке errors.As есть pgconn.PgError с кодом из pgInvalidArgCodes —
//     заменяем на InvalidArgument независимо от того, обёрнута ли ошибка в gRPC status.
//  2. Иначе возвращаем ошибку как есть (gRPC framework сам смаппирует в Internal).
//
// Добавляется один раз при создании grpc.Server и покрывает все методы сервиса:
//
//	grpc.ChainUnaryInterceptor(
//	    grpcutil.PgErrorUnaryInterceptor(),
//	    grpcutil.CallerIDServerInterceptor(),
//	)
func PgErrorUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		// pgErrToStatus проверяет errors.As(err, &pgconn.PgError) — работает
		// сквозь любую глубину fmt.Errorf(%w) обёрток, в том числе через gRPC
		// status-ошибки которые не реализуют Unwrap и поэтому pgconn.PgError
		// там не будет. Это правильное поведение: если handler вернул уже
		// корректный gRPC status (pkgerr.NotFound и т.д.) — pgErr там нет,
		// pgErrToStatus вернёт nil и мы пропустим ошибку без изменений.
		if mapped := pgErrToStatus(err); mapped != nil {
			return nil, mapped
		}
		return nil, err
	}
}

// SafePgErrorUnaryInterceptor converts known PostgreSQL errors without
// exposing pgErr.Detail to clients.
func SafePgErrorUnaryInterceptor() grpc.UnaryServerInterceptor {
	return PgErrorUnaryInterceptor()
}
