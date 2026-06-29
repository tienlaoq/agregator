package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ReviewHandler struct {
	client         reviewv1.ReviewServiceClient
	userClient     userv1.UserServiceClient
	venueClient    venuev1.VenueServiceClient   // optional; владелец заведения для уведомления об отзыве
	masterClient   masterv1.MasterServiceClient // optional; владелец профиля мастера для уведомления об отзыве
	ownerNotifier  reviewOwnerNotifier          // optional; «колокольчик» владельцу заведения о новом отзыве
	masterNotifier reviewMasterNotifier         // optional; «колокольчик» мастеру о новом отзыве
}

// reviewOwnerNotifier delivers the in-app "new review at your venue" bell to
// the venue owner. Defined at the consumer site; implemented by
// *NotificationHandler.
type reviewOwnerNotifier interface {
	NotifyVenueReviewCreated(ctx context.Context, ownerID, venueID, venueName string, rating int32)
}

// reviewMasterNotifier delivers the in-app "new review about you" bell to the
// master. Defined at the consumer site; implemented by *NotificationHandler.
type reviewMasterNotifier interface {
	NotifyMasterReviewCreated(ctx context.Context, masterUserID, masterID, masterName string, rating int32)
}

type ReviewHandlerOption func(*ReviewHandler)

// WithReviewOwnerNotifier wires the bell notification sent to the venue owner
// when a guest leaves a review. Both the venue client (owner lookup) and the
// notifier are required for delivery; when either is unset, reviews still
// succeed silently.
func WithReviewOwnerNotifier(venueClient venuev1.VenueServiceClient, n reviewOwnerNotifier) ReviewHandlerOption {
	return func(h *ReviewHandler) {
		h.venueClient = venueClient
		h.ownerNotifier = n
	}
}

// WithReviewMasterNotifier wires the bell sent to a master when a guest leaves
// a review about them. Both the master client (owner lookup) and the notifier
// are required; when either is unset, reviews still succeed silently.
func WithReviewMasterNotifier(masterClient masterv1.MasterServiceClient, n reviewMasterNotifier) ReviewHandlerOption {
	return func(h *ReviewHandler) {
		h.masterClient = masterClient
		h.masterNotifier = n
	}
}

func NewReviewHandler(client reviewv1.ReviewServiceClient, userClient userv1.UserServiceClient, opts ...ReviewHandlerOption) *ReviewHandler {
	h := &ReviewHandler{client: client, userClient: userClient}
	for _, o := range opts {
		o(h)
	}
	return h
}

// notifyVenueOwnerOfReview resolves the venue's owner and fires the bell.
// Best-effort: lookup failures are silently ignored (the review itself is
// already saved). Skipped when the author reviews their own venue. Call before
// writing the response so r.Context() is still live.
func (h *ReviewHandler) notifyVenueOwnerOfReview(r *http.Request, authorID, venueID string, rating int32) {
	if h.ownerNotifier == nil || h.venueClient == nil || venueID == "" {
		return
	}
	v, err := h.venueClient.GetVenue(r.Context(), &venuev1.GetVenueRequest{Id: venueID})
	if err != nil || v == nil {
		return
	}
	ownerID := strings.TrimSpace(v.GetOwnerId())
	if ownerID == "" || ownerID == authorID {
		return
	}
	h.ownerNotifier.NotifyVenueReviewCreated(r.Context(), ownerID, venueID, v.GetName(), rating)
}

// notifyMasterOfReview resolves the master's owner user_id + display_name and
// fires the bell. Best-effort: lookup failures are silently ignored (the review
// is already saved). Skipped when the author reviews their own master profile.
// Call before writing the response so r.Context() is still live.
func (h *ReviewHandler) notifyMasterOfReview(r *http.Request, authorID, masterID string, rating int32) {
	if h.masterNotifier == nil || h.masterClient == nil || masterID == "" {
		return
	}
	resp, err := h.masterClient.GetMaster(r.Context(), &masterv1.GetMasterRequest{Id: masterID})
	if err != nil || resp.GetMaster() == nil {
		return
	}
	m := resp.GetMaster()
	masterUserID := strings.TrimSpace(m.GetUserId())
	if masterUserID == "" || masterUserID == authorID {
		return
	}
	h.masterNotifier.NotifyMasterReviewCreated(r.Context(), masterUserID, masterID, m.GetDisplayName(), rating)
}

func (h *ReviewHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		VenueID     string `json:"venue_id"`
		MasterID    string `json:"master_id"`
		Rating      int32  `json:"rating"`
		Text        string `json:"text"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

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
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	h.notifyVenueOwnerOfReview(r, userID, req.VenueID, req.Rating)
	h.notifyMasterOfReview(r, userID, req.MasterID, req.Rating)

	httpx.WriteJSON(w, http.StatusCreated, reviewToJSON(resp, ""))
}

func (h *ReviewHandler) CreateForVenue(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	venueID := chi.URLParam(r, "venueId")
	if venueID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayReviewVenueIdRequired)
		return
	}

	var req struct {
		Rating      int32  `json:"rating"`
		Text        string `json:"text"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

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
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	h.notifyVenueOwnerOfReview(r, userID, venueID, req.Rating)

	httpx.WriteJSON(w, http.StatusCreated, reviewToJSON(resp, ""))
}

func (h *ReviewHandler) ListByVenue(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "venueId")

	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := httpx.QueryInt(w, r, "page_size", 0, 0, 200)
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
		httpx.GRPCErrorToHTTP(w, err)
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
		"total":   resp.GetTotal(),
	})
}

func (h *ReviewHandler) CreateForMaster(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	masterID := chi.URLParam(r, "masterId")
	if masterID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}

	var req struct {
		Rating      int32  `json:"rating"`
		Text        string `json:"text"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

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
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	h.notifyMasterOfReview(r, userID, masterID, req.Rating)

	httpx.WriteJSON(w, http.StatusCreated, reviewToJSON(resp, ""))
}

// MasterRating GET /api/v1/masters/{masterId}/rating
// Returns the cached aggregate (average rating + review count) maintained by
// review-service in O(1), so callers don't have to page through all reviews to
// compute it themselves.
func (h *ReviewHandler) MasterRating(w http.ResponseWriter, r *http.Request) {
	masterID := chi.URLParam(r, "masterId")
	resp, err := h.client.GetMasterRating(r.Context(), &reviewv1.GetMasterRatingRequest{
		MasterId: masterID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"master_id":    resp.GetMasterId(),
		"avg_rating":   resp.GetAvgRating(),
		"review_count": resp.GetReviewCount(),
	})
}

func (h *ReviewHandler) ListByMaster(w http.ResponseWriter, r *http.Request) {
	masterID := chi.URLParam(r, "masterId")
	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := httpx.QueryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}

	resp, err := h.client.ListMasterReviews(r.Context(), &reviewv1.ListMasterReviewsRequest{
		MasterId: masterID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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
	httpx.WriteCatalog(w, apicatalog.GatewayReviewBookingNotVerified)
	return true
}
