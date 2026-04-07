package domain

import "time"

type Payment struct {
	ID             string
	BookingID      string
	Amount         int64
	Status         string
	ProviderID     string
	PaymentURL     string
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
