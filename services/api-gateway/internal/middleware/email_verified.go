package middleware

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
)

// EmailVerifier is the subset of the auth gRPC client the gate needs.
// Defined at the usage site so tests can supply a stub without the full client.
type EmailVerifier interface {
	GetEmailVerification(ctx context.Context, in *authv1.GetEmailVerificationRequest, opts ...grpc.CallOption) (*authv1.GetEmailVerificationResponse, error)
}

// RequireEmailVerified blocks the request unless the authenticated user's email
// is verified. It is mounted on partner write actions that produce moderation
// load (venue / master profile creation) so a user cannot spam the moderation
// queue with throwaway, unverified email addresses.
//
// The flag is read live from auth-service rather than from the JWT, because the
// JWT is minted at login/refresh and would carry a stale "unverified" value for
// up to the refresh-token lifetime after the user clicks the verification link.
// This adds one gRPC call, but only on these rare write actions — never on the
// hot read path — so the stateless local-JWT design for normal requests is
// preserved.
//
// Failure modes:
//   - no user id in context (should be impossible behind Auth) → 401
//   - auth-service RPC error → 503 (fail closed; the client retries)
//   - email not verified → 403 EMAIL_NOT_VERIFIED (frontend shows resend CTA)
func RequireEmailVerified(log zerolog.Logger, client EmailVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := UserIDFromCtx(r.Context())
			if userID == "" {
				apicatalog.GatewayAuthUnauthorized.Write(w)
				return
			}

			resp, err := client.GetEmailVerification(r.Context(), &authv1.GetEmailVerificationRequest{
				UserId: userID,
			})
			if err != nil {
				log.Warn().Err(err).Str("user_id", userID).
					Msg("email gate: GetEmailVerification failed — blocking request")
				apicatalog.GatewayDependencyAuthServiceUnavailable.Write(w)
				return
			}

			if !resp.GetEmailVerified() {
				apicatalog.GatewayAccountEmailNotVerified.Write(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
