package domain

import (
	"context"

	"github.com/google/uuid"
)

type MasterRepository interface {
	Insert(ctx context.Context, m *Master) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*Master, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Master, error)
	GetBySlug(ctx context.Context, slug string) (*Master, error)
	UpdateProfile(ctx context.Context, m *Master) error
	UpdateStatus(ctx context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID) error
	ListByStatus(ctx context.Context, statusFilter string, limit, offset int32) ([]Master, int32, error)
	ListPublic(ctx context.Context, params ListPublicMastersParams) ([]Master, int32, error)
	ReplaceServices(ctx context.Context, masterID uuid.UUID, items []MasterServiceUpsert) error
	InsertModerationHistory(ctx context.Context, e *ModerationHistoryEntry) error
	ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]ModerationHistoryEntry, error)
	InsertBooking(ctx context.Context, b *MasterBooking) error
	GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*MasterBooking, error)
	ListBookingsByMaster(ctx context.Context, masterID uuid.UUID, statusFilter string) ([]MasterBooking, error)
	ListBookingsByClient(ctx context.Context, clientUserID uuid.UUID, statusFilter string) ([]MasterBooking, error)
	HasCompletedBookingByClientMaster(ctx context.Context, clientUserID, masterID uuid.UUID) (bool, error)

	CountPhotosByMaster(ctx context.Context, masterID uuid.UUID) (int32, error)
	AddMasterPhoto(ctx context.Context, masterID uuid.UUID, url string) (*MasterPhoto, error)
	DeleteMasterPhoto(ctx context.Context, masterID, photoID uuid.UUID) (deletedURL string, err error)
	SetMasterCoverPhoto(ctx context.Context, masterID, photoID uuid.UUID) error
}
