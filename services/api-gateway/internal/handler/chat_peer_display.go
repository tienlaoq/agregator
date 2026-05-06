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

func userDisplayName(ctx context.Context, users userGatewayClient, userID string) string {
	if users == nil || strings.TrimSpace(userID) == "" {
		return ""
	}
	u, err := users.GetUser(ctx, &userv1.GetUserRequest{Id: userID})
	if err != nil || u == nil {
		return ""
	}
	if n := strings.TrimSpace(u.GetName()); n != "" {
		return n
	}
	if e := strings.TrimSpace(u.GetEmail()); e != "" {
		if at := strings.IndexByte(e, '@'); at > 0 {
			return e[:at]
		}
		return e
	}
	return ""
}

// peerDisplayNamesBatch resolves peer_display_name for many threads with at most one
// GetBooking per distinct venue_booking ref_id, one GetVenue per distinct venue_id (guest view only),
// one GetMasterBooking per distinct master_booking ref_id, one GetUser per distinct user id needed.
func (h *ChatHandler) peerDisplayNamesBatch(ctx context.Context, viewerID string, threads []*chatv1.ChatThread) map[string]string {
	out := make(map[string]string, len(threads))
	if len(threads) == 0 || h == nil {
		return out
	}

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
			if _, ok := seenVenueRef[ref]; ok {
				continue
			}
			seenVenueRef[ref] = struct{}{}
			venueRefsUnique = append(venueRefsUnique, ref)
		case "master_booking":
			if _, ok := seenMasterRef[ref]; ok {
				continue
			}
			seenMasterRef[ref] = struct{}{}
			masterRefsUnique = append(masterRefsUnique, ref)
		}
	}

	bookingByRef := make(map[string]*bookingv1.BookingResponse, len(venueRefsUnique))
	if h.booking != nil {
		for _, ref := range venueRefsUnique {
			b, err := h.booking.GetBooking(ctx, &bookingv1.GetBookingRequest{Id: ref})
			if err != nil || b == nil {
				continue
			}
			bookingByRef[ref] = b
		}
	}

	// Гостю по брони нужно название заведения; владельцу/CRM — имя клиента (GetUser), не карточка venue.
	venueIDsNeedingName := make(map[string]struct{})
	for _, b := range bookingByRef {
		clientID := strings.TrimSpace(b.GetUserId())
		if !sameUserID(viewerID, clientID) {
			continue
		}
		if strings.TrimSpace(b.GetVenueName()) != "" {
			continue
		}
		if vid := strings.TrimSpace(b.GetVenueId()); vid != "" {
			venueIDsNeedingName[vid] = struct{}{}
		}
	}
	venueNameByID := make(map[string]string, len(venueIDsNeedingName))
	if h.venue != nil {
		for vid := range venueIDsNeedingName {
			v, err := h.venue.GetVenue(ctx, &venuev1.GetVenueRequest{Id: vid})
			if err != nil || v == nil {
				continue
			}
			venueNameByID[vid] = strings.TrimSpace(v.GetName())
		}
	}

	masterBkByRef := make(map[string]*masterv1.MasterBooking, len(masterRefsUnique))
	if h.master != nil {
		for _, ref := range masterRefsUnique {
			mb, err := h.master.GetMasterBooking(ctx, &masterv1.GetMasterBookingRequest{
				BookingId:   ref,
				ActorUserId: viewerID,
			})
			if err != nil || mb.GetBooking() == nil {
				continue
			}
			masterBkByRef[ref] = mb.GetBooking()
		}
	}

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
	if h.users != nil {
		for uid := range userIDs {
			userNameByID[uid] = userDisplayName(ctx, h.users, uid)
		}
	}

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
				if n := strings.TrimSpace(b.GetVenueName()); n != "" {
					out[threadID] = n
					continue
				}
				vid := strings.TrimSpace(b.GetVenueId())
				if n := venueNameByID[vid]; n != "" {
					out[threadID] = n
				}
				continue
			}
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
