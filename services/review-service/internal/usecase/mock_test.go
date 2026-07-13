package usecase

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
)

// ---------------------------------------------------------------------------
// mockTx — minimal pgx.Tx stub; only Commit/Rollback are exercised by tests.
// ---------------------------------------------------------------------------

type mockTx struct {
	committed  bool
	rolledBack bool
	commitErr  error
}

func (m *mockTx) Begin(ctx context.Context) (pgx.Tx, error) { return m, nil }
func (m *mockTx) Commit(ctx context.Context) error          { m.committed = true; return m.commitErr }
func (m *mockTx) Rollback(ctx context.Context) error        { m.rolledBack = true; return nil }
func (m *mockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (m *mockTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (m *mockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (m *mockTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}
func (m *mockTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (m *mockTx) Conn() *pgx.Conn                                               { return nil }

// ---------------------------------------------------------------------------
// mockReviewRepo
// ---------------------------------------------------------------------------

type mockReviewRepo struct {
	tx                        *mockTx
	CreateTxFunc              func(ctx context.Context, tx pgx.Tx, review *domain.Review) error
	GetByIDFunc               func(ctx context.Context, id string) (*domain.Review, error)
	ListByVenueFunc           func(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error)
	ListByMasterFunc          func(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error)
	GetVenueRatingFunc        func(ctx context.Context, venueID string) (*domain.VenueRating, error)
	GetVenueReviewSummaryFunc func(ctx context.Context, venueID string) (*domain.VenueReviewSummary, error)
	UpdateVenueRatingTxFunc   func(ctx context.Context, tx pgx.Tx, venueID string, rating int32) error
	GetMasterRatingFunc       func(ctx context.Context, masterID string) (*domain.MasterRating, error)
	UpdateMasterRatingTxFunc  func(ctx context.Context, tx pgx.Tx, masterID string, rating int32) error
	UpsertReplyFunc           func(ctx context.Context, reviewID, venueID, authorUserID, body string) (*domain.ReviewReply, error)
	DeleteReplyFunc           func(ctx context.Context, reviewID, venueID string) error
}

func (m *mockReviewRepo) BeginTx(ctx context.Context) (pgx.Tx, error) {
	if m.tx == nil {
		m.tx = &mockTx{}
	}
	return m.tx, nil
}

func (m *mockReviewRepo) CreateTx(ctx context.Context, tx pgx.Tx, review *domain.Review) error {
	if m.CreateTxFunc != nil {
		return m.CreateTxFunc(ctx, tx, review)
	}
	return nil
}

func (m *mockReviewRepo) GetByID(ctx context.Context, id string) (*domain.Review, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockReviewRepo) ListByVenue(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error) {
	if m.ListByVenueFunc != nil {
		return m.ListByVenueFunc(ctx, venueID, onlyUnanswered, page, pageSize)
	}
	return nil, 0, nil
}

func (m *mockReviewRepo) GetVenueReviewSummary(ctx context.Context, venueID string) (*domain.VenueReviewSummary, error) {
	if m.GetVenueReviewSummaryFunc != nil {
		return m.GetVenueReviewSummaryFunc(ctx, venueID)
	}
	return &domain.VenueReviewSummary{VenueID: venueID}, nil
}

func (m *mockReviewRepo) UpsertReply(ctx context.Context, reviewID, venueID, authorUserID, body string) (*domain.ReviewReply, error) {
	if m.UpsertReplyFunc != nil {
		return m.UpsertReplyFunc(ctx, reviewID, venueID, authorUserID, body)
	}
	return &domain.ReviewReply{Body: body, AuthorUserID: authorUserID}, nil
}

func (m *mockReviewRepo) DeleteReply(ctx context.Context, reviewID, venueID string) error {
	if m.DeleteReplyFunc != nil {
		return m.DeleteReplyFunc(ctx, reviewID, venueID)
	}
	return nil
}

func (m *mockReviewRepo) ListByMaster(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	if m.ListByMasterFunc != nil {
		return m.ListByMasterFunc(ctx, masterID, page, pageSize)
	}
	return nil, 0, nil
}

func (m *mockReviewRepo) GetVenueRating(ctx context.Context, venueID string) (*domain.VenueRating, error) {
	if m.GetVenueRatingFunc != nil {
		return m.GetVenueRatingFunc(ctx, venueID)
	}
	return nil, nil
}

func (m *mockReviewRepo) UpdateVenueRatingTx(ctx context.Context, tx pgx.Tx, venueID string, rating int32) error {
	if m.UpdateVenueRatingTxFunc != nil {
		return m.UpdateVenueRatingTxFunc(ctx, tx, venueID, rating)
	}
	return nil
}

func (m *mockReviewRepo) GetMasterRating(ctx context.Context, masterID string) (*domain.MasterRating, error) {
	if m.GetMasterRatingFunc != nil {
		return m.GetMasterRatingFunc(ctx, masterID)
	}
	return &domain.MasterRating{MasterID: masterID}, nil
}

func (m *mockReviewRepo) UpdateMasterRatingTx(ctx context.Context, tx pgx.Tx, masterID string, rating int32) error {
	if m.UpdateMasterRatingTxFunc != nil {
		return m.UpdateMasterRatingTxFunc(ctx, tx, masterID, rating)
	}
	return nil
}

// ---------------------------------------------------------------------------
// mockOutboxRepo
// ---------------------------------------------------------------------------

type mockOutboxRepo struct {
	CreateTxFunc        func(ctx context.Context, tx pgx.Tx, entry *domain.OutboxEntry) error
	ClaimBatchFunc      func(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.OutboxEntry, error)
	MarkPublishedFunc   func(ctx context.Context, ids []string) error
	ResetStaleLocksFunc func(ctx context.Context) error
	entries             []*domain.OutboxEntry
}

func (m *mockOutboxRepo) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return &mockTx{}, nil
}

func (m *mockOutboxRepo) CreateTx(ctx context.Context, tx pgx.Tx, entry *domain.OutboxEntry) error {
	if m.CreateTxFunc != nil {
		return m.CreateTxFunc(ctx, tx, entry)
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockOutboxRepo) ClaimBatch(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.OutboxEntry, error) {
	if m.ClaimBatchFunc != nil {
		return m.ClaimBatchFunc(ctx, tx, limit)
	}
	return nil, nil
}

func (m *mockOutboxRepo) MarkPublished(ctx context.Context, ids []string) error {
	if m.MarkPublishedFunc != nil {
		return m.MarkPublishedFunc(ctx, ids)
	}
	return nil
}

func (m *mockOutboxRepo) ResetStaleLocks(ctx context.Context) error {
	if m.ResetStaleLocksFunc != nil {
		return m.ResetStaleLocksFunc(ctx)
	}
	return nil
}

// ---------------------------------------------------------------------------
// mockBookingClient
// ---------------------------------------------------------------------------

type mockBookingClient struct {
	CreateBookingFunc       func(ctx context.Context, in *bookingv1.CreateBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	GetBookingFunc          func(ctx context.Context, in *bookingv1.GetBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	ListUserBookingsFunc    func(ctx context.Context, in *bookingv1.ListUserBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error)
	ListVenueBookingsFunc   func(ctx context.Context, in *bookingv1.ListVenueBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error)
	CancelBookingFunc       func(ctx context.Context, in *bookingv1.CancelBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	ConfirmBookingFunc      func(ctx context.Context, in *bookingv1.ConfirmBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	CompleteBookingFunc     func(ctx context.Context, in *bookingv1.CompleteBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	HasCompletedBookingFunc func(ctx context.Context, in *bookingv1.HasCompletedBookingRequest, opts ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error)
}

func (m *mockBookingClient) CreateBooking(ctx context.Context, in *bookingv1.CreateBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.CreateBookingFunc != nil {
		return m.CreateBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) GetBooking(ctx context.Context, in *bookingv1.GetBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.GetBookingFunc != nil {
		return m.GetBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) ListUserBookings(ctx context.Context, in *bookingv1.ListUserBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	if m.ListUserBookingsFunc != nil {
		return m.ListUserBookingsFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) ListVenueBookings(ctx context.Context, in *bookingv1.ListVenueBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	if m.ListVenueBookingsFunc != nil {
		return m.ListVenueBookingsFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) CancelBooking(ctx context.Context, in *bookingv1.CancelBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.CancelBookingFunc != nil {
		return m.CancelBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) ConfirmBooking(ctx context.Context, in *bookingv1.ConfirmBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.ConfirmBookingFunc != nil {
		return m.ConfirmBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) CompleteBooking(ctx context.Context, in *bookingv1.CompleteBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.CompleteBookingFunc != nil {
		return m.CompleteBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockBookingClient) HasCompletedBooking(ctx context.Context, in *bookingv1.HasCompletedBookingRequest, opts ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error) {
	if m.HasCompletedBookingFunc != nil {
		return m.HasCompletedBookingFunc(ctx, in, opts...)
	}
	return &bookingv1.HasCompletedBookingResponse{}, nil
}
func (m *mockBookingClient) GetBookingsBatch(ctx context.Context, in *bookingv1.GetBookingsBatchRequest, opts ...grpc.CallOption) (*bookingv1.GetBookingsBatchResponse, error) {
	return &bookingv1.GetBookingsBatchResponse{}, nil
}
func (m *mockBookingClient) AddBookingStaffNote(ctx context.Context, in *bookingv1.AddBookingStaffNoteRequest, opts ...grpc.CallOption) (*bookingv1.AddBookingStaffNoteResponse, error) {
	return &bookingv1.AddBookingStaffNoteResponse{}, nil
}
func (m *mockBookingClient) ListBookingStaffNotes(ctx context.Context, in *bookingv1.ListBookingStaffNotesRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingStaffNotesResponse, error) {
	return &bookingv1.ListBookingStaffNotesResponse{}, nil
}

// ---------------------------------------------------------------------------
// mockMasterClient — implements masterv1.MasterServiceClient
// ---------------------------------------------------------------------------

type mockMasterClient struct {
	hasCompleted bool
	err          error
}

func (m *mockMasterClient) HasCompletedMasterBooking(ctx context.Context, in *masterv1.HasCompletedMasterBookingRequest, opts ...grpc.CallOption) (*masterv1.HasCompletedMasterBookingResponse, error) {
	return &masterv1.HasCompletedMasterBookingResponse{HasCompleted: m.hasCompleted}, m.err
}
func (m *mockMasterClient) CreateMyProfile(ctx context.Context, in *masterv1.CreateMyProfileRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) GetMyProfile(ctx context.Context, in *masterv1.GetMyProfileRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) UpdateMyProfile(ctx context.Context, in *masterv1.UpdateMyProfileRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) SubmitForReview(ctx context.Context, in *masterv1.SubmitMasterForReviewRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListPublicMasters(ctx context.Context, in *masterv1.ListPublicMastersRequest, opts ...grpc.CallOption) (*masterv1.ListMastersResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) GetPublicMaster(ctx context.Context, in *masterv1.GetPublicMasterRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) GetMaster(ctx context.Context, in *masterv1.GetMasterRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListForModeration(ctx context.Context, in *masterv1.ListForModerationRequest, opts ...grpc.CallOption) (*masterv1.ListMastersResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ModerateMaster(ctx context.Context, in *masterv1.ModerateMasterRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListModerationHistory(ctx context.Context, in *masterv1.ListModerationHistoryRequest, opts ...grpc.CallOption) (*masterv1.ListModerationHistoryResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) CreateMasterBooking(ctx context.Context, in *masterv1.CreateMasterBookingRequest, opts ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListMyMasterBookings(ctx context.Context, in *masterv1.ListMyMasterBookingsRequest, opts ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListClientMasterBookings(ctx context.Context, in *masterv1.ListClientMasterBookingsRequest, opts ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListMyMasterClients(ctx context.Context, in *masterv1.ListMyMasterClientsRequest, opts ...grpc.CallOption) (*masterv1.ListMyMasterClientsResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) GetMasterBooking(ctx context.Context, in *masterv1.GetMasterBookingRequest, opts ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) GetMasterBookingsBatch(ctx context.Context, in *masterv1.GetMasterBookingsBatchRequest, opts ...grpc.CallOption) (*masterv1.GetMasterBookingsBatchResponse, error) {
	return &masterv1.GetMasterBookingsBatchResponse{}, nil
}
func (m *mockMasterClient) AddMasterPhoto(ctx context.Context, in *masterv1.AddMasterPhotoRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) DeleteMasterPhoto(ctx context.Context, in *masterv1.DeleteMasterPhotoRequest, opts ...grpc.CallOption) (*masterv1.DeleteMasterPhotoResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) SetMasterCoverPhoto(ctx context.Context, in *masterv1.SetMasterCoverPhotoRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) AddMasterVideo(ctx context.Context, in *masterv1.AddMasterVideoRequest, opts ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) DeleteMasterVideo(ctx context.Context, in *masterv1.DeleteMasterVideoRequest, opts ...grpc.CallOption) (*masterv1.DeleteMasterVideoResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) SuspendMasterByUser(ctx context.Context, in *masterv1.SuspendMasterByUserRequest, opts ...grpc.CallOption) (*masterv1.SuspendMasterByUserResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) CreateMasterSlotBlock(ctx context.Context, in *masterv1.CreateMasterSlotBlockRequest, opts ...grpc.CallOption) (*masterv1.MasterSlotBlockResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) ListMasterSlotBlocks(ctx context.Context, in *masterv1.ListMasterSlotBlocksRequest, opts ...grpc.CallOption) (*masterv1.ListMasterSlotBlocksResponse, error) {
	return nil, nil
}
func (m *mockMasterClient) DeleteMasterSlotBlock(ctx context.Context, in *masterv1.DeleteMasterSlotBlockRequest, opts ...grpc.CallOption) (*masterv1.DeleteMasterSlotBlockResponse, error) {
	return nil, nil
}
