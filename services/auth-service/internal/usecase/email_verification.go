package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
)

// EmailVerificationMailer sends the verification link when SMTP is configured.
type EmailVerificationMailer interface {
	Enabled() bool
	SendVerification(ctx context.Context, toEmail, rawToken string) error
}

// sendVerificationEmail issues a fresh single-use token and emails the link.
// Best-effort by contract: callers (Register, ResendVerification) decide how to
// treat a returned error. A nil verifyTokens repository or disabled mailer is a
// configuration state, not a request error, so those return nil after a Warn.
func (uc *AuthUseCase) sendVerificationEmail(ctx context.Context, userID, email string) error {
	email = normalizeAccountEmail(email)
	if email == "" {
		return nil
	}
	if uc.verifyTokens == nil {
		uc.appLog.Warn().Msg("email verification: token repository not configured")
		return nil
	}
	if uc.verifyMail == nil || !uc.verifyMail.Enabled() {
		uc.appLog.Warn().Msg("email verification: SMTP not configured")
		return nil
	}

	// Only the latest link stays valid: drop any outstanding unused tokens first.
	if err := uc.verifyTokens.InvalidateUnusedByUserID(ctx, userID); err != nil {
		return fmt.Errorf("invalidate verification tokens: %w", err)
	}

	rawToken, err := generateRefreshToken()
	if err != nil {
		return pkgerr.Internal("failed to generate verification token")
	}
	tokenHash := hashRefreshToken(rawToken)
	expiresAt := time.Now().Add(uc.verifyTTL)
	if err := uc.verifyTokens.Create(ctx, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("store verification token: %w", err)
	}

	if err := uc.verifyMail.SendVerification(ctx, email, rawToken); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

// VerifyEmail consumes a one-time verification token and marks the owning
// credential as email-verified. The token is single-use: a second call with the
// same token returns InvalidArgument.
func (uc *AuthUseCase) VerifyEmail(ctx context.Context, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return pkgerr.InvalidArgument("token is required")
	}
	if uc.verifyTokens == nil {
		return pkgerr.Internal("email verification not configured")
	}

	hash := hashRefreshToken(rawToken)
	userID, err := uc.verifyTokens.ConsumeByTokenHash(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return pkgerr.InvalidArgument("Ссылка подтверждения недействительна или истекла")
		}
		return err
	}

	if err := uc.creds.SetEmailVerified(ctx, userID, true); err != nil {
		return fmt.Errorf("set email verified: %w", err)
	}
	return nil
}

// ResendVerification re-issues a verification link for an account that is not yet
// verified. It is anti-enumeration: the caller cannot tell from the result
// whether the email exists, whether it is already verified, or whether mail is
// configured — every non-error path returns nil. Mirrors the log discipline of
// RequestPasswordReset: exactly one audit line per request, carrying only a
// non-reversible email fingerprint, so log access cannot be used to enumerate.
func (uc *AuthUseCase) ResendVerification(ctx context.Context, email string) error {
	email = normalizeAccountEmail(email)
	if email == "" {
		return nil
	}

	logProcessed := uc.auditOnce(email, "email verification resend: processed")

	audit := false
	defer func() {
		if audit {
			logProcessed()
		}
	}()

	cred, err := uc.creds.GetByEmail(ctx, email)
	if err != nil {
		if isNotFound(err) {
			audit = true
			return nil
		}
		return err
	}

	// Already verified — nothing to do, but stay silent so the response is
	// indistinguishable from the not-found and freshly-sent cases.
	if cred.EmailVerified {
		audit = true
		return nil
	}

	if err := uc.sendVerificationEmail(ctx, cred.UserID, cred.Email); err != nil {
		// Delivery/storage failure: log at Warn without account fields (same
		// rationale as RequestPasswordReset) so operators can act without the log
		// becoming an enumeration oracle.
		uc.appLog.Warn().Err(err).Msg("email verification resend: send failed")
	}

	audit = true
	return nil
}

// GetEmailVerified reports whether the account's email has been verified.
// Used by the gateway gate before allowing venue/master profile creation.
func (uc *AuthUseCase) GetEmailVerified(ctx context.Context, userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, pkgerr.InvalidArgument("user_id is required")
	}
	cred, err := uc.creds.GetByUserID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return false, pkgerr.NotFound("credential not found")
		}
		return false, err
	}
	return cred.EmailVerified, nil
}
