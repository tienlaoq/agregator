package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgcities "github.com/tienlao/agregator/pkg/cities"
	"github.com/tienlao/agregator/pkg/storage"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/suspendnotify"
	"google.golang.org/grpc/metadata"
)

type VenueHandler struct {
	client             venuev1.VenueServiceClient
	userClient         userv1.UserServiceClient // optional; used for staff invite by email
	crm                crmv1.CRMServiceClient   // staff & CRM tasks (extracted from venue-service)
	storage            storage.Uploader
	suspendNotifier    *suspendnotify.Sender   // optional; письма владельцу и персоналу при приостановке (SMTP)
	staffNotifier      staffInviteNotifier     // optional; in-app "колокольчик" при добавлении в персонал
	taskNotifier       crmTaskNotifier         // optional; "колокольчик" исполнителю при назначении CRM-задачи
	moderationNotifier venueModerationNotifier // optional; "колокольчик" владельцу о решении модерации
}

// staffInviteNotifier delivers the in-app "added to a venue's team" bell
// notification to a newly-invited staff member. Defined at the consumer site;
// implemented by *NotificationHandler.
type staffInviteNotifier interface {
	NotifyStaffInvited(ctx context.Context, userID, venueID, role string)
}

// crmTaskNotifier delivers the in-app "a CRM task was assigned to you" bell to
// a task's assignee. Defined at the consumer site; implemented by
// *NotificationHandler.
type crmTaskNotifier interface {
	NotifyTaskAssigned(ctx context.Context, assigneeID, venueID, taskID, title string)
}

// venueModerationNotifier delivers the in-app "your venue's listing status
// changed" bell to the venue owner. Defined at the consumer site; implemented
// by *NotificationHandler.
type venueModerationNotifier interface {
	NotifyVenueModerated(ctx context.Context, ownerID, venueID, venueName, action, comment string)
}

type VenueHandlerOption func(*VenueHandler)

func WithSuspendNotifier(n *suspendnotify.Sender) VenueHandlerOption {
	return func(h *VenueHandler) {
		h.suspendNotifier = n
	}
}

// WithStaffInviteNotifier wires the bell notification sent when an owner adds a
// user to a venue's staff. Optional: when unset, invites still succeed silently.
func WithStaffInviteNotifier(n staffInviteNotifier) VenueHandlerOption {
	return func(h *VenueHandler) {
		h.staffNotifier = n
	}
}

// WithCRMTaskNotifier wires the bell notification sent when a CRM task is
// assigned to someone other than its author. Optional: when unset, tasks still
// save silently.
func WithCRMTaskNotifier(n crmTaskNotifier) VenueHandlerOption {
	return func(h *VenueHandler) {
		h.taskNotifier = n
	}
}

// WithVenueModerationNotifier wires the bell notification sent to the venue
// owner on every moderation decision. Optional: when unset, moderation still
// succeeds silently.
func WithVenueModerationNotifier(n venueModerationNotifier) VenueHandlerOption {
	return func(h *VenueHandler) {
		h.moderationNotifier = n
	}
}

// NewVenueHandler creates a VenueHandler. The crm client serves the owner
// cabinet (staff CRUD, CRM tasks) and the management_access composition for
// owner-facing venue lists.
func NewVenueHandler(client venuev1.VenueServiceClient, userClient userv1.UserServiceClient, crm crmv1.CRMServiceClient, up storage.Uploader, opts ...VenueHandlerOption) *VenueHandler {
	h := &VenueHandler{client: client, userClient: userClient, crm: crm, storage: up}
	for _, o := range opts {
		o(h)
	}
	return h
}

func socialLinksMapForJSON(s string) map[string]any {
	s = strings.TrimSpace(s)
	if s == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func (h *VenueHandler) List(w http.ResponseWriter, r *http.Request) {
	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := httpx.QueryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}

	resp, err := h.client.ListVenues(r.Context(), &venuev1.ListVenuesRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
		Type:     r.URL.Query().Get("type"),
		SortBy:   r.URL.Query().Get("sort_by"),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), false),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

