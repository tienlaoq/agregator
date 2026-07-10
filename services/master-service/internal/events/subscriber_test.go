package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
	"github.com/tienlao/agregator/services/master-service/internal/usecase"
)

func TestNakBackoff(t *testing.T) {
	tests := []struct {
		numDelivered uint64
		want         time.Duration
	}{
		{0, 5 * time.Second},
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{4, 40 * time.Second},
		{5, 80 * time.Second},
		{6, 160 * time.Second},
		{7, 5 * time.Minute},  // capped
		{50, 5 * time.Minute}, // stays capped, no overflow
	}
	for _, tt := range tests {
		if got := nakBackoff(tt.numDelivered); got != tt.want {
			t.Errorf("nakBackoff(%d) = %v, want %v", tt.numDelivered, got, tt.want)
		}
	}
}

func TestTruncatePaymentID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"short", "short"},
		{"12345678", "12345678"},          // exactly 8 → unchanged
		{"123456789", "12345678…"},        // 9 → truncated with ellipsis
		{"abcdefghijklmnop", "abcdefgh…"}, // long
	}
	for _, tt := range tests {
		if got := truncatePaymentID(tt.in); got != tt.want {
			t.Errorf("truncatePaymentID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- handler behaviour ---

type bookingRepo struct {
	domain.MasterRepository

	confirmCalls []paymentCall
	cancelCalls  []paymentCall
	confirmErr   error
	cancelErr    error
}

type paymentCall struct {
	bookingID uuid.UUID
	paymentID string
}

func (r *bookingRepo) ConfirmBookingByPayment(_ context.Context, bid uuid.UUID, paymentID string) error {
	r.confirmCalls = append(r.confirmCalls, paymentCall{bid, paymentID})
	return r.confirmErr
}

func (r *bookingRepo) CancelBookingByPayment(_ context.Context, bid uuid.UUID, paymentID string) error {
	r.cancelCalls = append(r.cancelCalls, paymentCall{bid, paymentID})
	return r.cancelErr
}

type stubPayment struct{}

func (stubPayment) CreatePayment(context.Context, *paymentv1.CreatePaymentRequest, ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
	return nil, nil
}

func newSubscriber(repo domain.MasterRepository) *Subscriber {
	uc := usecase.NewMasterUseCase(repo, stubPayment{}, zerolog.Nop())
	return NewSubscriber(nil, uc, zerolog.Nop(), metrics.New("master-service-test"))
}

func msgFor(t *testing.T, subject string, evt paymentEvent) *nats.Msg {
	t.Helper()
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return &nats.Msg{Subject: subject, Data: data}
}

func TestHandlePaymentCompleted_ConfirmsBooking(t *testing.T) {
	bid := uuid.New()
	repo := &bookingRepo{}
	s := newSubscriber(repo)

	s.handlePaymentCompleted(msgFor(t, "payment.completed", paymentEvent{
		PaymentID: "pay-123456789",
		BookingID: bid.String(),
		Status:    paymentStatusCompleted,
	}))

	if len(repo.confirmCalls) != 1 {
		t.Fatalf("ConfirmBookingByPayment called %d times, want 1", len(repo.confirmCalls))
	}
	if repo.confirmCalls[0].bookingID != bid || repo.confirmCalls[0].paymentID != "pay-123456789" {
		t.Errorf("confirm called with %+v, want bid=%s pay-123456789", repo.confirmCalls[0], bid)
	}
}

func TestHandlePaymentCompleted_SkipsOnBadInput(t *testing.T) {
	tests := []struct {
		name string
		msg  *nats.Msg
	}{
		{"malformed json", &nats.Msg{Subject: "payment.completed", Data: []byte("{not json")}},
		{"status mismatch", mustMsg("payment.completed", paymentEvent{PaymentID: "p", BookingID: uuid.NewString(), Status: "failed"})},
		{"empty booking id", mustMsg("payment.completed", paymentEvent{PaymentID: "p", BookingID: "", Status: "completed"})},
		{"empty payment id", mustMsg("payment.completed", paymentEvent{PaymentID: "", BookingID: uuid.NewString(), Status: "completed"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &bookingRepo{}
			newSubscriber(repo).handlePaymentCompleted(tt.msg)
			if len(repo.confirmCalls) != 0 {
				t.Errorf("usecase must not be called for %s, got %d calls", tt.name, len(repo.confirmCalls))
			}
		})
	}
}

func TestHandlePaymentFailed_CancelsBooking(t *testing.T) {
	bid := uuid.New()
	repo := &bookingRepo{}
	s := newSubscriber(repo)

	s.handlePaymentFailed(msgFor(t, "payment.failed", paymentEvent{
		PaymentID: "pay-987654321",
		BookingID: bid.String(),
		Status:    paymentStatusFailed,
	}))

	if len(repo.cancelCalls) != 1 {
		t.Fatalf("CancelBookingByPayment called %d times, want 1", len(repo.cancelCalls))
	}
	if repo.cancelCalls[0].bookingID != bid {
		t.Errorf("cancel called with bid=%s, want %s", repo.cancelCalls[0].bookingID, bid)
	}
}

func TestHandlePaymentFailed_SkipsOnStatusMismatch(t *testing.T) {
	repo := &bookingRepo{}
	newSubscriber(repo).handlePaymentFailed(mustMsg("payment.failed", paymentEvent{
		PaymentID: "p", BookingID: uuid.NewString(), Status: "completed",
	}))
	if len(repo.cancelCalls) != 0 {
		t.Errorf("usecase must not be called on status mismatch, got %d calls", len(repo.cancelCalls))
	}
}

// TestHandlePaymentCompleted_ConfirmErrorPathsDoNotPanic drives the sentinel and
// transient error branches (Ack vs Nak) to ensure the error switch is exercised
// without a live NATS connection.
func TestHandlePaymentCompleted_ErrorPathsDoNotPanic(t *testing.T) {
	for _, confirmErr := range []error{
		domain.ErrBookingNotPending,
		domain.ErrPaymentMismatch,
		domain.ErrInvalidArgument,
		domain.ErrNotFound,
		context.DeadlineExceeded, // generic transient
	} {
		repo := &bookingRepo{confirmErr: confirmErr}
		newSubscriber(repo).handlePaymentCompleted(msgFor(t, "payment.completed", paymentEvent{
			PaymentID: "pay-1",
			BookingID: uuid.NewString(),
			Status:    paymentStatusCompleted,
		}))
		if len(repo.confirmCalls) != 1 {
			t.Errorf("confirm should be attempted once for err=%v", confirmErr)
		}
	}
}

// TestHandlePaymentFailed_SkipsOnBadInput mirrors the completed variant: every
// malformed/mismatched payload must Ack without touching the usecase.
func TestHandlePaymentFailed_SkipsOnBadInput(t *testing.T) {
	tests := []struct {
		name string
		msg  *nats.Msg
	}{
		{"malformed json", &nats.Msg{Subject: "payment.failed", Data: []byte("{not json")}},
		{"empty booking id", mustMsg("payment.failed", paymentEvent{PaymentID: "p", BookingID: "", Status: "failed"})},
		{"empty payment id", mustMsg("payment.failed", paymentEvent{PaymentID: "", BookingID: uuid.NewString(), Status: "failed"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &bookingRepo{}
			newSubscriber(repo).handlePaymentFailed(tt.msg)
			if len(repo.cancelCalls) != 0 {
				t.Errorf("usecase must not be called for %s, got %d calls", tt.name, len(repo.cancelCalls))
			}
		})
	}
}

// TestHandlePaymentFailed_ErrorPathsDoNotPanic drives the sentinel and transient
// error branches (Ack vs Nak) of the failed handler's error switch.
func TestHandlePaymentFailed_ErrorPathsDoNotPanic(t *testing.T) {
	for _, cancelErr := range []error{
		domain.ErrBookingNotPending,
		domain.ErrPaymentMismatch,
		domain.ErrInvalidArgument,
		domain.ErrNotFound,
		context.DeadlineExceeded, // generic transient
	} {
		repo := &bookingRepo{cancelErr: cancelErr}
		newSubscriber(repo).handlePaymentFailed(msgFor(t, "payment.failed", paymentEvent{
			PaymentID: "pay-1",
			BookingID: uuid.NewString(),
			Status:    paymentStatusFailed,
		}))
		if len(repo.cancelCalls) != 1 {
			t.Errorf("cancel should be attempted once for err=%v", cancelErr)
		}
	}
}

func mustMsg(subject string, evt paymentEvent) *nats.Msg {
	data, _ := json.Marshal(evt)
	return &nats.Msg{Subject: subject, Data: data}
}
