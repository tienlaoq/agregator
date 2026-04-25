package suspendnotify

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sender рассылает письма владельцу и CRM-персоналу при смене статуса в агрегаторе (если задан SMTP).
type Sender struct {
	venue      venuev1.VenueServiceClient
	userClient userv1.UserServiceClient
	log        zerolog.Logger

	host     string
	port     string
	smtpUser string
	smtpPass string
	from     string
}

func NewSender(log zerolog.Logger, venue venuev1.VenueServiceClient, userClient userv1.UserServiceClient) *Sender {
	return &Sender{
		venue:      venue,
		userClient: userClient,
		log:        log.With().Str("component", "suspendnotify").Logger(),
		host:       strings.TrimSpace(os.Getenv("SMTP_HOST")),
		port:       strings.TrimSpace(config.GetEnv("SMTP_PORT", "587")),
		smtpUser:   strings.TrimSpace(os.Getenv("SMTP_USER")),
		smtpPass:   strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		from:       strings.TrimSpace(os.Getenv("SMTP_FROM")),
	}
}

func (s *Sender) Enabled() bool {
	return s.host != "" && s.from != "" && s.smtpUser != "" && s.smtpPass != ""
}

// NotifyVenueSuspended не блокирует HTTP-ответ: письма уходят в фоне.
func (s *Sender) NotifyVenueSuspended(ctx context.Context, venueID, ownerID, venueName, moderationComment string) {
	if !s.Enabled() {
		return
	}
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.sendSuspended(bg, venueID, ownerID, venueName, moderationComment); err != nil {
			s.log.Warn().Err(err).Str("venue_id", venueID).Msg("suspend partner email notify failed")
		}
	}()
}

// NotifyVenueResumed — письмо после возобновления работы в агрегаторе (модератор).
func (s *Sender) NotifyVenueResumed(ctx context.Context, venueID, ownerID, venueName, moderatorNote string) {
	if !s.Enabled() {
		return
	}
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.sendResumed(bg, venueID, ownerID, venueName, moderatorNote); err != nil {
			s.log.Warn().Err(err).Str("venue_id", venueID).Msg("resume partner email notify failed")
		}
	}()
}

func (s *Sender) recipientEmails(ctx context.Context, venueID, ownerID string) ([]string, error) {
	userIDs := []string{ownerID}
	if ownerID != "" {
		staff, err := s.venue.ListVenueStaff(ctx, &venuev1.ListVenueStaffRequest{
			VenueId: venueID,
			ActorId: ownerID,
		})
		if err != nil {
			s.log.Warn().Err(err).Str("venue_id", venueID).Msg("ListVenueStaff for partner mail failed, owner only")
		} else {
			for _, m := range staff.GetMembers() {
				uid := strings.TrimSpace(m.GetUserId())
				if uid == "" {
					continue
				}
				if !contains(userIDs, uid) {
					userIDs = append(userIDs, uid)
				}
			}
		}
	}

	emails := make([]string, 0, len(userIDs))
	seen := map[string]struct{}{}
	for _, uid := range userIDs {
		u, err := s.userClient.GetUser(ctx, &userv1.GetUserRequest{Id: uid})
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				continue
			}
			return nil, fmt.Errorf("get user %s: %w", uid, err)
		}
		e := strings.TrimSpace(strings.ToLower(u.GetEmail()))
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		emails = append(emails, u.GetEmail())
	}
	return emails, nil
}

func (s *Sender) deliver(emails []string, subjectLine, body string) error {
	if len(emails) == 0 {
		return nil
	}
	subject := encodeRFC2047Word(subjectLine)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.host)
	msg := buildRFC822(s.from, emails, subject, body)
	return smtp.SendMail(addr, auth, s.from, emails, msg)
}

func (s *Sender) sendSuspended(ctx context.Context, venueID, ownerID, venueName, moderationComment string) error {
	emails, err := s.recipientEmails(ctx, venueID, ownerID)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Приостановка в агрегаторе: %s", venueName)
	body := fmt.Sprintf(
		"Здравствуйте.\n\n"+
			"Работа заведения «%s» в агрегаторе приостановлена модератором.\n\n"+
			"Комментарий:\n%s\n\n"+
			"Если это ошибка, ответьте на это письмо или свяжитесь с поддержкой через личный кабинет.\n",
		venueName,
		strings.TrimSpace(moderationComment),
	)
	return s.deliver(emails, subject, body)
}

func (s *Sender) sendResumed(ctx context.Context, venueID, ownerID, venueName, moderatorNote string) error {
	emails, err := s.recipientEmails(ctx, venueID, ownerID)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Возобновление в агрегаторе: %s", venueName)
	body := fmt.Sprintf(
		"Здравствуйте.\n\n"+
			"Работа заведения «%s» в агрегаторе снова активна (модератор возобновил показ в каталоге).\n",
		venueName,
	)
	if note := strings.TrimSpace(moderatorNote); note != "" {
		body += fmt.Sprintf("\nКомментарий модератора:\n%s\n", note)
	}
	body += "\nСпасибо, что вы с нами.\n"
	return s.deliver(emails, subject, body)
}

func contains(sl []string, v string) bool {
	for _, x := range sl {
		if x == v {
			return true
		}
	}
	return false
}

func encodeRFC2047Word(s string) string {
	return fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(s)))
}

func buildRFC822(from string, to []string, subject, body string) []byte {
	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n",
		from,
		strings.Join(to, ", "),
		subject,
	)
	return []byte(headers + body + "\r\n")
}
