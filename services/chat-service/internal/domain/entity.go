package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	ThreadKindVenueBooking  = "venue_booking"
	ThreadKindMasterBooking = "master_booking"
)

// ParticipantRead is the furthest chat_messages row another user has acknowledged (MarkRead).
type ParticipantRead struct {
	UserID            string
	LastReadMessageID *uuid.UUID
}

type Thread struct {
	ID                 uuid.UUID
	Kind               string
	RefID              uuid.UUID
	ParticipantUserIDs []string
	LastMessageID      *uuid.UUID
	LastMessageAt      *time.Time
	UnreadCount        int32
	ParticipantReads   []ParticipantRead
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Message struct {
	ID           uuid.UUID
	ThreadID     uuid.UUID
	AuthorUserID uuid.UUID
	Text         string
	ClientMsgID  string
	CreatedAt    time.Time
}
