//go:build integration

package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	_, b, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(b), "..", "..", "migrations")

	pgContainer, err := postgres.Run(ctx, "postgis/postgis:16-3.4",
		postgres.WithDatabase("venue_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(
			filepath.Join(migrationsDir, "001_init.up.sql"),
			filepath.Join(migrationsDir, "002_moderation.up.sql"),
			filepath.Join(migrationsDir, "003_city.up.sql"),
			filepath.Join(migrationsDir, "004_moderation_extended.up.sql"),
			filepath.Join(migrationsDir, "005_owner_verification.up.sql"),
			filepath.Join(migrationsDir, "006_reserved_slots_exclusion.up.sql"),
			filepath.Join(migrationsDir, "007_manual_slot_blocks.up.sql"),
			filepath.Join(migrationsDir, "008_venue_social_links.up.sql"),
			filepath.Join(migrationsDir, "009_venue_service_sort_order.up.sql"),
			filepath.Join(migrationsDir, "010_venue_halls.up.sql"),
			filepath.Join(migrationsDir, "011_venue_payout_profile.up.sql"),
			filepath.Join(migrationsDir, "012_venue_staff.up.sql"),
			filepath.Join(migrationsDir, "013_venue_crm_tasks.up.sql"),
			filepath.Join(migrationsDir, "017_reserved_slots_hall.up.sql"),
		),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(300*time.Second),
				wait.ForListeningPort("5432/tcp").
					WithStartupTimeout(300*time.Second),
			),
		),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}

	code := m.Run()

	testPool.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func TestIntegration_CreateAndGetVenue(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID:     uuid.New(),
		Name:        "Тестовая Баня " + uuid.New().String()[:8],
		Type:        "banya",
		Description: "Лучшая баня в городе",
		Address:     "ул. Тестовая, 1",
		City:        "Москва",
		Latitude:    55.7558,
		Longitude:   37.6176,
		PriceFrom:   2000,
		Capacity:    10,
		Amenities:   []string{"бассейн", "веники"},
		Phone:       "+79991234567",
	}

	err := repo.Create(ctx, venue)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, venue.ID)
	assert.NotEmpty(t, venue.Slug)
	assert.Equal(t, domain.StatusPendingReview, venue.Status)

	got, err := repo.GetByID(ctx, venue.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, venue.Name, got.Name)
	assert.Equal(t, "Москва", got.City)
	assert.Equal(t, domain.StatusPendingReview, got.Status)

	gotBySlug, err := repo.GetBySlug(ctx, venue.Slug)
	require.NoError(t, err)
	require.NotNil(t, gotBySlug)
	assert.Equal(t, venue.ID, gotBySlug.ID)
}

func TestIntegration_ListByStatus(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		v := &domain.Venue{
			OwnerID: uuid.New(),
			Name:    fmt.Sprintf("Баня ListByStatus %d %s", i, uuid.New().String()[:8]),
			Type:    "sauna",
			Address: "ул. Тестовая",
			City:    "Москва",
		}
		require.NoError(t, repo.Create(ctx, v))
	}

	withPhoto := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "ListByStatus с фото " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Тестовая",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, withPhoto))
	_, err := repo.AddVenuePhoto(ctx, withPhoto.ID, withPhoto.OwnerID, "https://example.com/moderation-list-by-status.jpg")
	require.NoError(t, err)

	result, err := repo.ListByStatus(ctx, domain.StatusPendingReview, 1, 100, "")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, int(result.Total), 4)

	var listed *domain.Venue
	for i := range result.Venues {
		if result.Venues[i].ID == withPhoto.ID {
			listed = &result.Venues[i]
			break
		}
	}
	require.NotNil(t, listed, "venue with photo must appear in ListByStatus")
	assert.Len(t, listed.Photos, 1)
	assert.Equal(t, "https://example.com/moderation-list-by-status.jpg", listed.Photos[0].URL)
}

func TestIntegration_ModerationWorkflow(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()
	adminID := uuid.New()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Модерируемая " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Модерации, 1",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))
	assert.Equal(t, domain.StatusPendingReview, venue.Status)

	err := repo.UpdateStatus(ctx, venue.ID, domain.StatusActive, "", adminID)
	require.NoError(t, err)

	err = repo.InsertModerationHistory(ctx, &domain.ModerationHistoryEntry{
		VenueID:   venue.ID,
		OldStatus: domain.StatusPendingReview,
		NewStatus: domain.StatusActive,
		Comment:   "Всё хорошо",
		ChangedBy: adminID,
	})
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, venue.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusActive, got.Status)
	assert.True(t, got.IsActive)
	assert.NotNil(t, got.ModeratedAt)
	assert.NotNil(t, got.ModeratedBy)
	assert.Equal(t, adminID, *got.ModeratedBy)

	history, err := repo.GetModerationHistory(ctx, venue.ID)
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, domain.StatusPendingReview, history[0].OldStatus)
	assert.Equal(t, domain.StatusActive, history[0].NewStatus)

	err = repo.UpdateStatus(ctx, venue.ID, domain.StatusRejected, "Не соответствует", adminID)
	require.NoError(t, err)

	err = repo.ResetToPendingReview(ctx, venue.ID)
	require.NoError(t, err)

	got, err = repo.GetByID(ctx, venue.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPendingReview, got.Status)
	assert.False(t, got.IsActive)
}

