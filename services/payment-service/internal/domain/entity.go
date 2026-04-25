package domain

import "time"

type Payment struct {
	ID                      string
	BookingID               string
	Amount                  int64
	Status                  string
	ProviderID              string
	PaymentURL              string
	IdempotencyKey          string
	PlatformFeeKopecks      int64
	CounterpartyNetKopecks  int64
	CounterpartyType        string
	CounterpartyID          string
	YooKassaSellerAccountID string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// UsesYooKassaSplitCapture is true when the payment was created with ЮKassa transfers and capture=false (needs POST /capture after authorization).
func (p *Payment) UsesYooKassaSplitCapture() bool {
	return p.YooKassaSellerAccountID != ""
}
