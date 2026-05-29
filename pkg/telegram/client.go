package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client отправляет сообщения в Telegram Bot API (parse_mode HTML).
type Client struct {
	botToken string
	chatID   string
	enabled  bool
	http     *http.Client
}

// NewClient создаёт клиента. Работает только если заданы и token, и chatID.
func NewClient(botToken, chatID string) *Client {
	return &Client{
		botToken: strings.TrimSpace(botToken),
		chatID:   strings.TrimSpace(chatID),
		enabled:  strings.TrimSpace(botToken) != "" && strings.TrimSpace(chatID) != "",
		http:     &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

// SendHTML отправляет текст с parse_mode=HTML. Безопасно вызывать при !Enabled() — no-op.
// ctx используется для отмены/deadline HTTP-запроса; передавайте контекст с коротким
// таймаутом (например 5 s) чтобы зависший Bot API не блокировал вызывающую горутину.
func (c *Client) SendHTML(ctx context.Context, text string) error {
	if c == nil || !c.enabled {
		return nil
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)
	body := map[string]any{
		"chat_id":    c.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var apiResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("telegram response JSON: %w; body=%q", err, Truncate(string(respBody), 500))
	}
	if !apiResp.OK {
		if apiResp.Description != "" {
			return fmt.Errorf("telegram API error %d: %s", apiResp.ErrorCode, apiResp.Description)
		}
		return fmt.Errorf("telegram API ok=false: %s", strings.TrimSpace(string(respBody)))
	}
	return nil
}

// Truncate обрезает строку по рунам (для подписей в сообщениях).
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
