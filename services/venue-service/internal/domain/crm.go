package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	ManagementAccessOwner   = "owner"
	ManagementAccessManager = "manager"
	ManagementAccessStaff   = "staff"

	VenueCRMStaffRoleManager = "manager"
	VenueCRMStaffRoleStaff   = "staff"

	VenueCRMTaskStatusOpen = "open"
	VenueCRMTaskStatusDone = "done"
)

type VenueStaff struct {
	UserID    uuid.UUID
	Role      string
	InvitedBy uuid.UUID
	CreatedAt time.Time
}

type VenueCRMTask struct {
	ID               uuid.UUID
	VenueID          uuid.UUID
	BookingID        *uuid.UUID
	Title            string
	Body             string
	Status           string
	AssigneeUserID   *uuid.UUID
	CreatedBy        uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
