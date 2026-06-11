package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

type BookingHandler struct {
	client        bookingv1.BookingServiceClient
	venueClient   venuev1.VenueServiceClient
	userClient    userv1.UserServiceClient
	ownerNotifier bookingOwnerNotifier // optional; «колокольчик» владельцу о новой брони
}

// bookingOwnerNotifier delivers the in-app "new booking at your venue" bell to
// the venue owner. Defined at the consumer site; implemented by
// *NotificationHandler.
type bookingOwnerNotifier interface {
	NotifyVenueBookingCreated(ctx context.Context, ownerID, venueID, venueName, bookingID, date, timeFrom, timeTo string, guests int32)
}

type BookingHandlerOption func(*BookingHandler)

// WithBookingOwnerNotifier wires the bell notification sent to the venue owner
// when a guest books. Optional: when unset, bookings succeed silently.
func WithBookingOwnerNotifier(n bookingOwnerNotifier) BookingHandlerOption {
	return func(h *BookingHandler) {
		h.ownerNotifier = n
	}
}

func NewBookingHandler(
	client bookingv1.BookingServiceClient,
	venueClient venuev1.VenueServiceClient,
	userClient userv1.UserServiceClient,
	opts ...BookingHandlerOption,
) *BookingHandler {
	h := &BookingHandler{client: client, venueClient: venueClient, userClient: userClient}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *BookingHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		VenueID    string   `json:"venue_id"`
		ServiceID  string   `json:"service_id"`
		ServiceIDs []string `json:"service_ids"`
		HallIDs    []string `json:"hall_ids"`
		Date       string   `json:"date"`
		TimeFrom   string   `json:"time_from"`
		TimeTo     string   `json:"time_to"`
		Time       string   `json:"time"`
		Guests     int32    `json:"guests"`
		Comment    string   `json:"comment"`
	}
	if !readJSONOrRespond(w, r, &req) { return }

	timeFrom := req.TimeFrom
	if timeFrom == "" {
		timeFrom = req.Time
	}
	if timeFrom == "" {
		writeCatalog(w, apicatalog.GatewayBookingTimeFromRequired)
		return
	}
	timeTo := req.TimeTo
	if timeTo == "" {
		if t, err := time.Parse("15:04", timeFrom); err == nil {
			timeTo = t.Add(2 * time.Hour).Format("15:04")
		}
	}

	var venueName, venueOwnerID string
	if req.VenueID != "" {
		venueResp, err := h.venueClient.GetVenue(r.Context(), &venuev1.GetVenueRequest{Id: req.VenueID})
		if err == nil && venueResp != nil {
			venueName = venueResp.GetName()
			venueOwnerID = venueResp.GetOwnerId()
		}
	}

	resp, err := h.client.CreateBooking(r.Context(), &bookingv1.CreateBookingRequest{
		UserId:     userID,
		VenueId:    req.VenueID,
		VenueName:  venueName,
		ServiceId:  req.ServiceID,
		ServiceIds: req.ServiceIDs,
		HallIds:    req.HallIDs,
		Date:       req.Date,
		TimeFrom:   timeFrom,
		TimeTo:     timeTo,
		Guests:     req.Guests,
		Comment:    req.Comment,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	// Bell for the venue owner (best-effort, never fails the booking). Skipped
	// when the owner books their own venue. Fired before the response is
	// written so r.Context() is still live.
	if h.ownerNotifier != nil && venueOwnerID != "" && venueOwnerID != userID {
		h.ownerNotifier.NotifyVenueBookingCreated(
			r.Context(),
			venueOwnerID, req.VenueID, venueName,
			resp.GetId(), req.Date, timeFrom, timeTo, req.Guests,
		)
	}

	writeJSON(w, http.StatusCreated, bookingToJSON(resp))
}

func (h *BookingHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	q := r.URL.Query()
	page, ok := queryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := queryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}

	resp, err := h.client.ListUserBookings(r.Context(), &bookingv1.ListUserBookingsRequest{
		UserId:   userID,
		Status:   q.Get("status"),
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"bookings": bookingList(resp.GetBookings()),
		"total":    resp.GetTotal(),
	})
}

