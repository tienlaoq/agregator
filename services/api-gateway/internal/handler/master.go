package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	pkgcities "github.com/tienlao/agregator/pkg/cities"
	"github.com/tienlao/agregator/pkg/storage"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type MasterHandler struct {
	client     masterv1.MasterServiceClient
	storage    storage.Uploader
	notifier   masterOwnerNotifier      // optional; «колокольчик» мастеру о брони и модерации
	userClient userv1.UserServiceClient // optional; для ленивого создания профиля при загрузке фото
}

// masterOwnerNotifier delivers in-app bells to a master: new booking and
// profile-moderation decisions. Defined at the consumer site; implemented by
// *NotificationHandler.
type masterOwnerNotifier interface {
	NotifyMasterBookingCreated(ctx context.Context, masterUserID, masterID, masterName, bookingID, date, timeFrom, timeTo string)
	NotifyMasterModerated(ctx context.Context, masterUserID, masterID, masterName, action, comment string)
}

type MasterHandlerOption func(*MasterHandler)

// WithMasterOwnerNotifier wires the bells sent to a master on new bookings and
// moderation decisions. Optional: when unset, those actions succeed silently.
func WithMasterOwnerNotifier(n masterOwnerNotifier) MasterHandlerOption {
	return func(h *MasterHandler) {
		h.notifier = n
	}
}

// WithMasterUserClient wires the user-service client used to look up the owner's
// name when lazily creating a master profile on first photo upload. Optional:
// when unset, photo upload still requires a pre-existing profile.
func WithMasterUserClient(c userv1.UserServiceClient) MasterHandlerOption {
	return func(h *MasterHandler) {
		h.userClient = c
	}
}

func NewMasterHandler(c masterv1.MasterServiceClient, up storage.Uploader, opts ...MasterHandlerOption) *MasterHandler {
	h := &MasterHandler{client: c, storage: up}
	for _, o := range opts {
		o(h)
	}
	return h
}

