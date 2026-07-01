//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/review-service/internal/domain"
)

func TestCreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	rev := createVenueReview(ctx, t, uuid.NewString(), uuid.NewString(), 5)

	got, err := repo.GetByID(ctx, rev.ID)
	require.NoError(t, err)
	require.Equal(t, rev.UserID, got.UserID)
	require.Equal(t, rev.VenueID, got.VenueID)
	require.Empty(t, got.MasterID)
	require.EqualValues(t, 5, got.Rating)
}

func TestGetByID_NotFound(t *testing.T) {
	_, err := newRepo().GetByID(context.Background(), uuid.NewString())
	require.Error(t, err)
}

func TestCreateTx_DuplicateRejected(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	venue := uuid.NewString()
	user := uuid.NewString()
	createVenueReview(ctx, t, venue, user, 4)

	// Same (user, venue) violates uq_user_venue → AlreadyExists.
	tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	dup := &domain.Review{UserID: user, UserName: "Гость", VenueID: venue, Rating: 4, Text: "повтор"}
	err = repo.CreateTx(ctx, tx, dup)
	require.Error(t, err)
}

func TestVenueRating_IncrementsAndAverages(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	venue := uuid.NewString()

	// Unknown venue → zero rating, no error.
	zero, err := repo.GetVenueRating(ctx, venue)
	require.NoError(t, err)
	require.EqualValues(t, 0, zero.ReviewCount)

	createVenueReview(ctx, t, venue, uuid.NewString(), 5)
	createVenueReview(ctx, t, venue, uuid.NewString(), 3)

	vr, err := repo.GetVenueRating(ctx, venue)
	require.NoError(t, err)
	require.EqualValues(t, 2, vr.ReviewCount)
	require.InDelta(t, 4.0, vr.AvgRating, 0.001, "avg of 5 and 3 is 4.0")
}

func TestMasterRating_IncrementsAndAverages(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	master := uuid.NewString()

	createMasterReview(ctx, t, master, uuid.NewString(), 4)
	createMasterReview(ctx, t, master, uuid.NewString(), 2)

	mr, err := repo.GetMasterRating(ctx, master)
	require.NoError(t, err)
	require.EqualValues(t, 2, mr.ReviewCount)
	require.InDelta(t, 3.0, mr.AvgRating, 0.001)
}

func TestListByVenue_PaginationAndTotal(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	venue := uuid.NewString()
	for i := 0; i < 3; i++ {
		createVenueReview(ctx, t, venue, uuid.NewString(), int32(3+i%3))
	}

	// page 1, size 2 → 2 rows, total 3.
	list, total, err := repo.ListByVenue(ctx, venue, 1, 2)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.EqualValues(t, 3, total)

	// page 2, size 2 → 1 row, total still 3.
	list2, total2, err := repo.ListByVenue(ctx, venue, 2, 2)
	require.NoError(t, err)
	require.Len(t, list2, 1)
	require.EqualValues(t, 3, total2)
}

func TestListByMaster(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	master := uuid.NewString()
	createMasterReview(ctx, t, master, uuid.NewString(), 5)

	list, total, err := repo.ListByMaster(ctx, master, 1, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.EqualValues(t, 1, total)
	require.Equal(t, master, list[0].MasterID)
}
