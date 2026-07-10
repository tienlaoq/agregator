package handler

// Integration tests: JWT → Auth middleware → chi router → BookingHandler → mock gRPC.
//
// Unlike the unit tests in other *_test.go files, these tests wire the full
// request path so that:
//   - URL parameters (chi.URLParam) are populated by the real chi router
//   - The Auth middleware extracts claims from the JWT and populates context
//   - Query-parameter validation (queryInt) runs on a real HTTP request
//   - The handler can only read userID if the middleware actually ran
//
// Each test builds a minimal chi.Mux with the same route structure as the
// production router and drives it through httptest.NewRecorder.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgauth "github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/pkg/roles"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// integTestPrivKey / integTestPubKey are generated once per test binary run.
// Using package-level keys avoids re-generating a key pair per test while
// still ensuring test tokens cannot be confused with production ones.
var (
	integTestPrivKey *ecdsa.PrivateKey
	integTestPubKey  *ecdsa.PublicKey
)

func init() {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic("booking_integration_test: failed to generate test EC key: " + err.Error())
	}
	integTestPrivKey = priv
	integTestPubKey = &priv.PublicKey
}

// ── noop base implementations ─────────────────────────────────────────────────
// These structs implement the full gRPC client interfaces with "unimplemented"
// stubs. Concrete mocks embed them and override only what the test needs.

type noopBookingClient struct{}

func (noopBookingClient) CreateBooking(context.Context, *bookingv1.CreateBookingRequest, ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) GetBooking(context.Context, *bookingv1.GetBookingRequest, ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) ListUserBookings(context.Context, *bookingv1.ListUserBookingsRequest, ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) ListVenueBookings(context.Context, *bookingv1.ListVenueBookingsRequest, ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) CancelBooking(context.Context, *bookingv1.CancelBookingRequest, ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) ConfirmBooking(context.Context, *bookingv1.ConfirmBookingRequest, ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) CompleteBooking(context.Context, *bookingv1.CompleteBookingRequest, ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) HasCompletedBooking(context.Context, *bookingv1.HasCompletedBookingRequest, ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) ListBookingStaffNotes(context.Context, *bookingv1.ListBookingStaffNotesRequest, ...grpc.CallOption) (*bookingv1.ListBookingStaffNotesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) AddBookingStaffNote(context.Context, *bookingv1.AddBookingStaffNoteRequest, ...grpc.CallOption) (*bookingv1.AddBookingStaffNoteResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopBookingClient) GetBookingsBatch(context.Context, *bookingv1.GetBookingsBatchRequest, ...grpc.CallOption) (*bookingv1.GetBookingsBatchResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}

type noopVenueClient struct{}

func (noopVenueClient) CreateVenue(context.Context, *venuev1.CreateVenueRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) SubmitVenueForReview(context.Context, *venuev1.SubmitVenueForReviewRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) UpdateVenue(context.Context, *venuev1.UpdateVenueRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) GetVenue(context.Context, *venuev1.GetVenueRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.NotFound, "no venue")
}
func (noopVenueClient) GetVenuesBatch(context.Context, *venuev1.GetVenuesBatchRequest, ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) GetVenueBySlug(context.Context, *venuev1.GetVenueBySlugRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) ListVenues(context.Context, *venuev1.ListVenuesRequest, ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) SearchVenues(context.Context, *venuev1.SearchVenuesRequest, ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}

// ListOwnerVenues removed from venue.proto (gateway composes from CRM + venue).

