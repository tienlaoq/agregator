package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

// ListMyClients returns the authenticated master's client base, aggregated from
// their bookings. Segments are computed on read; filtering, sorting and
// pagination are applied here in memory (a solo master's client count is
// modest, so a full fetch + in-memory slice is simpler and safe). Returns the
// page of clients plus the total number matching the segment filter (before
// pagination).
func (uc *MasterUseCase) ListMyClients(ctx context.Context, userID uuid.UUID, params domain.MasterClientListParams) ([]domain.MasterClient, int, error) {
	if params.Segment != "" && !domain.ValidClientSegment(params.Segment) {
		return nil, 0, pkgerrors.InvalidArgument("unknown segment")
	}
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if m == nil {
		return nil, 0, pkgerrors.NotFound("master profile not found")
	}
	clients, err := uc.repo.ListClientsByMaster(ctx, m.ID)
	if err != nil {
		return nil, 0, err
	}

	now := time.Now()
	filtered := clients[:0]
	for i := range clients {
		clients[i].Segments = domain.ComputeClientSegments(&clients[i], now)
		if params.Segment != "" && !containsString(clients[i].Segments, params.Segment) {
			continue
		}
		filtered = append(filtered, clients[i])
	}

	sortClients(filtered, params.Sort)
	total := len(filtered)

	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []domain.MasterClient{}, total, nil
	}
	end := total
	if params.Limit > 0 && offset+params.Limit < end {
		end = offset + params.Limit
	}
	return filtered[offset:end], total, nil
}

// sortClients orders clients in place per the sort key. Default ("" or
// "last_visit") is most-recently-visited first; clients with no visit sink to
// the bottom. "ltv" sorts by total spent desc, "visits" by visit count desc.
func sortClients(clients []domain.MasterClient, key string) {
	switch key {
	case "ltv":
		sort.SliceStable(clients, func(i, j int) bool {
			return clients[i].TotalSpent > clients[j].TotalSpent
		})
	case "visits":
		sort.SliceStable(clients, func(i, j int) bool {
			return clients[i].VisitsCount > clients[j].VisitsCount
		})
	default: // "" | last_visit
		sort.SliceStable(clients, func(i, j int) bool {
			return lastVisitAfter(clients[i].LastVisitAt, clients[j].LastVisitAt)
		})
	}
}

// lastVisitAfter reports whether a is a more recent visit than b. A nil visit
// is treated as the oldest possible, so never-visited clients sort last.
func lastVisitAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func containsString(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
