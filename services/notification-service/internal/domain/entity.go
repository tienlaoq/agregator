package domain

import (
	"time"

	"github.com/google/uuid"
)

// Notification is a single per-user inbox item rendered in the web "bell".
// ReadAt is nil while unread; set to the read timestamp once acknowledged.
type Notification struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Type   string
	Title  string
	Body   string
	// Data is an optional JSON object (as a string) with structured fields the
	// client uses to build a link, e.g. {"venue_id":"...","role":"staff"}.
	// Empty string when there is no structured payload.
	Data      string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// DeviceToken is a registered mobile push token plus the platform it belongs to.
// Platform ("ios" | "android" | "web" | "") routes the token to the right push
// provider during fan-out.
type DeviceToken struct {
	Token    string
	Platform string
}
