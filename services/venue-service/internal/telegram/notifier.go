package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

type Notifier struct {
	botToken string
	chatID   string
	adminURL string
	enabled  bool
}

func NewNotifier(botToken, chatID, adminURL string) *Notifier {
	return &Notifier{
		botToken: botToken,
		chatID:   chatID,
		adminURL: adminURL,
		enabled:  botToken != "" && chatID != "",
	}
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

	text := fmt.Sprintf(
		"🏛 *Новая заявка на модерацию*\n\n"+
			"*%s*\n"+
			"Тип: %s\n"+
			"Адрес: %s\n"+
			"Телефон: %s\n\n"+
			"📝 %s\n\n"+
			"[Открыть в админке](%s/admin/venues)",
		escapeMarkdown(venue.Name),
		typeLabel,
		escapeMarkdown(venue.Address),
		venue.Phone,
		truncate(venue.Description, 200),
		n.adminURL,
	)

	return n.sendMessage(text)
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
		"%s *Статус изменён*\n\n"+
			"*%s* → %s",
		emoji,
		escapeMarkdown(venue.Name),
		venue.Status,
	)

	if venue.ModerationComment != "" {
		text += fmt.Sprintf("\nКомментарий: %s", escapeMarkdown(venue.ModerationComment))
	}

	return n.sendMessage(text)
}

func (n *Notifier) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

	body := map[string]any{
		"chat_id":    n.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram body: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("send telegram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}

func escapeMarkdown(s string) string {
	replacer := bytes.NewBuffer(nil)
	for _, c := range s {
		switch c {
		case '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!':
			replacer.WriteRune('\\')
		}
		replacer.WriteRune(c)
	}
	return replacer.String()
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
