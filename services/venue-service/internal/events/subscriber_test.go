package events

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/pkg/metrics"
)

// fakeRatingFetcher records the venue_id it was asked about and returns a canned
// aggregate (or error).
type fakeRatingFetcher struct {
	resp      *reviewv1.VenueRatingResponse
	err       error
	gotVenue  string
	callCount int
}

func (f *fakeRatingFetcher) GetVenueRating(_ context.Context, in *reviewv1.GetVenueRatingRequest, _ ...grpc.CallOption) (*reviewv1.VenueRatingResponse, error) {
	f.callCount++
	f.gotVenue = in.GetVenueId()
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// fakeRatingUpdater records the arguments of the last UpdateRating call.
type fakeRatingUpdater struct {
	err       error
	gotVenue  uuid.UUID
	gotAvg    float64
	gotCount  int32
	callCount int
}

func (f *fakeRatingUpdater) UpdateRating(_ context.Context, venueID uuid.UUID, avg float64, count int32) error {
	f.callCount++
	f.gotVenue = venueID
	f.gotAvg = avg
	f.gotCount = count
	return f.err
}

func TestHandleReviewCreated(t *testing.T) {
	venueID := uuid.New()

	tests := []struct {
		name           string
		data           string
		fetcher        *fakeRatingFetcher
		updater        *fakeRatingUpdater
		wantFetchCalls int
		wantUpdateCall bool
		wantAvg        float64
		wantCount      int32
	}{
		{
			name: "venue event recomputes and updates rating",
			data: `{"review_id":"r1","target_type":"venue","venue_id":"` + venueID.String() + `","rating":5}`,
			fetcher: &fakeRatingFetcher{resp: &reviewv1.VenueRatingResponse{
				VenueId: venueID.String(), AvgRating: 4.5, ReviewCount: 12,
			}},
			updater:        &fakeRatingUpdater{},
			wantFetchCalls: 1,
			wantUpdateCall: true,
			wantAvg:        4.5,
			wantCount:      12,
		},
		{
			name:           "master event is ignored",
			data:           `{"review_id":"r2","target_type":"master","master_id":"m1","rating":3}`,
			fetcher:        &fakeRatingFetcher{},
			updater:        &fakeRatingUpdater{},
			wantFetchCalls: 0,
			wantUpdateCall: false,
		},
		{
			name:           "malformed json does not call downstream",
			data:           `{not-json`,
			fetcher:        &fakeRatingFetcher{},
			updater:        &fakeRatingUpdater{},
			wantFetchCalls: 0,
			wantUpdateCall: false,
		},
		{
			name:           "invalid venue_id does not fetch",
			data:           `{"review_id":"r3","target_type":"venue","venue_id":"not-a-uuid","rating":5}`,
			fetcher:        &fakeRatingFetcher{},
			updater:        &fakeRatingUpdater{},
			wantFetchCalls: 0,
			wantUpdateCall: false,
		},
		{
			name:           "fetch error skips update",
			data:           `{"review_id":"r4","target_type":"venue","venue_id":"` + venueID.String() + `","rating":5}`,
			fetcher:        &fakeRatingFetcher{err: errors.New("review-service down")},
			updater:        &fakeRatingUpdater{},
			wantFetchCalls: 1,
			wantUpdateCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSubscriber(nil, tt.fetcher, tt.updater, zerolog.Nop(), metrics.New("venue-service-test"))
			s.handleReviewCreated(&nats.Msg{Subject: subjectReviewCreated, Data: []byte(tt.data)})

			if tt.fetcher.callCount != tt.wantFetchCalls {
				t.Fatalf("fetch calls = %d, want %d", tt.fetcher.callCount, tt.wantFetchCalls)
			}
			if tt.wantUpdateCall {
				if tt.updater.callCount != 1 {
					t.Fatalf("update calls = %d, want 1", tt.updater.callCount)
				}
				if tt.updater.gotVenue != venueID {
					t.Errorf("update venueID = %s, want %s", tt.updater.gotVenue, venueID)
				}
				if tt.updater.gotAvg != tt.wantAvg {
					t.Errorf("update avg = %v, want %v", tt.updater.gotAvg, tt.wantAvg)
				}
				if tt.updater.gotCount != tt.wantCount {
					t.Errorf("update count = %d, want %d", tt.updater.gotCount, tt.wantCount)
				}
			} else if tt.updater.callCount != 0 {
				t.Fatalf("update calls = %d, want 0", tt.updater.callCount)
			}
		})
	}

	// Authoritative-aggregate contract: the single-review Rating in the event
	// payload must NOT leak into the stored aggregate — only GetVenueRating's
	// response is trusted.
	t.Run("ignores per-review rating in payload", func(t *testing.T) {
		fetcher := &fakeRatingFetcher{resp: &reviewv1.VenueRatingResponse{AvgRating: 2.0, ReviewCount: 1}}
		updater := &fakeRatingUpdater{}
		s := NewSubscriber(nil, fetcher, updater, zerolog.Nop(), metrics.New("venue-service-test"))
		s.handleReviewCreated(&nats.Msg{Subject: subjectReviewCreated,
			Data: []byte(`{"target_type":"venue","venue_id":"` + venueID.String() + `","rating":5}`)})
		if updater.gotAvg != 2.0 {
			t.Errorf("stored avg = %v, want 2.0 (from GetVenueRating, not payload rating 5)", updater.gotAvg)
		}
	})
}
