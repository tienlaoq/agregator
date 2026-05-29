package telegram

import (
	"context"
	"fmt"
	"html"
	"strings"

	pkgtelegram "github.com/tienlao/agregator/pkg/telegram"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

type Notifier struct {
	client   *pkgtelegram.Client
	adminURL string
}

func NewNotifier(botToken, chatID, adminURL string) *Notifier {
	adminURL = strings.TrimSuffix(strings.TrimSpace(adminURL), "/")
	return &Notifier{
		client:   pkgtelegram.NewClient(botToken, chatID),
		adminURL: adminURL,
	}
}

// Enabled is true when TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are both set.
func (n *Notifier) Enabled() bool {
	return n.client.Enabled()
}

func (n *Notifier) NotifyNewVenue(ctx context.Context, venue *domain.Venue) error {
	if !n.client.Enabled() {
		return nil
	}

	venueTypeLabel := map[string]string{
		"banya":  "Баня",
		"sauna":  "Сауна",
		"hammam": "Хаммам",
	}
	typeLabel := venueTypeLabel[venue.Type]
	if typeLabel == "" {
		typeLabel = venue.Type
	}

	verifyBlock := ""
	if venue.LegalEntityName != "" || venue.INN != "" {
		verifyBlock = fmt.Sprintf(
			"\n<b>Проверка владельца</b>\n"+
				"Юр. наименование: %s\n"+
				"ИНН: <code>%s</code> · ОГРН/ОГРНИП: <code>%s</code>\n",
			html.EscapeString(venue.LegalEntityName),
			html.EscapeString(venue.INN),
			html.EscapeString(venue.OGRN),
		)
		if venue.PublicListingURL != "" {
			verifyBlock += fmt.Sprintf("Карточка на картах: %s\n", html.EscapeString(venue.PublicListingURL))
		}
		if venue.VerificationNote != "" {
			verifyBlock += fmt.Sprintf("Комментарий: %s\n", html.EscapeString(pkgtelegram.Truncate(venue.VerificationNote, 300)))
		}
	}

	adminLink := n.adminURL + "/admin/venues"
	text := fmt.Sprintf(
		"🏛 <b>Новая заявка на модерацию</b>\n\n"+
			"<b>%s</b>\n"+
			"Тип: %s\n"+
			"Адрес: %s\n"+
			"Телефон: %s\n\n"+
			"📝 %s\n"+
			"%s"+
			`<a href="%s">Открыть в админке</a>`,
		html.EscapeString(venue.Name),
		html.EscapeString(typeLabel),
		html.EscapeString(venue.Address),
		html.EscapeString(venue.Phone),
		html.EscapeString(pkgtelegram.Truncate(venue.Description, 200)),
		verifyBlock,
		html.EscapeString(adminLink),
	)

	return n.client.SendHTML(ctx, text)
}

func (n *Notifier) NotifyModerated(ctx context.Context, venue *domain.Venue) error {
	if !n.client.Enabled() {
		return nil
	}

	statusEmoji := map[string]string{
		"active":    "✅",
		"rejected":  "❌",
		"suspended": "⏸",
	}
	statusLabelRU := map[string]string{
		"active":         "активно",
		"rejected":       "отклонено",
		"suspended":      "приостановлено",
		"pending_review": "на проверке",
		"draft":          "черновик",
	}
	emoji := statusEmoji[venue.Status]
	if emoji == "" {
		emoji = "ℹ️"
	}
	statusHuman := statusLabelRU[venue.Status]
	if statusHuman == "" {
		statusHuman = venue.Status
	}

	text := fmt.Sprintf(
		"%s <b>Статус изменён</b>\n\n"+
			"<b>%s</b> → %s",
		emoji,
		html.EscapeString(venue.Name),
		html.EscapeString(statusHuman),
	)

	if venue.ModerationComment != "" {
		text += fmt.Sprintf("\nКомментарий: %s", html.EscapeString(venue.ModerationComment))
	}

	return n.client.SendHTML(ctx, text)
}
