// Deleted: Publisher and all direct-publish helpers have been removed.
//
// Payment status events are now published exclusively via the transactional
// outbox pattern: PaymentUseCase writes an OutboxEvent row in the same DB
// transaction as the status UPDATE, and internal/outbox/worker.go relays it
// to NATS asynchronously.  This eliminates the race where a DB write succeeded
// but the in-process NATS publish was lost (service crash, network partition).
//
// Do NOT re-add a synchronous Publisher here.  If you need a new event type,
// add it to domain/outbox.go (OutboxSubject constant + subject string) and wire
// it through UpdateStatusWithOutbox in the usecase.
package events