func masterProtoToJSON(m *masterv1.Master) map[string]any {
	if m == nil {
		return nil
	}
	svcs := make([]map[string]any, 0, len(m.GetServices()))
	for _, s := range m.GetServices() {
		svcs = append(svcs, map[string]any{
			"id":           s.GetId(),
			"name":         s.GetName(),
			"description":  s.GetDescription(),
			"duration_min": s.GetDurationMin(),
			"price":        s.GetPrice(),
			"sort_order":   s.GetSortOrder(),
		})
	}
	photos := make([]map[string]any, 0, len(m.GetPhotos()))
	for _, p := range m.GetPhotos() {
		photos = append(photos, map[string]any{
			"id":         p.GetId(),
			"url":        p.GetUrl(),
			"sort_order": p.GetSortOrder(),
			"is_cover":   p.GetIsCover(),
		})
	}
	creds := make([]map[string]any, 0, len(m.GetCredentials()))
	for _, c := range m.GetCredentials() {
		creds = append(creds, map[string]any{
			"id":         c.GetId(),
			"kind":       c.GetKind(),
			"title":      c.GetTitle(),
			"issuer":     c.GetIssuer(),
			"year":       c.GetYear(),
			"sort_order": c.GetSortOrder(),
		})
	}
	out := map[string]any{
		"id":                           m.GetId(),
		"user_id":                      m.GetUserId(),
		"slug":                         m.GetSlug(),
		"display_name":                 m.GetDisplayName(),
		"bio":                          m.GetBio(),
		"phone":                        m.GetPhone(),
		"city":                         m.GetCity(),
		"work_format":                  m.GetWorkFormat(),
		"travel_radius_km":             m.GetTravelRadiusKm(),
		"experience_years":             m.GetExperienceYears(),
		"specializations":              m.GetSpecializations(),
		"hourly_rate":                  m.GetHourlyRate(),
		"availability_json":            m.GetAvailabilityJson(),
		"payout_legal_form":            m.GetPayoutLegalForm(),
		"payout_legal_name":            m.GetPayoutLegalName(),
		"payout_inn":                   m.GetPayoutInn(),
		"payout_kpp":                   m.GetPayoutKpp(),
		"payout_ogrn":                  m.GetPayoutOgrn(),
		"payout_ogrnip":                m.GetPayoutOgrnip(),
		"payout_bank_name":             m.GetPayoutBankName(),
		"payout_bik":                   m.GetPayoutBik(),
		"payout_settlement_account":    m.GetPayoutSettlementAccount(),
		"payout_correspondent_account": m.GetPayoutCorrespondentAccount(),
		"payout_verification_status":   m.GetPayoutVerificationStatus(),
		"payout_ready":                 m.GetPayoutReady(),
		"status":                       m.GetStatus(),
		"moderation_comment":           m.GetModerationComment(),
		"services":                     svcs,
		"photos":                       photos,
		"credentials":                  creds,
	}
	if m.GetCreatedAt() != nil {
		out["created_at"] = m.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	if m.GetUpdatedAt() != nil {
		out["updated_at"] = m.GetUpdatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	if m.ModeratedBy != nil {
		out["moderated_by"] = *m.ModeratedBy
	}
	if m.ModeratedAt != nil {
		out["moderated_at"] = m.GetModeratedAt().AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	if m.TravelBaseLatitude != nil {
		out["travel_base_latitude"] = *m.TravelBaseLatitude
	}
	if m.TravelBaseLongitude != nil {
		out["travel_base_longitude"] = *m.TravelBaseLongitude
	}
	zones := make([]map[string]any, 0, len(m.GetTravelExcludeZones()))
	for _, z := range m.GetTravelExcludeZones() {
		zones = append(zones, map[string]any{
			"id":        z.GetId(),
			"latitude":  z.GetLatitude(),
			"longitude": z.GetLongitude(),
			"radius_km": z.GetRadiusKm(),
			"label":     z.GetLabel(),
		})
	}
	out["travel_exclude_zones"] = zones
	return out
}

func masterProtoToJSONPublic(m *masterv1.Master) map[string]any {
	out := masterProtoToJSON(m)
	delete(out, "payout_legal_form")
	delete(out, "payout_legal_name")
	delete(out, "payout_inn")
	delete(out, "payout_kpp")
	delete(out, "payout_ogrn")
	delete(out, "payout_ogrnip")
	delete(out, "payout_bank_name")
	delete(out, "payout_bik")
	delete(out, "payout_settlement_account")
	delete(out, "payout_correspondent_account")
	delete(out, "payout_verification_status")
	delete(out, "payout_ready")
	return out
}

// ListPublic GET /api/v1/masters
func (h *MasterHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := httpx.QueryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}
	limit, ok := httpx.QueryInt(w, r, "limit", 0, 0, 500)
	if !ok {
		return
	}
	off, ok := httpx.QueryInt(w, r, "offset", 0, 0, 1000000)
	if !ok {
		return
	}
	limit32 := int32(limit)
	off32 := int32(off)
	if pageSize > 0 {
		limit32 = int32(pageSize)
		if page < 1 {
			page = 1
		}
		off32 = int32((page - 1) * pageSize)
	} else if limit32 <= 0 {
		limit32 = 50
	}

	query := strings.TrimSpace(q.Get("q"))
	var cityList []string
	if packed := strings.TrimSpace(q.Get("cities")); packed != "" {
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
	if len(cityList) == 0 {
		if c := strings.TrimSpace(q.Get("city")); c != "" {
			cityList = []string{c}
		}
	}

	priceMinRub, _ := strconv.ParseInt(q.Get("price_min"), 10, 64)
	priceMaxRub, _ := strconv.ParseInt(q.Get("price_max"), 10, 64)
	var priceMinKop, priceMaxKop int64
	if priceMinRub > 0 {
		priceMinKop = priceMinRub * 100
	}
	if priceMaxRub > 0 {
		priceMaxKop = priceMaxRub * 100
	}

	workFormat := strings.TrimSpace(q.Get("work_format"))

	resp, err := h.client.ListPublicMasters(r.Context(), &masterv1.ListPublicMastersRequest{
		City:            "",
		Limit:           limit32,
		Offset:          off32,
		Q:               query,
		Cities:          cityList,
		WorkFormat:      workFormat,
		PriceMinKopecks: priceMinKop,
		PriceMaxKopecks: priceMaxKop,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	list := make([]map[string]any, 0, len(resp.GetMasters()))
	for _, m := range resp.GetMasters() {
		list = append(list, masterProtoToJSONPublic(m))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"masters": list, "total": resp.GetTotal()})
}

// GetPublic GET /api/v1/masters/{slug}
func (h *MasterHandler) GetPublic(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	resp, err := h.client.GetPublicMaster(r.Context(), &masterv1.GetPublicMasterRequest{Slug: slug})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, masterProtoToJSONPublic(resp.GetMaster()))
}

// CreateMyProfile POST /api/v1/owner/master/profile
func (h *MasterHandler) CreateMyProfile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &body) {
		return
	}
	resp, err := h.client.CreateMyProfile(r.Context(), &masterv1.CreateMyProfileRequest{
		UserId:      uid,
		DisplayName: strings.TrimSpace(body.DisplayName),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, masterProtoToJSON(resp.GetMaster()))
}

// GetMyProfile GET /api/v1/owner/master/profile
// Всегда 200: { "profile": <object|null> } — отсутствие профиля не считается HTTP-ошибкой.
func (h *MasterHandler) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: uid})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": nil})
			return
		}
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": masterProtoToJSON(resp.GetMaster())})
}

