// Package fcm delivers notifications to mobile devices through Firebase Cloud
// Messaging HTTP v1. It is intentionally thin: one HTTP POST per token, OAuth2
// bearer tokens minted from a Google service-account JSON. No firebase-admin
// SDK dependency — v1 is a plain REST endpoint.
package fcm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

// messagingScope is the OAuth2 scope required to call FCM send.
const messagingScope = "https://www.googleapis.com/auth/firebase.messaging"

// sendTimeout bounds a single messages:send call so a stuck provider can't
// pin a goroutine indefinitely.
const sendTimeout = 10 * time.Second

// Sender posts notifications to FCM HTTP v1. Safe for concurrent use.
type Sender struct {
	projectID string
	endpoint  string
	client    *http.Client
}

// New builds a Sender from a Google service-account JSON (the file downloaded
// from the Firebase console). It returns (nil, nil) when credsJSON is empty so
// the caller can treat "push not configured" as a no-op rather than a failure —
// the service still runs and serves the in-app bell without mobile push.
func New(ctx context.Context, credsJSON []byte) (*Sender, error) {
	if len(bytes.TrimSpace(credsJSON)) == 0 {
		return nil, nil
	}
	creds, err := google.CredentialsFromJSON(ctx, credsJSON, messagingScope)
	if err != nil {
		return nil, fmt.Errorf("fcm: parse service-account credentials: %w", err)
	}
	if creds.ProjectID == "" {
		return nil, fmt.Errorf("fcm: service-account JSON missing project_id")
	}
	// oauth2.NewClient wraps the token source; the client refreshes bearer
	// tokens transparently. Timeout bounds the send POST (token refresh uses
	// its own context internally).
	client := oauth2.NewClient(ctx, creds.TokenSource)
	client.Timeout = sendTimeout
	return &Sender{
		projectID: creds.ProjectID,
		endpoint:  fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", creds.ProjectID),
		client:    client,
	}, nil
}

// fcm v1 request body. data values must all be strings.
type sendBody struct {
	Message message `json:"message"`
}

type message struct {
	Token        string            `json:"token"`
	Notification notificationBlock `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
}

type notificationBlock struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Push sends n to each token, one request per token. It returns the tokens FCM
// reported as permanently gone (HTTP 404 UNREGISTERED — app uninstalled) so the
// caller can prune them, plus the first transport error encountered (transient
// failures are not fatal: other tokens are still attempted).
func (s *Sender) Push(ctx context.Context, tokens []string, n *domain.Notification) (invalid []string, err error) {
	data := map[string]string{
		"type":            n.Type,
		"notification_id": n.ID.String(),
	}
	// The raw structured payload the client parses to build a deep link.
	if n.Data != "" {
		data["payload"] = n.Data
	}
	var firstErr error
	for _, tok := range tokens {
		status, sendErr := s.sendOne(ctx, tok, n, data)
		// Only 404 means the token is definitively dead (app uninstalled). A 400
		// could equally be a payload bug on our side, so we do NOT prune on it —
		// pruning valid tokens on a code bug would silently disable push for
		// everyone. ponytail: prune on 404 only; malformed-token 400s linger.
		if status == http.StatusNotFound {
			invalid = append(invalid, tok)
			continue
		}
		if sendErr != nil && firstErr == nil {
			firstErr = sendErr
		}
	}
	return invalid, firstErr
}

// sendOne posts a single message and returns the HTTP status code.
func (s *Sender) sendOne(ctx context.Context, token string, n *domain.Notification, data map[string]string) (int, error) {
	body, err := json.Marshal(sendBody{Message: message{
		Token:        token,
		Notification: notificationBlock{Title: n.Title, Body: n.Body},
		Data:         data,
	}})
	if err != nil {
		return 0, fmt.Errorf("fcm: marshal message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("fcm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fcm: send: %w", err)
	}
	defer resp.Body.Close()
	// Drain a bounded amount so the connection can be reused; the body is only
	// needed for diagnostics on error statuses.
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, fmt.Errorf("fcm: status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	return resp.StatusCode, nil
}
