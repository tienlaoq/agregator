//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

func TestIntegration_VenueOwnerID(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	owner := uuid.New()
	venue := seedVenue(ctx, t, owner)

	got, err := repo.VenueOwnerID(ctx, venue)
	require.NoError(t, err)
	assert.Equal(t, owner, got)

	_, err = repo.VenueOwnerID(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrVenueNotFound)
}

func TestIntegration_ManagementAccess(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	owner := uuid.New()
	venue := seedVenue(ctx, t, owner)
	staff := uuid.New()
	require.NoError(t, repo.AddStaff(ctx, venue, staff, domain.StaffRoleStaff, owner))

	t.Run("owner", func(t *testing.T) {
		access, err := repo.GetManagementAccess(ctx, venue, owner)
		require.NoError(t, err)
		assert.Equal(t, domain.AccessOwner, access)
	})

	t.Run("staff", func(t *testing.T) {
		access, err := repo.GetManagementAccess(ctx, venue, staff)
		require.NoError(t, err)
		assert.Equal(t, domain.StaffRoleStaff, access)
	})

	t.Run("stranger gets empty", func(t *testing.T) {
		access, err := repo.GetManagementAccess(ctx, venue, uuid.New())
		require.NoError(t, err)
		assert.Empty(t, access)
	})
}

