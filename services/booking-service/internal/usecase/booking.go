package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
	"github.com/tienlao/agregator/services/booking-service/internal/kpi"
)

type EventPublisher interface {
	// PublishBookingCompleted — прямой путь для booking.completed (защищён
	// completed_event_sent_at, не идёт через outbox).
	PublishBookingCompleted(ctx context.Context, b *domain.Booking) error
	// PublishRaw публикует staged-событие из outbox (booking.created/confirmed/cancelled).
	PublishRaw(ctx context.Context, subject string, payload []byte) error
}

type BookingUseCase struct {
	repo                domain.BookingRepository
	venueClient         venuev1.VenueServiceClient
	crmClient           crmv1.CRMServiceClient
	paymentClient       paymentv1.PaymentServiceClient
	publisher           EventPublisher
	log                 zerolog.Logger
	visitTimeZone       string
	cancelDeadlineHours int
}

func NewBookingUseCase(
	repo domain.BookingRepository,
	venueClient venuev1.VenueServiceClient,
	crmClient crmv1.CRMServiceClient,
	paymentClient paymentv1.PaymentServiceClient,
	publisher EventPublisher,
	log zerolog.Logger,
	visitTimeZone string,
	cancelDeadlineHours int,
) *BookingUseCase {
	if visitTimeZone == "" {
		visitTimeZone = "Europe/Moscow"
	}
	if cancelDeadlineHours < 0 {
		cancelDeadlineHours = 0
	}
	return &BookingUseCase{
		repo:                repo,
		venueClient:         venueClient,
		crmClient:           crmClient,
		paymentClient:       paymentClient,
		publisher:           publisher,
		log:                 log,
		visitTimeZone:       visitTimeZone,
		cancelDeadlineHours: cancelDeadlineHours,
	}
}

// outboxBatchSize — сколько неотправленных событий поллер забирает за тик.
const outboxBatchSize = 100

// ProcessOutbox публикует staged-события booking.* из транзакционного outbox.
// Запускается на отдельном коротком тикере (см. cmd/main.go). Каждое событие
// публикуется в NATS ДО того как его строка помечается sent — поэтому падение
// между publish и mark приводит лишь к повторной публикации на следующем тике
// (at-least-once; консьюмеры идемпотентны по booking_id).
//
// При ошибке publish цикл прерывается: NATS вероятно недоступен, а порядок FIFO
// требует не перескакивать через застрявшее событие. Уже опубликованный префикс
// помечается sent; остальное повторится. См. TECH_DEBT [BOOKING-PUBLISH-LOSS].
func (uc *BookingUseCase) ProcessOutbox(ctx context.Context, limit int) (int, error) {
	events, err := uc.repo.FetchUnsentOutbox(ctx, limit)
	if err != nil {
		return 0, err
	}
	sent := make([]int64, 0, len(events))
	for _, ev := range events {
		if perr := uc.publisher.PublishRaw(ctx, ev.Subject, ev.Payload); perr != nil {
			uc.log.Error().Err(perr).
				Str("subject", ev.Subject).
				Int64("outbox_id", ev.ID).
				Msg("booking: outbox publish failed — will retry next tick")
			break
		}
		sent = append(sent, ev.ID)
	}
	if len(sent) == 0 {
		return 0, nil
	}
	if err := uc.repo.MarkOutboxSent(ctx, sent); err != nil {
		// События опубликованы, но не помечены — повторятся на следующем тике.
		uc.log.Error().Err(err).Int("count", len(sent)).
			Msg("booking: outbox mark-sent failed — events will be republished")
		return len(sent), err
	}
	return len(sent), nil
}

type CreateBookingInput struct {
	UserID     string
	VenueID    string
	VenueName  string
	ServiceID  string
	ServiceIDs []string
	HallIDs    []string
	Date       string
	// TimeFrom/TimeTo — строки "HH:MM" с proto-границы; валидируются через ParseTimeOfDay
	// внутри CreateBooking до любых внешних вызовов.
	TimeFrom string
	TimeTo   string
	Guests   int32
	Comment  string
}

