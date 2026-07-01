package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
)

// newValidationServer returns a Server with nil dependencies. It is only safe to
// call handlers whose argument-validation rejects the request before any
// usecase/publisher/telegram call — exactly what these tests exercise.
func newValidationServer() *Server {
	return NewServer(nil, nil, nil, zerolog.Nop())
}

func wantCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}
	if st.Code() != code {
		t.Fatalf("code = %s, want %s (err: %v)", st.Code(), code, err)
	}
}

// TestHandlers_InvalidArgument covers the UUID/empty-field validation guards on
// every handler that rejects bad input before touching its dependencies.
func TestHandlers_InvalidArgument(t *testing.T) {
	s := newValidationServer()
	ctx := context.Background()
	good := uuid.New().String()

	tests := []struct {
		name string
		call func() error
	}{
		{"CreateVenue/owner", func() error {
			_, err := s.CreateVenue(ctx, &venuev1.CreateVenueRequest{OwnerId: "bad"})
			return err
		}},
		{"SubmitVenueForReview/venue", func() error {
			_, err := s.SubmitVenueForReview(ctx, &venuev1.SubmitVenueForReviewRequest{VenueId: "bad", OwnerId: good})
			return err
		}},
		{"SubmitVenueForReview/owner", func() error {
			_, err := s.SubmitVenueForReview(ctx, &venuev1.SubmitVenueForReviewRequest{VenueId: good, OwnerId: "bad"})
			return err
		}},
		{"UpdateVenue/id", func() error {
			_, err := s.UpdateVenue(ctx, &venuev1.UpdateVenueRequest{Id: "bad"})
			return err
		}},
		{"GetVenue/id", func() error {
			_, err := s.GetVenue(ctx, &venuev1.GetVenueRequest{Id: "bad"})
			return err
		}},
		{"CheckSlotAvailability/venue", func() error {
			_, err := s.CheckSlotAvailability(ctx, &venuev1.CheckSlotRequest{VenueId: "bad"})
			return err
		}},
		{"BatchCheckSlotAvailability/venue", func() error {
			_, err := s.BatchCheckSlotAvailability(ctx, &venuev1.BatchCheckSlotRequest{VenueId: "bad"})
			return err
		}},
		{"ReserveSlot/venue", func() error {
			_, err := s.ReserveSlot(ctx, &venuev1.ReserveSlotRequest{VenueId: "bad", BookingId: good})
			return err
		}},
		{"ReserveSlot/booking", func() error {
			_, err := s.ReserveSlot(ctx, &venuev1.ReserveSlotRequest{VenueId: good, BookingId: "bad"})
			return err
		}},
		{"ReleaseSlot/venue", func() error {
			_, err := s.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{VenueId: "bad", BookingId: good})
			return err
		}},
		{"ReleaseSlot/booking", func() error {
			_, err := s.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{VenueId: good, BookingId: "bad"})
			return err
		}},
		{"CreateManualSlotBlock/owner", func() error {
			_, err := s.CreateManualSlotBlock(ctx, &venuev1.CreateManualSlotBlockRequest{OwnerId: "bad", VenueId: good})
			return err
		}},
		{"CreateManualSlotBlock/venue", func() error {
			_, err := s.CreateManualSlotBlock(ctx, &venuev1.CreateManualSlotBlockRequest{OwnerId: good, VenueId: "bad"})
			return err
		}},
		{"DeleteManualSlotBlock/block", func() error {
			_, err := s.DeleteManualSlotBlock(ctx, &venuev1.DeleteManualSlotBlockRequest{OwnerId: good, VenueId: good, BlockId: "bad"})
			return err
		}},
		{"ListManualSlotBlocks/venue", func() error {
			_, err := s.ListManualSlotBlocks(ctx, &venuev1.ListManualSlotBlocksRequest{OwnerId: good, VenueId: "bad"})
			return err
		}},
		{"UpdateRating/venue", func() error {
			_, err := s.UpdateRating(ctx, &venuev1.UpdateRatingRequest{VenueId: "bad"})
			return err
		}},
		{"ModerateVenue/venue", func() error {
			_, err := s.ModerateVenue(ctx, &venuev1.ModerateVenueRequest{VenueId: "bad", ModeratedBy: good})
			return err
		}},
		{"ModerateVenue/moderatedBy", func() error {
			_, err := s.ModerateVenue(ctx, &venuev1.ModerateVenueRequest{VenueId: good, ModeratedBy: "bad"})
			return err
		}},
		{"SuspendVenuesByOwner/owner", func() error {
			_, err := s.SuspendVenuesByOwner(ctx, &venuev1.SuspendVenuesByOwnerRequest{OwnerId: "bad"})
			return err
		}},
		{"AddVenuePhoto/venue", func() error {
			_, err := s.AddVenuePhoto(ctx, &venuev1.AddVenuePhotoRequest{VenueId: "bad", OwnerId: good, Url: "https://x"})
			return err
		}},
		{"AddVenuePhoto/emptyURL", func() error {
			_, err := s.AddVenuePhoto(ctx, &venuev1.AddVenuePhotoRequest{VenueId: good, OwnerId: good, Url: "   "})
			return err
		}},
		{"DeleteVenuePhoto/photo", func() error {
			_, err := s.DeleteVenuePhoto(ctx, &venuev1.DeleteVenuePhotoRequest{VenueId: good, OwnerId: good, PhotoId: "bad"})
			return err
		}},
		{"SetVenueCoverPhoto/photo", func() error {
			_, err := s.SetVenueCoverPhoto(ctx, &venuev1.SetVenueCoverPhotoRequest{VenueId: good, OwnerId: good, PhotoId: "bad"})
			return err
		}},
		{"AddVenueHallPhoto/hall", func() error {
			_, err := s.AddVenueHallPhoto(ctx, &venuev1.AddVenueHallPhotoRequest{VenueId: good, HallId: "bad", OwnerId: good, Url: "https://x"})
			return err
		}},
		{"AddVenueHallPhoto/emptyURL", func() error {
			_, err := s.AddVenueHallPhoto(ctx, &venuev1.AddVenueHallPhotoRequest{VenueId: good, HallId: good, OwnerId: good, Url: " "})
			return err
		}},
		{"DeleteVenueHallPhoto/photo", func() error {
			_, err := s.DeleteVenueHallPhoto(ctx, &venuev1.DeleteVenueHallPhotoRequest{VenueId: good, HallId: good, OwnerId: good, PhotoId: "bad"})
			return err
		}},
		{"SetVenueHallCoverPhoto/photo", func() error {
			_, err := s.SetVenueHallCoverPhoto(ctx, &venuev1.SetVenueHallCoverPhotoRequest{VenueId: good, HallId: good, OwnerId: good, PhotoId: "bad"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantCode(t, tt.call(), codes.InvalidArgument)
		})
	}
}