func (noopVenueClient) GetPopularCities(context.Context, *venuev1.GetPopularCitiesRequest, ...grpc.CallOption) (*venuev1.GetPopularCitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) CheckSlotAvailability(context.Context, *venuev1.CheckSlotRequest, ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) BatchCheckSlotAvailability(context.Context, *venuev1.BatchCheckSlotRequest, ...grpc.CallOption) (*venuev1.BatchCheckSlotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) ReserveSlot(context.Context, *venuev1.ReserveSlotRequest, ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) ReleaseSlot(context.Context, *venuev1.ReleaseSlotRequest, ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) CreateManualSlotBlock(context.Context, *venuev1.CreateManualSlotBlockRequest, ...grpc.CallOption) (*venuev1.CreateManualSlotBlockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) DeleteManualSlotBlock(context.Context, *venuev1.DeleteManualSlotBlockRequest, ...grpc.CallOption) (*venuev1.DeleteManualSlotBlockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) ListManualSlotBlocks(context.Context, *venuev1.ListManualSlotBlocksRequest, ...grpc.CallOption) (*venuev1.ListManualSlotBlocksResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) GetVenueSchedule(context.Context, *venuev1.GetVenueScheduleRequest, ...grpc.CallOption) (*venuev1.GetVenueScheduleResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) GetVenueBookingMode(context.Context, *venuev1.GetVenueBookingModeRequest, ...grpc.CallOption) (*venuev1.GetVenueBookingModeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) SetVenueBookingMode(context.Context, *venuev1.SetVenueBookingModeRequest, ...grpc.CallOption) (*venuev1.SetVenueBookingModeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) UpdateRating(context.Context, *venuev1.UpdateRatingRequest, ...grpc.CallOption) (*venuev1.UpdateRatingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) ModerateVenue(context.Context, *venuev1.ModerateVenueRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) ListPendingVenues(context.Context, *venuev1.ListPendingVenuesRequest, ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) AddVenuePhoto(context.Context, *venuev1.AddVenuePhotoRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) DeleteVenuePhoto(context.Context, *venuev1.DeleteVenuePhotoRequest, ...grpc.CallOption) (*venuev1.DeleteVenuePhotoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) SetVenueCoverPhoto(context.Context, *venuev1.SetVenueCoverPhotoRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) AddVenueHallPhoto(context.Context, *venuev1.AddVenueHallPhotoRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) DeleteVenueHallPhoto(context.Context, *venuev1.DeleteVenueHallPhotoRequest, ...grpc.CallOption) (*venuev1.DeleteVenueHallPhotoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) SetVenueHallCoverPhoto(context.Context, *venuev1.SetVenueHallCoverPhotoRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopVenueClient) SuspendVenuesByOwner(context.Context, *venuev1.SuspendVenuesByOwnerRequest, ...grpc.CallOption) (*venuev1.SuspendVenuesByOwnerResponse, error) {
	return &venuev1.SuspendVenuesByOwnerResponse{}, nil
}

// CRM methods removed from venue.proto (now in proto/crm/v1/crm.proto).

type noopUserClient struct{}

func (noopUserClient) CreateUser(context.Context, *userv1.CreateUserRequest, ...grpc.CallOption) (*userv1.UserResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopUserClient) GetUser(context.Context, *userv1.GetUserRequest, ...grpc.CallOption) (*userv1.UserResponse, error) {
	return nil, status.Error(codes.NotFound, "no user")
}
func (noopUserClient) GetUsersBatch(context.Context, *userv1.GetUsersBatchRequest, ...grpc.CallOption) (*userv1.GetUsersBatchResponse, error) {
	return &userv1.GetUsersBatchResponse{}, nil
}
func (noopUserClient) UpdateUser(context.Context, *userv1.UpdateUserRequest, ...grpc.CallOption) (*userv1.UserResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopUserClient) GetUserByEmail(context.Context, *userv1.GetUserByEmailRequest, ...grpc.CallOption) (*userv1.UserResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not set up")
}
func (noopUserClient) DeleteUser(context.Context, *userv1.DeleteUserRequest, ...grpc.CallOption) (*userv1.DeleteUserResponse, error) {
	return &userv1.DeleteUserResponse{}, nil
}

// ── configurable booking mock ─────────────────────────────────────────────────

// bookingIntegMock embeds noopBookingClient and lets individual tests override
// only the methods they need.
type bookingIntegMock struct {
	noopBookingClient
	createFn     func(ctx context.Context, req *bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error)
	listUserFn   func(ctx context.Context, req *bookingv1.ListUserBookingsRequest) (*bookingv1.ListBookingsResponse, error)
	getBookingFn func(ctx context.Context, req *bookingv1.GetBookingRequest) (*bookingv1.BookingResponse, error)
}

func (m *bookingIntegMock) CreateBooking(ctx context.Context, req *bookingv1.CreateBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return m.noopBookingClient.CreateBooking(ctx, req)
}
func (m *bookingIntegMock) GetBooking(ctx context.Context, req *bookingv1.GetBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.getBookingFn != nil {
		return m.getBookingFn(ctx, req)
	}
	return m.noopBookingClient.GetBooking(ctx, req)
}
func (m *bookingIntegMock) ListUserBookings(ctx context.Context, req *bookingv1.ListUserBookingsRequest, _ ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	if m.listUserFn != nil {
		return m.listUserFn(ctx, req)
	}
	return m.noopBookingClient.ListUserBookings(ctx, req)
}

// ── router builder ────────────────────────────────────────────────────────────

// newBookingRouter returns a chi.Mux that mirrors the production route
// structure for booking endpoints, with a real Auth middleware.
func newBookingRouter(mock *bookingIntegMock) *chi.Mux {
	h := NewBookingHandler(mock, noopVenueClient{}, noopUserClient{})
	r := chi.NewRouter()
	r.Use(middleware.Auth(zerolog.Nop(), integTestPubKey, nil))
	r.Post("/api/v1/bookings", h.Create)
	r.Get("/api/v1/bookings", h.ListMy)
	r.Get("/api/v1/bookings/{id}", h.Get)
	return r
}

