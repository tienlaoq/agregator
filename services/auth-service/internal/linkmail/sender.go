// Package linkmail sends the transactional "click this link" emails used by
// auth-service — password reset and email verification. Both have the same
// shape: a normalised recipient, a one-time token appended to a frontend path,
// and a short plain-text body — so they share one Sender and one send helper,
// differing only in path and copy.
package linkmail

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	pkgmail "github.com/tienlao/agregator/pkg/mail"
)

// Sender formats and delivers auth link emails over SMTP. Its zero value is not
// usable; construct with New. A nil *Sender is safe for Enabled (reports false),
// mirroring pkgmail.Sender.
type Sender struct {
	smtp   *pkgmail.Sender
	prefix string // trimmed frontend base URL
}

// New creates a Sender. frontendURL must already be normalised (trimmed, no
// trailing slash, non-empty) — config.normaliseFrontendURL does this once at
// startup so callers do not repeat the sanitisation.
func New(smtp *pkgmail.Sender, frontendURL string) *Sender {
	return &Sender{smtp: smtp, prefix: frontendURL}
}

func (s *Sender) Enabled() bool {
	return s != nil && s.smtp != nil && s.smtp.Enabled()
}

// send emails a single-use link built as prefix+path?token=rawToken. tokenLabel
// names the token in the empty-token error ("reset token", "verification token")
// so failures stay distinguishable in logs; body renders the final message from
// the constructed link.
func (s *Sender) send(ctx context.Context, toEmail, rawToken, path, tokenLabel, subject string, body func(link string) string) error {
	if s == nil || s.smtp == nil {
		return fmt.Errorf("mail not configured")
	}
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		return fmt.Errorf("recipient email is empty")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return fmt.Errorf("%s is empty", tokenLabel)
	}
	q := url.Values{}
	q.Set("token", rawToken)
	link := s.prefix + path + "?" + q.Encode()
	return s.smtp.SendPlain(ctx, []string{toEmail}, subject, body(link))
}

// SendPasswordReset emails the password-reset link. Satisfies the usecase's
// PasswordResetMailer interface.
func (s *Sender) SendPasswordReset(ctx context.Context, toEmail, rawToken string) error {
	return s.send(ctx, toEmail, rawToken, "/auth/reset-password", "reset token", "Сброс пароля",
		func(link string) string {
			return fmt.Sprintf("Здравствуйте.\n\n"+
				"Чтобы задать новый пароль для входа в агрегатор, перейдите по ссылке (действует ограниченное время):\n\n"+
				"%s\n\n"+
				"Если вы не запрашивали сброс пароля, проигнорируйте это письмо.\n", link)
		})
}

// SendVerification emails the email-address verification link. Satisfies the
// usecase's EmailVerificationMailer interface.
func (s *Sender) SendVerification(ctx context.Context, toEmail, rawToken string) error {
	return s.send(ctx, toEmail, rawToken, "/auth/verify-email", "verification token", "Подтверждение email",
		func(link string) string {
			return fmt.Sprintf("Здравствуйте.\n\n"+
				"Подтвердите ваш email, чтобы публиковать баню или анкету мастера. "+
				"Перейдите по ссылке (действует ограниченное время):\n\n"+
				"%s\n\n"+
				"Если вы не регистрировались в агрегаторе, проигнорируйте это письмо.\n", link)
		})
}
