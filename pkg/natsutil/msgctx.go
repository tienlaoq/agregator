package natsutil

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

// msgIDKey — приватный тип ключа контекста для NATS Nats-Msg-Id.
// Изолирован от коллизий с ключами других пакетов.
type msgIDKey struct{}

// MsgContext создаёт context.Context с таймаутом для обработки одного NATS сообщения.
//
// Почему не пробрасывается родительский контекст: NATS-хендлер вызывается из горутины
// библиотеки nats.go — у неё нет прикладного контекста. context.Background() — правильная
// точка отсчёта; таймаут гарантирует что зависший downstream (gRPC, DB) не заблокирует
// consumer-горутину навсегда.
//
// Trace correlation: если сообщение содержит заголовок Nats-Msg-Id, его значение
// кладётся в контекст — достать через MsgIDFromCtx. Это позволяет логировать
// msg_id без полноценного OpenTelemetry.
//
// Вызывающий обязан вызвать cancel (обычно defer cancel()) после завершения обработки.
func MsgContext(msg *nats.Msg, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Пробрасываем Nats-Msg-Id для корреляции в логах.
	if msg.Header != nil {
		if id := msg.Header.Get("Nats-Msg-Id"); id != "" {
			ctx = context.WithValue(ctx, msgIDKey{}, id)
		}
	}

	return ctx, cancel
}

// MsgIDFromCtx возвращает Nats-Msg-Id из контекста, или пустую строку если не задан.
func MsgIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(msgIDKey{}).(string); ok {
		return v
	}
	return ""
}
