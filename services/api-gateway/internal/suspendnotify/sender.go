package suspendnotify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	pkgmail "github.com/tienlao/agregator/pkg/mail"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sender рассылает письма владельцу и CRM-персоналу при смене статуса в агрегаторе (если задан SMTP).
// Staff fan-out goes through crm-service (extracted from venue-service).
type Sender struct {
	crm        crmv1.CRMServiceClient
	userClient userv1.UserServiceClient
	log        zerolog.Logger
	mail       *pkgmail.Sender
}

func NewSender(log zerolog.Logger, crm crmv1.CRMServiceClient, userClient userv1.UserServiceClient) *Sender {
	return &Sender{
		crm:        crm,
		userClient: userClient,
		log:        log.With().Str("component", "suspendnotify").Logger(),
		mail:       pkgmail.NewSenderFromEnv(),
	}
}

func (s *Sender) Enabled() bool {
	return s.mail != nil && s.mail.Enabled()
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
		staff, err := s.crm.ListStaff(ctx, &crmv1.ListStaffRequest{
			VenueId: venueID,
			ActorId: ownerID,
		})
		if err != nil {
			s.log.Warn().Err(err).Str("venue_id", venueID).Msg("crm.ListStaff for partner mail failed, owner only")
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

func (s *Sender) deliver(ctx context.Context, emails []string, subjectLine, body string) error {
	if len(emails) == 0 {
		return nil
	}
	return s.mail.SendPlain(ctx, emails, subjectLine, body)
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
	return s.deliver(ctx, emails, subject, body)
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
	return s.deliver(ctx, emails, subject, body)
}

func contains(sl []string, v string) bool {
	for _, x := range sl {
		if x == v {
			return true
		}
	}
	return false
}