func (uc *BookingUseCase) CreateBooking(ctx context.Context, in CreateBookingInput) (*domain.Booking, error) {
	// Парсинг и валидация времени — единственное место в сервисе где "HH:MM" → TimeOfDay.
	timeFrom, err := domain.ParseTimeOfDay(in.TimeFrom)
	if err != nil {
		return nil, pkgerr.InvalidArgument("неверное время начала: " + err.Error())
	}
	timeTo, err := domain.ParseTimeOfDay(in.TimeTo)
	if err != nil {
		return nil, pkgerr.InvalidArgument("неверное время окончания: " + err.Error())
	}
	minutes, ok := timeFrom.MinutesUntil(timeTo)
	if !ok {
		return nil, pkgerr.InvalidArgument("время окончания должно быть позже времени начала")
	}
	if minutes < 30 {
		return nil, pkgerr.InvalidArgument("минимальная длительность брони 30 минут")
	}
	if minutes > 720 {
		return nil, pkgerr.InvalidArgument("максимальная длительность брони 12 часов")
	}

	vResp, err := uc.venueClient.GetVenue(ctx, &venuev1.GetVenueRequest{Id: in.VenueID})
	if err != nil {
		return nil, fmt.Errorf("get venue: %w", err)
	}
	if vResp == nil {
		return nil, pkgerr.NotFound("заведение не найдено")
	}

	effectiveIDs := normalizeBookingServiceIDs(in.ServiceIDs, in.ServiceID)
	hallIDs := normalizeBookingHallIDs(in.HallIDs)
	if err := validateHallIDs(vResp, hallIDs); err != nil {
		return nil, err
	}
	totalPrice, err := computeBookingTotalPriceMulti(vResp, effectiveIDs, hallIDs, minutes)
	if err != nil {
		return nil, err
	}

	// Быстрая pre-check без блокировки: отсекает очевидно занятые слоты раньше,
	// чем мы дойдём до ReserveSlot, и даёт более понятный ответ клиенту.
	slotResp, err := uc.venueClient.CheckSlotAvailability(ctx, &venuev1.CheckSlotRequest{
		VenueId:  in.VenueID,
		Date:     in.Date,
		TimeFrom: in.TimeFrom,
		TimeTo:   in.TimeTo,
	})
	if err != nil {
		return nil, fmt.Errorf("check slot: %w", err)
	}
	if !slotResp.Available {
		return nil, pkgerr.InvalidArgument("выбранное время уже занято, выберите другое")
	}

	dateParsed, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		return nil, pkgerr.InvalidArgument("invalid date format, expected YYYY-MM-DD")
	}

	loc, lerr := time.LoadLocation(uc.visitTimeZone)
	if lerr != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	visitStart := time.Date(
		dateParsed.Year(), dateParsed.Month(), dateParsed.Day(),
		0, 0, 0, 0, loc,
	)
	if visitStart.Before(todayStart) {
		return nil, pkgerr.InvalidArgument("дата визита не может быть в прошлом")
	}

	svcID, pkgIDs := persistServiceFields(effectiveIDs)
	b := &domain.Booking{
		UserID:            in.UserID,
		VenueID:           in.VenueID,
		VenueName:         in.VenueName,
		ServiceID:         svcID,
		PackageServiceIDs: pkgIDs,
		HallIDs:           hallIDs,
		Date:              dateParsed,
		TimeFrom:          timeFrom,
		TimeTo:            timeTo,
		Guests:            in.Guests,
		Comment:           in.Comment,
		Status:            domain.StatusPending,
		TotalPrice:        totalPrice,
	}

	// Порядок внешних вызовов:
	//
	//  1. repo.Create          — INSERT со status='pending', получаем booking_id.
	//                            EXCLUDE constraint (005_booking_slot_exclusion) даёт
	//                            атомарную защиту от двойной брони на уровне PG.
	//  2. ReserveSlot          — захват слота в venue-service с реальным booking_id.
	//                            Компенсация при ошибке: repo.Delete.
	//  3. CreatePayment        — создание платежа. Компенсация при ошибке:
	//                            ReleaseSlot + repo.Delete.
	//  4. repo.SetPaymentAndStatus — один UPDATE: payment_id + payment_url + status='payment_pending'.
	//                            Компенсация при ошибке: ReleaseSlot + repo.Delete.
	//                            Если этот UPDATE упадёт — платёж уже создан, но бронь
	//                            остаётся в 'pending' без payment_id. Это устраняемо
	//                            reaper-джобом (см. TECH_DEBT [BOOKING-ORPHAN]) или
	//                            добавлением CancelPayment RPC (см. [BOOKING-ORPHAN-PAYMENT]).

	// Шаг 1: атомарная запись в БД — защита от двойной брони через EXCLUDE.
	if err := uc.repo.Create(ctx, b); err != nil {
		return nil, fmt.Errorf("create booking: %w", err)
	}

	// Шаг 2: захват слота в venue-service.
	_, err = uc.venueClient.ReserveSlot(ctx, &venuev1.ReserveSlotRequest{
		VenueId:   in.VenueID,
		BookingId: b.ID,
		Date:      in.Date,
		TimeFrom:  in.TimeFrom,
		TimeTo:    in.TimeTo,
	})
	if err != nil {
		_ = uc.repo.Delete(ctx, b.ID)
		if st, ok := status.FromError(err); ok && st.Code() == codes.InvalidArgument {
			return nil, pkgerr.InvalidArgument("выбранное время уже занято, выберите другое")
		}
		return nil, fmt.Errorf("reserve slot: %w", err)
	}

	// Шаг 3: создание платежа.
	//
	// Idempotency key derivation: sha256(booking_id + ":" + user_id).
	// Binding the key to both the booking and the user prevents idempotency-key
	// squatting — a different user cannot pre-register the same key and intercept
	// this payment.  The booking_id is a server-generated UUID (unguessable from
	// outside) so the combined hash is unpredictable even without a separate nonce.
	idemKey := bookingIdempotencyKey(b.ID, b.UserID)
	// ServiceAt = absolute visit-start moment in the venue's timezone.
	// payment-service uses it to compute the payout hold window
	// (available_at = service_at + PAYOUT_HOLD_HOURS).
	serviceAt := b.TimeFrom.ToTimeIn(b.Date, loc)
	payResp, err := uc.paymentClient.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
		BookingId:        b.ID,
		Amount:           b.TotalPrice,
		Description:      fmt.Sprintf("Booking %s", b.ID),
		IdempotencyKey:   idemKey,
		CounterpartyType: "venue",
		CounterpartyId:   in.VenueID,
		ServiceAt:        timestamppb.New(serviceAt),
	})
	if err != nil {
		_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
			VenueId:   in.VenueID,
			BookingId: b.ID,
		})
		_ = uc.repo.Delete(ctx, b.ID)
		return nil, fmt.Errorf("create payment: %w", err)
	}

	b.Status = domain.StatusPaymentPending
	b.PaymentID = payResp.Id
	b.PaymentURL = payResp.PaymentUrl

	// booking.created пишется в outbox в той же транзакции, что и финализирующий
	// UPDATE — событие не может потеряться при недоступном NATS.
	createdEvent, err := domain.NewBookingEvent("booking.created", b)
	if err != nil {
		_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
			VenueId:   in.VenueID,
			BookingId: b.ID,
		})
		_ = uc.repo.Delete(ctx, b.ID)
		return nil, fmt.Errorf("build created event: %w", err)
	}

	// Шаг 4: один атомарный UPDATE (payment_id + status) + INSERT в outbox.
	if err := uc.repo.SetPaymentAndStatus(ctx, b.ID, payResp.Id, payResp.PaymentUrl, string(domain.StatusPaymentPending), createdEvent); err != nil {
		_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
			VenueId:   in.VenueID,
			BookingId: b.ID,
		})
		_ = uc.repo.Delete(ctx, b.ID)
		return nil, fmt.Errorf("finalize booking: %w", err)
	}
	// KPI считает доменный переход (БД закоммичена), независимо от факта публикации.
	kpi.BookingEvent("created")

	return b, nil
}

