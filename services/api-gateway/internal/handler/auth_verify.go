package handler

import (
	"net/http"
	"strings"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// resendVerificationSuccessMessageRU mirrors the anti-enumeration discipline of
// forgot-password: the response is identical whether or not the address exists
// or is already verified, so an attacker cannot probe account state.
const resendVerificationSuccessMessageRU = "Если для адреса требуется подтверждение, мы повторно отправили ссылку."

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

// VerifyEmail consumes the one-time token from the link in the verification
// email and marks the owning account's email as verified. Public: the link is
// opened in a browser that may not carry an access token.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}

	_, err := h.client.VerifyEmail(r.Context(), &authv1.VerifyEmailRequest{
		Token: strings.TrimSpace(req.Token),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "email_verified"})
}

// ResendVerification re-issues a verification link. Anti-enumeration: always
// returns 200 with a generic message regardless of whether the address exists
// or is already verified — auth-service silently no-ops those cases.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendVerificationRequest
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestEmailRequired)
		return
	}

	_, err := h.client.ResendVerification(r.Context(), &authv1.ResendVerificationRequest{
		Email: strings.TrimSpace(req.Email),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": resendVerificationSuccessMessageRU})
}

// GetEmailVerificationStatus reports whether the authenticated user's email is
// verified. Protected: the frontend polls this to advance the partner flow
// (e.g. show "check your inbox" until the user clicks the link).
func (h *AuthHandler) GetEmailVerificationStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	resp, err := h.client.GetEmailVerification(r.Context(), &authv1.GetEmailVerificationRequest{
		UserId: userID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"email_verified": resp.GetEmailVerified()})
}
