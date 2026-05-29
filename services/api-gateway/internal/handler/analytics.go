package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

const analyticsSubject = "analytics.web"

var analyticsEventName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// analyticsAllowedKeys is the server-side whitelist of prop keys that may be
// logged and forwarded to NATS. Any key absent from this set is silently
// dropped before the event leaves the gateway — regardless of client-side
// "no PII" conventions.
//
// Allowed key rules:
//   - utm_* — any UTM parameter (utm_source, utm_medium, utm_campaign, …)
//   - fixed keys below — non-PII dimensions used by product analytics
//
// To add a new dimension: extend the map here AND update the frontend contract.
var analyticsAllowedKeys = map[string]bool{
	"page":           true,
	"path":           true,
	"referrer":       true,
	"event_category": true,
	"event_label":    true,
	"value":          true,
	"locale":         true,
	"screen":         true,
	"duration_ms":    true,
}

// filterProps returns a copy of props containing only whitelisted keys.
// utm_* keys are always allowed; all others are checked against analyticsAllowedKeys.
func filterProps(props map[string]any) map[string]any {
	out := make(map[string]any, len(props))
	for k, v := range props {
		if strings.HasPrefix(k, "utm_") || analyticsAllowedKeys[k] {
			out[k] = v
		}
		// unknown / potentially PII-bearing keys are silently dropped
	}
	return out
}

// AnalyticsHandler accepts lightweight product events from the browser (JSON logs + optional NATS).
type AnalyticsHandler struct {
	log  zerolog.Logger
	nats *nats.Conn
}

func NewAnalyticsHandler(log zerolog.Logger, nc *nats.Conn) *AnalyticsHandler {
	return &AnalyticsHandler{log: log, nats: nc}
}

type analyticsRequest struct {
	Name  string         `json:"name"`
	Props map[string]any `json:"props"`
}

// CollectEvent POST /api/v1/analytics/events — публичный endpoint, без PII в props по контракту клиента.
func (h *AnalyticsHandler) CollectEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeCatalog(w, apicatalog.GatewayRequestMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}
	var req analyticsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidJson)
		return
	}
	name := strings.TrimSpace(req.Name)
	if !analyticsEventName.MatchString(name) {
		writeCatalog(w, apicatalog.GatewayAnalyticsInvalidEventName)
		return
	}
	if req.Props == nil {
		req.Props = map[string]any{}
	}
	// Server-side whitelist: drop any key not in analyticsAllowedKeys / utm_*.
	// This is a hard guarantee — we do not rely on the client honouring the
	// "no PII in props" contract.
	safeProps := filterProps(req.Props)
	propsJSON, err := json.Marshal(safeProps)
	if err != nil || len(propsJSON) > 4096 {
		writeCatalog(w, apicatalog.GatewayAnalyticsPropsInvalidOrTooLarge)
		return
	}

	rid := middleware.RequestIDFromCtx(r.Context())
	lg := h.log.Info().
		Str("kind", "product_analytics").
		Str("event", name).
		Str("request_id", rid).
		RawJSON("props", propsJSON)

	if h.nats != nil && h.nats.IsConnected() {
		payload := map[string]any{
			"event":      name,
			"props":      safeProps, // filtered — never raw client input
			"request_id": rid,
		}
		b, mErr := json.Marshal(payload)
		if mErr == nil {
			if pubErr := h.nats.Publish(analyticsSubject, b); pubErr != nil {
				h.log.Warn().Err(pubErr).Str("subject", analyticsSubject).Msg("analytics nats publish failed")
			}
		}
	}

	lg.Msg("analytics_event")
	w.WriteHeader(http.StatusNoContent)
}
