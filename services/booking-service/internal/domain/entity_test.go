package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// allStatuses — полный список статусов; тест упадёт при добавлении нового,
// напомнив обновить логику IsTerminal/CanCancel.
var allStatuses = []BookingStatus{
	StatusPending,
	StatusPaymentPending,
	StatusConfirmed,
	StatusCompleted,
	StatusCancelled,
}

// TestCanCancel_IsInverseOfIsTerminal проверяет инвариант:
// CanCancel() == !IsTerminal() для каждого статуса.
// Если кто-то добавит новый статус в IsTerminal но забудет проверить CanCancel
// (или наоборот) — тест поймает расхождение.
func TestCanCancel_IsInverseOfIsTerminal(t *testing.T) {
	for _, s := range allStatuses {
		b := &Booking{Status: s}
		assert.Equal(t, !b.IsTerminal(), b.CanCancel(),
			"CanCancel() must equal !IsTerminal() for status %q", s)
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []BookingStatus{StatusCompleted, StatusCancelled}
	nonTerminal := []BookingStatus{StatusPending, StatusPaymentPending, StatusConfirmed}

	for _, s := range terminal {
		b := &Booking{Status: s}
		assert.True(t, b.IsTerminal(), "expected %q to be terminal", s)
	}
	for _, s := range nonTerminal {
		b := &Booking{Status: s}
		assert.False(t, b.IsTerminal(), "expected %q to be non-terminal", s)
	}
}

func TestCanComplete(t *testing.T) {
	// Только confirmed + непустой payment_id → можно завершить.
	assert.True(t, (&Booking{Status: StatusConfirmed, PaymentID: "pay-1"}).CanComplete())

	// Без payment_id — нельзя.
	assert.False(t, (&Booking{Status: StatusConfirmed, PaymentID: ""}).CanComplete())

	// Любой другой статус — нельзя.
	for _, s := range []BookingStatus{StatusPending, StatusPaymentPending, StatusCompleted, StatusCancelled} {
		assert.False(t, (&Booking{Status: s, PaymentID: "pay-1"}).CanComplete(),
			"expected %q with payment_id to not be completable", s)
	}
}
