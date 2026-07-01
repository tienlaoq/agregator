package suspendnotify

import (
	"context"
	"sort"
	"testing"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
)

func TestContains(t *testing.T) {
	sl := []string{"a", "b", "c"}
	if !contains(sl, "b") {
		t.Fatal("contains should find existing element")
	}
	if contains(sl, "z") {
		t.Fatal("contains should not find missing element")
	}
	if contains(nil, "x") {
		t.Fatal("contains on nil slice should be false")
	}
}

// Without SMTP env configured, the sender is disabled and the public Notify*
// methods must be safe no-ops (they early-return without spawning goroutines or
// touching the gRPC clients).
func TestSender_DisabledByDefault(t *testing.T) {
	s := NewSender(zerolog.Nop(), nil, nil)
	if s.Enabled() {
		t.Skip("SMTP appears configured in this environment; skipping disabled-path test")
	}
	// These must not panic despite nil clients, because Enabled() short-circuits.
	s.NotifyVenueSuspended(context.Background(), "v1", "owner", "Баня", "comment")
	s.NotifyVenueResumed(context.Background(), "v1", "owner", "Баня", "note")
}

// ── recipientEmails: staff fan-out + de-duplication ──────────────────────────

type crmStub struct {
	crmv1.CRMServiceClient
	members []*crmv1.StaffMember
	err     error
}

func (s *crmStub) ListStaff(_ context.Context, _ *crmv1.ListStaffRequest, _ ...grpc.CallOption) (*crmv1.ListStaffResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &crmv1.ListStaffResponse{Members: s.members}, nil
}

type userStub struct {
	userv1.UserServiceClient
	emails map[string]string // userID → email; missing key → NotFound
}

func (s *userStub) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	email, ok := s.emails[in.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "no such user")
	}
	return &userv1.UserResponse{Email: email}, nil
}

func newSender(crm crmv1.CRMServiceClient, users userv1.UserServiceClient) *Sender {
	return &Sender{crm: crm, userClient: users, log: zerolog.Nop()}
}

func TestRecipientEmails_DedupsOwnerAndStaff(t *testing.T) {
	crm := &crmStub{members: []*crmv1.StaffMember{
		{UserId: "staff-1"},
		{UserId: "owner"}, // duplicate of owner → must collapse
		{UserId: ""},      // blank → skipped
	}}
	users := &userStub{emails: map[string]string{
		"owner":   "Owner@Example.com",
		"staff-1": "staff@example.com",
	}}
	s := newSender(crm, users)

	got, err := s.recipientEmails(context.Background(), "v1", "owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"Owner@Example.com", "staff@example.com"}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestRecipientEmails_SkipsNotFoundUsers(t *testing.T) {
	crm := &crmStub{members: []*crmv1.StaffMember{{UserId: "ghost"}}}
	users := &userStub{emails: map[string]string{"owner": "owner@example.com"}}
	s := newSender(crm, users)

	got, err := s.recipientEmails(context.Background(), "v1", "owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "owner@example.com" {
		t.Fatalf("NotFound staff should be skipped, got %v", got)
	}
}

// When crm.ListStaff fails, the owner must still be notified (graceful degrade).
func TestRecipientEmails_StaffErrorFallsBackToOwner(t *testing.T) {
	crm := &crmStub{err: status.Error(codes.Unavailable, "down")}
	users := &userStub{emails: map[string]string{"owner": "owner@example.com"}}
	s := newSender(crm, users)

	got, err := s.recipientEmails(context.Background(), "v1", "owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "owner@example.com" {
		t.Fatalf("owner should still be resolved despite staff error, got %v", got)
	}
}

// A non-NotFound error from GetUser must propagate.
func TestRecipientEmails_GetUserHardError(t *testing.T) {
	crm := &crmStub{}
	users := &errUserStub{}
	s := newSender(crm, users)

	if _, err := s.recipientEmails(context.Background(), "v1", "owner"); err == nil {
		t.Fatal("a non-NotFound GetUser error should propagate")
	}
}

type errUserStub struct{ userv1.UserServiceClient }

func (errUserStub) GetUser(_ context.Context, _ *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	return nil, status.Error(codes.Internal, "boom")
}

func TestDeliver_EmptyRecipientsIsNoOp(t *testing.T) {
	s := newSender(nil, nil)
	if err := s.deliver(context.Background(), nil, "subj", "body"); err != nil {
		t.Fatalf("deliver with no recipients should be a no-op, got %v", err)
	}
}
