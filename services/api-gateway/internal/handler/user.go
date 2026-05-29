package handler

import (
	"net/http"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

type UserHandler struct {
	client userv1.UserServiceClient
}

func NewUserHandler(client userv1.UserServiceClient) *UserHandler {
	return &UserHandler{client: client}
}

func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	resp, err := h.client.GetUser(r.Context(), &userv1.GetUserRequest{Id: userID})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         resp.GetId(),
		"email":      resp.GetEmail(),
		"phone":      resp.GetPhone(),
		"name":       resp.GetName(),
		"role":       resp.GetRole(),
		"avatar_url": resp.GetAvatarUrl(),
		"bio":        resp.GetBio(),
		"created_at": resp.GetCreatedAt().AsTime(),
	})
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		Name      *string `json:"name"`
		Phone     *string `json:"phone"`
		AvatarURL *string `json:"avatar_url"`
		Bio       *string `json:"bio"`
	}
	if !readJSONOrRespond(w, r, &req) { return }

	grpcReq := &userv1.UpdateUserRequest{Id: userID}
	if req.Name != nil {
		grpcReq.Name = req.Name
	}
	if req.Phone != nil {
		grpcReq.Phone = req.Phone
	}
	if req.AvatarURL != nil {
		grpcReq.AvatarUrl = req.AvatarURL
	}
	if req.Bio != nil {
		grpcReq.Bio = req.Bio
	}

	resp, err := h.client.UpdateUser(r.Context(), grpcReq)
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         resp.GetId(),
		"email":      resp.GetEmail(),
		"phone":      resp.GetPhone(),
		"name":       resp.GetName(),
		"role":       resp.GetRole(),
		"avatar_url": resp.GetAvatarUrl(),
		"bio":        resp.GetBio(),
		"created_at": resp.GetCreatedAt().AsTime(),
	})
}