func (uc *BookingUseCase) GetBooking(ctx context.Context, id string) (*domain.Booking, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *BookingUseCase) GetBookingsBatch(ctx context.Context, ids []string) ([]*domain.Booking, error) {
	return uc.repo.GetByIDs(ctx, ids)
}

func (uc *BookingUseCase) ListUserBookings(ctx context.Context, userID, statusFilter string, page, pageSize int32) ([]*domain.Booking, int, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int((page - 1) * pageSize)
	return uc.repo.ListByUser(ctx, userID, statusFilter, offset, int(pageSize))
}

func (uc *BookingUseCase) requireVenueCRMAccess(ctx context.Context, venueID, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return pkgerr.InvalidArgument("user id is required")
	}
	resp, err := uc.crmClient.GetManagementAccess(ctx, &crmv1.GetManagementAccessRequest{
		VenueId: venueID,
		UserId:  userID,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(resp.GetAccess()) == "" {
		return pkgerr.PermissionDenied("no venue management access")
	}
	return nil
}

func (uc *BookingUseCase) ListVenueBookings(ctx context.Context, venueID, requesterUserID, statusFilter, date, cursor string, pageSize int32) ([]*domain.Booking, int, string, error) {
	if err := uc.requireVenueCRMAccess(ctx, venueID, requesterUserID); err != nil {
		return nil, 0, "", err
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return uc.repo.ListByVenue(ctx, venueID, statusFilter, date, cursor, int(pageSize))
}

const maxBookingStaffNoteRunes = 4000

func (uc *BookingUseCase) ListBookingStaffNotes(ctx context.Context, bookingID, requesterUserID string) ([]domain.BookingStaffNote, error) {
	b, err := uc.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if err := uc.requireVenueCRMAccess(ctx, b.VenueID, requesterUserID); err != nil {
		return nil, err
	}
	return uc.repo.ListBookingStaffNotes(ctx, bookingID)
}

func (uc *BookingUseCase) AddBookingStaffNote(ctx context.Context, bookingID, requesterUserID, body string) (*domain.BookingStaffNote, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, pkgerr.InvalidArgument("текст заметки обязателен")
	}
	if utf8.RuneCountInString(body) > maxBookingStaffNoteRunes {
		return nil, pkgerr.InvalidArgument("текст заметки слишком длинный")
	}
	b, err := uc.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if err := uc.requireVenueCRMAccess(ctx, b.VenueID, requesterUserID); err != nil {
		return nil, err
	}
	n := &domain.BookingStaffNote{
		BookingID:    b.ID,
		VenueID:      b.VenueID,
		AuthorUserID: requesterUserID,
		Body:         body,
	}
	if err := uc.repo.AddBookingStaffNote(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (uc *BookingUseCase) CancelBooking(ctx context.Context, id, userID string) (*domain.Booking, error) {
	b, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.UserID != userID {
		return nil, pkgerr.PermissionDenied("not your booking")
	}
	if !b.CanCancel() {
		return nil, pkgerr.InvalidArgument("booking cannot be cancelled in current status")
	}

	// Проверка дедлайна отмены: запрещаем отмену, если до начала визита осталось
	// меньше cancelDeadlineHours. 0 означает «без ограничений» (только для тестов).
	if uc.cancelDeadlineHours > 0 {
		loc, lerr := time.LoadLocation(uc.visitTimeZone)
		if lerr != nil {
			loc = time.UTC
		}
		visitStart := b.TimeFrom.ToTimeIn(b.Date, loc)
		deadline := visitStart.Add(-time.Duration(uc.cancelDeadlineHours) * time.Hour)
		if time.Now().In(loc).After(deadline) {
			return nil, pkgerr.InvalidArgument(
				fmt.Sprintf("отмена невозможна: до начала визита менее %d ч.", uc.cancelDeadlineHours),
			)
		}
	}

	b.Status = domain.StatusCancelled
	// booking.cancelled триггерит рефанд в payment-service — пишем его в outbox в
	// той же транзакции, что и UPDATE, чтобы рефанд не потерялся при сбое NATS.
	cancelledEvent, err := domain.NewBookingEvent("booking.cancelled", b)
	if err != nil {
		return nil, fmt.Errorf("build cancelled event: %w", err)
	}
	if err := uc.repo.CancelWithEvent(ctx, id, cancelledEvent); err != nil {
		return nil, err
	}
	kpi.BookingEvent("cancelled")

	_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
		VenueId:   b.VenueID,
		BookingId: b.ID,
	})

	return b, nil
}

// ConfirmBooking переводит бронь в статус confirmed.
// Вызывается из обработчика payment.completed (NATS) — может прийти дважды при редублировании.
// Идемпотентно: если бронь уже confirmed/completed/cancelled — publish не делается, ошибки нет.
// Атомарность: один UPDATE … WHERE status='payment_pending' исключает двойное подтверждение
// при параллельных вызовах (второй UPDATE даст RowsAffected=0 и не опубликует событие).
func (uc *BookingUseCase) ConfirmBooking(ctx context.Context, id, paymentID string) error {
	// При успешном переходе ConfirmPayment пишет booking.confirmed в outbox в той
	// же транзакции, что и UPDATE status='confirmed'.
	_, confirmed, err := uc.repo.ConfirmPayment(ctx, id, paymentID, "booking.confirmed")
	if err != nil {
		return err
	}
	if !confirmed {
		// Бронь уже в терминальном статусе — событие не пишем (идемпотентность).
		return nil
	}
	kpi.BookingEvent("confirmed")
	return nil
}

func (uc *BookingUseCase) CompleteBooking(ctx context.Context, id string) (*domain.Booking, error) {
	b, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !b.CanComplete() {
		return nil, pkgerr.InvalidArgument("only confirmed bookings with payment can be completed")
	}
	if err := uc.repo.UpdateStatus(ctx, id, string(domain.StatusCompleted)); err != nil {
		return nil, err
	}
	b.Status = domain.StatusCompleted
	return b, nil
}

func (uc *BookingUseCase) CancelBookingByPayment(ctx context.Context, bookingID string) error {
	b, err := uc.repo.GetByID(ctx, bookingID)
	if err != nil {
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.NotFound {
			return nil
		}
		return err
	}
	if b.Status != domain.StatusPaymentPending {
		return nil
	}

	b.Status = domain.StatusCancelled
	cancelledEvent, err := domain.NewBookingEvent("booking.cancelled", b)
	if err != nil {
		return fmt.Errorf("build cancelled event: %w", err)
	}
	if err := uc.repo.CancelWithEvent(ctx, bookingID, cancelledEvent); err != nil {
		return err
	}
	kpi.BookingEvent("cancelled")

	_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
		VenueId:   b.VenueID,
		BookingId: b.ID,
	})
	return nil
}