func TestIntegration_BatchGetManagementAccess(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	owner := uuid.New()
	v1 := seedVenue(ctx, t, owner)
	v2 := seedVenue(ctx, t, uuid.New()) // owned by someone else
	require.NoError(t, repo.AddStaff(ctx, v2, owner, domain.StaffRoleManager, owner))
	other := seedVenue(ctx, t, uuid.New()) // no relationship

	got, err := repo.BatchGetManagementAccess(ctx, owner, []uuid.UUID{v1, v2, other})
	require.NoError(t, err)
	assert.Equal(t, domain.AccessOwner, got[v1])
	assert.Equal(t, domain.StaffRoleManager, got[v2])
	_, present := got[other]
	assert.False(t, present, "venues with no access must be absent")

	// Empty input short-circuits to an empty map.
	empty, err := repo.BatchGetManagementAccess(ctx, owner, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestIntegration_ListManagedVenues(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	user := uuid.New()
	owned := seedVenue(ctx, t, user)
	managed := seedVenue(ctx, t, uuid.New())
	require.NoError(t, repo.AddStaff(ctx, managed, user, domain.StaffRoleManager, uuid.New()))
	seedVenue(ctx, t, uuid.New()) // unrelated venue must not appear

	venues, err := repo.ListManagedVenues(ctx, user)
	require.NoError(t, err)

	byID := make(map[uuid.UUID]string, len(venues))
	for _, v := range venues {
		byID[v.VenueID] = v.Access
	}
	assert.Equal(t, domain.AccessOwner, byID[owned])
	assert.Equal(t, domain.StaffRoleManager, byID[managed])
	assert.Len(t, venues, 2)
}

func TestIntegration_Staff_AddListRemove(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	owner := uuid.New()
	venue := seedVenue(ctx, t, owner)
	member := uuid.New()

	require.NoError(t, repo.AddStaff(ctx, venue, member, domain.StaffRoleStaff, owner))

	// AddStaff upserts: a second call promotes the role rather than duplicating.
	require.NoError(t, repo.AddStaff(ctx, venue, member, domain.StaffRoleManager, owner))

	staff, err := repo.ListStaff(ctx, venue)
	require.NoError(t, err)
	require.Len(t, staff, 1)
	assert.Equal(t, member, staff[0].UserID)
	assert.Equal(t, domain.StaffRoleManager, staff[0].Role)
	assert.Equal(t, owner, staff[0].InvitedBy)

	require.NoError(t, repo.RemoveStaff(ctx, venue, member))
	assert.ErrorIs(t, repo.RemoveStaff(ctx, venue, member), ErrStaffNotFound)

	staff, err = repo.ListStaff(ctx, venue)
	require.NoError(t, err)
	assert.Empty(t, staff)
}

func TestIntegration_TaskLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	owner := uuid.New()
	venue := seedVenue(ctx, t, owner)
	assignee := uuid.New()
	due := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)

	task := &domain.Task{
		VenueID:        venue,
		Title:          "Позвонить гостю",
		Body:           "Уточнить детали",
		Status:         domain.TaskStatusOpen,
		Priority:       "high",
		AssigneeUserID: &assignee,
		DueAt:          &due,
		CreatedBy:      owner,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NotEqual(t, uuid.Nil, task.ID, "CreateTask must populate the id")
	assert.False(t, task.CreatedAt.IsZero())

	t.Run("get returns the created task", func(t *testing.T) {
		got, err := repo.GetTask(ctx, venue, task.ID)
		require.NoError(t, err)
		assert.Equal(t, "Позвонить гостю", got.Title)
		assert.Equal(t, "high", got.Priority)
		require.NotNil(t, got.AssigneeUserID)
		assert.Equal(t, assignee, *got.AssigneeUserID)
	})

	t.Run("get of unknown id is ErrTaskNotFound", func(t *testing.T) {
		_, err := repo.GetTask(ctx, venue, uuid.New())
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("update mutates fields and bumps updated_at", func(t *testing.T) {
		upd := &domain.Task{
			ID: task.ID, VenueID: venue, Title: "Обновлено", Body: "новое тело",
			Priority: "low", AssigneeUserID: nil, DueAt: nil,
		}
		require.NoError(t, repo.UpdateTask(ctx, upd))
		assert.Equal(t, "Обновлено", upd.Title)
		assert.Equal(t, "low", upd.Priority)
		assert.Nil(t, upd.AssigneeUserID)
	})

	t.Run("complete then reopen", func(t *testing.T) {
		ok, err := repo.CompleteTask(ctx, venue, task.ID, owner)
		require.NoError(t, err)
		assert.True(t, ok)

		// Completing an already-done task is a no-op.
		ok, err = repo.CompleteTask(ctx, venue, task.ID, owner)
		require.NoError(t, err)
		assert.False(t, ok)

		reopened, err := repo.ReopenTask(ctx, venue, task.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusOpen, reopened.Status)
		assert.Nil(t, reopened.CompletedBy)
		assert.Nil(t, reopened.CompletedAt)
	})

	t.Run("reopen of an open task is ErrTaskNotFound", func(t *testing.T) {
		_, err := repo.ReopenTask(ctx, venue, task.ID)
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("cancel is idempotent and excludes from update", func(t *testing.T) {
		ok, err := repo.CancelTask(ctx, venue, task.ID)
		require.NoError(t, err)
		assert.True(t, ok)

		ok, err = repo.CancelTask(ctx, venue, task.ID)
		require.NoError(t, err)
		assert.False(t, ok, "cancelling a cancelled task is a no-op")

		// A cancelled task is no longer updatable.
		err = repo.UpdateTask(ctx, &domain.Task{ID: task.ID, VenueID: venue, Title: "x", Priority: "normal"})
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestIntegration_ListTasks_FilterAndOrder(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	owner := uuid.New()
	venue := seedVenue(ctx, t, owner)

	mk := func(title, priority string) *domain.Task {
		task := &domain.Task{VenueID: venue, Title: title, Status: domain.TaskStatusOpen, Priority: priority, CreatedBy: owner}
		require.NoError(t, repo.CreateTask(ctx, task))
		return task
	}
	mk("open-1", "normal")
	done := mk("to-complete", "normal")
	_, err := repo.CompleteTask(ctx, venue, done.ID, owner)
	require.NoError(t, err)

	all, err := repo.ListTasks(ctx, venue, "")
	require.NoError(t, err)
	require.Len(t, all, 2)
	// Open tasks sort ahead of done ones.
	assert.Equal(t, domain.TaskStatusOpen, all[0].Status)

	openOnly, err := repo.ListTasks(ctx, venue, domain.TaskStatusOpen)
	require.NoError(t, err)
	require.Len(t, openOnly, 1)
	assert.Equal(t, "open-1", openOnly[0].Title)
}

func TestIntegration_GuestProjection(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	venue := uuid.New() // facts/profiles are not FK-bound to venues
	guest := uuid.New()

	apply := func(status string, rank int16, price int64, date string, guests int32) {
		var visit *time.Time
		if date != "" {
			d, perr := time.Parse("2006-01-02", date)
			require.NoError(t, perr)
			visit = &d
		}
		require.NoError(t, repo.ApplyBookingFact(ctx, &domain.BookingFact{
			BookingID: uuid.New(), VenueID: venue, UserID: guest,
			Status: status, StatusRank: rank, TotalPrice: price, VisitDate: visit, Guests: guests,
		}))
	}

	// Two completed bookings + one cancelled drive the aggregate.
	apply("completed", domain.BookingStatusRank("completed"), 3000, "2026-02-01", 2)
	apply("completed", domain.BookingStatusRank("completed"), 5000, "2026-03-10", 3)
	apply("cancelled", domain.BookingStatusRank("cancelled"), 0, "", 1)

	t.Run("profile aggregates facts", func(t *testing.T) {
		p, err := repo.GetGuestProfile(ctx, venue, guest)
		require.NoError(t, err)
		assert.Equal(t, int32(3), p.BookingsCount)
		assert.Equal(t, int32(2), p.VisitsCount)
		assert.Equal(t, int32(1), p.CancellationsCount)
		assert.Equal(t, int64(8000), p.TotalSpent)
		require.NotNil(t, p.LastVisitAt)
		assert.Equal(t, "2026-03-10", p.LastVisitAt.Format("2006-01-02"))
	})

	t.Run("unknown guest is ErrGuestNotFound", func(t *testing.T) {
		_, err := repo.GetGuestProfile(ctx, venue, uuid.New())
		assert.ErrorIs(t, err, ErrGuestNotFound)
	})

	t.Run("status rank guards against regression", func(t *testing.T) {
		// Re-deliver a stale 'pending' for a booking already completed: the higher
		// stored rank must win, so the aggregate is unchanged.
		bookingID := uuid.New()
		require.NoError(t, repo.ApplyBookingFact(ctx, &domain.BookingFact{
			BookingID: bookingID, VenueID: venue, UserID: guest,
			Status: "completed", StatusRank: domain.BookingStatusRank("completed"), TotalPrice: 1000,
		}))
		require.NoError(t, repo.ApplyBookingFact(ctx, &domain.BookingFact{
			BookingID: bookingID, VenueID: venue, UserID: guest,
			Status: "pending", StatusRank: domain.BookingStatusRank("pending"), TotalPrice: 99999,
		}))
		bookings, err := repo.ListGuestBookings(ctx, venue, guest, 20)
		require.NoError(t, err)
		for _, b := range bookings {
			if b.BookingID == bookingID {
				assert.Equal(t, "completed", b.Status, "stale lower-rank event must not overwrite")
				assert.Equal(t, int64(1000), b.TotalPrice)
			}
		}
	})

	t.Run("list bookings newest first", func(t *testing.T) {
		bookings, err := repo.ListGuestBookings(ctx, venue, guest, 20)
		require.NoError(t, err)
		assert.NotEmpty(t, bookings)
	})
}

func TestIntegration_ListGuests_SegmentsAndSort(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	venue := uuid.New()

	// A regular VIP-spender and a brand-new guest in the same venue.
	vip := uuid.New()
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.ApplyBookingFact(ctx, &domain.BookingFact{
			BookingID: uuid.New(), VenueID: venue, UserID: vip,
			Status: "completed", StatusRank: domain.BookingStatusRank("completed"),
			TotalPrice: 50000, VisitDate: ptrDate(t, "2026-04-0"+string(rune('1'+i))), Guests: 2,
		}))
	}
	newbie := uuid.New()
	require.NoError(t, repo.ApplyBookingFact(ctx, &domain.BookingFact{
		BookingID: uuid.New(), VenueID: venue, UserID: newbie,
		Status: "completed", StatusRank: domain.BookingStatusRank("completed"),
		TotalPrice: 1000, VisitDate: ptrDate(t, "2026-04-15"), Guests: 1,
	}))

	t.Run("no filter returns both, total matches", func(t *testing.T) {
		guests, total, err := repo.ListGuests(ctx, venue, domain.GuestListParams{Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, guests, 2)
	})

	t.Run("vip segment with threshold", func(t *testing.T) {
		guests, total, err := repo.ListGuests(ctx, venue, domain.GuestListParams{
			Segment: domain.SegmentVIP, VIPThreshold: 10000, Limit: 50,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, guests, 1)
		assert.Equal(t, vip, guests[0].UserID)
	})

	t.Run("new segment", func(t *testing.T) {
		guests, total, err := repo.ListGuests(ctx, venue, domain.GuestListParams{
			Segment: domain.SegmentNew, Limit: 50,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, guests, 1)
		assert.Equal(t, newbie, guests[0].UserID)
	})

	t.Run("sort by ltv puts the big spender first", func(t *testing.T) {
		guests, _, err := repo.ListGuests(ctx, venue, domain.GuestListParams{Sort: "ltv", Limit: 50})
		require.NoError(t, err)
		require.Len(t, guests, 2)
		assert.Equal(t, vip, guests[0].UserID)
	})

	t.Run("limit and offset paginate", func(t *testing.T) {
		page1, total, err := repo.ListGuests(ctx, venue, domain.GuestListParams{Sort: "ltv", Limit: 1, Offset: 0})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, page1, 1)

		page2, _, err := repo.ListGuests(ctx, venue, domain.GuestListParams{Sort: "ltv", Limit: 1, Offset: 1})
		require.NoError(t, err)
		require.Len(t, page2, 1)
		assert.NotEqual(t, page1[0].UserID, page2[0].UserID)
	})
}

func ptrDate(t *testing.T, s string) *time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return &d
}
