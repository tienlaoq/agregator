package domain

import "context"

type PaymentRepository interface {
	// CreateIdempotent inserts p if no row with the same idempotency_key exists.
	// When the INSERT succeeds it fills p.ID, p.CreatedAt, p.UpdatedAt and
	// returns isNew=true.  When a conflict is detected it returns isNew=false
	// without modifying p; the caller must fetch the existing row with
	// GetByIdempotencyKey.
	CreateIdempotent(ctx context.Context, p *Payment) (isNew bool, err error)

	// UpdateProviderInfo persists the provider reference (provider_id, payment_url)
	// returned by the external payment provider after a successful payment creation.
	// Called in the second phase of CreatePayment, after CreateIdempotent.
	UpdateProviderInfo(ctx context.Context, id, providerID, paymentURL string) error

	GetByID(ctx context.Context, id string) (*Payment, error)
	GetByBookingID(ctx context.Context, bookingID string) (*Payment, error)
	GetByProviderID(ctx context.Context, providerID string) (*Payment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Payment, error)
	// UpdateStatus transitions a payment to the given status only when it is not
	// yet in a terminal state (succeeded, cancelled, or refunded).  It returns
	// updated=true when the row was actually changed, and updated=false when the
	// payment was already terminal (duplicate webhook delivery).
	UpdateStatus(ctx context.Context, id string, status PaymentStatus, providerID string) (updated bool, err error)

	// UpdateStatusWithOutbox is the atomic variant of UpdateStatus: it runs the
	// UPDATE and the outbox INSERT in a single database transaction.  Use this
	// whenever the caller needs to publish a domain event — the event row is
	// visible to the outbox worker only after the transaction commits, so there
	// is no window where the payment is updated but the event is lost.
	//
	// Returns updated=false (no error) when the payment is already terminal, in
	// which case no outbox row is written.
	UpdateStatusWithOutbox(ctx context.Context, id string, status PaymentStatus, providerID string, event *OutboxEvent) (updated bool, err error)

	// DriveProviderWithLock serialises concurrent crash-recovery attempts for the
	// same idempotency key using a PostgreSQL advisory lock.
	//
	// Problem it solves:
	//   After CreateIdempotent returns isNew=false with an empty ProviderID, two
	//   concurrent requests (original + retry) can both enter the "re-drive"
	//   branch and call the external payment provider twice.  YooKassa is safe
	//   because it deduplicates on Idempotence-Key, but other providers (TBank,
	//   Sber) may not offer the same guarantee, and in mock mode two calls
	//   produce two different UUIDs that overwrite each other in UpdateProviderInfo.
	//
	// How it works:
	//   1. Opens a transaction.
	//   2. Calls pg_advisory_xact_lock(hash(idempotencyKey)) — blocks until the
	//      lock is acquired; automatically released when the transaction ends.
	//   3. Re-reads the row inside the transaction.  If ProviderID is already
	//      filled (the first requester finished while we waited), returns the row
	//      without calling fn — zero provider calls for the second requester.
	//   4. Otherwise calls fn(p) which must return (providerID, paymentURL, error).
	//      On success, writes the result via UpdateProviderInfo inside the same
	//      transaction and commits.
	//   5. On fn error, rolls back and returns the error — the caller retries.
	//
	// fn is called at most once per lock acquisition, guaranteeing exactly-once
	// provider invocation even under concurrent retries.
	DriveProviderWithLock(ctx context.Context, idempotencyKey string, fn func(p *Payment) (providerID, paymentURL string, err error)) (*Payment, error)
}
