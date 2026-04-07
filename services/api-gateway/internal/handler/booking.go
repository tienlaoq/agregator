package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

type BookingHandler struct {
	client bookingv1.BookingServiceClient
}

func NewBookingHandler(client bookingv1.BookingServiceClient) *BookingHandler {
	return &BookingHandler{client: client}
}

func (h *BookingHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		VenueID   string `json:"venue_id"`
		ServiceID string `json:"service_id"`
		Date      string `json:"date"`
		TimeFrom  string `json:"time_from"`
		TimeTo    string `json:"time_to"`
		Guests    int32  `json:"guests"`
		Comment   string `json:"comment"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	resp, err := h.client.CreateBooking(r.Context(), &bookingv1.CreateBookingRequest{
		UserId:    userID,
		VenueId:   req.VenueID,
		ServiceId: req.ServiceID,
		Date:      req.Date,
		TimeFrom:  req.TimeFrom,
		TimeTo:    req.TimeTo,
		Guests:    req.Guests,
		Comment:   req.Comment,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, bookingToJSON(resp))
}

func (h *BookingHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

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
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
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
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "venueId")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	resp, err := h.client.ListVenueBookings(r.Context(), &bookingv1.ListVenueBookingsRequest{
		VenueId:  venueID,
		OwnerId:  ownerID,
		Status:   q.Get("status"),
		Date:     q.Get("date"),
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

func bookingToJSON(b *bookingv1.BookingResponse) map[string]any {
	m := map[string]any{
		"id":          b.GetId(),
		"user_id":     b.GetUserId(),
		"venue_id":    b.GetVenueId(),
		"service_id":  b.GetServiceId(),
		"date":        b.GetDate(),
		"time_from":   b.GetTimeFrom(),
		"time_to":     b.GetTimeTo(),
		"guests":      b.GetGuests(),
		"comment":     b.GetComment(),
		"status":      b.GetStatus(),
		"total_price": b.GetTotalPrice(),
		"created_at":  b.GetCreatedAt().AsTime(),
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