// jwtFor issues a valid ES256 JWT for the given user with a 1-hour TTL.
func jwtFor(t *testing.T, userID, role string) string {
	t.Helper()
	tok, err := pkgauth.GenerateAccessToken(integTestPrivKey, userID, userID+"@test.com", role, time.Hour)
	require.NoError(t, err)
	return tok
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestBooking_AuthChain_NoToken verifies that every booking endpoint rejects
// unauthenticated requests.  The 401 must come from the Auth middleware, not
// from the handler's own auth check, so no mock expectations are needed.
func TestBooking_AuthChain_NoToken(t *testing.T) {
	r := newBookingRouter(&bookingIntegMock{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/bookings"},
		{http.MethodGet, "/api/v1/bookings"},
		{http.MethodGet, "/api/v1/bookings/booking-123"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

// TestBooking_AuthChain_ExpiredToken verifies that an expired JWT is rejected
// by the middleware before the handler runs.
func TestBooking_AuthChain_ExpiredToken(t *testing.T) {
	expiredToken, err := pkgauth.GenerateAccessToken(
		integTestPrivKey, "user-1", "user-1@test.com", string(roles.RoleUser),
		-time.Second,
	)
	require.NoError(t, err)

	r := newBookingRouter(&bookingIntegMock{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestBooking_Create_RequireRole verifies that a valid JWT with the wrong role
// is rejected with 403 when the route enforces RequireRole.  The test wires
// RequireRole(roles.RoleUser) on the POST route, mirroring production.
//
// Note: the base newBookingRouter intentionally omits RequireRole so that
// shared helper routes stay simple. This test builds its own router variant.
// In production the booking create route allows RoleUser (end-customers).
func TestBooking_Create_RequireRole(t *testing.T) {
	guardFn := func(_ context.Context, _ *bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error) {
		t.Fatal("CreateBooking must not be called when role is rejected")
		return nil, nil
	}
	mock := &bookingIntegMock{createFn: guardFn}

	h := NewBookingHandler(mock, noopVenueClient{}, noopUserClient{})
	r := chi.NewRouter()
	r.Use(middleware.Auth(zerolog.Nop(), integTestPubKey, nil))
	r.With(middleware.RequireRole(roles.RoleUser)).Post("/api/v1/bookings", h.Create)

	cases := []struct {
		name       string
		role       roles.Role
		wantStatus int
	}{
		{"user role is allowed", roles.RoleUser, http.StatusBadRequest}, // 400 from missing fields, not 403
		{"venue_owner role is forbidden", roles.RoleVenueOwner, http.StatusForbidden},
		{"admin role is forbidden", roles.RoleAdmin, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"venue_id": "v1"}) // intentionally incomplete
			req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+jwtFor(t, "user-1", string(tc.role)))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.Equal(t, tc.wantStatus, rec.Code, "role=%s body=%s", tc.role, rec.Body.String())
		})
	}
}

// TestBooking_Create_RoundTrip is the primary end-to-end test:
//
//	JWT in Authorization header
//	→ Auth middleware decodes claims, puts userID into context
//	→ chi routes POST /api/v1/bookings to BookingHandler.Create
//	→ handler reads userID from context and calls mock CreateBooking
//	→ 201 response with the booking JSON
//
// The key assertion is that req.GetUserId() == userID — proving the middleware
// correctly plumbed the JWT claim into the context that the handler reads.
func TestBooking_Create_RoundTrip(t *testing.T) {
	const userID = "user-abc"

	mock := &bookingIntegMock{
		createFn: func(_ context.Context, req *bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error) {
			if req.GetUserId() != userID {
				return nil, fmt.Errorf("unexpected user_id: got %q, want %q", req.GetUserId(), userID)
			}
			return &bookingv1.BookingResponse{
				Id:        "booking-xyz",
				UserId:    req.GetUserId(),
				VenueId:   req.GetVenueId(),
				TimeFrom:  req.GetTimeFrom(),
				TimeTo:    req.GetTimeTo(),
				Status:    "pending",
				CreatedAt: timestamppb.Now(),
			}, nil
		},
	}

	body, _ := json.Marshal(map[string]any{
		"venue_id":  "venue-1",
		"date":      "2026-06-01",
		"time_from": "14:00",
		"time_to":   "16:00",
		"guests":    2,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtFor(t, userID, string(roles.RoleUser)))

	rec := httptest.NewRecorder()
	newBookingRouter(mock).ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "booking-xyz", resp["id"])
	assert.Equal(t, userID, resp["user_id"])
}

// TestBooking_Create_MissingTimeFrom verifies that the handler rejects a
// request where both time_from and time are absent.  The JWT must be valid;
// the 400 comes from inside the handler, not the middleware.
func TestBooking_Create_MissingTimeFrom(t *testing.T) {
	// CreateBooking must never be called: validation should reject the request
	// before reaching the gRPC layer. The guard below catches regressions where
	// validation is accidentally bypassed.
	mock := &bookingIntegMock{
		createFn: func(_ context.Context, _ *bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error) {
			t.Fatal("CreateBooking must not be called when time_from is missing")
			return nil, nil
		},
	}

	body, _ := json.Marshal(map[string]any{"venue_id": "v1", "date": "2026-06-01"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtFor(t, "user-1", string(roles.RoleUser)))

	rec := httptest.NewRecorder()
	newBookingRouter(mock).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestBooking_ListMy_QueryParams verifies query-parameter parsing through the
// real HTTP path: valid values are forwarded to gRPC; invalid values return 400.
// This catches regressions in queryInt that would be invisible without routing.
func TestBooking_ListMy_QueryParams(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantStatus int
		wantPage   int32
		wantSize   int32
	}{
		{
			name:       "defaults when absent",
			query:      "",
			wantStatus: http.StatusOK,
			wantPage:   0,
			wantSize:   0,
		},
		{
			name:       "explicit page and size forwarded to gRPC",
			query:      "?page=2&page_size=10",
			wantStatus: http.StatusOK,
			wantPage:   2,
			wantSize:   10,
		},
		{
			name:       "non-integer page returns 400",
			query:      "?page=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-integer page_size returns 400",
			query:      "?page_size=xyz",
			wantStatus: http.StatusBadRequest,
		},
		{
			// Exactly at the hard-coded max (booking.go: queryInt "page_size", 0, 0, 200).
			// If someone bumps the limit without updating this test, the boundary cases below will catch it.
			name:       "page_size at max (200) is accepted",
			query:      "?page_size=200",
			wantStatus: http.StatusOK,
			wantPage:   0,
			wantSize:   200,
		},
		{
			name:       "page_size at max+1 (201) returns 400",
			query:      "?page_size=201",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "page_size well over max (9999) returns 400",
			query:      "?page_size=9999",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPage, gotSize int32
			mock := &bookingIntegMock{
				listUserFn: func(_ context.Context, req *bookingv1.ListUserBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
					gotPage = req.GetPage()
					gotSize = req.GetPageSize()
					return &bookingv1.ListBookingsResponse{}, nil
				},
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings"+tc.query, http.NoBody)
			req.Header.Set("Authorization", "Bearer "+jwtFor(t, "user-1", string(roles.RoleUser)))
			rec := httptest.NewRecorder()
			newBookingRouter(mock).ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, "query=%q body=%s", tc.query, rec.Body.String())
			if tc.wantStatus == http.StatusOK {
				assert.Equal(t, tc.wantPage, gotPage, "page forwarded to gRPC")
				assert.Equal(t, tc.wantSize, gotSize, "page_size forwarded to gRPC")
			}
		})
	}
}

// TestBooking_Get_URLParam is the critical chi-routing test: chi.URLParam("id")
// only returns the correct value when the request goes through a real chi router
// with a registered "{id}" route pattern.  A handler called directly (without
// the router) would always receive an empty string, silently querying gRPC with
// id="" — a bug that this test catches.
func TestBooking_Get_URLParam(t *testing.T) {
	const bookingID = "booking-999"
	var receivedID string

	mock := &bookingIntegMock{
		getBookingFn: func(_ context.Context, req *bookingv1.GetBookingRequest) (*bookingv1.BookingResponse, error) {
			receivedID = req.GetId()
			return &bookingv1.BookingResponse{
				Id:        bookingID,
				UserId:    "user-1",
				CreatedAt: timestamppb.Now(),
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/"+bookingID, http.NoBody)
	req.Header.Set("Authorization", "Bearer "+jwtFor(t, "user-1", string(roles.RoleUser)))
	rec := httptest.NewRecorder()
	newBookingRouter(mock).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, bookingID, receivedID,
		"booking ID must be extracted from URL by chi router, not be empty")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, bookingID, resp["id"])
}

// TestBooking_Get_NotFound verifies the full gRPC→HTTP error mapping chain:
// a codes.NotFound from the mock becomes a 404 HTTP response.
func TestBooking_Get_NotFound(t *testing.T) {
	mock := &bookingIntegMock{
		getBookingFn: func(_ context.Context, _ *bookingv1.GetBookingRequest) (*bookingv1.BookingResponse, error) {
			return nil, status.Error(codes.NotFound, "booking not found")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookings/no-such-id", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+jwtFor(t, "user-1", string(roles.RoleUser)))
	rec := httptest.NewRecorder()
	newBookingRouter(mock).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