func TestGetVenuesBatch_EmptyIDs(t *testing.T) {
	s := newValidationServer()
	resp, err := s.GetVenuesBatch(context.Background(), &venuev1.GetVenuesBatchRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.GetVenues() == nil {
		t.Fatal("expected non-nil empty venues map")
	}
	if len(resp.GetVenues()) != 0 {
		t.Fatalf("venues len = %d, want 0", len(resp.GetVenues()))
	}
}

func TestBatchCheckSlotAvailability_EmptySlots(t *testing.T) {
	s := newValidationServer()
	resp, err := s.BatchCheckSlotAvailability(context.Background(), &venuev1.BatchCheckSlotRequest{
		VenueId: uuid.New().String(),
		Date:    "2026-06-30",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetAvailable()) != 0 {
		t.Fatalf("available len = %d, want 0", len(resp.GetAvailable()))
	}
}

func TestBatchCheckSlotAvailability_TooManySlots(t *testing.T) {
	s := newValidationServer()
	slots := make([]*venuev1.SlotInterval, 101)
	for i := range slots {
		slots[i] = &venuev1.SlotInterval{TimeFrom: "10:00", TimeTo: "11:00"}
	}
	_, err := s.BatchCheckSlotAvailability(context.Background(), &venuev1.BatchCheckSlotRequest{
		VenueId: uuid.New().String(),
		Date:    "2026-06-30",
		Slots:   slots,
	})
	wantCode(t, err, codes.InvalidArgument)
}
