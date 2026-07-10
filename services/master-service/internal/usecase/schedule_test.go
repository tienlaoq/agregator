package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func TestCreateSlotBlock(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	tests := []struct {
		name           string
		date, from, to string
		note           string
		wantCode       codes.Code
		wantWholeDay   bool
	}{
		{name: "whole day", date: tomorrow, wantCode: codes.OK, wantWholeDay: true},
		{name: "timed interval", date: tomorrow, from: "14:00", to: "16:00", wantCode: codes.OK},
		{name: "past date rejected", date: yesterday, wantCode: codes.InvalidArgument},
		{name: "bad date", date: "31-12-2026", wantCode: codes.InvalidArgument},
		{name: "half interval rejected", date: tomorrow, from: "14:00", wantCode: codes.InvalidArgument},
		{name: "to before from", date: tomorrow, from: "16:00", to: "14:00", wantCode: codes.InvalidArgument},
		{name: "note too long", date: tomorrow, note: string(make([]rune, 201)), wantCode: codes.InvalidArgument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newStubRepo()
			m := activeMaster()
			repo.addMaster(m)
			uc := newUC(repo, &stubPayment{})

			b, err := uc.CreateSlotBlock(context.Background(), m.UserID, tc.date, tc.from, tc.to, tc.note)
			if status.Code(err) != tc.wantCode {
				t.Fatalf("got code %v (err=%v), want %v", status.Code(err), err, tc.wantCode)
			}
			if tc.wantCode != codes.OK {
				return
			}
			if b.WholeDay() != tc.wantWholeDay {
				t.Fatalf("WholeDay()=%v, want %v", b.WholeDay(), tc.wantWholeDay)
			}
			if b.CreatedAt.IsZero() {
				t.Fatal("CreatedAt not set by repo")
			}
		})
	}
}

func TestSlotBlockLifecycle(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := newUC(repo, &stubPayment{})
	ctx := context.Background()
	date := time.Now().AddDate(0, 0, 3).Format("2006-01-02")

	b, err := uc.CreateSlotBlock(ctx, m.UserID, date, "", "", "Отпуск")
	if err != nil {
		t.Fatal(err)
	}

	list, err := uc.ListSlotBlocks(ctx, m.UserID)
	if err != nil || len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("list=%v err=%v, want 1 block %v", list, err, b.ID)
	}

	t.Run("delete by another master is not found", func(t *testing.T) {
		other := activeMaster()
		other.UserID = uuid.New()
		repo.addMaster(other)
		if err := uc.DeleteSlotBlock(ctx, other.UserID, b.ID); status.Code(err) != codes.NotFound {
			t.Fatalf("got %v, want NotFound", err)
		}
	})

	t.Run("owner deletes", func(t *testing.T) {
		if err := uc.DeleteSlotBlock(ctx, m.UserID, b.ID); err != nil {
			t.Fatal(err)
		}
		list, _ := uc.ListSlotBlocks(ctx, m.UserID)
		if len(list) != 0 {
			t.Fatalf("after delete list has %d, want 0", len(list))
		}
	})
}

func TestSlotBlockNoProfile(t *testing.T) {
	repo := newStubRepo()
	uc := newUC(repo, &stubPayment{})
	date := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	if _, err := uc.CreateSlotBlock(context.Background(), uuid.New(), date, "", "", ""); status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	_ = domain.MasterSlotBlock{}
}
