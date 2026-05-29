package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// noopUploader satisfies storage.Uploader without touching the filesystem.
type noopUploader struct{}

func (noopUploader) Put(_ context.Context, _, _ string, _ int64, _ io.Reader) (string, error) {
	return "", nil
}
func (noopUploader) Delete(_ context.Context, _ string) error { return nil }

type mockMasterClient struct {
	onListClientMasterBookings func(ctx context.Context, in *masterv1.ListClientMasterBookingsRequest, opts ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error)
}

func (m *mockMasterClient) ListClientMasterBookings(ctx context.Context, in *masterv1.ListClientMasterBookingsRequest, opts ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error) {
	if m.onListClientMasterBookings != nil {
		return m.onListClientMasterBookings(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "ListClientMasterBookings")
}

func (m *mockMasterClient) CreateMyProfile(context.Context, *masterv1.CreateMyProfileRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateMyProfile")
}
func (m *mockMasterClient) GetMyProfile(context.Context, *masterv1.GetMyProfileRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetMyProfile")
}
func (m *mockMasterClient) UpdateMyProfile(context.Context, *masterv1.UpdateMyProfileRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UpdateMyProfile")
}
func (m *mockMasterClient) SubmitForReview(context.Context, *masterv1.SubmitMasterForReviewRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "SubmitForReview")
}
func (m *mockMasterClient) ListPublicMasters(context.Context, *masterv1.ListPublicMastersRequest, ...grpc.CallOption) (*masterv1.ListMastersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListPublicMasters")
}
func (m *mockMasterClient) GetPublicMaster(context.Context, *masterv1.GetPublicMasterRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetPublicMaster")
}
func (m *mockMasterClient) ListForModeration(context.Context, *masterv1.ListForModerationRequest, ...grpc.CallOption) (*masterv1.ListMastersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListForModeration")
}
func (m *mockMasterClient) ModerateMaster(context.Context, *masterv1.ModerateMasterRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ModerateMaster")
}
func (m *mockMasterClient) ListModerationHistory(context.Context, *masterv1.ListModerationHistoryRequest, ...grpc.CallOption) (*masterv1.ListModerationHistoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListModerationHistory")
}
func (m *mockMasterClient) CreateMasterBooking(context.Context, *masterv1.CreateMasterBookingRequest, ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateMasterBooking")
}
func (m *mockMasterClient) ListMyMasterBookings(context.Context, *masterv1.ListMyMasterBookingsRequest, ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListMyMasterBookings")
}
func (m *mockMasterClient) GetMasterBooking(context.Context, *masterv1.GetMasterBookingRequest, ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetMasterBooking")
}
func (m *mockMasterClient) HasCompletedMasterBooking(context.Context, *masterv1.HasCompletedMasterBookingRequest, ...grpc.CallOption) (*masterv1.HasCompletedMasterBookingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "HasCompletedMasterBooking")
}
func (m *mockMasterClient) AddMasterPhoto(context.Context, *masterv1.AddMasterPhotoRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "AddMasterPhoto")
}
func (m *mockMasterClient) DeleteMasterPhoto(context.Context, *masterv1.DeleteMasterPhotoRequest, ...grpc.CallOption) (*masterv1.DeleteMasterPhotoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteMasterPhoto")
}
func (m *mockMasterClient) SetMasterCoverPhoto(context.Context, *masterv1.SetMasterCoverPhotoRequest, ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "SetMasterCoverPhoto")
}

func (m *mockMasterClient) GetMasterBookingsBatch(_ context.Context, in *masterv1.GetMasterBookingsBatchRequest, _ ...grpc.CallOption) (*masterv1.GetMasterBookingsBatchResponse, error) {
	return &masterv1.GetMasterBookingsBatchResponse{Bookings: make(map[string]*masterv1.MasterBooking)}, nil
}

func TestMasterHandler_ListMyClientBookings_Success(t *testing.T) {
	mock := &mockMasterClient{
		onListClientMasterBookings: func(ctx context.Context, in *masterv1.ListClientMasterBookingsRequest, opts ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error) {
			if in.GetUserId() != "user-1" {
				t.Fatalf("unexpected user id: %s", in.GetUserId())
			}
			if in.GetStatusFilter() != "confirmed" {
				t.Fatalf("unexpected status filter: %s", in.GetStatusFilter())
			}
			return &masterv1.ListMasterBookingsResponse{
				Bookings: []*masterv1.MasterBooking{
					{
						Id:           "booking-1",
						MasterId:     "master-1",
						ClientUserId: "user-1",
						Date:         "2026-05-06",
						TimeFrom:     "10:00",
						TimeTo:       "11:00",
						Status:       "confirmed",
						CreatedAt:    timestamppb.New(time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)),
					},
				},
			}, nil
		},
	}
	h := NewMasterHandler(mock, noopUploader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/my/master-bookings?status=confirmed", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user-1"))
	rr := httptest.NewRecorder()

	h.ListMyClientBookings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rr.Code)
	}
	var body struct {
		Bookings []map[string]any `json:"bookings"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Bookings) != 1 {
		t.Fatalf("unexpected bookings len: %d", len(body.Bookings))
	}
	if body.Bookings[0]["id"] != "booking-1" {
		t.Fatalf("unexpected booking id: %v", body.Bookings[0]["id"])
	}
}

func TestMasterHandler_ListMyClientBookings_Unauthorized(t *testing.T) {
	h := NewMasterHandler(&mockMasterClient{}, noopUploader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/my/master-bookings", nil)
	rr := httptest.NewRecorder()

	h.ListMyClientBookings(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status code: %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != apicatalog.GatewayAuthUnauthorized.Code {
		t.Fatalf("unexpected code: %s", body["code"])
	}
}
