package domain

import "context"

type BookingRepository interface {
	Create(ctx context.Context, b *Booking) error
	GetByID(ctx context.Context, id string) (*Booking, error)
	ListByUser(ctx context.Context, userID, status string, offset, limit int) ([]*Booking, int, error)
	ListByVenue(ctx context.Context, venueID, status, date string, offset, limit int) ([]*Booking, int, error)
	UpdateStatus(ctx context.Context, id, status string) error
	SetPaymentID(ctx context.Context, bookingID, paymentID string) error
	HasCompleted(ctx context.Context, userID, venueID string) (bool, error)
}
