package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
)

const forgotPasswordSuccessMessageRU = "Если указанный адрес зарегистрирован и для него задан пароль, мы отправили на него ссылку для сброса."

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type completePasswordResetRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestEmailRequired)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()
	_, err := h.client.RequestPasswordReset(ctx, &authv1.RequestPasswordResetRequest{
		Email: strings.TrimSpace(req.Email),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": forgotPasswordSuccessMessageRU})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req completePasswordResetRequest
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Token) == "" || strings.TrimSpace(req.NewPassword) == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}

	_, err := h.client.CompletePasswordReset(r.Context(), &authv1.CompletePasswordResetRequest{
		Token:       strings.TrimSpace(req.Token),
		NewPassword: strings.TrimSpace(req.NewPassword),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "password_updated"})
}
