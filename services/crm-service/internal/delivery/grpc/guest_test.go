package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
	"github.com/tienlao/agregator/services/crm-service/internal/repository"
)

// grantAccess wires a mock that always reports the actor as a venue owner, so
// member-access checks pass and the handler reaches its real work.
func grantAccess(repo *mockRepo) *mockRepo {
	repo.GetManagementAccessFunc = func(context.Context, uuid.UUID, uuid.UUID) (string, error) {
		return domain.AccessOwner, nil
	}
	return repo
}

func TestListGuests(t *testing.T) {
	ctx := context.Background()
	venue, actor := uuid.NewString(), uuid.NewString()

	t.Run("invalid venue id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).ListGuests(ctx, &crmv1.ListGuestsRequest{VenueId: "bad", ActorId: actor})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("invalid actor id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).ListGuests(ctx, &crmv1.ListGuestsRequest{VenueId: venue, ActorId: "bad"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("no access denied", func(t *testing.T) {
		repo := &mockRepo{GetManagementAccessFunc: func(context.Context, uuid.UUID, uuid.UUID) (string, error) {
			return "", nil
		}}
		_, err := newServer(repo).ListGuests(ctx, &crmv1.ListGuestsRequest{VenueId: venue, ActorId: actor})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("repo error becomes internal", func(t *testing.T) {
		repo := grantAccess(&mockRepo{ListGuestsFunc: func(context.Context, uuid.UUID, domain.GuestListParams) ([]domain.GuestProfile, int, error) {
			return nil, 0, errBoom
		}})
		_, err := newServer(repo).ListGuests(ctx, &crmv1.ListGuestsRequest{VenueId: venue, ActorId: actor})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("success maps profiles and total", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		profiles := []domain.GuestProfile{
			{VenueID: uuid.New(), UserID: uuid.New(), BookingsCount: 3, TotalSpent: 9000, LastVisitAt: &now},
			{VenueID: uuid.New(), UserID: uuid.New(), BookingsCount: 1},
		}
		repo := grantAccess(&mockRepo{ListGuestsFunc: func(_ context.Context, _ uuid.UUID, p domain.GuestListParams) ([]domain.GuestProfile, int, error) {
			assert.Equal(t, 10, p.Limit)
			assert.Equal(t, 5, p.Offset)
			return profiles, 42, nil
		}})
		resp, err := newServer(repo).ListGuests(ctx, &crmv1.ListGuestsRequest{
			VenueId: venue, ActorId: actor, Limit: 10, Offset: 5,
		})
		require.NoError(t, err)
		require.Len(t, resp.GetGuests(), 2)
		assert.Equal(t, int32(42), resp.GetTotal())
		assert.Equal(t, int32(3), resp.GetGuests()[0].GetBookingsCount())
		assert.Equal(t, int64(9000), resp.GetGuests()[0].GetTotalSpent())
		assert.NotNil(t, resp.GetGuests()[0].GetLastVisitAt())
		assert.Nil(t, resp.GetGuests()[1].GetLastVisitAt())
	})
}

func TestGetGuest(t *testing.T) {
	ctx := context.Background()
	venue, actor, user := uuid.NewString(), uuid.NewString(), uuid.NewString()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).GetGuest(ctx, &crmv1.GetGuestRequest{
			VenueId: venue, ActorId: actor, UserId: "bad",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("guest not found", func(t *testing.T) {
		repo := grantAccess(&mockRepo{GetGuestProfileFunc: func(context.Context, uuid.UUID, uuid.UUID) (*domain.GuestProfile, error) {
			return nil, repository.ErrGuestNotFound
		}})
		_, err := newServer(repo).GetGuest(ctx, &crmv1.GetGuestRequest{VenueId: venue, ActorId: actor, UserId: user})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("bookings error becomes internal", func(t *testing.T) {
		repo := grantAccess(&mockRepo{
			GetGuestProfileFunc: func(_ context.Context, vID, uID uuid.UUID) (*domain.GuestProfile, error) {
				return &domain.GuestProfile{VenueID: vID, UserID: uID}, nil
			},
			ListGuestBookingsFunc: func(context.Context, uuid.UUID, uuid.UUID, int) ([]domain.GuestBookingSummary, error) {
				return nil, errBoom
			},
		})
		_, err := newServer(repo).GetGuest(ctx, &crmv1.GetGuestRequest{VenueId: venue, ActorId: actor, UserId: user})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("success maps profile and recent bookings", func(t *testing.T) {
		visit := time.Now().UTC().Truncate(time.Second)
		bookingID := uuid.New()
		repo := grantAccess(&mockRepo{
			GetGuestProfileFunc: func(_ context.Context, vID, uID uuid.UUID) (*domain.GuestProfile, error) {
				return &domain.GuestProfile{VenueID: vID, UserID: uID, VisitsCount: 2, TotalSpent: 5000}, nil
			},
			ListGuestBookingsFunc: func(context.Context, uuid.UUID, uuid.UUID, int) ([]domain.GuestBookingSummary, error) {
				return []domain.GuestBookingSummary{
					{BookingID: bookingID, Status: "completed", TotalPrice: 2500, VisitDate: &visit, Guests: 2},
				}, nil
			},
		})
		resp, err := newServer(repo).GetGuest(ctx, &crmv1.GetGuestRequest{VenueId: venue, ActorId: actor, UserId: user})
		require.NoError(t, err)
		assert.Equal(t, int32(2), resp.GetProfile().GetVisitsCount())
		require.Len(t, resp.GetRecentBookings(), 1)
		b := resp.GetRecentBookings()[0]
		assert.Equal(t, bookingID.String(), b.GetBookingId())
		assert.Equal(t, "completed", b.GetStatus())
		assert.Equal(t, int64(2500), b.GetTotalPrice())
		assert.Equal(t, int32(2), b.GetGuests())
		assert.NotNil(t, b.GetVisitDate())
	})
}

func TestProtoToTimePtr(t *testing.T) {
	assert.Nil(t, protoToTimePtr(nil))

	want := time.Now().UTC().Truncate(time.Second)
	got := protoToTimePtr(timestamppb.New(want))
	require.NotNil(t, got)
	assert.True(t, got.Equal(want))
}
