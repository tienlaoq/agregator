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

// pastDate returns today minus n days in YYYY-MM-DD, so confirmed bookings on it
// count as "visited" (their date is strictly before CURRENT_DATE / today).
func pastDate(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("2006-01-02")
}

func booking(masterID, clientID uuid.UUID, status, date string, price int64, createdDaysAgo int) *domain.MasterBooking {
	return &domain.MasterBooking{
		ID:           uuid.New(),
		MasterID:     masterID,
		ClientUserID: clientID,
		Status:       status,
		Date:         date,
		TotalPrice:   price,
		CreatedAt:    time.Now().AddDate(0, 0, -createdDaysAgo),
	}
}

func TestListMyClients(t *testing.T) {
	master := activeMaster()
	regular := uuid.New() // 3 completed visits, recent
	fresh := uuid.New()   // 1 pending booking, never visited
	sleeping := uuid.New()

	seed := func(r *stubRepo) {
		r.addMaster(master)
		// regular: 3 visited bookings, most recent 2 days ago
		r.addBooking(booking(master.ID, regular, domain.BookingStatusCompleted, pastDate(40), 2000, 40))
		r.addBooking(booking(master.ID, regular, domain.BookingStatusCompleted, pastDate(20), 2000, 20))
		r.addBooking(booking(master.ID, regular, domain.BookingStatusConfirmed, pastDate(2), 3000, 2))
		// fresh: single pending booking → segment "new", never visited
		r.addBooking(booking(master.ID, fresh, domain.BookingStatusPending, futureDate(), 1500, 1))
		// sleeping: 2 visits, last one 200 days ago → "at_risk"
		r.addBooking(booking(master.ID, sleeping, domain.BookingStatusCompleted, pastDate(300), 1000, 300))
		r.addBooking(booking(master.ID, sleeping, domain.BookingStatusCompleted, pastDate(200), 1000, 200))
	}

	t.Run("aggregates counts, spend and segments", func(t *testing.T) {
		repo := newStubRepo()
		seed(repo)
		uc := newUC(repo, &stubPayment{})

		clients, total, err := uc.ListMyClients(context.Background(), master.UserID, domain.MasterClientListParams{})
		if err != nil {
			t.Fatal(err)
		}
		if total != 3 || len(clients) != 3 {
			t.Fatalf("got total=%d len=%d, want 3/3", total, len(clients))
		}
		by := map[uuid.UUID]domain.MasterClient{}
		for _, c := range clients {
			by[c.UserID] = c
		}
		if c := by[regular]; c.BookingsCount != 3 || c.VisitsCount != 3 || c.TotalSpent != 7000 ||
			!contains(c.Segments, domain.ClientSegmentRegular) {
			t.Fatalf("regular projection wrong: %+v", c)
		}
		if c := by[fresh]; c.VisitsCount != 0 || c.TotalSpent != 0 ||
			!contains(c.Segments, domain.ClientSegmentNew) {
			t.Fatalf("fresh projection wrong: %+v", c)
		}
		if c := by[sleeping]; !contains(c.Segments, domain.ClientSegmentAtRisk) {
			t.Fatalf("sleeping should be at_risk: %+v", c)
		}
	})

	t.Run("default sort is most-recent-visit first", func(t *testing.T) {
		repo := newStubRepo()
		seed(repo)
		uc := newUC(repo, &stubPayment{})

		clients, _, err := uc.ListMyClients(context.Background(), master.UserID, domain.MasterClientListParams{})
		if err != nil {
			t.Fatal(err)
		}
		// regular visited 2 days ago (most recent); fresh never visited (last).
		if clients[0].UserID != regular {
			t.Fatalf("first client = %v, want regular", clients[0].UserID)
		}
		if clients[len(clients)-1].UserID != fresh {
			t.Fatalf("last client = %v, want fresh (never visited)", clients[len(clients)-1].UserID)
		}
	})

	t.Run("segment filter narrows total and page", func(t *testing.T) {
		repo := newStubRepo()
		seed(repo)
		uc := newUC(repo, &stubPayment{})

		clients, total, err := uc.ListMyClients(context.Background(), master.UserID,
			domain.MasterClientListParams{Segment: domain.ClientSegmentAtRisk})
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(clients) != 1 || clients[0].UserID != sleeping {
			t.Fatalf("at_risk filter got total=%d len=%d, want the sleeping client", total, len(clients))
		}
	})

	t.Run("ltv sort orders by spend", func(t *testing.T) {
		repo := newStubRepo()
		seed(repo)
		uc := newUC(repo, &stubPayment{})

		clients, _, err := uc.ListMyClients(context.Background(), master.UserID,
			domain.MasterClientListParams{Sort: "ltv"})
		if err != nil {
			t.Fatal(err)
		}
		if clients[0].UserID != regular { // 7000 kopecks, the top spender
			t.Fatalf("ltv sort first = %v, want regular", clients[0].UserID)
		}
	})

	t.Run("limit and offset paginate over the filtered total", func(t *testing.T) {
		repo := newStubRepo()
		seed(repo)
		uc := newUC(repo, &stubPayment{})

		page, total, err := uc.ListMyClients(context.Background(), master.UserID,
			domain.MasterClientListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatal(err)
		}
		if total != 3 || len(page) != 2 {
			t.Fatalf("got total=%d len=%d, want total=3 len=2", total, len(page))
		}
	})

	t.Run("invalid segment is rejected", func(t *testing.T) {
		repo := newStubRepo()
		seed(repo)
		uc := newUC(repo, &stubPayment{})

		_, _, err := uc.ListMyClients(context.Background(), master.UserID,
			domain.MasterClientListParams{Segment: "platinum"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got %v, want InvalidArgument", err)
		}
	})

	t.Run("missing profile is not found", func(t *testing.T) {
		repo := newStubRepo()
		uc := newUC(repo, &stubPayment{})

		_, _, err := uc.ListMyClients(context.Background(), uuid.New(), domain.MasterClientListParams{})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got %v, want NotFound", err)
		}
	})
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
