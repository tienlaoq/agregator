package handler

import (
	"context"
	"strings"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
)

// peerDisplayNamesBatch resolves peer_display_name for many threads in at most 4 RPC
// round-trips regardless of the number of threads:
//  1. GetBookingsBatch  — all distinct venue_booking ref_ids (one call)
//  2. GetVenuesBatch    — all venue_ids where the viewer is the guest and venue_name
//     was not stored on the booking (one call, may be empty)
//  3. GetMasterBookingsBatch — all distinct master_booking ref_ids (one call)
//  4. GetUsersBatch     — all user_ids that need display names (one call)
func (h *chatThreadResolver) peerDisplayNamesBatch(ctx context.Context, viewerID string, threads []*chatv1.ChatThread) map[string]string {
	out := make(map[string]string, len(threads))
	if len(threads) == 0 || h == nil {
		return out
	}

	// ── Collect distinct ref_ids per kind ─────────────────────────────────────
	venueRefsUnique := make([]string, 0)
	seenVenueRef := make(map[string]struct{}, len(threads))
	masterRefsUnique := make([]string, 0)
	seenMasterRef := make(map[string]struct{}, len(threads))

	for _, t := range threads {
		if t == nil {
			continue
		}
		ref := strings.TrimSpace(t.GetRefId())
		if ref == "" {
			continue
		}
		switch strings.TrimSpace(t.GetKind()) {
		case "venue_booking":
			if _, ok := seenVenueRef[ref]; !ok {
				seenVenueRef[ref] = struct{}{}
				venueRefsUnique = append(venueRefsUnique, ref)
			}
		case "master_booking":
			if _, ok := seenMasterRef[ref]; !ok {
				seenMasterRef[ref] = struct{}{}
				masterRefsUnique = append(masterRefsUnique, ref)
			}
		}
	}

	// ── Round-trip 1: GetBookingsBatch ────────────────────────────────────────
	bookingByRef := make(map[string]*bookingv1.BookingResponse, len(venueRefsUnique))
	if h.booking != nil && len(venueRefsUnique) > 0 {
		resp, err := h.booking.GetBookingsBatch(ctx, &bookingv1.GetBookingsBatchRequest{Ids: venueRefsUnique})
		if err == nil && resp != nil {
			for id, b := range resp.GetBookings() {
				if b != nil {
					bookingByRef[id] = b
				}
			}
		}
	}

	// ── Round-trip 2: GetVenuesBatch (guest view only, when venue_name absent) ─
	venueIDsNeedingName := make([]string, 0)
	seenVenueID := make(map[string]struct{})
	for _, b := range bookingByRef {
		clientID := strings.TrimSpace(b.GetUserId())
		if !sameUserID(viewerID, clientID) {
			continue // owner side: will use user name, not venue card
		}
		if strings.TrimSpace(b.GetVenueName()) != "" {
			continue // venue_name already stored on booking — no extra fetch needed
		}
		if vid := strings.TrimSpace(b.GetVenueId()); vid != "" {
			if _, ok := seenVenueID[vid]; !ok {
				seenVenueID[vid] = struct{}{}
				venueIDsNeedingName = append(venueIDsNeedingName, vid)
			}
		}
	}
	venueNameByID := make(map[string]string, len(venueIDsNeedingName))
	if h.venue != nil && len(venueIDsNeedingName) > 0 {
		resp, err := h.venue.GetVenuesBatch(ctx, &venuev1.GetVenuesBatchRequest{Ids: venueIDsNeedingName})
		if err == nil && resp != nil {
			for id, v := range resp.GetVenues() {
				if v != nil {
					venueNameByID[id] = strings.TrimSpace(v.GetName())
				}
			}
		}
	}

	// ── Round-trip 3: GetMasterBookingsBatch ──────────────────────────────────
	masterBkByRef := make(map[string]*masterv1.MasterBooking, len(masterRefsUnique))
	if h.master != nil && len(masterRefsUnique) > 0 {
		resp, err := h.master.GetMasterBookingsBatch(ctx, &masterv1.GetMasterBookingsBatchRequest{
			BookingIds:  masterRefsUnique,
			ActorUserId: viewerID,
		})
		if err == nil && resp != nil {
			for id, bk := range resp.GetBookings() {
				if bk != nil {
					masterBkByRef[id] = bk
				}
			}
		}
	}

	// ── Collect all user IDs → Round-trip 4: GetUsersBatch ───────────────────
	userIDs := make(map[string]struct{})
	for _, b := range bookingByRef {
		cid := strings.TrimSpace(b.GetUserId())
		if cid == "" || sameUserID(viewerID, cid) {
			continue
		}
		userIDs[cid] = struct{}{}
	}
	for _, bk := range masterBkByRef {
		if id := strings.TrimSpace(bk.GetClientUserId()); id != "" {
			userIDs[id] = struct{}{}
		}
		if id := strings.TrimSpace(bk.GetMasterUserId()); id != "" {
			userIDs[id] = struct{}{}
		}
	}

	userNameByID := make(map[string]string, len(userIDs))
	if h.users != nil && len(userIDs) > 0 {
		ids := make([]string, 0, len(userIDs))
		for uid := range userIDs {
			ids = append(ids, uid)
		}
		batchResp, err := h.users.GetUsersBatch(ctx, &userv1.GetUsersBatchRequest{Ids: ids})
		if err == nil && batchResp != nil {
			for uid, u := range batchResp.GetUsers() {
				if u == nil {
					continue
				}
				if n := strings.TrimSpace(u.GetName()); n != "" {
					userNameByID[uid] = n
				} else if e := strings.TrimSpace(u.GetEmail()); e != "" {
					if at := strings.IndexByte(e, '@'); at > 0 {
						userNameByID[uid] = e[:at]
					} else {
						userNameByID[uid] = e
					}
				}
			}
		}
	}

	// ── Assemble output ───────────────────────────────────────────────────────
	for _, t := range threads {
		if t == nil {
			continue
		}
		threadID := t.GetId()
		ref := strings.TrimSpace(t.GetRefId())
		if ref == "" {
			continue
		}
		switch strings.TrimSpace(t.GetKind()) {
		case "venue_booking":
			b := bookingByRef[ref]
			if b == nil {
				continue
			}
			clientID := strings.TrimSpace(b.GetUserId())
			if sameUserID(viewerID, clientID) {
				// viewer is the guest: show venue name
				if n := strings.TrimSpace(b.GetVenueName()); n != "" {
					out[threadID] = n
					continue
				}
				if n := venueNameByID[strings.TrimSpace(b.GetVenueId())]; n != "" {
					out[threadID] = n
				}
				continue
			}
			// viewer is the owner/CRM: show client name
			if h.users == nil {
				continue
			}
			out[threadID] = userNameByID[clientID]
		case "master_booking":
			if h.users == nil {
				continue
			}
			bk := masterBkByRef[ref]
			if bk == nil {
				continue
			}
			clientID := strings.TrimSpace(bk.GetClientUserId())
			masterUID := strings.TrimSpace(bk.GetMasterUserId())
			if sameUserID(viewerID, clientID) {
				out[threadID] = userNameByID[masterUID]
			} else {
				out[threadID] = userNameByID[clientID]
			}
		}
	}
	return out
}

func attachPeerDisplayToThreadJSON(item map[string]any, threadID string, peerByThread map[string]string) {
	if peerByThread == nil || item == nil {
		return
	}
	if p := peerByThread[threadID]; p != "" {
		item["peer_display_name"] = p
	}
}
