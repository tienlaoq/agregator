package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

type ReviewHandler struct {
	client reviewv1.ReviewServiceClient
}

func NewReviewHandler(client reviewv1.ReviewServiceClient) *ReviewHandler {
	return &ReviewHandler{client: client}
}

func (h *ReviewHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		VenueID string `json:"venue_id"`
		Rating  int32  `json:"rating"`
		Text    string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	resp, err := h.client.CreateReview(r.Context(), &reviewv1.CreateReviewRequest{
		UserId:  userID,
		VenueId: req.VenueID,
		Rating:  req.Rating,
		Text:    req.Text,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, reviewToJSON(resp))
}

func (h *ReviewHandler) CreateForVenue(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	venueID := chi.URLParam(r, "venueId")
	if venueID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "venue_id is required"})
		return
	}

	var req struct {
		Rating int32  `json:"rating"`
		Text   string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	resp, err := h.client.CreateReview(r.Context(), &reviewv1.CreateReviewRequest{
		UserId:  userID,
		VenueId: venueID,
		Rating:  req.Rating,
		Text:    req.Text,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, reviewToJSON(resp))
}

func (h *ReviewHandler) ListByVenue(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "venueId")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	resp, err := h.client.ListVenueReviews(r.Context(), &reviewv1.ListVenueReviewsRequest{
		VenueId:  venueID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	reviews := make([]map[string]any, len(resp.GetReviews()))
	for i, rv := range resp.GetReviews() {
		reviews[i] = reviewToJSON(rv)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
		"total":   resp.GetTotal(),
	})
}

func reviewToJSON(rv *reviewv1.ReviewResponse) map[string]any {
	return map[string]any{
		"id":         rv.GetId(),
		"user_id":    rv.GetUserId(),
		"user_name":  rv.GetUserName(),
		"venue_id":   rv.GetVenueId(),
		"rating":     rv.GetRating(),
		"text":       rv.GetText(),
		"verified":   rv.GetIsVerified(),
		"created_at": rv.GetCreatedAt().AsTime(),
	}
}
