package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

const minAccountPasswordLen = 8

// PasswordResetMailer sends the reset link when SMTP is configured.
type PasswordResetMailer interface {
	Enabled() bool
	SendPasswordReset(ctx context.Context, toEmail, rawToken string) error
}

func normalizeAccountEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func accountCredentialHasPassword(cred *domain.Credential) bool {
	return cred != nil && strings.TrimSpace(cred.PasswordHash) != ""
}

// emailFingerprint is a short stable hash for logs (not reversible to the address).
func emailFingerprint(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])[:8]
}

func passwordResetVerboseLogs() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PASSWORD_RESET_VERBOSE")))
	return v == "1" || v == "true" || v == "yes"
}

// RequestPasswordReset completes without distinguishing cases in the public API
// response OR in structured logs visible to anyone with log access.
//
// Log discipline — every exit path emits exactly one audit line with the same
// message text ("password reset: processed") and the same field set {email_fp}.
// No field that varies with the outcome (to_domain, status, "skipped", "sent",
// …) may appear on that line, because aggregated log queries on such fields
// would allow email enumeration just as reliably as a timing attack on the API.
//
// Implementation: a single deferred closure emits the audit line on every
// return path, including early returns. Each branch that returns early marks
// audit = true only when a log line is warranted; error returns skip it (the
// caller's error will surface via the gRPC layer). This avoids duplicating the
// log call in each branch while keeping the "exactly one line per request"
// invariant visible in one place.
//
// Operational detail (SMTP errors, misconfiguration) is emitted at Warn/Error
// level WITHOUT any account-identifying fields, so operators can act on
// delivery failures without the log stream becoming an enumeration oracle.
//
// Use PASSWORD_RESET_VERBOSE=1 to promote the audit line from Debug to Info
// (useful in staging; never enable in production).
func (uc *AuthUseCase) RequestPasswordReset(ctx context.Context, email string) error {
	email = normalizeAccountEmail(email)
	if email == "" {
		return nil
	}

	fp := emailFingerprint(email)
	verbose := passwordResetVerboseLogs()

	// audit gates the deferred log line. It is set to true on every non-error
	// return path so that exactly one "processed" line appears in the log
	// regardless of which branch was taken. Error returns leave audit=false so
	// we don't emit a misleading "processed" line when the operation failed.
	audit := false
	defer func() {
		if !audit {
			return
		}
		if verbose {
			uc.appLog.Info().Str("email_fp", fp).Msg("password reset: processed")
		} else {
			uc.appLog.Debug().Str("email_fp", fp).Msg("password reset: processed")
		}
	}()

	// Resolve the credential. All non-error outcomes (not found, no password
	// hash, SMTP disabled) are deliberately silent at Info level so that the
	// presence or absence of a log line cannot be used to infer account existence.
	cred, err := uc.creds.GetByEmail(ctx, email)
	if err != nil {
		if isNotFound(err) {
			audit = true
			return nil
		}
		return err
	}
	if !accountCredentialHasPassword(cred) {
		audit = true
		return nil
	}

	if uc.resetMail == nil || !uc.resetMail.Enabled() {
		// Misconfiguration — log at Warn without account fields so this is
		// actionable for operators but not useful for enumeration.
		uc.appLog.Warn().Msg("password reset: SMTP not configured")
		audit = true
		return nil
	}
	if uc.resetTokens == nil {
		uc.appLog.Warn().Msg("password reset: token repository not configured")
		audit = true
		return nil
	}

	if err := uc.resetTokens.InvalidateUnusedByUserID(ctx, cred.UserID); err != nil {
		return fmt.Errorf("invalidate reset tokens: %w", err)
	}

	rawToken, err := generateRefreshToken()
	if err != nil {
		return pkgerr.Internal("failed to generate reset token")
	}
	tokenHash := hashRefreshToken(rawToken)
	expiresAt := time.Now().Add(uc.resetTTL)
	if err := uc.resetTokens.Create(ctx, cred.UserID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	if err := uc.resetMail.SendPasswordReset(ctx, cred.Email, rawToken); err != nil {
		// Log the delivery failure at Warn WITHOUT to_domain or email_fp —
		// neither field adds actionable information here (the operator cannot
		// re-send per-address from this log line) and their presence would
		// let a log reader distinguish "account found + SMTP failed" from
		// "account not found".
		uc.appLog.Warn().Err(err).Msg("password reset: SMTP delivery failed")
	}

	audit = true
	return nil
}

// CompletePasswordReset applies a new password after validating a one-time token.
func (uc *AuthUseCase) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) error {
	rawToken = strings.TrimSpace(rawToken)
	newPassword = strings.TrimSpace(newPassword)
	if rawToken == "" || newPassword == "" {
		return pkgerr.InvalidArgument("token and new_password are required")
	}
	if err := validateAccountPassword(newPassword); err != nil {
		return err
	}
	if uc.resetTokens == nil {
		return pkgerr.Internal("reset not configured")
	}

	hash := hashRefreshToken(rawToken)
	userID, err := uc.resetTokens.ConsumeByTokenHash(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return pkgerr.InvalidArgument("Ссылка сброса недействительна или истекла")
		}
		return err
	}

	passHash, err := hashPassword(newPassword)
	if err != nil {
		return pkgerr.Internal("failed to hash password")
	}
	if err := uc.creds.UpdatePasswordHash(ctx, userID, passHash); err != nil {
		return err
	}
	if err := uc.tokens.DeleteByUserID(ctx, userID); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}
