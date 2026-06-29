package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"google.golang.org/grpc"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

type MasterUseCase struct {
	repo          domain.MasterRepository
	paymentClient paymentGatewayClient
	log           zerolog.Logger
}

type paymentGatewayClient interface {
	CreatePayment(ctx context.Context, in *paymentv1.CreatePaymentRequest, opts ...grpc.CallOption) (*paymentv1.PaymentResponse, error)
}

// NewMasterUseCase constructs a MasterUseCase. paymentClient must be non-nil;
// a nil payment client is a programmer error and panics at startup rather than
// surfacing as a runtime error deep inside CreateBooking.
func NewMasterUseCase(repo domain.MasterRepository, paymentClient paymentGatewayClient, log zerolog.Logger) *MasterUseCase {
	if paymentClient == nil {
		panic("NewMasterUseCase: paymentClient must not be nil")
	}
	return &MasterUseCase{repo: repo, paymentClient: paymentClient, log: log}
}

// maxMasterPhotos moved to domain.MaxMasterPhotos so the repo and usecase
// share the same value; the limit is now enforced inside the repo transaction.

// GetMasterUserIDsBatch returns map[masterID]userID for a set of master profile ids.
// Used by gRPC handlers to populate master_user_id on batch responses without N+1.
func (uc *MasterUseCase) GetMasterUserIDsBatch(ctx context.Context, masterIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	return uc.repo.GetMasterUserIDsByIDs(ctx, masterIDs)
}

// MasterOwnerUserID returns the platform user id for the master profile (venue owner account).
func (uc *MasterUseCase) MasterOwnerUserID(ctx context.Context, masterID uuid.UUID) (uuid.UUID, error) {
	m, err := uc.repo.GetByID(ctx, masterID)
	if err != nil {
		return uuid.Nil, err
	}
	if m == nil {
		return uuid.Nil, pkgerrors.NotFound("master not found")
	}
	return m.UserID, nil
}

// GetByID returns the full master profile by id. Used for internal lookups
// (e.g. resolving master_id → owner user_id + display_name for notifications).
func (uc *MasterUseCase) GetByID(ctx context.Context, masterID uuid.UUID) (*domain.Master, error) {
	m, err := uc.repo.GetByID(ctx, masterID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master not found")
	}
	return m, nil
}
