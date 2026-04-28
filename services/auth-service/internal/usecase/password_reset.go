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

func domainSuffix(email string) string {
	e := strings.TrimSpace(strings.ToLower(email))
	if i := strings.LastIndexByte(e, '@'); i >= 0 && i+1 < len(e) {
		return e[i+1:]
	}
	return ""
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

// RequestPasswordReset completes without distinguishing cases in the public API response.
// Logs may record pipeline steps; use PASSWORD_RESET_VERBOSE=1 for an extra line on each request.
func (uc *AuthUseCase) RequestPasswordReset(ctx context.Context, email string) error {
	email = normalizeAccountEmail(email)
	if email == "" {
		return nil
	}
	if passwordResetVerboseLogs() {
		uc.appLog.Info().Str("email_fp", emailFingerprint(email)).Msg("password reset: request received")
	} else {
		uc.appLog.Debug().Str("email_fp", emailFingerprint(email)).Msg("password reset: request received")
	}

	cred, err := uc.creds.GetByEmail(ctx, email)
	if err != nil {
		if isNotFound(err) {
			uc.appLog.Info().Str("email_fp", emailFingerprint(email)).Msg("password reset: skipped (no password-based account for this email)")
			return nil
		}
		return err
	}
	if !accountCredentialHasPassword(cred) {
		uc.appLog.Info().Str("email_fp", emailFingerprint(email)).Msg("password reset: skipped (no password-based account for this email)")
		return nil
	}

	if uc.resetMail == nil || !uc.resetMail.Enabled() {
		uc.appLog.Warn().Msg("password reset requested but SMTP is not configured")
		return nil
	}
	if uc.resetTokens == nil {
		uc.appLog.Warn().Msg("password reset requested but reset token repository is nil")
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

	uc.appLog.Info().Str("to_domain", domainSuffix(cred.Email)).Msg("password reset: sending SMTP")
	if err := uc.resetMail.SendPasswordReset(ctx, cred.Email, rawToken); err != nil {
		uc.appLog.Warn().Err(err).
			Str("email_fp", emailFingerprint(email)).
			Str("to_domain", domainSuffix(cred.Email)).
			Msg("password reset email failed")
	} else {
		uc.appLog.Info().Str("to_domain", domainSuffix(cred.Email)).Msg("password reset email sent")
	}
	return nil
}

// CompletePasswordReset applies a new password after validating a one-time token.
func (uc *AuthUseCase) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) error {
	rawToken = strings.TrimSpace(rawToken)
	newPassword = strings.TrimSpace(newPassword)
	if rawToken == "" || newPassword == "" {
		return pkgerr.InvalidArgument("token and new_password are required")
	}
	if len(newPassword) < minAccountPasswordLen {
		return pkgerr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minAccountPasswordLen))
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