func (uc *BookingUseCase) HasCompletedBooking(ctx context.Context, userID, venueID string) (bool, error) {
	return uc.repo.HasCompleted(ctx, userID, venueID)
}

func hourlyTotal(priceFrom int64, minutes int) int64 {
	if minutes <= 0 || priceFrom <= 0 {
		return 0
	}
	h := (minutes + 59) / 60
	if h < 1 {
		h = 1
	}
	return priceFrom * int64(h)
}

func normalizeBookingHallIDs(fromList []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range fromList {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// validateHallIDs проверяет, что все переданные hall_id принадлежат заведению.
//
// Статус архивирования/отключения залов не проверяется: proto VenueHall (venue/v1/venue.proto)
// не содержит поля archived/disabled/is_active — функциональности нет на уровне схемы.
// TODO: добавить проверку h.GetIsActive() == false → InvalidArgument, когда поле появится в proto.
func validateHallIDs(v *venuev1.VenueResponse, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(v.GetHalls()))
	for _, h := range v.GetHalls() {
		known[h.GetId()] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return pkgerr.InvalidArgument("выбранный зал не найден в этом заведении")
		}
	}
	return nil
}

func effectiveHourlyPriceFromVenueAndHalls(v *venuev1.VenueResponse, hallIDs []string) int64 {
	base := v.GetPriceFrom()
	if len(hallIDs) == 0 {
		return base
	}
	// Строим карту id→priceFrom один раз — O(venueHalls) вместо O(hallIDs×venueHalls).
	// Типичный случай: 1-2 выбранных зала из ~5-50 в заведении.
	hallPrice := make(map[string]int64, len(v.GetHalls()))
	for _, h := range v.GetHalls() {
		hallPrice[h.GetId()] = h.GetPriceFrom()
	}
	for _, hid := range hallIDs {
		hid = strings.TrimSpace(hid)
		if hid == "" {
			continue
		}
		if pf := hallPrice[hid]; pf > base {
			base = pf
		}
	}
	return base
}

