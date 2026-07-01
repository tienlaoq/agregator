//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

// TestCreateAndGet_WithArrays is the regression test for migration 006: the repo
// writes package_service_ids / booking_hall_ids as uuid[] and scans them back
// into []string. Before 006 was fixed these columns were TEXT and this failed.
func TestCreateAndGet_WithArrays(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	b := seedBooking(ctx, t, domain.StatusPending, true)

	got, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	require.Equal(t, b.UserID, got.UserID)
	require.Equal(t, "Тестовая Баня", got.VenueName)
	require.Equal(t, "10:00", got.TimeFrom.String())
	require.Equal(t, "12:00", got.TimeTo.String())
	require.Len(t, got.HallIDs, 1)
	require.Len(t, got.PackageServiceIDs, 2)
	require.ElementsMatch(t, b.PackageServiceIDs, got.PackageServiceIDs)
}

func TestCreateAndGet_NoArrays(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	b := seedBooking(ctx, t, domain.StatusPending, false)

	got, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	require.Empty(t, got.HallIDs)
	require.Empty(t, got.PackageServiceIDs)
}

func TestGetByID_NotFound(t *testing.T) {
	_, err := newRepo().GetByID(context.Background(), uuid.NewString())
	require.Error(t, err)
}

func TestGetByIDs_PartialMiss(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	b1 := seedBooking(ctx, t, domain.StatusPending, false)
	b2 := seedBooking(ctx, t, domain.StatusConfirmed, true)
	missing := uuid.NewString()

	out, err := repo.GetByIDs(ctx, []string{b1.ID, b2.ID, missing})
	require.NoError(t, err)
	require.Len(t, out, 2, "unknown ids must be omitted, not error")

	ids := map[string]bool{}
	for _, b := range out {
		ids[b.ID] = true
	}
	require.True(t, ids[b1.ID] && ids[b2.ID])
}

func TestListByUser_FiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	user := uuid.NewString()

	// Two bookings for the same user: one pending, one confirmed.
	for _, st := range []domain.BookingStatus{domain.StatusPending, domain.StatusConfirmed} {
		b := &domain.Booking{
			UserID: user, VenueID: uuid.NewString(), VenueName: "V",
			Date: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), TimeFrom: mustTOD(t, "10:00"), TimeTo: mustTOD(t, "11:00"),
			Guests: 2, Status: st, TotalPrice: 1000,
		}
		require.NoError(t, repo.Create(ctx, b))
	}

	all, total, err := repo.ListByUser(ctx, user, "", 0, 50)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, all, 2)

	confirmed, total, err := repo.ListByUser(ctx, user, string(domain.StatusConfirmed), 0, 50)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, confirmed, 1)
	require.Equal(t, domain.StatusConfirmed, confirmed[0].Status)
}

func TestCancelWithEvent_WritesOutbox(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	b := seedBooking(ctx, t, domain.StatusConfirmed, false)

	ev, err := domain.NewBookingEvent("booking.cancelled", b)
	require.NoError(t, err)
	require.NoError(t, repo.CancelWithEvent(ctx, b.ID, ev))

	got, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusCancelled, got.Status)

	// The event was staged in the outbox in the same transaction.
	events, err := repo.FetchUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	var found *domain.OutboxEvent
	for i := range events {
		if strings.Contains(string(events[i].Payload), b.ID) {
			found = &events[i]
		}
	}
	require.NotNil(t, found, "booking.cancelled event must be in the outbox")
	require.Equal(t, "booking.cancelled", found.Subject)

	// MarkOutboxSent removes it from the unsent set.
	require.NoError(t, repo.MarkOutboxSent(ctx, []int64{found.ID}))
	after, err := repo.FetchUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	for _, e := range after {
		require.NotEqual(t, found.ID, e.ID, "marked-sent event must not reappear")
	}
}

func TestCancelWithEvent_NotFound(t *testing.T) {
	ctx := context.Background()
	ev := domain.OutboxEvent{Subject: "booking.cancelled", Payload: []byte(`{}`)}
	err := newRepo().CancelWithEvent(ctx, uuid.NewString(), ev)
	require.Error(t, err)
}

func TestHasCompleted(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	// A fresh pending booking is not a completed visit.
	b := seedBooking(ctx, t, domain.StatusPending, false)
	ok, err := repo.HasCompleted(ctx, b.UserID, b.VenueID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestStaffNotes_AddAndList(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	b := seedBooking(ctx, t, domain.StatusConfirmed, false)

	note := &domain.BookingStaffNote{
		BookingID:    b.ID,
		VenueID:      b.VenueID,
		AuthorUserID: uuid.NewString(),
		Body:         "гость опаздывает",
	}
	require.NoError(t, repo.AddBookingStaffNote(ctx, note))
	require.NotEmpty(t, note.ID)

	notes, err := repo.ListBookingStaffNotes(ctx, b.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	require.Equal(t, "гость опаздывает", notes[0].Body)
}
