package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgcities "github.com/tienlao/agregator/pkg/cities"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockVenueClient implements venuev1.VenueServiceClient for tests.
type mockVenueClient struct {
	OnCreateVenue           func(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnSubmitVenueForReview  func(ctx context.Context, in *venuev1.SubmitVenueForReviewRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnUpdateVenue           func(ctx context.Context, in *venuev1.UpdateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnGetVenue              func(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnGetVenueBySlug        func(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnListVenues            func(ctx context.Context, in *venuev1.ListVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
	OnSearchVenues          func(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
	OnCheckSlotAvailability func(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error)
	OnReserveSlot           func(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error)
	OnReleaseSlot           func(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error)
	OnUpdateRating          func(ctx context.Context, in *venuev1.UpdateRatingRequest, opts ...grpc.CallOption) (*venuev1.UpdateRatingResponse, error)
	OnModerateVenue         func(ctx context.Context, in *venuev1.ModerateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	OnListPendingVenues     func(ctx context.Context, in *venuev1.ListPendingVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error)
}

func (m *mockVenueClient) CreateVenue(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnCreateVenue != nil {
		return m.OnCreateVenue(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) SubmitVenueForReview(ctx context.Context, in *venuev1.SubmitVenueForReviewRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.OnSubmitVenueForReview != nil {
		return m.OnSubmitVenueForReview(ctx, in, opts...)
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

// ListOwnerVenues was removed from venue.proto (gateway now composes the
// list from crm.ListManagedVenues + venue.GetVenuesBatch).

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

func (m *mockVenueClient) CreateManualSlotBlock(ctx context.Context, in *venuev1.CreateManualSlotBlockRequest, opts ...grpc.CallOption) (*venuev1.CreateManualSlotBlockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateManualSlotBlock")
}

func (m *mockVenueClient) DeleteManualSlotBlock(ctx context.Context, in *venuev1.DeleteManualSlotBlockRequest, opts ...grpc.CallOption) (*venuev1.DeleteManualSlotBlockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteManualSlotBlock")
}

func (m *mockVenueClient) ListManualSlotBlocks(ctx context.Context, in *venuev1.ListManualSlotBlocksRequest, opts ...grpc.CallOption) (*venuev1.ListManualSlotBlocksResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListManualSlotBlocks")
}

func (m *mockVenueClient) AddVenuePhoto(ctx context.Context, in *venuev1.AddVenuePhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "AddVenuePhoto")
}

func (m *mockVenueClient) DeleteVenuePhoto(ctx context.Context, in *venuev1.DeleteVenuePhotoRequest, opts ...grpc.CallOption) (*venuev1.DeleteVenuePhotoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteVenuePhoto")
}

func (m *mockVenueClient) SetVenueCoverPhoto(ctx context.Context, in *venuev1.SetVenueCoverPhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "SetVenueCoverPhoto")
}

func (m *mockVenueClient) AddVenueHallPhoto(ctx context.Context, in *venuev1.AddVenueHallPhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "AddVenueHallPhoto")
}

func (m *mockVenueClient) DeleteVenueHallPhoto(ctx context.Context, in *venuev1.DeleteVenueHallPhotoRequest, opts ...grpc.CallOption) (*venuev1.DeleteVenueHallPhotoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteVenueHallPhoto")
}

func (m *mockVenueClient) SetVenueHallCoverPhoto(ctx context.Context, in *venuev1.SetVenueHallCoverPhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "SetVenueHallCoverPhoto")
}

// CRM RPCs (GetVenueManagementAccess, ListVenueStaff, AddVenueStaff,
// RemoveVenueStaff, ListVenueCRMTasks, CreateVenueCRMTask,
// CompleteVenueCRMTask) were removed from venue.proto and live in crm-service.

func (m *mockVenueClient) GetVenuesBatch(ctx context.Context, in *venuev1.GetVenuesBatchRequest, opts ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetVenuesBatch")
}

func (m *mockVenueClient) BatchCheckSlotAvailability(ctx context.Context, in *venuev1.BatchCheckSlotRequest, opts ...grpc.CallOption) (*venuev1.BatchCheckSlotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "BatchCheckSlotAvailability")
}

// noopUploader satisfies storage.Uploader without touching the filesystem.
type noopVenueUploader struct{}

func (noopVenueUploader) Put(_ context.Context, _, _ string, _ int64, _ io.Reader) (string, error) {
	return "https://example.com/photo.jpg", nil
}

func (noopVenueUploader) Delete(_ context.Context, _ string) error { return nil }

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
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

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

func TestSearch_CityOnlyDuplicatesIntoQuery(t *testing.T) {
	mock := &mockVenueClient{
		OnSearchVenues: func(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
			assert.Equal(t, "Пенза", in.GetQuery())
			assert.Equal(t, "Пенза", in.GetCity())
			return &venuev1.ListVenuesResponse{Total: 0, Page: 1, PageSize: 12}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	req := httptest.NewRequest(http.MethodGet, "/venues/search?city=Пенза&page=1&page_size=12", nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestSearch_MultipleCitiesJoinsGRPCCity(t *testing.T) {
	var got *venuev1.SearchVenuesRequest
	mock := &mockVenueClient{
		OnSearchVenues: func(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
			got = in
			return &venuev1.ListVenuesResponse{Total: 0, Page: 1, PageSize: 12}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	// Рекомендуемый формат в query: `|` между городами.
	q := url.Values{}
	q.Set("cities", "Пенза|Москва")
	q.Set("page", "1")
	q.Set("page_size", "12")
	req := httptest.NewRequest(http.MethodGet, "/venues/search?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, got)
	assert.Contains(t, got.GetCity(), pkgcities.Sep)
	assert.Equal(t, "", got.GetQuery())
}

func TestSearch_MultipleCitiesLegacyUnitSeparatorInQuery(t *testing.T) {
	var got *venuev1.SearchVenuesRequest
	mock := &mockVenueClient{
		OnSearchVenues: func(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
			got = in
			return &venuev1.ListVenuesResponse{Total: 0, Page: 1, PageSize: 12}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	// Как в адресной строке браузера: Москва \x1e Пенза
	raw := "/venues/search?cities=%D0%9C%D0%BE%D1%81%D0%BA%D0%B2%D0%B0%1E%D0%9F%D0%B5%D0%BD%D0%B7%D0%B0&page=1&page_size=12"
	req := httptest.NewRequest(http.MethodGet, raw, nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, got)
	assert.Contains(t, got.GetCity(), pkgcities.Sep)
	assert.Equal(t, "", got.GetQuery())
}

func TestSearch_QAndCityKeepsSeparate(t *testing.T) {
	mock := &mockVenueClient{
		OnSearchVenues: func(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
			assert.Equal(t, "сауна", in.GetQuery())
			assert.Equal(t, "Пенза", in.GetCity())
			return &venuev1.ListVenuesResponse{Total: 0, Page: 1, PageSize: 12}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	req := httptest.NewRequest(http.MethodGet, "/venues/search?q=сауна&city=Пенза&page=1&page_size=12", nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
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
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

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
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	req := httptest.NewRequest(http.MethodGet, "/venues/missing", nil)
	ctx := withRouteParams(req.Context(), "slug", "missing")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.GetBySlug(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, apicatalog.GatewayUpstreamNotFound.Code, body["code"])
	assert.Equal(t, "venue not found", body["error"])
}

func TestGetBySlug_DraftHiddenForAnonymous(t *testing.T) {
	mock := &mockVenueClient{
		OnGetVenueBySlug: func(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			v := sampleVenueResponse()
			v.Status = "draft"
			return v, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	req := httptest.NewRequest(http.MethodGet, "/venues/cozy-banya", nil)
	ctx := withRouteParams(req.Context(), "slug", "cozy-banya")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.GetBySlug(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetBySlug_DraftVisibleForAdmin(t *testing.T) {
	mock := &mockVenueClient{
		OnGetVenueBySlug: func(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			v := sampleVenueResponse()
			v.Status = "draft"
			return v, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	req := httptest.NewRequest(http.MethodGet, "/venues/cozy-banya", nil)
	ctx := withRouteParams(req.Context(), "slug", "cozy-banya")
	ctx = middleware.WithUserID(ctx, "admin-user")
	ctx = middleware.WithRole(ctx, "admin")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.GetBySlug(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
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
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	payload := `{"name":"New Spa","type":"banya","description":"d","address":"Main st","city":"X","latitude":1,"longitude":2,"price_from":100,"capacity":10,"amenities":["pool"],"working_hours":"9-5","phone":"1","services":[],"legal_entity_name":"ИП Тестов Тест Тестович","inn":"7707083893","ogrn":"1027700132195","public_listing_url":"https://yandex.ru/maps/org/x"}`
	req := httptest.NewRequest(http.MethodPost, "/venues", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.WithUserID(req.Context(), "user-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "venue-1", body["id"])
}

func TestCreate_Unauthorized(t *testing.T) {
	h := NewVenueHandler(&mockVenueClient{}, nil, nil, noopVenueUploader{})

	payload := `{"name":"X","type":"banya"}`
	req := httptest.NewRequest(http.MethodPost, "/venues", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, apicatalog.GatewayAuthUnauthorized.Code, body["code"])
	assert.Equal(t, apicatalog.GatewayAuthUnauthorized.Message, body["error"])
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
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

	payload := `{"action":"approve","comment":"looks good"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/venues/venue-id/moderate", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.WithUserID(req.Context(), "admin-1")
	ctx = middleware.WithRole(ctx, "admin")
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
	h := NewVenueHandler(&mockVenueClient{}, nil, nil, noopVenueUploader{})

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
	assert.Equal(t, apicatalog.GatewayAuthUnauthorized.Code, body["code"])
	assert.Equal(t, apicatalog.GatewayAuthUnauthorized.Message, body["error"])
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
	h := NewVenueHandler(mock, nil, nil, noopVenueUploader{})

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
