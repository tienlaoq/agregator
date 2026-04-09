package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

type VenueHandler struct {
	client venuev1.VenueServiceClient
}

func NewVenueHandler(client venuev1.VenueServiceClient) *VenueHandler {
	return &VenueHandler{client: client}
}

func (h *VenueHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	resp, err := h.client.ListVenues(r.Context(), &venuev1.ListVenuesRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
		Type:     r.URL.Query().Get("type"),
		SortBy:   r.URL.Query().Get("sort_by"),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), false),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

func (h *VenueHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	lng, _ := strconv.ParseFloat(q.Get("lng"), 64)
	radius, _ := strconv.ParseFloat(q.Get("radius"), 64)
	priceMin, _ := strconv.ParseInt(q.Get("price_min"), 10, 64)
	priceMax, _ := strconv.ParseInt(q.Get("price_max"), 10, 64)
	ratingMin, _ := strconv.ParseFloat(q.Get("rating_min"), 64)
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	var amenities []string
	if v := q.Get("amenities"); v != "" {
		amenities = strings.Split(v, ",")
	}

	resp, err := h.client.SearchVenues(r.Context(), &venuev1.SearchVenuesRequest{
		Query:     q.Get("q"),
		Latitude:  lat,
		Longitude: lng,
		RadiusKm:  radius,
		Type:      q.Get("type"),
		PriceMin:  priceMin,
		PriceMax:  priceMax,
		RatingMin: ratingMin,
		Amenities: amenities,
		Page:      int32(page),
		PageSize:  int32(pageSize),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), false),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

func (h *VenueHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	resp, err := h.client.GetVenueBySlug(r.Context(), &venuev1.GetVenueBySlugRequest{
		Slug: slug,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, venueToJSON(resp, false))
}

func (h *VenueHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		Name              string                `json:"name"`
		Type              string                `json:"type"`
		Description       string                `json:"description"`
		Address           string                `json:"address"`
		City              string                `json:"city"`
		Latitude          float64               `json:"latitude"`
		Longitude         float64               `json:"longitude"`
		PriceFrom         int64                 `json:"price_from"`
		Capacity          int32                 `json:"capacity"`
		Amenities         []string              `json:"amenities"`
		WorkingHours      string                `json:"working_hours"`
		Phone             string                `json:"phone"`
		Services          []venueServiceItemReq `json:"services"`
		LegalEntityName   string                `json:"legal_entity_name"`
		INN               string                `json:"inn"`
		OGRN              string                `json:"ogrn"`
		PublicListingURL  string                `json:"public_listing_url"`
		VerificationNote  string                `json:"verification_note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	grpcServices := make([]*venuev1.VenueServiceItem, len(req.Services))
	for i, s := range req.Services {
		grpcServices[i] = &venuev1.VenueServiceItem{
			Name:        s.Name,
			DurationMin: s.DurationMin,
			Price:       s.Price,
			Description: s.Description,
		}
	}

	resp, err := h.client.CreateVenue(r.Context(), &venuev1.CreateVenueRequest{
		OwnerId:           userID,
		Name:              req.Name,
		Type:              req.Type,
		Description:       req.Description,
		Address:           req.Address,
		City:              req.City,
		Latitude:          req.Latitude,
		Longitude:         req.Longitude,
		PriceFrom:         req.PriceFrom,
		Capacity:          req.Capacity,
		Amenities:         req.Amenities,
		WorkingHours:      req.WorkingHours,
		Phone:             req.Phone,
		Services:          grpcServices,
		LegalEntityName:   req.LegalEntityName,
		Inn:               req.INN,
		Ogrn:              req.OGRN,
		PublicListingUrl:  req.PublicListingURL,
		VerificationNote:  req.VerificationNote,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, venueToJSON(resp, true))
}

func (h *VenueHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")

	var req struct {
		Name              *string  `json:"name"`
		Description       *string  `json:"description"`
		Address           *string  `json:"address"`
		City              *string  `json:"city"`
		Latitude          *float64 `json:"latitude"`
		Longitude         *float64 `json:"longitude"`
		PriceFrom         *int64   `json:"price_from"`
		Capacity          *int32   `json:"capacity"`
		Amenities         []string `json:"amenities"`
		WorkingHours      *string  `json:"working_hours"`
		Phone             *string  `json:"phone"`
		LegalEntityName   *string  `json:"legal_entity_name"`
		INN               *string  `json:"inn"`
		OGRN              *string  `json:"ogrn"`
		PublicListingURL  *string  `json:"public_listing_url"`
		VerificationNote  *string  `json:"verification_note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	grpcReq := &venuev1.UpdateVenueRequest{
		Id:        venueID,
		OwnerId:   userID,
		Amenities: req.Amenities,
	}
	if req.Name != nil {
		grpcReq.Name = req.Name
	}
	if req.Description != nil {
		grpcReq.Description = req.Description
	}
	if req.Address != nil {
		grpcReq.Address = req.Address
	}
	if req.City != nil {
		grpcReq.City = req.City
	}
	if req.Latitude != nil {
		grpcReq.Latitude = req.Latitude
	}
	if req.Longitude != nil {
		grpcReq.Longitude = req.Longitude
	}
	if req.PriceFrom != nil {
		grpcReq.PriceFrom = req.PriceFrom
	}
	if req.Capacity != nil {
		grpcReq.Capacity = req.Capacity
	}
	if req.WorkingHours != nil {
		grpcReq.WorkingHours = req.WorkingHours
	}
	if req.Phone != nil {
		grpcReq.Phone = req.Phone
	}
	if req.LegalEntityName != nil {
		grpcReq.LegalEntityName = req.LegalEntityName
	}
	if req.INN != nil {
		grpcReq.Inn = req.INN
	}
	if req.OGRN != nil {
		grpcReq.Ogrn = req.OGRN
	}
	if req.PublicListingURL != nil {
		grpcReq.PublicListingUrl = req.PublicListingURL
	}
	if req.VerificationNote != nil {
		grpcReq.VerificationNote = req.VerificationNote
	}

	resp, err := h.client.UpdateVenue(r.Context(), grpcReq)
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, venueToJSON(resp, true))
}

