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
	propsJSON, err := json.Marshal(req.Props)
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
			"props":      req.Props,
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