func normalizeBookingServiceIDs(fromList []string, legacy string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range fromList {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > 0 {
		return out
	}
	if id := strings.TrimSpace(legacy); id != "" {
		return []string{id}
	}
	return nil
}

func persistServiceFields(ids []string) (serviceID string, packageIDs []string) {
	switch len(ids) {
	case 0:
		return "", nil
	case 1:
		return ids[0], nil
	default:
		return "", ids
	}
}

func computeBookingTotalPriceMulti(v *venuev1.VenueResponse, serviceIDs []string, hallIDs []string, minutes int) (int64, error) {
	hourlyBase := effectiveHourlyPriceFromVenueAndHalls(v, hallIDs)
	if len(serviceIDs) == 0 {
		return hourlyTotal(hourlyBase, minutes), nil
	}
	var sum int64
	hourlyParts := 0
	for _, sid := range serviceIDs {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}
		var found bool
		for _, s := range v.GetServices() {
			if s.GetId() != sid {
				continue
			}
			found = true
			if p := s.GetPrice(); p > 0 {
				sum += p
			} else {
				hourlyParts++
				sum += hourlyTotal(hourlyBase, minutes)
			}
			break
		}
		if !found {
			return 0, pkgerr.InvalidArgument("выбранная услуга не найдена у этого заведения")
		}
	}
	if hourlyParts > 1 {
		return 0, pkgerr.InvalidArgument("в одной брони можно не более одной услуги без фиксированной цены — выберите пакеты с ценой или только почасовой тариф")
	}
	return sum, nil
}

