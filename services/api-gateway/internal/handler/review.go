package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ReviewHandler struct {
	client     reviewv1.ReviewServiceClient
	userClient userv1.UserServiceClient
}

func NewReviewHandler(client reviewv1.ReviewServiceClient, userClient userv1.UserServiceClient) *ReviewHandler {
	return &ReviewHandler{client: client, userClient: userClient}
}

func (h *ReviewHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		VenueID     string `json:"venue_id"`
		MasterID    string `json:"master_id"`
		Rating      int32  `json:"rating"`
		Text        string `json:"text"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if !readJSONOrRespond(w, r, &req) { return }

	userName := h.resolveUserName(r, userID)
	resp, err := h.client.CreateReview(r.Context(), &reviewv1.CreateReviewRequest{
		UserId:      userID,
		UserName:    userName,
		VenueId:     req.VenueID,
		MasterId:    req.MasterID,
		Rating:      req.Rating,
		Text:        req.Text,
		IsAnonymous: req.IsAnonymous,
	})
	if err != nil {
		if handleReviewCreateError(w, err) {
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, reviewToJSON(resp, ""))
}

func (h *ReviewHandler) CreateForVenue(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	venueID := chi.URLParam(r, "venueId")
	if venueID == "" {
		writeCatalog(w, apicatalog.GatewayReviewVenueIdRequired)
		return
	}

	var req struct {
		Rating      int32  `json:"rating"`
		Text        string `json:"text"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if !readJSONOrRespond(w, r, &req) { return }

	userName := h.resolveUserName(r, userID)
	resp, err := h.client.CreateReview(r.Context(), &reviewv1.CreateReviewRequest{
		UserId:      userID,
		UserName:    userName,
		VenueId:     venueID,
		Rating:      req.Rating,
		Text:        req.Text,
		IsAnonymous: req.IsAnonymous,
	})
	if err != nil {
		if handleReviewCreateError(w, err) {
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, reviewToJSON(resp, ""))
}

func (h *ReviewHandler) ListByVenue(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "venueId")

	page, ok := queryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := queryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}

	resp, err := h.client.ListVenueReviews(r.Context(), &reviewv1.ListVenueReviewsRequest{
		VenueId:  venueID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		if handleReviewCreateError(w, err) {
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}

	nameCache := make(map[string]string, len(resp.GetReviews()))
	reviews := make([]map[string]any, len(resp.GetReviews()))
	for i, rv := range resp.GetReviews() {
		name := rv.GetUserName()
		if !rv.GetIsAnonymous() && strings.TrimSpace(name) == "" {
			name = h.resolveUserNameCached(r, rv.GetUserId(), nameCache)
		}
		reviews[i] = reviewToJSON(rv, name)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
		"total":   resp.GetTotal(),
	})
}

func (h *ReviewHandler) CreateForMaster(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	masterID := chi.URLParam(r, "masterId")
	if masterID == "" {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}

	var req struct {
		Rating      int32  `json:"rating"`
		Text        string `json:"text"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if !readJSONOrRespond(w, r, &req) { return }

	userName := h.resolveUserName(r, userID)
	resp, err := h.client.CreateReview(r.Context(), &reviewv1.CreateReviewRequest{
		UserId:      userID,
		UserName:    userName,
		MasterId:    masterID,
		Rating:      req.Rating,
		Text:        req.Text,
		IsAnonymous: req.IsAnonymous,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, reviewToJSON(resp, ""))
}

func (h *ReviewHandler) ListByMaster(w http.ResponseWriter, r *http.Request) {
	masterID := chi.URLParam(r, "masterId")
	page, ok := queryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := queryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}

	resp, err := h.client.ListMasterReviews(r.Context(), &reviewv1.ListMasterReviewsRequest{
		MasterId: masterID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	nameCache := make(map[string]string, len(resp.GetReviews()))
	reviews := make([]map[string]any, len(resp.GetReviews()))
	for i, rv := range resp.GetReviews() {
		name := rv.GetUserName()
		if !rv.GetIsAnonymous() && strings.TrimSpace(name) == "" {
			name = h.resolveUserNameCached(r, rv.GetUserId(), nameCache)
		}
		reviews[i] = reviewToJSON(rv, name)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
		"total":   resp.GetTotal(),
	})
}

func reviewToJSON(rv *reviewv1.ReviewResponse, userName string) map[string]any {
	if userName == "" {
		userName = rv.GetUserName()
	}
	return map[string]any{
		"id":           rv.GetId(),
		"user_id":      rv.GetUserId(),
		"user_name":    userName,
		"venue_id":     rv.GetVenueId(),
		"master_id":    rv.GetMasterId(),
		"rating":       rv.GetRating(),
		"text":         rv.GetText(),
		"verified":     rv.GetIsVerified(),
		"is_anonymous": rv.GetIsAnonymous(),
		"created_at":   rv.GetCreatedAt().AsTime(),
	}
}

func (h *ReviewHandler) resolveUserName(r *http.Request, userID string) string {
	if h.userClient == nil || strings.TrimSpace(userID) == "" {
		return ""
	}
	u, err := h.userClient.GetUser(r.Context(), &userv1.GetUserRequest{Id: userID})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.GetName())
}

func (h *ReviewHandler) resolveUserNameCached(r *http.Request, userID string, cache map[string]string) string {
	id := strings.TrimSpace(userID)
	if id == "" {
		return ""
	}
	if name, ok := cache[id]; ok {
		return name
	}
	name := h.resolveUserName(r, id)
	cache[id] = name
	return name
}

func handleReviewCreateError(w http.ResponseWriter, err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	if st.Code() != codes.FailedPrecondition {
		return false
	}
	if !strings.Contains(strings.ToLower(st.Message()), "booking is not confirmed by platform") {
		return false
	}
	writeCatalog(w, apicatalog.GatewayReviewBookingNotVerified)
	return true
}
