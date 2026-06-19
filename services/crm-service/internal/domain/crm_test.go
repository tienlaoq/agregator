package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestCan(t *testing.T) {
	tests := []struct {
		access string
		cap    Capability
		want   bool
	}{
		{AccessOwner, CapViewCRM, true},
		{AccessManager, CapViewCRM, true},
		{AccessStaff, CapViewCRM, true},
		{"", CapViewCRM, false},

		{AccessOwner, CapManageStaff, true},
		{AccessManager, CapManageStaff, false},
		{AccessStaff, CapManageStaff, false},

		{AccessOwner, CapManageAnyTask, true},
		{AccessManager, CapManageAnyTask, true},
		{AccessStaff, CapManageAnyTask, false},
		{"", CapManageAnyTask, false},
	}
	for _, tt := range tests {
		if got := Can(tt.access, tt.cap); got != tt.want {
			t.Errorf("Can(%q, %d) = %v, want %v", tt.access, tt.cap, got, tt.want)
		}
	}
}

func TestNormalizePriority(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", TaskPriorityNormal, true},
		{"  ", TaskPriorityNormal, true},
		{"low", TaskPriorityLow, true},
		{"NORMAL", TaskPriorityNormal, true},
		{" High ", TaskPriorityHigh, true},
		{"urgent", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizePriority(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("NormalizePriority(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestTaskOwnedBy(t *testing.T) {
	creator, assignee, stranger := uuid.New(), uuid.New(), uuid.New()
	byCreator := &Task{CreatedBy: creator}
	byAssignee := &Task{CreatedBy: creator, AssigneeUserID: &assignee}

	if !byCreator.OwnedBy(creator) {
		t.Error("creator should own their task")
	}
	if !byAssignee.OwnedBy(assignee) {
		t.Error("assignee should own the task")
	}
	if byCreator.OwnedBy(stranger) {
		t.Error("stranger must not own the task")
	}
	if byAssignee.OwnedBy(stranger) {
		t.Error("stranger must not own the assigned task")
	}
}
