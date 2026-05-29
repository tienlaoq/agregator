package domain

// PartnerRegisteredEvent carries the data needed to notify admins when a
// partner (master or venue_owner) completes registration.
type PartnerRegisteredEvent struct {
	Role   string // "master" or "venue_owner"
	UserID string
	Email  string
	Phone  string
	Name   string
}

// PartnerNotifier is the port through which the usecase layer fires partner-
// registration notifications without knowing about the transport (Telegram,
// Slack, webhook, …).
//
// Enqueue MUST be non-blocking: implementations should buffer the event and
// return immediately. If the buffer is full the implementation may log a
// warning and drop the event — losing a notification is preferable to blocking
// the registration response path.
type PartnerNotifier interface {
	Enqueue(event PartnerRegisteredEvent)
}
