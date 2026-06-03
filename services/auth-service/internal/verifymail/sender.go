package verifymail

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	pkgmail "github.com/tienlao/agregator/pkg/mail"
)

// Sender sends the email-address verification link. It is a near-twin of
// passwordmail.Sender, differing only in the link path and message copy.
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

func (s *Sender) SendVerification(ctx context.Context, toEmail, rawToken string) error {
	if s == nil || s.smtp == nil {
		return fmt.Errorf("mail not configured")
	}
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		return fmt.Errorf("recipient email is empty")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return fmt.Errorf("verification token is empty")
	}
	q := url.Values{}
	q.Set("token", rawToken)
	verifyURL := s.prefix + "/auth/verify-email?" + q.Encode()
	body := fmt.Sprintf(
		"Здравствуйте.\n\n"+
			"Подтвердите ваш email, чтобы публиковать баню или анкету мастера. "+
			"Перейдите по ссылке (действует ограниченное время):\n\n"+
			"%s\n\n"+
			"Если вы не регистрировались в агрегаторе, проигнорируйте это письмо.\n",
		verifyURL,
	)
	return s.smtp.SendPlain(ctx, []string{toEmail}, "Подтверждение email", body)
}
