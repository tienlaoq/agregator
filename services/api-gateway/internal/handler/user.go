package handler

import (
	"net/http"
	"strings"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

type UserHandler struct {
	client userv1.UserServiceClient
	auth   authv1.AuthServiceClient
	venue  venuev1.VenueServiceClient
	master masterv1.MasterServiceClient
}

func NewUserHandler(
	client userv1.UserServiceClient,
	auth authv1.AuthServiceClient,
	venue venuev1.VenueServiceClient,
	master masterv1.MasterServiceClient,
) *UserHandler {
	return &UserHandler{client: client, auth: auth, venue: venue, master: master}
}

func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	resp, err := h.client.GetUser(r.Context(), &userv1.GetUserRequest{Id: userID})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

// DeleteMe permanently deactivates the authenticated user's own account
// (soft delete + anonymise) and cascades to their business data. Flow:
//  1. Confirm the caller typed their exact account email (guards against
//     accidental/CSRF-style deletion; the token alone is not enough).
//  2. Block admin self-deletion — admins must be removed via admin tooling,
//     never self-serve, so an attacker with a hijacked admin session can't
//     destroy the account.
//  3. Cascade-suspend owned venues / master profile so they leave public
//     listings.
//  4. Revoke all auth material (credential + refresh tokens) — kills every
//     session immediately.
//  5. Soft-delete + anonymise the user row.
//
// Every downstream call is idempotent, so a client retry after a partial
// failure converges. The refresh cookie is cleared by the frontend on logout.
func (h *UserHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Confirmation) == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAccountConfirmationRequired)
		return
	}

	// Fetch the authoritative account record: email for confirmation, role for
	// the cascade decision (more trustworthy than the JWT claim).
	user, err := h.client.GetUser(r.Context(), &userv1.GetUserRequest{Id: userID})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	if user.GetRole() == "admin" {
		httpx.WriteCatalog(w, apicatalog.GatewayAccountAdminSelfDeleteForbidden)
		return
	}

	if !strings.EqualFold(strings.TrimSpace(req.Confirmation), strings.TrimSpace(user.GetEmail())) {
		httpx.WriteCatalog(w, apicatalog.GatewayAccountConfirmationMismatch)
		return
	}

	// 1. Cascade-suspend business data tied to this identity.
	switch user.GetRole() {
	case "venue_owner":
		if _, err := h.venue.SuspendVenuesByOwner(r.Context(), &venuev1.SuspendVenuesByOwnerRequest{OwnerId: userID}); err != nil {
			httpx.GRPCErrorToHTTP(w, err)
			return
		}
	case "master":
		if _, err := h.master.SuspendMasterByUser(r.Context(), &masterv1.SuspendMasterByUserRequest{UserId: userID}); err != nil {
			httpx.GRPCErrorToHTTP(w, err)
			return
		}
	}

	// 2. Revoke authentication material (credential + refresh tokens).
	if _, err := h.auth.DeleteAccount(r.Context(), &authv1.DeleteAccountRequest{UserId: userID}); err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	// 3. Soft-delete + anonymise the user record.
	if _, err := h.client.DeleteUser(r.Context(), &userv1.DeleteUserRequest{Id: userID}); err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	var req struct {
		Name      *string `json:"name"`
		Phone     *string `json:"phone"`
		AvatarURL *string `json:"avatar_url"`
		Bio       *string `json:"bio"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

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
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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
