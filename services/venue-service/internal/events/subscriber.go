package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/pkg/natsutil"
)

// handlerTimeout — максимальное время обработки одного NATS сообщения.
// Включает gRPC-вызов к review-service (GetVenueRating) и DB-апдейт рейтинга.
// При превышении контекст отменяется → downstream вернёт DeadlineExceeded → Nak → retry.
const handlerTimeout = 15 * time.Second

// subjectReviewCreated дублирует review-service'овый events.SubjectReviewCreated.
// Пакет events review-сервиса internal — импортировать его нельзя, поэтому
// контракт темы фиксируется здесь как строковая константа. При изменении темы
// в review-service обнови и здесь (тест ловит расхождение по факту простоя).
const subjectReviewCreated = "review.created"

// targetTypeVenue — значение target_type для событий по площадке.
const targetTypeVenue = "venue"

// reviewCreatedEvent — локальная копия контракта review.created. Поля отражают
// JSON, который сериализует review-service (см. его internal/events.ReviewCreatedEvent).
// Декодируем только то, что нужно подписчику venue-service: тип цели и venue_id.
// Поле Rating в событии — оценка одного отзыва; для агрегата мы НЕ доверяем ему,
// а запрашиваем авторитетное значение у review-service (GetVenueRating).
type reviewCreatedEvent struct {
	ReviewID   string `json:"review_id"`
	TargetType string `json:"target_type"`
	VenueID    string `json:"venue_id,omitempty"`
}

// ratingUpdater — то, что подписчик умеет делать с площадкой: записать
// пересчитанный агрегат. Интерфейс объявлен на месте использования
// (*usecase.VenueUseCase его реализует).
type ratingUpdater interface {
	UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
}

// ratingFetcher — источник авторитетного агрегата рейтинга. Реализуется
// reviewv1.ReviewServiceClient. Объявлен здесь, чтобы подписчик не зависел от
// конкретного gRPC-клиента и был тестируемым без сети.
type ratingFetcher interface {
	GetVenueRating(ctx context.Context, in *reviewv1.GetVenueRatingRequest, opts ...grpc.CallOption) (*reviewv1.VenueRatingResponse, error)
}

// Subscriber потребляет события review-service и поддерживает denormalized-рейтинг
// площадок в актуальном состоянии.
type Subscriber struct {
	js      nats.JetStreamContext
	reviews ratingFetcher
	venues  ratingUpdater
	log     zerolog.Logger
}

func NewSubscriber(js nats.JetStreamContext, reviews ratingFetcher, venues ratingUpdater, log zerolog.Logger) *Subscriber {
	return &Subscriber{js: js, reviews: reviews, venues: venues, log: log}
}

// SubscribeReviewEvents подписывается на review.created долговечным push-консьюмером.
// Стрим REVIEWS создаётся review-service'ом; здесь только привязываемся к нему.
func (s *Subscriber) SubscribeReviewEvents() error {
	_, err := s.js.Subscribe(subjectReviewCreated, func(msg *nats.Msg) {
		s.handleReviewCreated(msg)
	}, nats.Durable("venue-review-created"), nats.ManualAck())
	return err
}

// handleReviewCreated пересчитывает рейтинг площадки по событию review.created.
//
// Идемпотентность: обработчик не накапливает состояние — он запрашивает у
// review-service текущий агрегат (avg + count) и записывает его в venue-service
// как абсолютное значение (SET, не increment). Поэтому повторная доставка одного
// и того же события (at-least-once) безопасна и не требует дедупликации по ReviewID:
// повтор просто перезапишет тот же или более свежий агрегат.
func (s *Subscriber) handleReviewCreated(msg *nats.Msg) {
	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()
	msgID := natsutil.MsgIDFromCtx(ctx)

	var evt reviewCreatedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		// Битый payload не починится ретраями — Ack, чтобы не зациклить доставку.
		s.log.Error().Err(err).Str("msg_id", msgID).Msg("unmarshal review.created")
		_ = msg.Ack()
		return
	}

	// Подписчик отвечает только за площадки. События по мастерам игнорируем
	// (их рейтинг ведёт отдельный владелец), но Ack'аем — это не ошибка.
	if evt.TargetType != targetTypeVenue {
		_ = msg.Ack()
		return
	}

	venueID, err := uuid.Parse(evt.VenueID)
	if err != nil {
		s.log.Error().Err(err).Str("msg_id", msgID).Str("venue_id", evt.VenueID).Msg("review.created: invalid venue_id")
		_ = msg.Ack()
		return
	}

	rating, err := s.reviews.GetVenueRating(ctx, &reviewv1.GetVenueRatingRequest{VenueId: evt.VenueID})
	if err != nil {
		s.log.Error().Err(err).Str("msg_id", msgID).Str("venue_id", evt.VenueID).Msg("fetch venue rating from review-service")
		_ = msg.Nak()
		return
	}

	if err := s.venues.UpdateRating(ctx, venueID, rating.GetAvgRating(), rating.GetReviewCount()); err != nil {
		// InvalidArgument/NotFound терминальны — ретрай не поможет, Ack.
		if st, ok := status.FromError(err); ok && (st.Code() == codes.InvalidArgument || st.Code() == codes.NotFound) {
			s.log.Warn().Err(err).Str("msg_id", msgID).Str("venue_id", evt.VenueID).Msg("update venue rating skipped (terminal)")
			_ = msg.Ack()
			return
		}
		s.log.Error().Err(err).Str("msg_id", msgID).Str("venue_id", evt.VenueID).Msg("update venue rating failed")
		_ = msg.Nak()
		return
	}

	s.log.Info().
		Str("msg_id", msgID).
		Str("venue_id", evt.VenueID).
		Float64("avg_rating", rating.GetAvgRating()).
		Int32("review_count", rating.GetReviewCount()).
		Msg("venue rating updated from review.created")
	_ = msg.Ack()
}