// TestIntegration_UpdateResendsActiveToReview proves repo.Update performs the
// active/rejected → pending_review transition in the same statement as the
// content write, so an edit can never leave changed content live under the old
// status. Also checks a pending card is not spuriously re-transitioned.
func TestIntegration_UpdateResendsActiveToReview(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()
	adminID := uuid.New()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Правки " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Правок, 1",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))
	require.NoError(t, repo.UpdateStatus(ctx, venue.ID, domain.StatusActive, "", adminID))

	venue.Name = "Правки обновлены"
	require.NoError(t, repo.Update(ctx, venue))

	got, err := repo.GetByID(ctx, venue.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPendingReview, got.Status)
	assert.False(t, got.IsActive)
	assert.Equal(t, "Правки обновлены", got.Name)

	// Already pending — a further edit keeps it pending, no spurious transition.
	venue.Name = "Ещё правки"
	require.NoError(t, repo.Update(ctx, venue))
	got, err = repo.GetByID(ctx, venue.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPendingReview, got.Status)
}

// TestIntegration_SetBookingModeGuardedByReservations proves a real mode switch
// is refused while the venue has upcoming reserved slots (which are keyed to the
// current mode), while a same-mode call stays a no-op.
func TestIntegration_SetBookingModeGuardedByReservations(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Баня Режим " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Режимов, 4",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))

	futureDate := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	bookingID := uuid.New()
	require.NoError(t, repo.ReserveSlot(ctx, venue.ID, bookingID, futureDate, "10:00", "12:00", nil))

	// Same mode is always a no-op, even with an upcoming reservation.
	require.NoError(t, repo.SetBookingMode(ctx, venue.ID, domain.BookingModeWhole))

	// A real switch is refused while the upcoming reservation exists.
	err := repo.SetBookingMode(ctx, venue.ID, domain.BookingModePerHall)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBookingModeHasReservations)

	// After releasing it, the switch goes through.
	require.NoError(t, repo.ReleaseSlot(ctx, venue.ID, bookingID))
	require.NoError(t, repo.SetBookingMode(ctx, venue.ID, domain.BookingModePerHall))
	mode, err := repo.BookingMode(ctx, venue.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookingModePerHall, mode)
}

// TestIntegration_CreateWithHallsIsAtomic proves venue + halls land in one
// transaction: hall IDs and the derived venue fields (cheapest price, max
// capacity) come back populated and persist.
func TestIntegration_CreateWithHallsIsAtomic(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Баня Залы " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Залов, 5",
		City:    "Москва",
		Halls: []domain.VenueHall{
			{Name: "Большой зал", PriceFrom: 3000, Capacity: 10, Amenities: []string{"бассейн"}},
			{Name: "Малый зал", PriceFrom: 1500, Capacity: 4},
		},
	}
	require.NoError(t, repo.Create(ctx, venue))

	require.Len(t, venue.Halls, 2)
	assert.NotEqual(t, uuid.Nil, venue.Halls[0].ID)
	assert.Equal(t, int64(1500), venue.PriceFrom) // cheapest hall

	got, err := repo.GetByID(ctx, venue.ID)
	require.NoError(t, err)
	require.Len(t, got.Halls, 2)
	assert.Equal(t, int64(1500), got.PriceFrom)
	assert.Equal(t, int32(10), got.Capacity) // max hall capacity
}

func TestIntegration_SlotReservation(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Баня Слоты " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Слотов, 1",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))

	testDate := "2025-06-15"
	available, err := repo.CheckSlot(ctx, venue.ID, nil, testDate, "10:00", "12:00")
	require.NoError(t, err)
	assert.True(t, available)

	bookingID := uuid.New()
	err = repo.ReserveSlot(ctx, venue.ID, bookingID, testDate, "10:00", "12:00", nil)
	require.NoError(t, err)

	available, err = repo.CheckSlot(ctx, venue.ID, nil, testDate, "10:00", "12:00")
	require.NoError(t, err)
	assert.False(t, available)

	err = repo.ReleaseSlot(ctx, venue.ID, bookingID)
	require.NoError(t, err)

	available, err = repo.CheckSlot(ctx, venue.ID, nil, testDate, "10:00", "12:00")
	require.NoError(t, err)
	assert.True(t, available)
}

func TestIntegration_SlotOverlapRejected(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Баня Пересечение " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Пересечений, 2",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))

	testDate := "2025-07-20"
	require.NoError(t, repo.ReserveSlot(ctx, venue.ID, uuid.New(), testDate, "10:00", "12:00", nil))

	err := repo.ReserveSlot(ctx, venue.ID, uuid.New(), testDate, "11:00", "13:00", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSlotUnavailable)
}

