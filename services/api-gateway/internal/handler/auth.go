package handler

import (
	"net/http"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
)

type AuthHandler struct {
	client authv1.AuthServiceClient
}

func NewAuthHandler(client authv1.AuthServiceClient) *AuthHandler {
	return &AuthHandler{client: client}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Role     string `json:"role"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	resp, err := h.client.Register(r.Context(), &authv1.RegisterRequest{
		Email:    req.Email,
		Phone:    req.Phone,
		Password: req.Password,
		Name:     req.Name,
		Role:     req.Role,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"user_id":       resp.GetUserId(),
		"access_token":  resp.GetAccessToken(),
		"refresh_token": resp.GetRefreshToken(),
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	resp, err := h.client.Login(r.Context(), &authv1.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":       resp.GetUserId(),
		"access_token":  resp.GetAccessToken(),
		"refresh_token": resp.GetRefreshToken(),
	})
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	resp, err := h.client.RefreshToken(r.Context(), &authv1.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  resp.GetAccessToken(),
		"refresh_token": resp.GetRefreshToken(),
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	_, err := h.client.Logout(r.Context(), &authv1.LogoutRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}