func (h *VenueHandler) ListOwnerVenues(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	resp, err := h.client.ListOwnerVenues(r.Context(), &venuev1.ListOwnerVenuesRequest{
		OwnerId: userID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), true),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

type venueServiceItemReq struct {
	Name        string `json:"name"`
	DurationMin int32  `json:"duration_min"`
	Price       int64  `json:"price"`
	Description string `json:"description"`
}

func (h *VenueHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	status := r.URL.Query().Get("status")

	resp, err := h.client.ListPendingVenues(r.Context(), &venuev1.ListPendingVenuesRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
		Status:   status,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), true),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

func (h *VenueHandler) Moderate(w http.ResponseWriter, r *http.Request) {
	adminID := middleware.UserIDFromCtx(r.Context())
	if adminID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	venueID := chi.URLParam(r, "id")

	var req struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	resp, err := h.client.ModerateVenue(r.Context(), &venuev1.ModerateVenueRequest{
		VenueId:     venueID,
		Action:      req.Action,
		Comment:     req.Comment,
		ModeratedBy: adminID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, venueToJSON(resp, true))
}

func venueToJSON(v *venuev1.VenueResponse, includeVerification bool) map[string]any {
	services := make([]map[string]any, len(v.GetServices()))
	for i, s := range v.GetServices() {
		services[i] = map[string]any{
			"id":           s.GetId(),
			"name":         s.GetName(),
			"duration_min": s.GetDurationMin(),
			"price":        s.GetPrice(),
			"description":  s.GetDescription(),
		}
	}
	photos := make([]map[string]any, len(v.GetPhotos()))
	for i, p := range v.GetPhotos() {
		photos[i] = map[string]any{
			"id":         p.GetId(),
			"url":        p.GetUrl(),
			"sort_order": p.GetSortOrder(),
			"is_cover":   p.GetIsCover(),
		}
	}
	result := map[string]any{
		"id":                 v.GetId(),
		"owner_id":           v.GetOwnerId(),
		"slug":               v.GetSlug(),
		"name":               v.GetName(),
		"type":               v.GetType(),
		"description":        v.GetDescription(),
		"address":            v.GetAddress(),
		"city":               v.GetCity(),
		"latitude":           v.GetLatitude(),
		"longitude":          v.GetLongitude(),
		"price_from":         v.GetPriceFrom(),
		"capacity":           v.GetCapacity(),
		"amenities":          v.GetAmenities(),
		"working_hours":      v.GetWorkingHours(),
		"phone":              v.GetPhone(),
		"avg_rating":         v.GetAvgRating(),
		"review_count":       v.GetReviewCount(),
		"is_active":          v.GetIsActive(),
		"status":             v.GetStatus(),
		"moderation_comment": v.GetModerationComment(),
		"moderated_by":       v.GetModeratedBy(),
		"services":           services,
		"photos":             photos,
		"created_at":         v.GetCreatedAt().AsTime(),
	}

	if v.GetModeratedAt() != nil {
		result["moderated_at"] = v.GetModeratedAt().AsTime()
	}

	if includeVerification {
		result["legal_entity_name"] = v.GetLegalEntityName()
		result["inn"] = v.GetInn()
		result["ogrn"] = v.GetOgrn()
		result["public_listing_url"] = v.GetPublicListingUrl()
		result["verification_note"] = v.GetVerificationNote()
	}

	return result
}

func venueList(venues []*venuev1.VenueResponse, includeVerification bool) []map[string]any {
	out := make([]map[string]any, len(venues))
	for i, v := range venues {
		out[i] = venueToJSON(v, includeVerification)
	}
	return out
}
