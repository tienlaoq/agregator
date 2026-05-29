package handler

// chatThreadResolver encapsulates the business logic for determining which
// users participate in a chat thread for a given booking reference.
//
// P1#7: This logic must live somewhere above chat-service (which is a pure
// messaging store and does not know about bookings or venues).  The correct
// location is the API gateway's orchestration layer — not spread across the
// ChatHandler HTTP methods.  By isolating it here the gateway stays thin:
// ChatHandler only routes and authenticates; chatThreadResolver owns the
// "who participates in this thread?" domain rule.
//
// When a dedicated chat-orchestrator micro-service is introduced, this file
// moves there with minimal changes.

import (
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"
)

// chatThreadResolver resolves participants and calls EnsureThread on the
// chat-service.  It is embedded in ChatHandler and can be tested in isolation.
type chatThreadResolver struct {
	log     zerolog.Logger
	chat    chatGatewayClient
	booking bookingGatewayClient
	venue   venueGatewayClient
	master  masterGatewayClient
	users   userGatewayClient
	crm     crmGatewayClient
}

func newChatThreadResolver(
	log zerolog.Logger,
	chat chatGatewayClient,
	booking bookingGatewayClient,
	venue venueGatewayClient,
	master masterGatewayClient,
	users userGatewayClient,
	crm crmGatewayClient,
) *chatThreadResolver {
	return &chatThreadResolver{
		log:     log,
		chat:    chat,
		booking: booking,
		venue:   venue,
		master:  master,
		users:   users,
		crm:     crm,
	}
}

// ensureVenueBookingThread resolves participants for a venue booking and
// creates/returns the chat thread.
//
// Access rules:
//   - The booking client is always a participant.
//   - The venue owner is always a participant (when resolvable).
//   - Staff members listed by ListVenueStaff are participants.
//   - userID must be the booking client, venue owner, or have management access.
//
// All three venue-service calls are fail-closed: an error means we cannot
// determine access or build the full participant set, so we deny (P0#2).
func (rs *chatThreadResolver) ensureVenueBookingThread(r *http.Request, userID, refID string) (*chatv1.ChatThread, error) {
	ctx := r.Context()

	b, err := rs.booking.GetBooking(ctx, &bookingv1.GetBookingRequest{Id: refID})
	if err != nil {
		return nil, err
	}
	venueID := b.GetVenueId()
	clientID := b.GetUserId()
	allowed := sameUserID(clientID, userID)
	participants := []string{clientID}
	var ownerID string

	if venueID != "" {
		v, vErr := rs.venue.GetVenue(ctx, &venuev1.GetVenueRequest{Id: venueID})
		if vErr != nil {
			rs.log.Error().Err(vErr).Str("venue_id", venueID).Str("user_id", userID).
				Msg("chat: GetVenue failed, denying access")
			return nil, pkgerrors.PermissionDenied("chat access denied")
		}
		ownerID = v.GetOwnerId()
		if sameUserID(ownerID, userID) {
			allowed = true
		}

		acc, aErr := rs.crm.GetManagementAccess(ctx, &crmv1.GetManagementAccessRequest{
			VenueId: venueID,
			UserId:  userID,
		})
		if aErr != nil {
			rs.log.Error().Err(aErr).Str("venue_id", venueID).Str("user_id", userID).
				Msg("chat: crm.GetManagementAccess failed, denying access")
			return nil, pkgerrors.PermissionDenied("chat access denied")
		}
		if strings.TrimSpace(acc.GetAccess()) != "" {
			allowed = true
		}

		staffResp, sErr := rs.crm.ListStaff(ctx, &crmv1.ListStaffRequest{
			VenueId: venueID,
			ActorId: userID,
		})
		if sErr != nil {
			rs.log.Error().Err(sErr).Str("venue_id", venueID).Str("user_id", userID).
				Msg("chat: crm.ListStaff failed, denying access")
			return nil, pkgerrors.PermissionDenied("chat access denied")
		}
		for _, m := range staffResp.GetMembers() {
			uid := strings.TrimSpace(m.GetUserId())
			if uid == "" || sameUserID(uid, clientID) || sameUserID(uid, ownerID) {
				continue
			}
			participants = append(participants, uid)
		}
	}

	if !allowed {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	if ownerID != "" && !sameUserID(ownerID, clientID) {
		participants = append(participants, ownerID)
	}
	participants = uniqueNonEmpty(participants)
	// P2#23: if the booking has no resolvable counterpart (e.g. missing clientID
	// or owner) we cannot form a valid 2-party thread.  Silently adding the actor
	// produces a "thread with yourself" — deny instead so the caller surfaces the
	// data inconsistency rather than creating a phantom thread.
	if len(participants) < 2 {
		rs.log.Warn().Str("ref_id", refID).Str("user_id", userID).
			Strs("participants", participants).
			Msg("chat: venue booking has fewer than 2 participants, denying thread creation")
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}

	resp, err := rs.chat.EnsureThread(ctx, &chatv1.EnsureThreadRequest{
		Kind:               "venue_booking",
		RefId:              refID,
		ParticipantUserIds: participants,
		ActorUserId:        userID,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetThread(), nil
}

// ensureMasterBookingThread resolves participants for a master booking.
// Access is enforced by GetMasterBooking (the master-service returns an error
// for unauthorised actors), so we trust its response rather than re-checking.
func (rs *chatThreadResolver) ensureMasterBookingThread(r *http.Request, userID, refID string) (*chatv1.ChatThread, error) {
	ctx := r.Context()

	b, err := rs.master.GetMasterBooking(ctx, &masterv1.GetMasterBookingRequest{
		BookingId:   refID,
		ActorUserId: userID,
	})
	if err != nil {
		return nil, err
	}
	bk := b.GetBooking()
	if bk == nil {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	clientID := strings.TrimSpace(bk.GetClientUserId())
	masterUserID := strings.TrimSpace(bk.GetMasterUserId())
	if clientID == "" || masterUserID == "" {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	participants := uniqueNonEmpty([]string{clientID, masterUserID})
	if len(participants) < 2 {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}

	resp, err := rs.chat.EnsureThread(ctx, &chatv1.EnsureThreadRequest{
		Kind:               "master_booking",
		RefId:              refID,
		ParticipantUserIds: participants,
		ActorUserId:        userID,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetThread(), nil
}
