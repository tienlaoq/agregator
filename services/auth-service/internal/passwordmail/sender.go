package passwordmail

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	pkgmail "github.com/tienlao/agregator/pkg/mail"
)

// Sender sends the password reset link email.
type Sender struct {
	smtp   *pkgmail.Sender
	prefix string // trimmed frontend base URL
}

// New creates a Sender. frontendURL must already be normalised (trimmed,
// no trailing slash, non-empty) — config.normaliseFrontendURL does this once
// at startup so callers do not need to repeat the sanitisation.
func New(smtp *pkgmail.Sender, frontendURL string) *Sender {
	return &Sender{smtp: smtp, prefix: frontendURL}
}

func (s *Sender) Enabled() bool {
	return s != nil && s.smtp != nil && s.smtp.Enabled()
}

func (s *Sender) SendPasswordReset(ctx context.Context, toEmail, rawToken string) error {
	if s == nil || s.smtp == nil {
		return fmt.Errorf("mail not configured")
	}
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		return fmt.Errorf("recipient email is empty")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return fmt.Errorf("reset token is empty")
	}
	q := url.Values{}
	q.Set("token", rawToken)
	resetURL := s.prefix + "/auth/reset-password?" + q.Encode()
	body := fmt.Sprintf(
		"Здравствуйте.\n\n"+
			"Чтобы задать новый пароль для входа в агрегатор, перейдите по ссылке (действует ограниченное время):\n\n"+
			"%s\n\n"+
			"Если вы не запрашивали сброс пароля, проигнорируйте это письмо.\n",
		resetURL,
	)
	return s.smtp.SendPlain(ctx, []string{toEmail}, "Сброс пароля", body)
}
