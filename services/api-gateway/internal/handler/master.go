package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	pkgcities "github.com/tienlao/agregator/pkg/cities"
	"github.com/tienlao/agregator/pkg/storage"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type MasterHandler struct {
	client  masterv1.MasterServiceClient
	storage storage.Uploader
}

func NewMasterHandler(c masterv1.MasterServiceClient, up storage.Uploader) *MasterHandler {
	return &MasterHandler{client: c, storage: up}
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
		"yookassa_seller_account_id":   m.GetYookassaSellerAccountId(),
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
	delete(out, "yookassa_seller_account_id")
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
	page, ok := queryInt(w, r, "page", 0, 0, 10000)
	if !ok {
		return
	}
	pageSize, ok := queryInt(w, r, "page_size", 0, 0, 200)
	if !ok {
		return
	}
	limit, ok := queryInt(w, r, "limit", 0, 0, 500)
	if !ok {
		return
	}
	off, ok := queryInt(w, r, "offset", 0, 0, 1000000)
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
		grpcErrorToHTTP(w, err)
		return
	}
	list := make([]map[string]any, 0, len(resp.GetMasters()))
	for _, m := range resp.GetMasters() {
		list = append(list, masterProtoToJSONPublic(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"masters": list, "total": resp.GetTotal()})
}

// GetPublic GET /api/v1/masters/{slug}
func (h *MasterHandler) GetPublic(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	resp, err := h.client.GetPublicMaster(r.Context(), &masterv1.GetPublicMasterRequest{Slug: slug})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, masterProtoToJSONPublic(resp.GetMaster()))
}

// CreateMyProfile POST /api/v1/owner/master/profile
func (h *MasterHandler) CreateMyProfile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if !readJSONOrRespond(w, r, &body) { return }
	resp, err := h.client.CreateMyProfile(r.Context(), &masterv1.CreateMyProfileRequest{
		UserId:      uid,
		DisplayName: strings.TrimSpace(body.DisplayName),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, masterProtoToJSON(resp.GetMaster()))
}

// GetMyProfile GET /api/v1/owner/master/profile
// Всегда 200: { "profile": <object|null> } — отсутствие профиля не считается HTTP-ошибкой.
func (h *MasterHandler) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: uid})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			writeJSON(w, http.StatusOK, map[string]any{"profile": nil})
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": masterProtoToJSON(resp.GetMaster())})
}

type masterServicePatch struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DurationMin int32  `json:"duration_min"`
	Price       int64  `json:"price"`
	SortOrder   int32  `json:"sort_order"`
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
	var ysa, pln, pinn, pkpp, pogrn, pogrnip, pbank, pbik, psettle, pcorr, pver *string
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
	setString("yookassa_seller_account_id", &ysa)
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
	req.YookassaSellerAccountId = ysa
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
	return req, nil
}

// PatchMyProfile PATCH /api/v1/owner/master/profile
func (h *MasterHandler) PatchMyProfile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	var raw map[string]json.RawMessage
	if !readJSONOrRespond(w, r, &raw) { return }
	req, err := h.updateReqFromRaw(uid, raw)
	if err != nil {
		writeCatalog(w, apicatalog.GatewayMasterInvalidServices)
		return
	}

	resp, err := h.client.UpdateMyProfile(r.Context(), req)
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}

// SubmitForReview POST /api/v1/owner/master/profile/submit-for-review
// Тело — те же поля, что у PATCH (опционально). Если переданы — сначала сохраняются, затем отправка на модерацию (одна сессия, без гонки с отдельным PATCH).
func (h *MasterHandler) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limits.MasterImportMaxBodyBytes))
	if err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}
	trimmed := bytes.TrimSpace(bodyBytes)
	if len(trimmed) > 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			writeCatalog(w, apicatalog.GatewayRequestInvalidJson)
			return
		}
		if len(raw) > 0 {
			upd, err := h.updateReqFromRaw(uid, raw)
			if err != nil {
				writeCatalog(w, apicatalog.GatewayMasterInvalidServices)
				return
			}
			if _, err := h.client.UpdateMyProfile(r.Context(), upd); err != nil {
				grpcErrorToHTTP(w, err)
				return
			}
		}
	}
	resp, err := h.client.SubmitForReview(r.Context(), &masterv1.SubmitMasterForReviewRequest{UserId: uid})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}

// ListMyBookings GET /api/v1/owner/master/bookings
func (h *MasterHandler) ListMyBookings(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.ListMyMasterBookings(r.Context(), &masterv1.ListMyMasterBookingsRequest{
		UserId:       uid,
		StatusFilter: r.URL.Query().Get("status"),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"bookings": out})
}

// ListMyClientBookings GET /api/v1/my/master-bookings
func (h *MasterHandler) ListMyClientBookings(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.ListClientMasterBookings(r.Context(), &masterv1.ListClientMasterBookingsRequest{
		UserId:       uid,
		StatusFilter: r.URL.Query().Get("status"),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"bookings": out})
}

// CreateBooking POST /api/v1/masters/{slug}/bookings
func (h *MasterHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromCtx(r.Context())
	if uid == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
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
	if !readJSONOrRespond(w, r, &body) { return }
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
		grpcErrorToHTTP(w, err)
		return
	}
	b := resp.GetBooking()
	writeJSON(w, http.StatusCreated, map[string]any{
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
	limit, ok := queryInt(w, r, "limit", 0, 0, 500)
	if !ok {
		return
	}
	off, ok := queryInt(w, r, "offset", 0, 0, 1000000)
	if !ok {
		return
	}
	resp, err := h.client.ListForModeration(r.Context(), &masterv1.ListForModerationRequest{
		StatusFilter: q.Get("status"),
		Limit:        int32(limit),
		Offset:       int32(off),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	list := make([]map[string]any, 0, len(resp.GetMasters()))
	for _, m := range resp.GetMasters() {
		list = append(list, masterProtoToJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"masters": list, "total": resp.GetTotal()})
}

// Moderate POST /api/v1/admin/masters/{id}/moderate
func (h *MasterHandler) Moderate(w http.ResponseWriter, r *http.Request) {
	modID := middleware.UserIDFromCtx(r.Context())
	if modID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if !readJSONOrRespond(w, r, &body) { return }
	resp, err := h.client.ModerateMaster(r.Context(), &masterv1.ModerateMasterRequest{
		MasterId:    id,
		ModeratorId: modID,
		Action:      body.Action,
		Comment:     body.Comment,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}

// ModerationHistory GET /api/v1/admin/masters/{id}/moderation-history
func (h *MasterHandler) ModerationHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	lim, ok := queryInt(w, r, "limit", 0, 0, 500)
	if !ok {
		return
	}
	resp, err := h.client.ListModerationHistory(r.Context(), &masterv1.ListModerationHistoryRequest{
		MasterId: id,
		Limit:    int32(lim),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
