package domain

import (
	"time"

	"github.com/google/uuid"
)

// MaxSlotBlockNote bounds the free-text note on a slot block. Enforced in the
// usecase and mirrored by a CHECK constraint in migration 023.
const MaxSlotBlockNote = 200

// MasterSlotBlock is an interval when the master does not accept bookings —
// vacation, a busy day, or a one-off break. TimeFrom/TimeTo are "HH:MM"; both
// empty means the whole DATE is blocked.
type MasterSlotBlock struct {
	ID        uuid.UUID
	MasterID  uuid.UUID
	Date      string // YYYY-MM-DD
	TimeFrom  string // "HH:MM" or "" for whole day
	TimeTo    string // "HH:MM" or "" for whole day
	Note      string
	CreatedAt time.Time
}

// WholeDay reports whether the block covers the entire date (no time interval).
func (b *MasterSlotBlock) WholeDay() bool {
	return b.TimeFrom == "" && b.TimeTo == ""
}