// PopularCities GET /api/v1/analytics/popular-cities — города с наибольшим числом
// активных заведений (фолбэк «Популярных» в героe, пока нет статистики поиска).
func (h *VenueHandler) PopularCities(w http.ResponseWriter, r *http.Request) {
	limit, ok := httpx.QueryInt(w, r, "limit", 6, 1, 20)
	if !ok {
		return
	}
	resp, err := h.client.GetPopularCities(r.Context(), &venuev1.GetPopularCitiesRequest{Limit: int32(limit)})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	cities := make([]map[string]any, 0, len(resp.GetCities()))
	for _, c := range resp.GetCities() {
		cities = append(cities, map[string]any{"city": c.GetCity(), "count": c.GetCount()})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"cities": cities})
}

func (h *VenueHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	lng, _ := strconv.ParseFloat(q.Get("lng"), 64)
	radius, _ := strconv.ParseFloat(q.Get("radius"), 64)
	priceMin, _ := strconv.ParseInt(q.Get("price_min"), 10, 64)
	priceMax, _ := strconv.ParseInt(q.Get("price_max"), 10, 64)
	ratingMin, _ := strconv.ParseFloat(q.Get("rating_min"), 64)
	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := httpx.QueryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}

	var amenities []string
	if v := q.Get("amenities"); v != "" {
		amenities = strings.Split(v, ",")
	}

	query := strings.TrimSpace(q.Get("q"))
	var cityList []string
	if packed := strings.TrimSpace(q.Get("cities")); packed != "" {
		// В query предпочитаем `|` (не режется прокси/браузером); `\x1e` — совместимость со старыми ссылками.
		if strings.Contains(packed, pkgcities.Sep) {
			cityList = pkgcities.Split(packed)
		} else {
			for _, p := range strings.Split(packed, "|") {
				if t := strings.TrimSpace(p); t != "" {
					cityList = append(cityList, t)
				}
			}
		}
	} else {
		for _, c := range q["city"] {
			if t := strings.TrimSpace(c); t != "" {
				cityList = append(cityList, t)
			}
		}
	}
	grpcCity := pkgcities.Join(cityList)
	// Один город и без текста — дублируем в query (старые venue / полнотекст).
	if len(cityList) == 1 && query == "" {
		query = cityList[0]
	}

	ctx := r.Context()
	if len(cityList) > 0 {
		// Значения gRPC metadata попадают в HTTP/2 headers — только ASCII; кириллица через base64.
		pairs := make([]string, 0, len(cityList)*2)
		for _, c := range cityList {
			pairs = append(pairs, "x-city-b64", base64.RawURLEncoding.EncodeToString([]byte(c)))
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	resp, err := h.client.SearchVenues(ctx, &venuev1.SearchVenuesRequest{
		Query:     query,
		City:      grpcCity,
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
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), false),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

// venueDraftVisibleToViewer returns false if venue is draft and caller is neither owner nor admin.
func venueDraftVisibleToViewer(status, ownerID, viewerUserID, viewerRole string) bool {
	if status != "draft" {
		return true
	}
	if viewerUserID != "" && viewerUserID == ownerID {
		return true
	}
	return viewerRole == "admin"
}

func (h *VenueHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	resp, err := h.client.GetVenueBySlug(r.Context(), &venuev1.GetVenueBySlugRequest{
		Slug: slug,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	viewer := middleware.UserIDFromCtx(r.Context())
	role := middleware.RoleFromCtx(r.Context())
	if !venueDraftVisibleToViewer(resp.GetStatus(), resp.GetOwnerId(), viewer, role) {
		httpx.WriteCatalog(w, apicatalog.GatewayVenueNotFound)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, venueToJSON(resp, false))
}

// slotEndFromDuration returns time_from + durationMin minutes (HH:MM).
func slotEndFromDuration(timeFrom string, durationMin int) string {
	t, err := time.Parse("15:04", timeFrom)
	if err != nil {
		return timeFrom
	}
	return t.Add(time.Duration(durationMin) * time.Minute).Format("15:04")
}

// bookingCandidateStartTimes returns HH:MM from 10:00 to 22:00 inclusive, step 30 minutes.
func bookingCandidateStartTimes() []string {
	const (
		firstMin = 10 * 60
		lastMin  = 22 * 60
		step     = 30
	)
	out := make([]string, 0, (lastMin-firstMin)/step+1)
	for m := firstMin; m <= lastMin; m += step {
		out = append(out, fmt.Sprintf("%02d:%02d", m/60, m%60))
	}
	return out
}

// AvailabilityBySlug returns start times that are free for booking (same window as POST /bookings).
func (h *VenueHandler) AvailabilityBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	date := r.URL.Query().Get("date")
	if date == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestMissingDate)
		return
	}

	durationMin, ok := httpx.QueryInt(w, r, "duration_min", 120, 30, 720)
	if !ok {
		return
	}

	v, err := h.client.GetVenueBySlug(r.Context(), &venuev1.GetVenueBySlugRequest{Slug: slug})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	viewer := middleware.UserIDFromCtx(r.Context())
	role := middleware.RoleFromCtx(r.Context())
	if !venueDraftVisibleToViewer(v.GetStatus(), v.GetOwnerId(), viewer, role) {
		httpx.WriteCatalog(w, apicatalog.GatewayVenueNotFound)
		return
	}

	starts := bookingCandidateStartTimes()

	// Build batch request: one interval per candidate start time.
	batchSlots := make([]*venuev1.SlotInterval, len(starts))
	for i, start := range starts {
		batchSlots[i] = &venuev1.SlotInterval{
			TimeFrom: start,
			TimeTo:   slotEndFromDuration(start, durationMin),
		}
	}

	// Optional ?hall_id (repeatable) narrows availability to specific halls in
	// per-hall venues; ignored in whole mode.
	batchResp, err := h.client.BatchCheckSlotAvailability(r.Context(), &venuev1.BatchCheckSlotRequest{
		VenueId: v.GetId(),
		Date:    date,
		Slots:   batchSlots,
		HallIds: r.URL.Query()["hall_id"],
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	avail := batchResp.GetAvailable()
	available := make([]string, 0, len(starts))
	for i, start := range starts {
		if i < len(avail) && avail[i] {
			available = append(available, start)
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"date":            date,
		"available_slots": available,
	})
}

func (h *VenueHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		Name             string                `json:"name"`
		Type             string                `json:"type"`
		Description      string                `json:"description"`
		Address          string                `json:"address"`
		City             string                `json:"city"`
		Latitude         float64               `json:"latitude"`
		Longitude        float64               `json:"longitude"`
		PriceFrom        int64                 `json:"price_from"`
		PriceWeekend     int64                 `json:"price_weekend"`
		Capacity         int32                 `json:"capacity"`
		Amenities        []string              `json:"amenities"`
		WorkingHours     string                `json:"working_hours"`
		Phone            string                `json:"phone"`
		Services         []venueServiceItemReq `json:"services"`
		LegalEntityName  string                `json:"legal_entity_name"`
		INN              string                `json:"inn"`
		OGRN             string                `json:"ogrn"`
		PublicListingURL string                `json:"public_listing_url"`
		VerificationNote string                `json:"verification_note"`
		SocialLinks      json.RawMessage       `json:"social_links"`
		Halls            []venueHallItemReq    `json:"halls"`
		StartAsDraft     bool                  `json:"start_as_draft"`
		PayoutLegalForm  string                `json:"payout_legal_form"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
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

	var grpcHalls []*venuev1.VenueHallInput
	if len(req.Halls) > 0 {
		grpcHalls = venueHallItemsToProto(req.Halls)
	}

	socialJSON := "{}"
	if len(req.SocialLinks) > 0 {
		socialJSON = string(req.SocialLinks)
	}

	resp, err := h.client.CreateVenue(r.Context(), &venuev1.CreateVenueRequest{
		OwnerId:          userID,
		Name:             req.Name,
		Type:             req.Type,
		Description:      req.Description,
		Address:          req.Address,
		City:             req.City,
		Latitude:         req.Latitude,
		Longitude:        req.Longitude,
		PriceFrom:        req.PriceFrom,
		PriceWeekend:     req.PriceWeekend,
		Capacity:         req.Capacity,
		Amenities:        req.Amenities,
		WorkingHours:     req.WorkingHours,
		Phone:            req.Phone,
		Services:         grpcServices,
		LegalEntityName:  req.LegalEntityName,
		Inn:              req.INN,
		Ogrn:             req.OGRN,
		PublicListingUrl: req.PublicListingURL,
		VerificationNote: req.VerificationNote,
		SocialLinks:      socialJSON,
		Halls:            grpcHalls,
		StartAsDraft:     req.StartAsDraft,
		PayoutLegalForm:  req.PayoutLegalForm,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, venueToJSON(resp, true))
}

func (h *VenueHandler) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	resp, err := h.client.SubmitVenueForReview(r.Context(), &venuev1.SubmitVenueForReviewRequest{
		VenueId: venueID,
		OwnerId: userID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, venueToJSON(resp, true))
}

func (h *VenueHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")

	var req struct {
		Name             *string                `json:"name"`
		Description      *string                `json:"description"`
		Address          *string                `json:"address"`
		City             *string                `json:"city"`
		Latitude         *float64               `json:"latitude"`
		Longitude        *float64               `json:"longitude"`
		PriceFrom        *int64                 `json:"price_from"`
		PriceWeekend     *int64                 `json:"price_weekend"`
		Capacity         *int32                 `json:"capacity"`
		Amenities        *[]string              `json:"amenities"`
		WorkingHours     *string                `json:"working_hours"`
		Phone            *string                `json:"phone"`
		LegalEntityName  *string                `json:"legal_entity_name"`
		INN              *string                `json:"inn"`
		OGRN             *string                `json:"ogrn"`
		PublicListingURL *string                `json:"public_listing_url"`
		VerificationNote *string                `json:"verification_note"`
		SocialLinks      *json.RawMessage       `json:"social_links"`
		Services         *[]venueServiceItemReq `json:"services"`
		Halls            *[]venueHallItemReq    `json:"halls"`
		PayoutLegalForm  *string                `json:"payout_legal_form"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	grpcReq := &venuev1.UpdateVenueRequest{
		Id:      venueID,
		OwnerId: userID,
	}
	if req.Amenities != nil {
		grpcReq.Amenities = *req.Amenities
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
	if req.PriceWeekend != nil {
		grpcReq.PriceWeekend = req.PriceWeekend
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
	if req.SocialLinks != nil {
		s := string(*req.SocialLinks)
		grpcReq.SocialLinks = &s
	}
	if req.Services != nil {
		items := make([]*venuev1.VenueServiceItem, len(*req.Services))
		for i, s := range *req.Services {
			items[i] = &venuev1.VenueServiceItem{
				Name:        s.Name,
				DurationMin: s.DurationMin,
				Price:       s.Price,
				Description: s.Description,
			}
		}
		grpcReq.ServicesReplace = &venuev1.VenueServicesReplace{Items: items}
	}
	if req.Halls != nil {
		grpcReq.HallsReplace = &venuev1.VenueHallsReplace{Items: venueHallItemsToProto(*req.Halls)}
	}
	if req.PayoutLegalForm != nil {
		grpcReq.PayoutLegalForm = req.PayoutLegalForm
	}

	resp, err := h.client.UpdateVenue(r.Context(), grpcReq)
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, venueToJSON(resp, true))
}

func (h *VenueHandler) ListOwnerVenues(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	// 1) Ask CRM which venues this user can manage (and in what role).
	// CRM owns the staff / management-access model; venue-service no longer
	// knows about ownership beyond the owner_id column.
	managed, err := h.crm.ListManagedVenues(r.Context(), &crmv1.ListManagedVenuesRequest{
		UserId: userID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	mvs := managed.GetVenues()
	if len(mvs) == 0 {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"venues":    []map[string]any{},
			"total":     0,
			"page":      1,
			"page_size": 0,
		})
		return
	}

	// 2) Fetch the venue cards in a single batch RPC.
	ids := make([]string, 0, len(mvs))
	accessByID := make(map[string]string, len(mvs))
	for _, mv := range mvs {
		id := mv.GetVenueId()
		if id == "" {
			continue
		}
		ids = append(ids, id)
		accessByID[id] = mv.GetAccess()
	}
	batch, err := h.client.GetVenuesBatch(r.Context(), &venuev1.GetVenuesBatchRequest{Ids: ids})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	// 3) Compose the response preserving CRM's ordering (newest first).
	venues := batch.GetVenues()
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		v := venues[id]
		if v == nil {
			continue
		}
		m := venueToJSON(v, true)
		if access := accessByID[id]; access != "" {
			m["management_access"] = access
		}
		out = append(out, m)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"venues":    out,
		"total":     int32(len(out)),
		"page":      int32(1),
		"page_size": int32(len(out)),
	})
}

// ListOwnerSlotBlocks returns manual busy intervals for the venue (owner only).
func (h *VenueHandler) ListOwnerSlotBlocks(w http.ResponseWriter, r *http.Request) {
	ownerID := middleware.UserIDFromCtx(r.Context())
	if ownerID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	q := r.URL.Query()
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	if dateFrom == "" || dateTo == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestMissingDateRange)
		return
	}

	resp, err := h.client.ListManualSlotBlocks(r.Context(), &venuev1.ListManualSlotBlocksRequest{
		OwnerId:  ownerID,
		VenueId:  venueID,
		DateFrom: dateFrom,
		DateTo:   dateTo,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	blocks := make([]map[string]any, 0, len(resp.GetBlocks()))
	for _, b := range resp.GetBlocks() {
		blocks = append(blocks, manualSlotBlockToJSON(b))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

// GetVenueSchedule returns occupied intervals (aggregator bookings + manual
// blocks) for the owner/staff schedule grid. Booking details (guest, status,
// price) are joined client-side by booking_id from the owner bookings list.
func (h *VenueHandler) GetVenueSchedule(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	q := r.URL.Query()
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	if dateFrom == "" || dateTo == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestMissingDateRange)
		return
	}

	resp, err := h.client.GetVenueSchedule(r.Context(), &venuev1.GetVenueScheduleRequest{
		ActorId:  actorID,
		VenueId:  venueID,
		DateFrom: dateFrom,
		DateTo:   dateTo,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	entries := make([]map[string]any, 0, len(resp.GetEntries()))
	for _, e := range resp.GetEntries() {
		entries = append(entries, scheduleEntryToJSON(e))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// GetOwnerBookingMode returns the venue's booking mode (whole | per_hall).
func (h *VenueHandler) GetOwnerBookingMode(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	resp, err := h.client.GetVenueBookingMode(r.Context(), &venuev1.GetVenueBookingModeRequest{
		ActorId: actorID,
		VenueId: venueID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"booking_mode": resp.GetBookingMode()})
}

// SetOwnerBookingMode switches the venue between whole-venue and per-hall booking.
func (h *VenueHandler) SetOwnerBookingMode(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	var req struct {
		BookingMode string `json:"booking_mode"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	resp, err := h.client.SetVenueBookingMode(r.Context(), &venuev1.SetVenueBookingModeRequest{
		ActorId:     actorID,
		VenueId:     venueID,
		BookingMode: req.BookingMode,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"booking_mode": resp.GetBookingMode()})
}

// scheduleEntryToJSON maps a schedule entry to its wire form. Aggregator
// bookings carry booking_id (kind="booking"); manual owner blocks carry the
// note (kind="manual_block").
func scheduleEntryToJSON(e *venuev1.ScheduleEntry) map[string]any {
	m := map[string]any{
		"id":        e.GetId(),
		"date":      e.GetDate(),
		"time_from": e.GetTimeFrom(),
		"time_to":   e.GetTimeTo(),
	}
	if e.BookingId != nil {
		m["kind"] = "booking"
		m["booking_id"] = e.GetBookingId()
	} else {
		m["kind"] = "manual_block"
		m["note"] = e.GetNote()
	}
	if e.HallId != nil {
		m["hall_id"] = e.GetHallId()
	}
	return m
}

// CreateOwnerSlotBlock adds a manual busy interval (e.g. external booking).
func (h *VenueHandler) CreateOwnerSlotBlock(w http.ResponseWriter, r *http.Request) {
	ownerID := middleware.UserIDFromCtx(r.Context())
	if ownerID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	var req struct {
		Date     string   `json:"date"`
		TimeFrom string   `json:"time_from"`
		TimeTo   string   `json:"time_to"`
		Note     string   `json:"note"`
		HallIDs  []string `json:"hall_ids"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	resp, err := h.client.CreateManualSlotBlock(r.Context(), &venuev1.CreateManualSlotBlockRequest{
		OwnerId:  ownerID,
		VenueId:  venueID,
		Date:     req.Date,
		TimeFrom: req.TimeFrom,
		TimeTo:   req.TimeTo,
		Note:     req.Note,
		HallIds:  req.HallIDs,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	blocks := make([]map[string]any, 0, len(resp.GetBlocks()))
	for _, b := range resp.GetBlocks() {
		blocks = append(blocks, manualSlotBlockToJSON(b))
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"blocks": blocks})
}

// DeleteOwnerSlotBlock removes a manual busy interval.
func (h *VenueHandler) DeleteOwnerSlotBlock(w http.ResponseWriter, r *http.Request) {
	ownerID := middleware.UserIDFromCtx(r.Context())
	if ownerID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	blockID := chi.URLParam(r, "blockId")

	_, err := h.client.DeleteManualSlotBlock(r.Context(), &venuev1.DeleteManualSlotBlockRequest{
		OwnerId: ownerID,
		VenueId: venueID,
		BlockId: blockID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func manualSlotBlockToJSON(b *venuev1.ManualSlotBlock) map[string]any {
	if b == nil {
		return nil
	}
	m := map[string]any{
		"id":        b.GetId(),
		"venue_id":  b.GetVenueId(),
		"date":      b.GetDate(),
		"time_from": b.GetTimeFrom(),
		"time_to":   b.GetTimeTo(),
		"note":      b.GetNote(),
	}
	if b.HallId != nil {
		m["hall_id"] = b.GetHallId()
	}
	return m
}

type venueServiceItemReq struct {
	Name        string `json:"name"`
	DurationMin int32  `json:"duration_min"`
	Price       int64  `json:"price"`
	Description string `json:"description"`
}

type venueHallItemReq struct {
	ID           *string  `json:"id,omitempty"`
	Name         string   `json:"name"`
	PriceFrom    int64    `json:"price_from"`
	PriceWeekend int64    `json:"price_weekend"`
	Capacity     int32    `json:"capacity"`
	Amenities    []string `json:"amenities"`
	SortOrder    int32    `json:"sort_order"`
}

func venueHallItemsToProto(items []venueHallItemReq) []*venuev1.VenueHallInput {
	out := make([]*venuev1.VenueHallInput, 0, len(items))
	for _, h := range items {
		am := h.Amenities
		if am == nil {
			am = []string{}
		}
		hi := &venuev1.VenueHallInput{
			Name:         strings.TrimSpace(h.Name),
			PriceFrom:    h.PriceFrom,
			PriceWeekend: h.PriceWeekend,
			Capacity:     h.Capacity,
			Amenities:    am,
			SortOrder:    h.SortOrder,
		}
		if h.ID != nil {
			s := strings.TrimSpace(*h.ID)
			if s != "" {
				hi.Id = &s
			}
		}
		out = append(out, hi)
	}
	return out
}

func (h *VenueHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := httpx.QueryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	nameQuery := strings.TrimSpace(r.URL.Query().Get("q"))

	resp, err := h.client.ListPendingVenues(r.Context(), &venuev1.ListPendingVenuesRequest{
		Page:      int32(page),
		PageSize:  int32(pageSize),
		Status:    status,
		NameQuery: nameQuery,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"venues":    venueList(resp.GetVenues(), true),
		"total":     resp.GetTotal(),
		"page":      resp.GetPage(),
		"page_size": resp.GetPageSize(),
	})
}

func (h *VenueHandler) Moderate(w http.ResponseWriter, r *http.Request) {
	adminID := middleware.UserIDFromCtx(r.Context())
	if adminID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	venueID := chi.URLParam(r, "id")

	var req struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	resp, err := h.client.ModerateVenue(r.Context(), &venuev1.ModerateVenueRequest{
		VenueId:     venueID,
		Action:      req.Action,
		Comment:     req.Comment,
		ModeratedBy: adminID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	act := strings.ToLower(strings.TrimSpace(req.Action))
	if h.suspendNotifier != nil {
		if act == "suspend" && strings.EqualFold(resp.GetStatus(), "suspended") {
			h.suspendNotifier.NotifyVenueSuspended(
				r.Context(),
				venueID,
				strings.TrimSpace(resp.GetOwnerId()),
				strings.TrimSpace(resp.GetName()),
				strings.TrimSpace(req.Comment),
			)
		}
		if act == "resume" && strings.EqualFold(resp.GetStatus(), "active") {
			h.suspendNotifier.NotifyVenueResumed(
				r.Context(),
				venueID,
				strings.TrimSpace(resp.GetOwnerId()),
				strings.TrimSpace(resp.GetName()),
				strings.TrimSpace(req.Comment),
			)
		}
	}

	// Bell for the venue owner on every moderation decision (best-effort,
	// never fails the moderation). Fired before the response is written so
	// r.Context() is still live.
	if h.moderationNotifier != nil {
		h.moderationNotifier.NotifyVenueModerated(
			r.Context(),
			strings.TrimSpace(resp.GetOwnerId()),
			venueID,
			strings.TrimSpace(resp.GetName()),
			act,
			strings.TrimSpace(req.Comment),
		)
	}

	httpx.WriteJSON(w, http.StatusOK, venueToJSON(resp, true))
}

// venuePrimaryImageURL — обложка или первое фото (для image_url в JSON каталога/карточки).
func venuePrimaryImageURL(v *venuev1.VenueResponse) string {
	for _, p := range v.GetPhotos() {
		if p.GetIsCover() && p.GetUrl() != "" {
			return p.GetUrl()
		}
	}
	for _, p := range v.GetPhotos() {
		if p.GetUrl() != "" {
			return p.GetUrl()
		}
	}
	return ""
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
	videos := make([]map[string]any, len(v.GetVideos()))
	for i, vid := range v.GetVideos() {
		videos[i] = map[string]any{
			"id":         vid.GetId(),
			"url":        vid.GetUrl(),
			"sort_order": vid.GetSortOrder(),
		}
	}
	halls := make([]map[string]any, len(v.GetHalls()))
	for i, hall := range v.GetHalls() {
		hPhotos := make([]map[string]any, len(hall.GetPhotos()))
		for j, hp := range hall.GetPhotos() {
			hPhotos[j] = map[string]any{
				"id":         hp.GetId(),
				"url":        hp.GetUrl(),
				"sort_order": hp.GetSortOrder(),
				"is_cover":   hp.GetIsCover(),
			}
		}
		halls[i] = map[string]any{
			"id":            hall.GetId(),
			"name":          hall.GetName(),
			"price_from":    hall.GetPriceFrom(),
			"price_weekend": hall.GetPriceWeekend(),
			"capacity":      hall.GetCapacity(),
			"amenities":     hall.GetAmenities(),
			"sort_order":    hall.GetSortOrder(),
			"photos":        hPhotos,
		}
	}
	var createdAt time.Time
	if ts := v.GetCreatedAt(); ts != nil {
		createdAt = ts.AsTime()
	}
	result := map[string]any{
		"id":            v.GetId(),
		"owner_id":      v.GetOwnerId(),
		"slug":          v.GetSlug(),
		"name":          v.GetName(),
		"type":          v.GetType(),
		"description":   v.GetDescription(),
		"address":       v.GetAddress(),
		"city":          v.GetCity(),
		"latitude":      v.GetLatitude(),
		"longitude":     v.GetLongitude(),
		"price_from":    v.GetPriceFrom(),
		"price_weekend": v.GetPriceWeekend(),
		"capacity":      v.GetCapacity(),
		"amenities":     v.GetAmenities(),
		"working_hours": v.GetWorkingHours(),
		"phone":         v.GetPhone(),
		"avg_rating":    v.GetAvgRating(),
		// `rating` is a legacy alias for `avg_rating`: the frontend Venue type
		// reads `venue.rating` (catalog cards, venue page, owner dashboard).
		// Emit both so neither contract breaks.
		"rating":             v.GetAvgRating(),
		"review_count":       v.GetReviewCount(),
		"is_active":          v.GetIsActive(),
		"status":             v.GetStatus(),
		"moderation_comment": v.GetModerationComment(),
		"moderated_by":       v.GetModeratedBy(),
		"services":           services,
		"photos":             photos,
		"videos":             videos,
		"halls":              halls,
		"created_at":         createdAt,
		"social_links":       socialLinksMapForJSON(v.GetSocialLinks()),
	}

	if v.GetModeratedAt() != nil {
		result["moderated_at"] = v.GetModeratedAt().AsTime()
	}

	if u := venuePrimaryImageURL(v); u != "" {
		result["image_url"] = u
	}

	if includeVerification {
		result["legal_entity_name"] = v.GetLegalEntityName()
		result["inn"] = v.GetInn()
		result["ogrn"] = v.GetOgrn()
		result["public_listing_url"] = v.GetPublicListingUrl()
		result["verification_note"] = v.GetVerificationNote()
		result["payout_legal_form"] = v.GetPayoutLegalForm()
	}

	// management_access is no longer carried inside the Venue proto: it is a
	// CRM concern. ListOwnerVenues composes it via crm.ListManagedVenues; other
	// venue endpoints intentionally omit it.

	return result
}

func venueList(venues []*venuev1.VenueResponse, includeVerification bool) []map[string]any {
	out := make([]map[string]any, len(venues))
	for i, v := range venues {
		out[i] = venueToJSON(v, includeVerification)
	}
	return out
}
