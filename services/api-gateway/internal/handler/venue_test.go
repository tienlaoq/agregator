package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockVenueClient implements venuev1.VenueServiceClient for tests.
type mockVenueClient struct {
	OnCreateVenue            func(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnUpdateVenue            func(ctx context.Context, in *venuev1.UpdateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnGetVenue               func(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnGetVenueBySlug         func(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnListVenues             func(ctx context.Context, in *venuev1.ListVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
	OnSearchVenues           func(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
	OnListOwnerVenues        func(ctx context.Context, in *venuev1.ListOwnerVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
	OnCheckSlotAvailability  func(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error)
	OnReserveSlot            func(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error)
	OnReleaseSlot            func(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error)
	OnUpdateRating           func(ctx context.Context, in *venuev1.UpdateRatingRequest, opts ...grpc.CallOption) (*venuev1.UpdateRatingResponse, error)
	OnModerateVenue          func(ctx context.Context, in *venuev1.ModerateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnListPendingVenues      func(ctx context.Context, in *venuev1.ListPendingVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
}

func (m *mockVenueClient) CreateVenue(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnCreateVenue != nil {
		return m.OnCreateVenue(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) UpdateVenue(ctx context.Context, in *venuev1.UpdateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnUpdateVenue != nil {
		return m.OnUpdateVenue(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) GetVenue(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnGetVenue != nil {
		return m.OnGetVenue(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) GetVenueBySlug(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnGetVenueBySlug != nil {
		return m.OnGetVenueBySlug(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ListVenues(ctx context.Context, in *venuev1.ListVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	if m.OnListVenues != nil {
		return m.OnListVenues(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) SearchVenues(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	if m.OnSearchVenues != nil {
		return m.OnSearchVenues(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ListOwnerVenues(ctx context.Context, in *venuev1.ListOwnerVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	if m.OnListOwnerVenues != nil {
		return m.OnListOwnerVenues(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) CheckSlotAvailability(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
	if m.OnCheckSlotAvailability != nil {
		return m.OnCheckSlotAvailability(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ReserveSlot(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error) {
	if m.OnReserveSlot != nil {
		return m.OnReserveSlot(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ReleaseSlot(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error) {
	if m.OnReleaseSlot != nil {
		return m.OnReleaseSlot(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) UpdateRating(ctx context.Context, in *venuev1.UpdateRatingRequest, opts ...grpc.CallOption) (*venuev1.UpdateRatingResponse, error) {
	if m.OnUpdateRating != nil {
		return m.OnUpdateRating(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ModerateVenue(ctx context.Context, in *venuev1.ModerateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnModerateVenue != nil {
		return m.OnModerateVenue(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ListPendingVenues(ctx context.Context, in *venuev1.ListPendingVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	if m.OnListPendingVenues != nil {
		return m.OnListPendingVenues(ctx, in, opts...)
	}
	return nil, nil
}

func sampleVenueResponse() *venuev1.VenueResponse {
	return &venuev1.VenueResponse{
		Id:        "venue-1",
		OwnerId:   "user-123",
		Slug:      "cozy-banya",
		Name:      "Cozy Banya",
		Type:      "banya",
		CreatedAt: timestamppb.New(time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)),
	}
}

func withRouteParams(ctx context.Context, pairs ...string) context.Context {
	rctx := chi.NewRouteContext()
	for i := 0; i+1 < len(pairs); i += 2 {
		rctx.URLParams.Add(pairs[i], pairs[i+1])
	}
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

func TestList_Success(t *testing.T) {
	mock := &mockVenueClient{
		OnListVenues: func(ctx context.Context, in *venuev1.ListVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
			assert.Equal(t, int32(1), in.GetPage())
			assert.Equal(t, int32(10), in.GetPageSize())
			return &venuev1.ListVenuesResponse{
				Venues:   []*venuev1.VenueResponse{sampleVenueResponse()},
				Total:    1,
				Page:     1,
				PageSize: 10,
			}, nil
		},
	}
	h := NewVenueHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/venues?page=1&page_size=10", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	venues, ok := body["venues"].([]any)
	require.True(t, ok)
	require.Len(t, venues, 1)
	assert.EqualValues(t, 1, body["total"])
	assert.EqualValues(t, 1, body["page"])
	assert.EqualValues(t, 10, body["page_size"])
}

func TestGetBySlug_Success(t *testing.T) {
	mock := &mockVenueClient{
		OnGetVenueBySlug: func(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			assert.Equal(t, "cozy-banya", in.GetSlug())
			v := sampleVenueResponse()
			v.Inn = "7707083893"
			v.LegalEntityName = "ООО Секрет"
			return v, nil
		},
	}
	h := NewVenueHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/venues/cozy-banya", nil)
	ctx := withRouteParams(req.Context(), "slug", "cozy-banya")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.GetBySlug(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "venue-1", body["id"])
	assert.Equal(t, "cozy-banya", body["slug"])
	assert.Equal(t, "Cozy Banya", body["name"])
	_, hasInn := body["inn"]
	assert.False(t, hasInn, "public venue JSON must not expose verification fields")
}

func TestGetBySlug_NotFound(t *testing.T) {
	mock := &mockVenueClient{
		OnGetVenueBySlug: func(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			return nil, status.Error(codes.NotFound, "venue not found")
		},
	}
	h := NewVenueHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/venues/missing", nil)
	ctx := withRouteParams(req.Context(), "slug", "missing")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.GetBySlug(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "venue not found", body["error"])
}

func TestCreate_Success(t *testing.T) {
	mock := &mockVenueClient{
		OnCreateVenue: func(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			assert.Equal(t, "user-123", in.GetOwnerId())
			assert.Equal(t, "New Spa", in.GetName())
			assert.Equal(t, "banya", in.GetType())
			assert.Equal(t, "Main st", in.GetAddress())
			assert.Equal(t, "ИП Тестов Тест Тестович", in.GetLegalEntityName())
			assert.Equal(t, "7707083893", in.GetInn())
			assert.Equal(t, "1027700132195", in.GetOgrn())
			assert.Equal(t, "https://yandex.ru/maps/org/x", in.GetPublicListingUrl())
			return sampleVenueResponse(), nil
		},
	}
	h := NewVenueHandler(mock)

	payload := `{"name":"New Spa","type":"banya","description":"d","address":"Main st","city":"X","latitude":1,"longitude":2,"price_from":100,"capacity":10,"amenities":["pool"],"working_hours":"9-5","phone":"1","services":[],"legal_entity_name":"ИП Тестов Тест Тестович","inn":"7707083893","ogrn":"1027700132195","public_listing_url":"https://yandex.ru/maps/org/x"}`
	req := httptest.NewRequest(http.MethodPost, "/venues", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.CtxUserID, "user-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "venue-1", body["id"])
}

func TestCreate_Unauthorized(t *testing.T) {
	h := NewVenueHandler(&mockVenueClient{})

	payload := `{"name":"X","type":"banya"}`
	req := httptest.NewRequest(http.MethodPost, "/venues", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "unauthorized", body["error"])
}

func TestModerate_Success(t *testing.T) {
	mock := &mockVenueClient{
		OnModerateVenue: func(ctx context.Context, in *venuev1.ModerateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			assert.Equal(t, "venue-id", in.GetVenueId())
			assert.Equal(t, "approve", in.GetAction())
			assert.Equal(t, "looks good", in.GetComment())
			assert.Equal(t, "admin-1", in.GetModeratedBy())
			v := sampleVenueResponse()
			v.Id = "venue-id"
			v.Status = "active"
			return v, nil
		},
	}
	h := NewVenueHandler(mock)

	payload := `{"action":"approve","comment":"looks good"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/venues/venue-id/moderate", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.CtxUserID, "admin-1")
	ctx = context.WithValue(ctx, middleware.CtxRole, "admin")
	ctx = withRouteParams(ctx, "id", "venue-id")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.Moderate(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "venue-id", body["id"])
	assert.Equal(t, "active", body["status"])
}

func TestModerate_Unauthorized(t *testing.T) {
	h := NewVenueHandler(&mockVenueClient{})

	payload := `{"action":"approve","comment":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/venues/v1/moderate", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := withRouteParams(req.Context(), "id", "v1")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.Moderate(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "unauthorized", body["error"])
}

func TestListPending_Success(t *testing.T) {
	mock := &mockVenueClient{
		OnListPendingVenues: func(ctx context.Context, in *venuev1.ListPendingVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
			assert.Equal(t, int32(2), in.GetPage())
			assert.Equal(t, int32(5), in.GetPageSize())
			assert.Equal(t, "pending_review", in.GetStatus())
			return &venuev1.ListVenuesResponse{
				Venues:   []*venuev1.VenueResponse{sampleVenueResponse()},
				Total:    3,
				Page:     2,
				PageSize: 5,
			}, nil
		},
	}
	h := NewVenueHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/admin/venues/pending?page=2&page_size=5&status=pending_review", nil)
	rr := httptest.NewRecorder()
	h.ListPending(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	venues, ok := body["venues"].([]any)
	require.True(t, ok)
	require.Len(t, venues, 1)
	assert.EqualValues(t, 3, body["total"])
}
