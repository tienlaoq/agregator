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
		"venues":    venueList(resp.GetVenues()),
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
		"venues":    venueList(resp.GetVenues()),
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

	writeJSON(w, http.StatusOK, venueToJSON(resp))
}

func (h *VenueHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		Name         string                `json:"name"`
		Type         string                `json:"type"`
		Description  string                `json:"description"`
		Address      string                `json:"address"`
		Latitude     float64               `json:"latitude"`
		Longitude    float64               `json:"longitude"`
		PriceFrom    int64                 `json:"price_from"`
		Capacity     int32                 `json:"capacity"`
		Amenities    []string              `json:"amenities"`
		WorkingHours string                `json:"working_hours"`
		Phone        string                `json:"phone"`
		Services     []venueServiceItemReq `json:"services"`
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
		OwnerId:      userID,
		Name:         req.Name,
		Type:         req.Type,
		Description:  req.Description,
		Address:      req.Address,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		PriceFrom:    req.PriceFrom,
		Capacity:     req.Capacity,
		Amenities:    req.Amenities,
		WorkingHours: req.WorkingHours,
		Phone:        req.Phone,
		Services:     grpcServices,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, venueToJSON(resp))
}

func (h *VenueHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")

	var req struct {
		Name         *string  `json:"name"`
		Description  *string  `json:"description"`
		Address      *string  `json:"address"`
		Latitude     *float64 `json:"latitude"`
		Longitude    *float64 `json:"longitude"`
		PriceFrom    *int64   `json:"price_from"`
		Capacity     *int32   `json:"capacity"`
		Amenities    []string `json:"amenities"`
		WorkingHours *string  `json:"working_hours"`
		Phone        *string  `json:"phone"`
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

	resp, err := h.client.UpdateVenue(r.Context(), grpcReq)
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, venueToJSON(resp))
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
		"venues":    venueList(resp.GetVenues()),
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

func venueToJSON(v *venuev1.VenueResponse) map[string]any {
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
	return map[string]any{
		"id":            v.GetId(),
		"owner_id":      v.GetOwnerId(),
		"slug":          v.GetSlug(),
		"name":          v.GetName(),
		"type":          v.GetType(),
		"description":   v.GetDescription(),
		"address":       v.GetAddress(),
		"latitude":      v.GetLatitude(),
		"longitude":     v.GetLongitude(),
		"price_from":    v.GetPriceFrom(),
		"capacity":      v.GetCapacity(),
		"amenities":     v.GetAmenities(),
		"working_hours": v.GetWorkingHours(),
		"phone":         v.GetPhone(),
		"avg_rating":    v.GetAvgRating(),
		"review_count":  v.GetReviewCount(),
		"is_active":     v.GetIsActive(),
		"services":      services,
		"photos":        photos,
		"created_at":    v.GetCreatedAt().AsTime(),
	}
}

func venueList(venues []*venuev1.VenueResponse) []map[string]any {
	out := make([]map[string]any, len(venues))
	for i, v := range venues {
		out[i] = venueToJSON(v)
	}
	return out
}