type masterServicePatch struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DurationMin int32  `json:"duration_min"`
	Price       int64  `json:"price"`
	SortOrder   int32  `json:"sort_order"`
}

type masterCredentialPatch struct {
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Issuer string `json:"issuer"`
	Year   int32  `json:"year"`
}

// updateReqFromRaw builds gRPC UpdateMyProfileRequest from JSON fields (same rules as PATCH body).
func (h *MasterHandler) updateReqFromRaw(uid string, raw map[string]json.RawMessage) (*masterv1.UpdateMyProfileRequest, error) {
	req := &masterv1.UpdateMyProfileRequest{UserId: uid}

	setString := func(key string, field **string) {
		if v, ok := raw[key]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				*field = proto.String(s)
			}
		}
	}
	setInt32 := func(key string, field **int32) {
		if v, ok := raw[key]; ok {
			var n int32
			if err := json.Unmarshal(v, &n); err == nil {
				*field = proto.Int32(n)
			}
		}
	}
	setInt64 := func(key string, field **int64) {
		if v, ok := raw[key]; ok {
			var n int64
			if err := json.Unmarshal(v, &n); err == nil {
				*field = proto.Int64(n)
			}
		}
	}
	setFloat64 := func(key string, field **float64) {
		if v, ok := raw[key]; ok {
			var n float64
			if err := json.Unmarshal(v, &n); err == nil {
				*field = proto.Float64(n)
			}
		}
	}
	var dn, bio, phone, city, wf, avj, plf *string
	var pln, pinn, pkpp, pogrn, pogrnip, pbank, pbik, psettle, pcorr, pver *string
	var tr, ex *int32
	var hr *int64
	var tblat, tblon *float64
	setString("display_name", &dn)
	setString("bio", &bio)
	setString("phone", &phone)
	setString("city", &city)
	setString("work_format", &wf)
	setString("availability_json", &avj)
	setString("payout_legal_form", &plf)
	setString("payout_legal_name", &pln)
	setString("payout_inn", &pinn)
	setString("payout_kpp", &pkpp)
	setString("payout_ogrn", &pogrn)
	setString("payout_ogrnip", &pogrnip)
	setString("payout_bank_name", &pbank)
	setString("payout_bik", &pbik)
	setString("payout_settlement_account", &psettle)
	setString("payout_correspondent_account", &pcorr)
	setString("payout_verification_status", &pver)
	setInt32("travel_radius_km", &tr)
	setInt32("experience_years", &ex)
	setInt64("hourly_rate", &hr)
	setFloat64("travel_base_latitude", &tblat)
	setFloat64("travel_base_longitude", &tblon)
	req.DisplayName = dn
	req.Bio = bio
	req.Phone = phone
	req.City = city
	req.WorkFormat = wf
	req.AvailabilityJson = avj
	req.TravelRadiusKm = tr
	req.ExperienceYears = ex
	req.HourlyRate = hr
	req.TravelBaseLatitude = tblat
	req.TravelBaseLongitude = tblon
	req.PayoutLegalForm = plf
	req.PayoutLegalName = pln
	req.PayoutInn = pinn
	req.PayoutKpp = pkpp
	req.PayoutOgrn = pogrn
	req.PayoutOgrnip = pogrnip
	req.PayoutBankName = pbank
	req.PayoutBik = pbik
	req.PayoutSettlementAccount = psettle
	req.PayoutCorrespondentAccount = pcorr
	req.PayoutVerificationStatus = pver

	if _, ok := raw["specializations"]; ok {
		req.ApplySpecializations = true
		var specs []string
		_ = json.Unmarshal(raw["specializations"], &specs)
		req.Specializations = specs
	}
	if _, ok := raw["travel_exclude_zones"]; ok {
		var zones []struct {
			ID        string  `json:"id"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			RadiusKm  float64 `json:"radius_km"`
			Label     string  `json:"label"`
		}
		if err := json.Unmarshal(raw["travel_exclude_zones"], &zones); err != nil {
			return nil, errors.New("invalid travel_exclude_zones")
		}
		req.ApplyTravelExcludeZones = true
		for _, z := range zones {
			req.TravelExcludeZones = append(req.TravelExcludeZones, &masterv1.MasterTravelExcludeZone{
				Id:        z.ID,
				Latitude:  z.Latitude,
				Longitude: z.Longitude,
				RadiusKm:  z.RadiusKm,
				Label:     z.Label,
			})
		}
	}
	if v, ok := raw["services"]; ok {
		var items []masterServicePatch
		if err := json.Unmarshal(v, &items); err != nil {
			return nil, errors.New("invalid services")
		}
		// Пустой массив не заменяет услуги в БД (иначе легко получить 0 услуг и 400 при отправке на модерацию).
		if len(items) > 0 {
			req.ApplyServicesReplace = true
			for _, it := range items {
				inp := &masterv1.MasterServiceItemInput{
					Name:        it.Name,
					Description: it.Description,
					DurationMin: it.DurationMin,
					Price:       it.Price,
					SortOrder:   proto.Int32(it.SortOrder),
				}
				if it.ID != "" {
					inp.Id = proto.String(it.ID)
				}
				req.ServicesReplace = append(req.ServicesReplace, inp)
			}
		}
	}
	// Credentials (certificates/awards): unlike services, an empty list is a
	// valid edit (the master removed all of them), so presence of the key — not
	// a non-empty list — drives apply_credentials.
	if v, ok := raw["credentials"]; ok {
		var items []masterCredentialPatch
		if err := json.Unmarshal(v, &items); err != nil {
			return nil, errors.New("invalid credentials")
		}
		req.ApplyCredentials = true
		for _, it := range items {
			req.CredentialsReplace = append(req.CredentialsReplace, &masterv1.MasterCredentialItemInput{
				Kind:   it.Kind,
				Title:  it.Title,
				Issuer: it.Issuer,
				Year:   it.Year,
			})
		}
	}
	return req, nil
}

// PatchMyProfile PATCH /api/v1/owner/master/profile
func (h *MasterHandler) PatchMyProfile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	var raw map[string]json.RawMessage
	if !httpx.ReadJSONOrRespond(w, r, &raw) {
		return
	}
	req, err := h.updateReqFromRaw(uid, raw)
	if err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayMasterInvalidServices)
		return
	}

	resp, err := h.client.UpdateMyProfile(r.Context(), req)
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}

// SubmitForReview POST /api/v1/owner/master/profile/submit-for-review
// Тело — те же поля, что у PATCH (опционально). Если переданы — сначала сохраняются, затем отправка на модерацию (одна сессия, без гонки с отдельным PATCH).
func (h *MasterHandler) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limits.MasterImportMaxBodyBytes))
	if err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}
	trimmed := bytes.TrimSpace(bodyBytes)
	if len(trimmed) > 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidJson)
			return
		}
		if len(raw) > 0 {
			upd, err := h.updateReqFromRaw(uid, raw)
			if err != nil {
				httpx.WriteCatalog(w, apicatalog.GatewayMasterInvalidServices)
				return
			}
			if _, err := h.client.UpdateMyProfile(r.Context(), upd); err != nil {
				httpx.GRPCErrorToHTTP(w, err)
				return
			}
		}
	}
	resp, err := h.client.SubmitForReview(r.Context(), &masterv1.SubmitMasterForReviewRequest{UserId: uid})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}

// resolveClientName returns the booking client's display name, or "" when it
// cannot be resolved (no user client wired, lookup failed, or empty id). The
// caller passes a per-request cache so a repeat client costs one lookup.
func (h *MasterHandler) resolveClientName(r *http.Request, userID string, cache map[string]string) string {
	id := strings.TrimSpace(userID)
	if id == "" || h.userClient == nil {
		return ""
	}
	if name, ok := cache[id]; ok {
		return name
	}
	name := ""
	if u, err := h.userClient.GetUser(r.Context(), &userv1.GetUserRequest{Id: id}); err == nil {
		name = strings.TrimSpace(u.GetName())
	}
	cache[id] = name
	return name
}

// ListMyBookings GET /api/v1/owner/master/bookings
func (h *MasterHandler) ListMyBookings(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.ListMyMasterBookings(r.Context(), &masterv1.ListMyMasterBookingsRequest{
		UserId:       uid,
		StatusFilter: r.URL.Query().Get("status"),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	nameCache := make(map[string]string, len(resp.GetBookings()))
	out := make([]map[string]any, 0, len(resp.GetBookings()))
	for _, b := range resp.GetBookings() {
		out = append(out, map[string]any{
			"id":                b.GetId(),
			"master_id":         b.GetMasterId(),
			"client_user_id":    b.GetClientUserId(),
			"client_name":       h.resolveClientName(r, b.GetClientUserId(), nameCache),
			"master_service_id": b.GetMasterServiceId(),
			"date":              b.GetDate(),
			"time_from":         b.GetTimeFrom(),
			"time_to":           b.GetTimeTo(),
			"comment":           b.GetComment(),
			"status":            b.GetStatus(),
			"payment_id":        b.GetPaymentId(),
			"payment_url":       b.GetPaymentUrl(),
			"total_price":       b.GetTotalPrice(),
			"created_at":        b.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bookings": out})
}

// ListMyClients GET /api/v1/owner/master/clients
// Returns the authenticated master's client base, aggregated from their
// bookings. Client display names are enriched from user-service (the projection
// itself carries only UUIDs — see docs/CRM_DESIGN.md §4.2 on the PII boundary).
func (h *MasterHandler) ListMyClients(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	q := r.URL.Query()
	limit, ok := httpx.QueryInt(w, r, "limit", 50, 1, 100)
	if !ok {
		return
	}
	offset, ok := httpx.QueryInt(w, r, "offset", 0, 0, 0)
	if !ok {
		return
	}
	resp, err := h.client.ListMyMasterClients(r.Context(), &masterv1.ListMyMasterClientsRequest{
		UserId:  uid,
		Segment: q.Get("segment"),
		Sort:    q.Get("sort"),
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	nameCache := make(map[string]string, len(resp.GetClients()))
	out := make([]map[string]any, 0, len(resp.GetClients()))
	for _, c := range resp.GetClients() {
		m := map[string]any{
			"user_id":        c.GetUserId(),
			"user_name":      h.resolveClientName(r, c.GetUserId(), nameCache),
			"bookings_count": c.GetBookingsCount(),
			"visits_count":   c.GetVisitsCount(),
			"total_spent":    c.GetTotalSpent(),
			"segments":       c.GetSegments(),
		}
		if c.GetFirstVisitAt() != nil {
			m["first_visit_at"] = c.GetFirstVisitAt().AsTime()
		}
		if c.GetLastVisitAt() != nil {
			m["last_visit_at"] = c.GetLastVisitAt().AsTime()
		}
		if c.GetLastBookingAt() != nil {
			m["last_booking_at"] = c.GetLastBookingAt().AsTime()
		}
		out = append(out, m)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"clients": out, "total": resp.GetTotal()})
}

func slotBlockToJSON(b *masterv1.MasterSlotBlock) map[string]any {
	if b == nil {
		return nil
	}
	return map[string]any{
		"id":         b.GetId(),
		"master_id":  b.GetMasterId(),
		"date":       b.GetDate(),
		"time_from":  b.GetTimeFrom(),
		"time_to":    b.GetTimeTo(),
		"note":       b.GetNote(),
		"created_at": b.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListSlotBlocks GET /api/v1/owner/master/schedule/blocks
func (h *MasterHandler) ListSlotBlocks(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.ListMasterSlotBlocks(r.Context(), &masterv1.ListMasterSlotBlocksRequest{UserId: uid})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	out := make([]map[string]any, 0, len(resp.GetBlocks()))
	for _, b := range resp.GetBlocks() {
		out = append(out, slotBlockToJSON(b))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"blocks": out})
}

// CreateSlotBlock POST /api/v1/owner/master/schedule/blocks
func (h *MasterHandler) CreateSlotBlock(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	var body struct {
		Date     string `json:"date"`
		TimeFrom string `json:"time_from"`
		TimeTo   string `json:"time_to"`
		Note     string `json:"note"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &body) {
		return
	}
	resp, err := h.client.CreateMasterSlotBlock(r.Context(), &masterv1.CreateMasterSlotBlockRequest{
		UserId:   uid,
		Date:     body.Date,
		TimeFrom: body.TimeFrom,
		TimeTo:   body.TimeTo,
		Note:     body.Note,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, slotBlockToJSON(resp.GetBlock()))
}

// DeleteSlotBlock DELETE /api/v1/owner/master/schedule/blocks/{blockId}
func (h *MasterHandler) DeleteSlotBlock(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	blockID := chi.URLParam(r, "blockId")
	if _, err := h.client.DeleteMasterSlotBlock(r.Context(), &masterv1.DeleteMasterSlotBlockRequest{
		UserId:  uid,
		BlockId: blockID,
	}); err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ListMyClientBookings GET /api/v1/my/master-bookings
func (h *MasterHandler) ListMyClientBookings(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.ListClientMasterBookings(r.Context(), &masterv1.ListClientMasterBookingsRequest{
		UserId:       uid,
		StatusFilter: r.URL.Query().Get("status"),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	out := make([]map[string]any, 0, len(resp.GetBookings()))
	for _, b := range resp.GetBookings() {
		out = append(out, map[string]any{
			"id":                b.GetId(),
			"master_id":         b.GetMasterId(),
			"client_user_id":    b.GetClientUserId(),
			"master_service_id": b.GetMasterServiceId(),
			"date":              b.GetDate(),
			"time_from":         b.GetTimeFrom(),
			"time_to":           b.GetTimeTo(),
			"comment":           b.GetComment(),
			"status":            b.GetStatus(),
			"payment_id":        b.GetPaymentId(),
			"payment_url":       b.GetPaymentUrl(),
			"total_price":       b.GetTotalPrice(),
			"created_at":        b.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bookings": out})
}

// CreateBooking POST /api/v1/masters/{slug}/bookings
func (h *MasterHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	slug := chi.URLParam(r, "slug")
	var body struct {
		MasterServiceID string `json:"master_service_id"`
		Date            string `json:"date"`
		TimeFrom        string `json:"time_from"`
		TimeTo          string `json:"time_to"`
		Comment         string `json:"comment"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &body) {
		return
	}
	grpcReq := &masterv1.CreateMasterBookingRequest{
		ClientUserId: uid,
		MasterSlug:   slug,
		Date:         body.Date,
		TimeFrom:     body.TimeFrom,
		TimeTo:       body.TimeTo,
		Comment:      body.Comment,
	}
	if body.MasterServiceID != "" {
		grpcReq.MasterServiceId = proto.String(body.MasterServiceID)
	}
	resp, err := h.client.CreateMasterBooking(r.Context(), grpcReq)
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	b := resp.GetBooking()

	// Bell for the master (best-effort, never fails the booking). Skipped when
	// the master books themselves. Fired before the response is written so
	// r.Context() is still live.
	if h.notifier != nil {
		masterUserID := strings.TrimSpace(b.GetMasterUserId())
		if masterUserID != "" && masterUserID != uid {
			h.notifier.NotifyMasterBookingCreated(
				r.Context(),
				masterUserID, b.GetMasterId(), "",
				b.GetId(), b.GetDate(), b.GetTimeFrom(), b.GetTimeTo(),
			)
		}
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":                b.GetId(),
		"master_id":         b.GetMasterId(),
		"client_user_id":    b.GetClientUserId(),
		"master_service_id": b.GetMasterServiceId(),
		"date":              b.GetDate(),
		"time_from":         b.GetTimeFrom(),
		"time_to":           b.GetTimeTo(),
		"comment":           b.GetComment(),
		"status":            b.GetStatus(),
		"payment_id":        b.GetPaymentId(),
		"payment_url":       b.GetPaymentUrl(),
		"total_price":       b.GetTotalPrice(),
		"created_at":        b.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ListForModeration GET /api/v1/admin/masters
func (h *MasterHandler) ListForModeration(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := httpx.QueryInt(w, r, "limit", 0, 0, 500)
	if !ok {
		return
	}
	off, ok := httpx.QueryInt(w, r, "offset", 0, 0, 1000000)
	if !ok {
		return
	}
	resp, err := h.client.ListForModeration(r.Context(), &masterv1.ListForModerationRequest{
		StatusFilter: q.Get("status"),
		Limit:        int32(limit),
		Offset:       int32(off),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	list := make([]map[string]any, 0, len(resp.GetMasters()))
	for _, m := range resp.GetMasters() {
		list = append(list, masterProtoToJSON(m))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"masters": list, "total": resp.GetTotal()})
}

// Moderate POST /api/v1/admin/masters/{id}/moderate
func (h *MasterHandler) Moderate(w http.ResponseWriter, r *http.Request) {
	modID := middleware.UserIDFromCtx(r.Context())
	if modID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &body) {
		return
	}
	resp, err := h.client.ModerateMaster(r.Context(), &masterv1.ModerateMasterRequest{
		MasterId:    id,
		ModeratorId: modID,
		Action:      body.Action,
		Comment:     body.Comment,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	// Bell for the master on every moderation decision (best-effort, never
	// fails the moderation). Fired before the response is written so
	// r.Context() is still live.
	if h.notifier != nil {
		m := resp.GetMaster()
		h.notifier.NotifyMasterModerated(
			r.Context(),
			strings.TrimSpace(m.GetUserId()),
			m.GetId(),
			strings.TrimSpace(m.GetDisplayName()),
			body.Action,
			strings.TrimSpace(body.Comment),
		)
	}

	httpx.WriteJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}

// ModerationHistory GET /api/v1/admin/masters/{id}/moderation-history
func (h *MasterHandler) ModerationHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	lim, ok := httpx.QueryInt(w, r, "limit", 0, 0, 500)
	if !ok {
		return
	}
	resp, err := h.client.ListModerationHistory(r.Context(), &masterv1.ListModerationHistoryRequest{
		MasterId: id,
		Limit:    int32(lim),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	entries := make([]map[string]any, 0, len(resp.GetEntries()))
	for _, e := range resp.GetEntries() {
		entries = append(entries, map[string]any{
			"id":         e.GetId(),
			"master_id":  e.GetMasterId(),
			"old_status": e.GetOldStatus(),
			"new_status": e.GetNewStatus(),
			"comment":    e.GetComment(),
			"changed_by": e.GetChangedBy(),
			"created_at": e.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