// orphanPendingMaxAgeMinutes — максимальный возраст pending-брони без payment_id.
// Нормальный путь Create→SetPaymentAndStatus занимает секунды; 15 минут — консервативный
// порог при котором ложных срабатываний быть не может.
// См. TECH_DEBT [BOOKING-ORPHAN].
const orphanPendingMaxAgeMinutes = 15

// AutoCompletePastVisits переводит confirmed → completed после окончания слота,
// публикует booking.completed для каждой затронутой брони,
// и попутно удаляет осиротевшие pending-брони (reaper).
//
// Гарантия at-least-once publish:
//  1. UPDATE status='completed' (completed_event_sent_at остаётся NULL).
//  2. Для каждой строки: publish → MarkCompletedEventSent (метка).
//  3. publishUnpublishedCompleted — catch-up для строк где п.2 прервался.
//  4. Reaper — DELETE pending без payment_id старше 15 минут (BOOKING-ORPHAN).
//
// При падении между UPDATE и publish: следующий вызов AutoCompletePastVisits
// не тронет строку (статус уже completed), но publishUnpublishedCompleted
// найдёт её через partial-индекс и повторит publish.
//
// Downstream (review/notification) должны быть идемпотентны по booking_id.
func (uc *BookingUseCase) AutoCompletePastVisits(ctx context.Context) (int, error) {
	// Шаг 1: перевести все просроченные confirmed → completed.
	refs, err := uc.repo.AutoCompleteVisitEnded(ctx, uc.visitTimeZone)
	if err != nil {
		return 0, err
	}
	// KPI: считаем переходы на шаге 1 (а не на publish), чтобы catch-up-повторы
	// публикации не задваивали счётчик.
	kpi.BookingEvents("completed", len(refs))

	published := 0
	// Шаг 2: publish + метка для только что завершённых броней.
	for _, ref := range refs {
		b := &domain.Booking{ID: ref.ID, UserID: ref.UserID, VenueID: ref.VenueID, Status: domain.StatusCompleted}
		if err := uc.publisher.PublishBookingCompleted(ctx, b); err == nil {
			_ = uc.repo.MarkCompletedEventSent(ctx, ref.ID)
			published++
		}
		// Если publish упал — completed_event_sent_at остаётся NULL;
		// catch-up (шаг 3) подхватит на следующей итерации поллера.
	}

	// Шаг 3: catch-up — найти completed без метки (упали на предыдущих итерациях).
	catchUp, err := uc.repo.FindUnpublishedCompleted(ctx, 100)
	if err != nil {
		// Catch-up не критичен — основная работа (шаги 1-2) уже выполнена.
		// Логируем для видимости, возврат ошибки не нужен.
		uc.log.Error().Err(err).Msg("booking: catch-up FindUnpublishedCompleted failed")
	}
	for _, ref := range catchUp {
		b := &domain.Booking{ID: ref.ID, UserID: ref.UserID, VenueID: ref.VenueID, Status: domain.StatusCompleted}
		if err := uc.publisher.PublishBookingCompleted(ctx, b); err == nil {
			_ = uc.repo.MarkCompletedEventSent(ctx, ref.ID)
			published++
		}
	}

	// Шаг 4: reaper — удалить pending-брони без payment_id старше порога.
	// Такие строки появляются если booking-service упал между repo.Create и
	// repo.SetPaymentAndStatus и компенсационный repo.Delete не отработал.
	// Они блокируют EXCLUDE-слот навсегда — reaper освобождает его.
	if n, err := uc.repo.DeleteOrphanPending(ctx, orphanPendingMaxAgeMinutes); err != nil {
		uc.log.Error().Err(err).Msg("booking: orphan pending reaper failed")
	} else if n > 0 {
		uc.log.Warn().Int64("count", n).Msg("booking: deleted orphan pending bookings — check BOOKING-ORPHAN in TECH_DEBT")
	}

	return published, nil
}

// bookingIdempotencyKey derives a stable, user-bound idempotency key for the
// CreatePayment call.
//
// key = hex(sha256(bookingID + ":" + userID))
//
// Binding both IDs prevents idempotency-key squatting: even if an attacker
// learns the booking UUID, they cannot register the same key on behalf of a
// different user because the key is cryptographically tied to userID.
// The booking UUID is server-generated and unguessable, so the hash is
// unpredictable without a separate nonce.
func bookingIdempotencyKey(bookingID, userID string) string {
	h := sha256.Sum256([]byte(bookingID + ":" + userID))
	return hex.EncodeToString(h[:])
}
