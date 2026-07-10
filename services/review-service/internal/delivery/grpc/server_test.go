package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/usecase"
)

// mockRepo embeds domain.ReviewRepository so only the read methods the tested
// handlers touch need an implementation.
type mockRepo struct {
	domain.ReviewRepository
	GetByIDFunc         func(ctx context.Context, id string) (*domain.Review, error)
	ListByVenueFunc     func(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error)
	ListByMasterFunc    func(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error)
	GetVenueRatingFunc  func(ctx context.Context, venueID string) (*domain.VenueRating, error)
	GetMasterRatingFunc func(ctx context.Context, masterID string) (*domain.MasterRating, error)
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.Review, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *mockRepo) ListByVenue(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error) {
	return m.ListByVenueFunc(ctx, venueID, onlyUnanswered, page, pageSize)
}
func (m *mockRepo) ListByMaster(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	return m.ListByMasterFunc(ctx, masterID, page, pageSize)
}
func (m *mockRepo) GetVenueRating(ctx context.Context, venueID string) (*domain.VenueRating, error) {
	return m.GetVenueRatingFunc(ctx, venueID)
}
func (m *mockRepo) GetMasterRating(ctx context.Context, masterID string) (*domain.MasterRating, error) {
	return m.GetMasterRatingFunc(ctx, masterID)
}

// BeginTx is unused by the read handlers but referenced by the interface; the
// embedded nil would panic, so provide a no-op returning an error.
func (m *mockRepo) BeginTx(context.Context) (pgx.Tx, error) {
	return nil, errors.New("BeginTx not stubbed")
}

func newServer(repo domain.ReviewRepository) *Server {
	// nil outbox/booking/master clients are unused on the read + validation paths.
	return NewServer(usecase.NewReviewUseCaseWithOutbox(repo, nil, nil, nil))
}

func wantCode(t *testing.T, err error, c codes.Code) {
	t.Helper()
	if status.Code(err) != c {
		t.Fatalf("status = %v, want %v (err: %v)", status.Code(err), c, err)
	}
}

func TestReviewToProto(t *testing.T) {
	r := &domain.Review{
		ID: "r1", UserID: "u1", UserName: "Иван", VenueID: "v1",
		Rating: 4, Text: "ок", IsVerified: true, IsAnonymous: true,
		CreatedAt: time.Unix(1700000000, 0),
	}
	p := reviewToProto(r)
	if p.GetId() != "r1" || p.GetUserName() != "Иван" || p.GetVenueId() != "v1" {
		t.Errorf("basic fields mismatch: %+v", p)
	}
	if p.GetRating() != 4 || !p.GetIsVerified() || !p.GetIsAnonymous() {
		t.Errorf("flags/rating mismatch: %+v", p)
	}
	if p.GetCreatedAt() == nil {
		t.Error("created_at not set")
	}
}

func TestGetReview_Success(t *testing.T) {
	repo := &mockRepo{GetByIDFunc: func(_ context.Context, id string) (*domain.Review, error) {
		return &domain.Review{ID: id, MasterID: "m1", Rating: 5}, nil
	}}
	resp, err := newServer(repo).GetReview(context.Background(), &reviewv1.GetReviewRequest{Id: "r1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetId() != "r1" || resp.GetMasterId() != "m1" || resp.GetRating() != 5 {
		t.Errorf("response mismatch: %+v", resp)
	}
}

func TestGetReview_ErrorPropagates(t *testing.T) {
	repo := &mockRepo{GetByIDFunc: func(context.Context, string) (*domain.Review, error) {
		return nil, status.Error(codes.NotFound, "no review")
	}}
	_, err := newServer(repo).GetReview(context.Background(), &reviewv1.GetReviewRequest{Id: "r1"})
	wantCode(t, err, codes.NotFound)
}

func TestListVenueReviews_Success(t *testing.T) {
	repo := &mockRepo{ListByVenueFunc: func(_ context.Context, venueID string, _ bool, page, pageSize int32) ([]*domain.Review, int32, error) {
		return []*domain.Review{{ID: "r1", VenueID: venueID}, {ID: "r2", VenueID: venueID}}, 7, nil
	}}
	resp, err := newServer(repo).ListVenueReviews(context.Background(), &reviewv1.ListVenueReviewsRequest{VenueId: "v1", Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetReviews()) != 2 || resp.GetTotal() != 7 {
		t.Errorf("list mismatch: count=%d total=%d", len(resp.GetReviews()), resp.GetTotal())
	}
}

func TestListMasterReviews_Success(t *testing.T) {
	repo := &mockRepo{ListByMasterFunc: func(_ context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error) {
		return []*domain.Review{{ID: "r1", MasterID: masterID}}, 1, nil
	}}
	resp, err := newServer(repo).ListMasterReviews(context.Background(), &reviewv1.ListMasterReviewsRequest{MasterId: "m1", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetReviews()) != 1 || resp.GetTotal() != 1 {
		t.Errorf("list mismatch: %+v", resp)
	}
}

func TestGetVenueRating_Success(t *testing.T) {
	repo := &mockRepo{GetVenueRatingFunc: func(_ context.Context, venueID string) (*domain.VenueRating, error) {
		return &domain.VenueRating{VenueID: venueID, AvgRating: 4.5, ReviewCount: 10}, nil
	}}
	resp, err := newServer(repo).GetVenueRating(context.Background(), &reviewv1.GetVenueRatingRequest{VenueId: "v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAvgRating() != 4.5 || resp.GetReviewCount() != 10 {
		t.Errorf("rating mismatch: %+v", resp)
	}
}

func TestGetMasterRating_Success(t *testing.T) {
	repo := &mockRepo{GetMasterRatingFunc: func(_ context.Context, masterID string) (*domain.MasterRating, error) {
		return &domain.MasterRating{MasterID: masterID, AvgRating: 3.0, ReviewCount: 2}, nil
	}}
	resp, err := newServer(repo).GetMasterRating(context.Background(), &reviewv1.GetMasterRatingRequest{MasterId: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAvgRating() != 3.0 || resp.GetReviewCount() != 2 {
		t.Errorf("rating mismatch: %+v", resp)
	}
}

func TestCreateReview_ValidationErrors(t *testing.T) {
	s := newServer(&mockRepo{})
	tests := []struct {
		name string
		req  *reviewv1.CreateReviewRequest
	}{
		{"rating too low", &reviewv1.CreateReviewRequest{VenueId: "v1", Rating: 0}},
		{"rating too high", &reviewv1.CreateReviewRequest{VenueId: "v1", Rating: 6}},
		{"no target", &reviewv1.CreateReviewRequest{Rating: 4}},
		{"both targets", &reviewv1.CreateReviewRequest{VenueId: "v1", MasterId: "m1", Rating: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateReview(context.Background(), tt.req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
}