func (h *BookingHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	resp, err := h.client.GetBooking(r.Context(), &bookingv1.GetBookingRequest{
		Id: id,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, bookingToJSON(resp))
}

func (h *BookingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	resp, err := h.client.CancelBooking(r.Context(), &bookingv1.CancelBookingRequest{
		Id:     id,
		UserId: userID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, bookingToJSON(resp))
}

func (h *BookingHandler) ListVenueBookings(w http.ResponseWriter, r *http.Request) {
	ownerID := middleware.UserIDFromCtx(r.Context())
	if ownerID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	q := r.URL.Query()
	pageSize, ok := queryInt(w, r, "page_size", 20, 1, 200)
	if !ok {
		return
	}
	cursor := q.Get("cursor") // keyset token; empty = first page

	resp, err := h.client.ListVenueBookings(r.Context(), &bookingv1.ListVenueBookingsRequest{
		VenueId:  venueID,
		OwnerId:  ownerID,
		Status:   q.Get("status"),
		Date:     q.Get("date"),
		PageSize: int32(pageSize),
		Cursor:   cursor,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	out := bookingList(resp.GetBookings())
	names := h.resolveUserNames(r, out)
	for _, b := range out {
		uid, _ := b["user_id"].(string)
		if n := names[uid]; n != "" {
			b["user_name"] = n
		}
	}
	result := map[string]any{"bookings": out, "total": resp.GetTotal()}
	if nc := resp.GetNextCursor(); nc != "" {
		result["next_cursor"] = nc
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *BookingHandler) resolveUserNames(r *http.Request, bookings []map[string]any) map[string]string {
	out := make(map[string]string, len(bookings))
	if h.userClient == nil {
		return out
	}
	seen := map[string]struct{}{}
	for _, b := range bookings {
		uid, _ := b["user_id"].(string)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		u, err := h.userClient.GetUser(r.Context(), &userv1.GetUserRequest{Id: uid})
		if err != nil {
			continue
		}
		if name := u.GetName(); name != "" {
			out[uid] = name
		}
	}
	return out
}

func bookingToJSON(b *bookingv1.BookingResponse) map[string]any {
	timeDisplay := b.GetTimeFrom()
	if b.GetTimeTo() != "" {
		timeDisplay = fmt.Sprintf("%s–%s", b.GetTimeFrom(), b.GetTimeTo())
	}

	m := map[string]any{
		"id":          b.GetId(),
		"user_id":     b.GetUserId(),
		"venue_id":    b.GetVenueId(),
		"venue_name":  b.GetVenueName(),
		"service_id":  b.GetServiceId(),
		"date":        b.GetDate(),
		"time":        timeDisplay,
		"time_from":   b.GetTimeFrom(),
		"time_to":     b.GetTimeTo(),
		"guests":      b.GetGuests(),
		"comment":     b.GetComment(),
		"status":      b.GetStatus(),
		"total_price": b.GetTotalPrice(),
		"created_at":  b.GetCreatedAt().AsTime(),
	}
	if p := b.GetPackageServiceIds(); len(p) > 0 {
		m["package_service_ids"] = p
	}
	if h := b.GetHallIds(); len(h) > 0 {
		m["hall_ids"] = h
	}
	if url := b.GetPaymentUrl(); url != "" {
		m["payment_url"] = url
	}
	return m
}

func bookingList(bookings []*bookingv1.BookingResponse) []map[string]any {
	out := make([]map[string]any, len(bookings))
	for i, b := range bookings {
		out[i] = bookingToJSON(b)
	}
	return out
}

func (h *BookingHandler) ListBookingStaffNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	bookingID := chi.URLParam(r, "bookingId")

	resp, err := h.client.ListBookingStaffNotes(r.Context(), &bookingv1.ListBookingStaffNotesRequest{
		BookingId:       bookingID,
		RequesterUserId: userID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	notes := make([]map[string]any, 0, len(resp.GetNotes()))
	for _, n := range resp.GetNotes() {
		notes = append(notes, map[string]any{
			"id":             n.GetId(),
			"booking_id":     n.GetBookingId(),
			"venue_id":       n.GetVenueId(),
			"author_user_id": n.GetAuthorUserId(),
			"body":           n.GetBody(),
			"created_at":     n.GetCreatedAt().AsTime(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

func (h *BookingHandler) AddBookingStaffNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	bookingID := chi.URLParam(r, "bookingId")

	var req struct {
		Body string `json:"body"`
	}
	if !readJSONOrRespond(w, r, &req) { return }

	resp, err := h.client.AddBookingStaffNote(r.Context(), &bookingv1.AddBookingStaffNoteRequest{
		BookingId:       bookingID,
		RequesterUserId: userID,
		Body:            req.Body,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	n := resp.GetNote()
	writeJSON(w, http.StatusCreated, map[string]any{
		"note": map[string]any{
			"id":             n.GetId(),
			"booking_id":     n.GetBookingId(),
			"venue_id":       n.GetVenueId(),
			"author_user_id": n.GetAuthorUserId(),
			"body":           n.GetBody(),
			"created_at":     n.GetCreatedAt().AsTime(),
		},
	})
}
