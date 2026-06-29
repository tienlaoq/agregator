package usecase

import (
	"context"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func (uc *MasterUseCase) ListPublic(ctx context.Context, params domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	return uc.repo.ListPublic(ctx, params)
}

func (uc *MasterUseCase) GetPublicBySlug(ctx context.Context, slug string) (*domain.Master, error) {
	m, err := uc.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if m == nil || m.Status != domain.StatusActive {
		return nil, pkgerrors.NotFound("master not found")
	}
	return m, nil
}
