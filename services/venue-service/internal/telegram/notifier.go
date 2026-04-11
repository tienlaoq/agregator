package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

type Notifier struct {
	botToken string
	chatID   string
	adminURL string
	enabled  bool
	client   *http.Client
}

func NewNotifier(botToken, chatID, adminURL string) *Notifier {
	adminURL = strings.TrimSuffix(strings.TrimSpace(adminURL), "/")
	if adminURL == "" {
		adminURL = "http://localhost:3000"
	}
	return &Notifier{
		botToken: strings.TrimSpace(botToken),
		chatID:   strings.TrimSpace(chatID),
		adminURL: adminURL,
		enabled:  strings.TrimSpace(botToken) != "" && strings.TrimSpace(chatID) != "",
		client:   &http.Client{Timeout: 8 * time.Second},
	}
}

// Enabled is true when TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are both set.
func (n *Notifier) Enabled() bool {
	return n.enabled
}

func (n *Notifier) NotifyNewVenue(venue *domain.Venue) error {
	if !n.enabled {
		return nil
	}

	venueTypeLabel := map[string]string{
		"banya":   "Баня",
		"sauna":   "Сауна",
		"hammam":  "Хаммам",
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
			verifyBlock += fmt.Sprintf("Комментарий: %s\n", html.EscapeString(truncate(venue.VerificationNote, 300)))
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
		html.EscapeString(truncate(venue.Description, 200)),
		verifyBlock,
		html.EscapeString(adminLink),
	)

	return n.sendMessageHTML(text)
}

func (n *Notifier) NotifyModerated(venue *domain.Venue) error {
	if !n.enabled {
		return nil
	}

	statusEmoji := map[string]string{
		"active":    "✅",
		"rejected":  "❌",
		"suspended": "⏸",
	}
	emoji := statusEmoji[venue.Status]
	if emoji == "" {
		emoji = "ℹ️"
	}

	text := fmt.Sprintf(
		"%s <b>Статус изменён</b>\n\n"+
			"<b>%s</b> → <code>%s</code>",
		emoji,
		html.EscapeString(venue.Name),
		html.EscapeString(venue.Status),
	)

	if venue.ModerationComment != "" {
		text += fmt.Sprintf("\nКомментарий: %s", html.EscapeString(venue.ModerationComment))
	}

	return n.sendMessageHTML(text)
}

func (n *Notifier) sendMessageHTML(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

	body := map[string]any{
		"chat_id":    n.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