func TestIntegration_ManualSlotBlockOccupiesSlot(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Баня РучнойБлок " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Блоков, 3",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))

	testDate := "2025-08-10"
	blocks, err := repo.CreateManualSlotBlock(ctx, venue.ID, nil, testDate, "14:00", "16:00", "по телефону")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.NotEqual(t, uuid.Nil, blocks[0].ID)

	available, err := repo.CheckSlot(ctx, venue.ID, nil, testDate, "15:00", "17:00")
	require.NoError(t, err)
	assert.False(t, available)

	err = repo.ReserveSlot(ctx, venue.ID, uuid.New(), testDate, "15:00", "17:00", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSlotUnavailable)

	deleted, err := repo.DeleteManualSlotBlock(ctx, venue.ID, blocks[0].ID)
	require.NoError(t, err)
	assert.True(t, deleted)

	available, err = repo.CheckSlot(ctx, venue.ID, nil, testDate, "15:00", "17:00")
	require.NoError(t, err)
	assert.True(t, available)
}

func TestIntegration_Search(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()
	adminID := uuid.New()

	uniqueName := "УникальнаяПарная" + uuid.New().String()[:8]
	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    uniqueName,
		Type:    "banya",
		Address: "ул. Поиска, 5",
		City:    "Санкт-Петербург",
	}
	require.NoError(t, repo.Create(ctx, venue))
	require.NoError(t, repo.UpdateStatus(ctx, venue.ID, domain.StatusActive, "", adminID))

	result, err := repo.Search(ctx, domain.SearchParams{
		Query:    uniqueName,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, int(result.Total), 1)

	found := false
	for _, v := range result.Venues {
		if v.ID == venue.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "search should find the venue")
}

func TestIntegration_ListOnlyActive(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()
	adminID := uuid.New()

	activeVenue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Активная " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Активная",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, activeVenue))
	require.NoError(t, repo.UpdateStatus(ctx, activeVenue.ID, domain.StatusActive, "", adminID))

	pendingVenue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "На модерации " + uuid.New().String()[:8],
		Type:    "sauna",
		Address: "ул. Pending",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, pendingVenue))

	result, err := repo.List(ctx, 1, 100, "", "")
	require.NoError(t, err)

	for _, v := range result.Venues {
		assert.True(t, v.IsActive, "List should return only active venues")
		assert.Equal(t, domain.StatusActive, v.Status)
	}
}

func TestIntegration_PerHallReservations(t *testing.T) {
	repo := NewVenueRepo(testPool)
	ctx := context.Background()

	venue := &domain.Venue{
		OwnerID: uuid.New(),
		Name:    "Баня Позальная " + uuid.New().String()[:8],
		Type:    "banya",
		Address: "ул. Залов, 4",
		City:    "Москва",
	}
	require.NoError(t, repo.Create(ctx, venue))

	date := "2025-09-15"
	hallA, hallB := uuid.New(), uuid.New()

	// Same interval in different halls: both succeed (independent resources).
	require.NoError(t, repo.ReserveSlot(ctx, venue.ID, uuid.New(), date, "10:00", "12:00", []uuid.UUID{hallA}))
	require.NoError(t, repo.ReserveSlot(ctx, venue.ID, uuid.New(), date, "10:00", "12:00", []uuid.UUID{hallB}))

	// Availability is per hall: a reserved hall reads busy, an untouched hall free.
	busyA, err := repo.CheckSlot(ctx, venue.ID, []uuid.UUID{hallA}, date, "10:00", "12:00")
	require.NoError(t, err)
	assert.False(t, busyA)
	freeC, err := repo.CheckSlot(ctx, venue.ID, []uuid.UUID{uuid.New()}, date, "10:00", "12:00")
	require.NoError(t, err)
	assert.True(t, freeC)

	// Overlapping interval in the same hall: rejected.
	err = repo.ReserveSlot(ctx, venue.ID, uuid.New(), date, "11:00", "13:00", []uuid.UUID{hallA})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSlotUnavailable)

	// A multi-hall booking is atomic: a conflict on any hall aborts all inserts.
	err = repo.ReserveSlot(ctx, venue.ID, uuid.New(), date, "11:30", "12:30", []uuid.UUID{uuid.New(), hallB})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSlotUnavailable)

	// A per-hall manual block blocks bookings on that hall, and only that hall.
	blockHall := uuid.New()
	blocks, err := repo.CreateManualSlotBlock(ctx, venue.ID, []uuid.UUID{blockHall}, date, "14:00", "16:00", "ремонт")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	err = repo.ReserveSlot(ctx, venue.ID, uuid.New(), date, "15:00", "17:00", []uuid.UUID{blockHall})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSlotUnavailable)
	require.NoError(t, repo.ReserveSlot(ctx, venue.ID, uuid.New(), date, "15:00", "17:00", []uuid.UUID{uuid.New()}))
}
