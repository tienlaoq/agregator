package usecase

// Unit tests for the read/write wrapper methods and pure helper functions of
// MasterUseCase that were previously uncovered:
//
//   - master.go     : GetByID, MasterOwnerUserID, GetMasterUserIDsBatch
//   - booking.go    : ListMyBookings, ListClientBookings, GetBookingForActor,
//                     HasCompletedBookingByClientMaster
//   - moderation.go : ListForModeration, SuspendByUser, ListModerationHistory
//   - public.go     : ListPublic, GetPublicBySlug
//   - photo.go      : AddMasterPhoto, DeleteMasterPhoto, SetMasterCoverPhoto
//   - profile.go    : GetMyProfile, SubmitForReview, UpdateMyProfile and the
//                     travel/payout validators.
//
// All tests reuse the in-memory stubRepo / stubPayment doubles defined in
// master_usecase_test.go, so they run without a DB or network.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

// nopPayment is a payment client that is never called; the read paths below
// don't touch the payment gateway.
func nopUC(repo *stubRepo) *MasterUseCase { return newUC(repo, &stubPayment{}) }

// ── master.go ────────────────────────────────────────────────────────────────

func TestGetByID(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := nopUC(repo)

	t.Run("found", func(t *testing.T) {
		got, err := uc.GetByID(context.Background(), m.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != m.ID {
			t.Fatalf("got id %v, want %v", got.ID, m.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.GetByID(context.Background(), uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo := newStubRepo()
		repo.getByIDErr = errors.New("db down")
		_, err := nopUC(repo).GetByID(context.Background(), uuid.New())
		if err == nil || status.Code(err) == codes.NotFound {
			t.Fatalf("expected raw repo error, got %v", err)
		}
	})
}

func TestMasterOwnerUserID(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := nopUC(repo)

	t.Run("found", func(t *testing.T) {
		got, err := uc.MasterOwnerUserID(context.Background(), m.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != m.UserID {
			t.Fatalf("got owner %v, want %v", got, m.UserID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		got, err := uc.MasterOwnerUserID(context.Background(), uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
		if got != uuid.Nil {
			t.Fatalf("expected uuid.Nil on error, got %v", got)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo := newStubRepo()
		repo.getByIDErr = errors.New("db down")
		_, err := nopUC(repo).MasterOwnerUserID(context.Background(), uuid.New())
		if err == nil || status.Code(err) == codes.NotFound {
			t.Fatalf("expected raw repo error, got %v", err)
		}
	})
}

func TestGetMasterUserIDsBatch(t *testing.T) {
	repo := newStubRepo()
	m1, m2 := activeMaster(), activeMaster()
	repo.addMaster(m1)
	repo.addMaster(m2)
	uc := nopUC(repo)

	got, err := uc.GetMasterUserIDsBatch(context.Background(), []uuid.UUID{m1.ID, m2.ID, uuid.New()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[m1.ID] != m1.UserID || got[m2.ID] != m2.UserID {
		t.Fatalf("owner map mismatch: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("unknown master id should be absent, got %d entries", len(got))
	}
}

// ── booking.go ───────────────────────────────────────────────────────────────

func TestListMyBookings(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, Status: domain.BookingStatusConfirmed})
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, Status: domain.BookingStatusPending})
	uc := nopUC(repo)

	t.Run("all", func(t *testing.T) {
		out, err := uc.ListMyBookings(context.Background(), m.UserID, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("got %d bookings, want 2", len(out))
		}
	})

	t.Run("status filter", func(t *testing.T) {
		out, err := uc.ListMyBookings(context.Background(), m.UserID, domain.BookingStatusConfirmed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 || out[0].Status != domain.BookingStatusConfirmed {
			t.Fatalf("status filter failed: %+v", out)
		}
	})

	t.Run("no profile", func(t *testing.T) {
		_, err := uc.ListMyBookings(context.Background(), uuid.New(), "")
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("lookup error", func(t *testing.T) {
		repo := newStubRepo()
		repo.getByUserErr = errors.New("db down")
		_, err := nopUC(repo).ListMyBookings(context.Background(), uuid.New(), "")
		if err == nil || status.Code(err) == codes.NotFound {
			t.Fatalf("expected raw repo error, got %v", err)
		}
	})
}

func TestListClientBookings(t *testing.T) {
	repo := newStubRepo()
	client := uuid.New()
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), ClientUserID: client, Status: domain.BookingStatusConfirmed})
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), ClientUserID: uuid.New(), Status: domain.BookingStatusConfirmed})
	uc := nopUC(repo)

	out, err := uc.ListClientBookings(context.Background(), client, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].ClientUserID != client {
		t.Fatalf("expected only the caller's booking, got %+v", out)
	}
}

func TestGetBookingForActor(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	client := uuid.New()
	booking := &domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, ClientUserID: client}
	repo.addBooking(booking)
	uc := nopUC(repo)

	t.Run("client can view own booking", func(t *testing.T) {
		got, err := uc.GetBookingForActor(context.Background(), booking.ID, client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != booking.ID {
			t.Fatalf("got %v, want %v", got.ID, booking.ID)
		}
	})

	t.Run("master owner can view booking", func(t *testing.T) {
		got, err := uc.GetBookingForActor(context.Background(), booking.ID, m.UserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != booking.ID {
			t.Fatalf("got %v, want %v", got.ID, booking.ID)
		}
	})

	t.Run("stranger denied", func(t *testing.T) {
		_, err := uc.GetBookingForActor(context.Background(), booking.ID, uuid.New())
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("got code %v, want PermissionDenied", status.Code(err))
		}
	})

	t.Run("booking not found", func(t *testing.T) {
		_, err := uc.GetBookingForActor(context.Background(), uuid.New(), client)
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("master profile vanished", func(t *testing.T) {
		repo := newStubRepo()
		orphan := &domain.MasterBooking{ID: uuid.New(), MasterID: uuid.New(), ClientUserID: uuid.New()}
		repo.addBooking(orphan)
		_, err := nopUC(repo).GetBookingForActor(context.Background(), orphan.ID, uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

func TestHasCompletedBookingByClientMaster(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		repo := newStubRepo()
		repo.hasCompletedVal = true
		ok, err := nopUC(repo).HasCompletedBookingByClientMaster(context.Background(), uuid.New(), uuid.New())
		if err != nil || !ok {
			t.Fatalf("got (%v, %v), want (true, nil)", ok, err)
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := newStubRepo()
		repo.hasCompletedErr = errors.New("db down")
		_, err := nopUC(repo).HasCompletedBookingByClientMaster(context.Background(), uuid.New(), uuid.New())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// ── moderation.go ────────────────────────────────────────────────────────────

func TestListForModeration(t *testing.T) {
	repo := newStubRepo()
	for i := 0; i < 3; i++ {
		m := activeMaster()
		m.Status = domain.StatusPendingReview
		repo.addMaster(m)
	}
	active := activeMaster()
	repo.addMaster(active)
	uc := nopUC(repo)

	t.Run("filter by status", func(t *testing.T) {
		out, total, err := uc.ListForModeration(context.Background(), domain.StatusPendingReview, 10, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 3 || len(out) != 3 {
			t.Fatalf("got total=%d len=%d, want 3/3", total, len(out))
		}
	})

	t.Run("limit and offset", func(t *testing.T) {
		out, total, err := uc.ListForModeration(context.Background(), domain.StatusPendingReview, 2, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 3 || len(out) != 2 {
			t.Fatalf("got total=%d len=%d, want total=3 len=2", total, len(out))
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := newStubRepo()
		repo.listByStatusErr = errors.New("db down")
		_, _, err := nopUC(repo).ListForModeration(context.Background(), "", 10, 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestSuspendByUser(t *testing.T) {
	t.Run("suspends active profile", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		ok, err := nopUC(repo).SuspendByUser(context.Background(), m.UserID)
		if err != nil || !ok {
			t.Fatalf("got (%v, %v), want (true, nil)", ok, err)
		}
		if repo.masters[m.ID].Status != domain.StatusSuspended {
			t.Fatalf("status = %q, want suspended", repo.masters[m.ID].Status)
		}
	})

	t.Run("no profile is a no-op", func(t *testing.T) {
		repo := newStubRepo()
		ok, err := nopUC(repo).SuspendByUser(context.Background(), uuid.New())
		if err != nil || ok {
			t.Fatalf("got (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := newStubRepo()
		repo.suspendByUserErr = errors.New("db down")
		_, err := nopUC(repo).SuspendByUser(context.Background(), uuid.New())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestListModerationHistory(t *testing.T) {
	repo := newStubRepo()
	masterID := uuid.New()
	repo.history = []domain.ModerationHistoryEntry{
		{ID: uuid.New(), MasterID: masterID, NewStatus: domain.StatusActive},
		{ID: uuid.New(), MasterID: masterID, NewStatus: domain.StatusSuspended},
		{ID: uuid.New(), MasterID: uuid.New(), NewStatus: domain.StatusActive},
	}
	uc := nopUC(repo)

	t.Run("filters by master and honours limit", func(t *testing.T) {
		out, err := uc.ListModerationHistory(context.Background(), masterID, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("got %d entries, want 1 (limit)", len(out))
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := newStubRepo()
		repo.listModHistoryErr = errors.New("db down")
		_, err := nopUC(repo).ListModerationHistory(context.Background(), masterID, 10)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// ── public.go ────────────────────────────────────────────────────────────────

func TestListPublic(t *testing.T) {
	repo := newStubRepo()
	repo.addMaster(activeMaster())
	draft := activeMaster()
	draft.Status = domain.StatusDraft
	repo.addMaster(draft)
	uc := nopUC(repo)

	out, total, err := uc.ListPublic(context.Background(), domain.ListPublicMastersParams{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(out) != 1 {
		t.Fatalf("got total=%d len=%d, want 1/1 (only active)", total, len(out))
	}
}

func TestGetPublicBySlug(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	m.Slug = "ivan-banshchik"
	repo.addMaster(m)

	suspended := activeMaster()
	suspended.Slug = "hidden"
	suspended.Status = domain.StatusSuspended
	repo.addMaster(suspended)
	uc := nopUC(repo)

	t.Run("active master visible", func(t *testing.T) {
		got, err := uc.GetPublicBySlug(context.Background(), "ivan-banshchik")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Slug != "ivan-banshchik" {
			t.Fatalf("got slug %q", got.Slug)
		}
	})

	t.Run("non-active master hidden", func(t *testing.T) {
		_, err := uc.GetPublicBySlug(context.Background(), "hidden")
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("unknown slug", func(t *testing.T) {
		_, err := uc.GetPublicBySlug(context.Background(), "nope")
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

// ── photo.go ─────────────────────────────────────────────────────────────────

func TestAddMasterPhoto(t *testing.T) {
	t.Run("adds photo and first is cover", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		got, err := nopUC(repo).AddMasterPhoto(context.Background(), m.UserID, "https://cdn.example.com/masters/"+m.ID.String()+"/a.jpg")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Photos) != 1 || !got.Photos[0].IsCover {
			t.Fatalf("first photo should be cover: %+v", got.Photos)
		}
	})

	t.Run("no profile", func(t *testing.T) {
		repo := newStubRepo()
		_, err := nopUC(repo).AddMasterPhoto(context.Background(), uuid.New(), "https://x/y.jpg")
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("invalid url rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).AddMasterPhoto(context.Background(), m.UserID, "://bad url with spaces")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("photo limit reached maps to InvalidArgument", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		repo.addPhotoErr = domain.ErrPhotoLimitReached
		_, err := nopUC(repo).AddMasterPhoto(context.Background(), m.UserID, "https://cdn.example.com/masters/"+m.ID.String()+"/a.jpg")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})
}

func TestDeleteMasterPhoto(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := nopUC(repo)
	added, err := uc.AddMasterPhoto(context.Background(), m.UserID, "https://cdn.example.com/masters/"+m.ID.String()+"/a.jpg")
	if err != nil {
		t.Fatalf("setup add failed: %v", err)
	}
	photoID := added.Photos[0].ID

	t.Run("deletes and returns url", func(t *testing.T) {
		url, err := uc.DeleteMasterPhoto(context.Background(), m.UserID, photoID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url == "" {
			t.Fatal("expected non-empty url of deleted photo")
		}
	})

	t.Run("unknown photo maps to NotFound", func(t *testing.T) {
		_, err := uc.DeleteMasterPhoto(context.Background(), m.UserID, uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("no profile", func(t *testing.T) {
		repo := newStubRepo()
		_, err := nopUC(repo).DeleteMasterPhoto(context.Background(), uuid.New(), uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

func TestSetMasterCoverPhoto(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := nopUC(repo)
	base := "https://cdn.example.com/masters/" + m.ID.String() + "/"
	if _, err := uc.AddMasterPhoto(context.Background(), m.UserID, base+"a.jpg"); err != nil {
		t.Fatalf("setup add a failed: %v", err)
	}
	second, err := uc.AddMasterPhoto(context.Background(), m.UserID, base+"b.jpg")
	if err != nil {
		t.Fatalf("setup add b failed: %v", err)
	}
	secondID := second.Photos[1].ID

	t.Run("sets cover", func(t *testing.T) {
		got, err := uc.SetMasterCoverPhoto(context.Background(), m.UserID, secondID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, p := range got.Photos {
			if p.ID == secondID && !p.IsCover {
				t.Fatal("selected photo should be cover")
			}
			if p.ID != secondID && p.IsCover {
				t.Fatal("only the selected photo should be cover")
			}
		}
	})

	t.Run("unknown photo maps to NotFound", func(t *testing.T) {
		_, err := uc.SetMasterCoverPhoto(context.Background(), m.UserID, uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("no profile", func(t *testing.T) {
		repo := newStubRepo()
		_, err := nopUC(repo).SetMasterCoverPhoto(context.Background(), uuid.New(), uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

// ── profile.go: GetMyProfile / SubmitForReview ───────────────────────────────

func TestGetMyProfile(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := nopUC(repo)

	t.Run("found", func(t *testing.T) {
		got, err := uc.GetMyProfile(context.Background(), m.UserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != m.ID {
			t.Fatalf("got %v, want %v", got.ID, m.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.GetMyProfile(context.Background(), uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := newStubRepo()
		repo.getByUserErr = errors.New("db down")
		_, err := nopUC(repo).GetMyProfile(context.Background(), uuid.New())
		if err == nil || status.Code(err) == codes.NotFound {
			t.Fatalf("expected raw repo error, got %v", err)
		}
	})
}

// submittableMaster returns a draft profile that passes validateReadyForReview.
func submittableMaster() *domain.Master {
	m := activeMaster()
	m.Status = domain.StatusDraft
	m.DisplayName = "Иван Банщик"
	m.City = "москва"
	m.Phone = "79991234567"
	m.Bio = "Опытный банщик с большим стажем работы"
	return m
}

func TestSubmitForReview(t *testing.T) {
	t.Run("draft → pending_review", func(t *testing.T) {
		repo := newStubRepo()
		m := submittableMaster()
		repo.addMaster(m)
		got, err := nopUC(repo).SubmitForReview(context.Background(), m.UserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != domain.StatusPendingReview {
			t.Fatalf("status = %q, want pending_review", got.Status)
		}
	})

	t.Run("already pending is idempotent", func(t *testing.T) {
		repo := newStubRepo()
		m := submittableMaster()
		m.Status = domain.StatusPendingReview
		repo.addMaster(m)
		got, err := nopUC(repo).SubmitForReview(context.Background(), m.UserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != domain.StatusPendingReview {
			t.Fatalf("status = %q, want pending_review", got.Status)
		}
	})

	t.Run("active cannot be submitted", func(t *testing.T) {
		repo := newStubRepo()
		m := submittableMaster()
		m.Status = domain.StatusActive
		repo.addMaster(m)
		_, err := nopUC(repo).SubmitForReview(context.Background(), m.UserID)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("incomplete profile rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := submittableMaster()
		m.Bio = "too short"
		repo.addMaster(m)
		_, err := nopUC(repo).SubmitForReview(context.Background(), m.UserID)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("no profile", func(t *testing.T) {
		repo := newStubRepo()
		_, err := nopUC(repo).SubmitForReview(context.Background(), uuid.New())
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

// ── profile.go: UpdateMyProfile ──────────────────────────────────────────────

func strptr(s string) *string   { return &s }
func i32ptr(v int32) *int32     { return &v }
func i64ptr(v int64) *int64     { return &v }
func f64ptr(v float64) *float64 { return &v }

func TestUpdateMyProfile(t *testing.T) {
	t.Run("no profile", func(t *testing.T) {
		repo := newStubRepo()
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), uuid.New(), UpdateMasterInput{})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("updates fields and lowercases city", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		m.Status = domain.StatusDraft
		repo.addMaster(m)
		got, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			DisplayName: strptr("  Пётр  "),
			City:        strptr("Санкт-Петербург"),
			Bio:         strptr("  описание  "),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.DisplayName != "Пётр" {
			t.Fatalf("display name = %q, want trimmed", got.DisplayName)
		}
		if got.City != "санкт-петербург" {
			t.Fatalf("city = %q, want lowercased", got.City)
		}
	})

	t.Run("active profile goes back to pending_review after edit", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		got, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			Bio: strptr("новое описание профиля мастера"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != domain.StatusPendingReview {
			t.Fatalf("status = %q, want pending_review", got.Status)
		}
	})

	t.Run("invalid services list does not mutate the profile", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			Bio:                  strptr("новое описание профиля мастера"),
			ApplyServicesReplace: true,
			ServicesReplace:      []domain.MasterServiceUpsert{{Name: ""}}, // empty name → invalid
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
		// Validation runs before the write: status must still be active and the
		// new bio must not have been persisted.
		saved := repo.masters[m.ID]
		if saved.Status != domain.StatusActive {
			t.Fatalf("status = %q, want unchanged active (write leaked before validation)", saved.Status)
		}
		if saved.Bio == "новое описание профиля мастера" {
			t.Fatal("bio was persisted despite invalid services — write ran before validation")
		}
	})

	t.Run("failed credentials replace rolls back the whole save", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		repo.replaceCredsErr = errors.New("boom") // credentials step fails inside the tx
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			Bio:                     strptr("новое описание профиля мастера"),
			ApplyCredentialsReplace: true,
			CredentialsReplace: []domain.MasterCredentialUpsert{
				{Kind: domain.CredentialKindCertificate, Title: "Диплом"},
			},
		})
		if err == nil {
			t.Fatal("expected error when the credentials step fails, got nil")
		}
		// Single transaction: the scalar UPDATE must not survive the rollback.
		saved := repo.masters[m.ID]
		if saved.Status != domain.StatusActive {
			t.Fatalf("status = %q, want unchanged active (scalar write leaked past a failed tx)", saved.Status)
		}
		if saved.Bio == "новое описание профиля мастера" {
			t.Fatal("bio persisted despite a failed credentials step — writes were not atomic")
		}
	})

	t.Run("clearing an existing phone is rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		m.Phone = "79991234567"
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			Phone: strptr(""),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("malformed phone rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			Phone: strptr("abc"),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("invalid work_format rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			WorkFormat: strptr("teleport"),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("travel fields rejected for venue format", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			WorkFormat:     strptr(domain.WorkFormatVenue),
			TravelRadiusKm: i32ptr(10),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("lone latitude rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			TravelBaseLatitude: f64ptr(55.75),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("invalid availability json rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			AvailabilityJSON: strptr("[1,2,3]"),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("too many specializations rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		specs := make([]string, domain.MaxSpecializations+1)
		for i := range specs {
			specs[i] = "spec"
		}
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			ApplySpecializations: true,
			Specializations:      specs,
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("services and credentials replaced", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		m.Status = domain.StatusDraft
		repo.addMaster(m)
		got, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			ApplyServicesReplace: true,
			ServicesReplace: []domain.MasterServiceUpsert{
				{Name: "Парение", DurationMin: 60, Price: 5000},
			},
			ApplyCredentialsReplace: true,
			CredentialsReplace: []domain.MasterCredentialUpsert{
				{Kind: domain.CredentialKindCertificate, Title: "Сертификат банщика"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Services) != 1 {
			t.Fatalf("got %d services, want 1", len(got.Services))
		}
	})

	t.Run("invalid service rejected", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			ApplyServicesReplace: true,
			ServicesReplace:      []domain.MasterServiceUpsert{{Name: "  "}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("update error propagates", func(t *testing.T) {
		repo := newStubRepo()
		m := activeMaster()
		repo.addMaster(m)
		repo.updateProfileErr = errors.New("db down")
		_, err := nopUC(repo).UpdateMyProfile(context.Background(), m.UserID, UpdateMasterInput{
			Bio: strptr("новое описание профиля"),
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
