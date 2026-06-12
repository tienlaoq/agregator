package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	pkgmail "github.com/tienlao/agregator/pkg/mail"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	gwmetrics "github.com/tienlao/agregator/services/api-gateway/internal/metrics"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/supportstore"
)

// Support limits — canonical values in internal/limits.
var (
	supportWebhookTimeout   = limits.SupportWebhookTimeout
	supportMaxBodyBytes     = limits.SupportMaxBodyBytes
	supportMaxTopicLen      = limits.SupportMaxTopicLen
	supportMaxMessageLen    = limits.SupportMaxMessageLen
	supportMaxEmailLen      = limits.SupportMaxEmailLen
	supportMaxRefFieldLen   = limits.SupportMaxRefFieldLen
	supportMaxSourcePageLen = limits.SupportMaxSourcePageLen
)

var errHelpdeskRequestFailed = errors.New("helpdesk request failed")

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type SupportHandler struct {
	log          zerolog.Logger
	webhookURL   string
	webhookToken string
	recipients   []string
	mail         *pkgmail.Sender
	client       httpDoer
	tickets      SupportTicketsPersistence
}

func NewSupportHandler(log zerolog.Logger, webhookURL, webhookToken string, recipients []string, tickets SupportTicketsPersistence) *SupportHandler {
	return &SupportHandler{
		log:          log,
		webhookURL:   strings.TrimSpace(webhookURL),
		webhookToken: strings.TrimSpace(webhookToken),
		recipients:   uniqueEmails(recipients),
		mail:         pkgmail.NewSenderFromEnv(),
		client: &http.Client{
			Timeout: supportWebhookTimeout,
		},
		tickets: tickets,
	}
}

type supportContactRequest struct {
	Topic      string `json:"topic"`
	Message    string `json:"message"`
	Email      string `json:"email"`
	BookingID  string `json:"booking_id"`
	PaymentID  string `json:"payment_id"`
	SourcePage string `json:"source_page"`
}

type helpdeskSupportPayload struct {
	RequestID       string   `json:"request_id"`
	Topic           string   `json:"topic"`
	Message         string   `json:"message"`
	Email           string   `json:"email"`
	BookingID       string   `json:"booking_id,omitempty"`
	PaymentID       string   `json:"payment_id,omitempty"`
	SourcePage      string   `json:"source_page,omitempty"`
	UserID          string   `json:"user_id"`
	Role            string   `json:"role"`
	CreatedAt       string   `json:"created_at"`
	TargetRoles     []string `json:"target_roles,omitempty"`
	RecipientEmails []string `json:"recipient_emails,omitempty"`
}

// Contact POST /api/v1/support/contact
func (h *SupportHandler) Contact(w http.ResponseWriter, r *http.Request) {
	canEmailMods := len(h.recipients) > 0 && h.mail != nil && h.mail.Enabled()
	canWebhook := strings.TrimSpace(h.webhookURL) != ""
	if !canEmailMods && !canWebhook && h.tickets == nil {
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("support service is not configured"))
		return
	}

	var req supportContactRequest
	if !readJSONOrRespond(w, r, &req) {
		return
	}

	req.Topic = strings.TrimSpace(req.Topic)
	req.Message = strings.TrimSpace(req.Message)
	req.Email = strings.TrimSpace(req.Email)
	req.BookingID = strings.TrimSpace(req.BookingID)
	req.PaymentID = strings.TrimSpace(req.PaymentID)
	req.SourcePage = strings.TrimSpace(req.SourcePage)

	req.Topic = clampString(req.Topic, supportMaxTopicLen)
	req.Message = clampString(req.Message, supportMaxMessageLen)
	req.Email = clampString(req.Email, supportMaxEmailLen)
	req.BookingID = clampString(req.BookingID, supportMaxRefFieldLen)
	req.PaymentID = clampString(req.PaymentID, supportMaxRefFieldLen)
	req.SourcePage = clampString(req.SourcePage, supportMaxSourcePageLen)

	if req.Topic == "" || req.Message == "" {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("topic and message are required"))
		return
	}
	if req.Email == "" {
		req.Email = strings.TrimSpace(middleware.EmailFromCtx(r.Context()))
	}

	requestID := uuid.NewString()
	ticketNumber := supportTicketNumber(requestID)
	reqUUID, err := uuid.Parse(requestID)
	if err != nil {
		writeCatalog(w, apicatalog.GatewayUpstreamInternal.WithMessage("failed to allocate ticket id"))
		return
	}

	if h.tickets != nil {
		insert := supportstore.InsertParams{
			RequestID:    reqUUID,
			TicketNumber: ticketNumber,
			Topic:        req.Topic,
			Message:      req.Message,
			UserEmail:    req.Email,
			UserID:       strings.TrimSpace(middleware.UserIDFromCtx(r.Context())),
			Role:         strings.TrimSpace(middleware.RoleFromCtx(r.Context())),
			BookingID:    req.BookingID,
			PaymentID:    req.PaymentID,
			SourcePage:   req.SourcePage,
		}
		if err := h.tickets.Insert(r.Context(), insert); err != nil {
			h.log.Warn().Err(err).Str("request_id", requestID).Msg("support ticket insert failed")
			writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to store support request"))
			return
		}
	}

	payload := helpdeskSupportPayload{
		RequestID:       requestID,
		Topic:           req.Topic,
		Message:         req.Message,
		Email:           req.Email,
		BookingID:       req.BookingID,
		PaymentID:       req.PaymentID,
		SourcePage:      req.SourcePage,
		UserID:          strings.TrimSpace(middleware.UserIDFromCtx(r.Context())),
		Role:            strings.TrimSpace(middleware.RoleFromCtx(r.Context())),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		TargetRoles:     []string{"admin"},
		RecipientEmails: h.recipients,
	}
	if canEmailMods {
		if err := h.sendEmail(r.Context(), payload); err != nil {
			if h.tickets != nil {
				if err2 := h.tickets.SetNotifyStatus(r.Context(), reqUUID, supportstore.NotifyFailed); err2 != nil {
					h.log.Warn().Err(err2).Str("request_id", requestID).Msg("support SetNotifyStatus failed")
				}
			}
			h.log.Warn().
				Err(err).
				Str("request_id", requestID).
				Str("user_id", strings.TrimSpace(middleware.UserIDFromCtx(r.Context()))).
				Msg("support contact email send failed")
			writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to send support request"))
			return
		}
		if h.tickets != nil {
			if err2 := h.tickets.SetNotifyStatus(r.Context(), reqUUID, supportstore.NotifyOK); err2 != nil {
				h.log.Warn().Err(err2).Str("request_id", requestID).Msg("support SetNotifyStatus ok")
			}
		}
	} else if canWebhook {
		body, err := json.Marshal(payload)
		if err != nil {
			writeCatalog(w, apicatalog.GatewayUpstreamInternal.WithMessage("failed to serialize support request"))
			return
		}
		lastErr := h.postWebhook(body)
		if lastErr != nil {
			lastErr = h.postWebhook(body)
		}
		if lastErr != nil {
			gwmetrics.ObserveSupportWebhookDelivery("error")
			if h.tickets != nil {
				if err2 := h.tickets.SetNotifyStatus(r.Context(), reqUUID, supportstore.NotifyFailed); err2 != nil {
					h.log.Warn().Err(err2).Str("request_id", requestID).Msg("support SetNotifyStatus failed")
				}
			}
			h.log.Warn().
				Err(lastErr).
				Str("request_id", requestID).
				Str("user_id", strings.TrimSpace(middleware.UserIDFromCtx(r.Context()))).
				Msg("support contact forward failed")
			writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to send support request"))
			return
		}
		gwmetrics.ObserveSupportWebhookDelivery("success")
		if h.tickets != nil {
			if err2 := h.tickets.SetNotifyStatus(r.Context(), reqUUID, supportstore.NotifyOK); err2 != nil {
				h.log.Warn().Err(err2).Str("request_id", requestID).Msg("support SetNotifyStatus ok")
			}
		}
	} else if h.tickets != nil {
		if err2 := h.tickets.SetNotifyStatus(r.Context(), reqUUID, supportstore.NotifyOK); err2 != nil {
			h.log.Warn().Err(err2).Str("request_id", requestID).Msg("support SetNotifyStatus ok (inbox-only)")
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"request_id":    requestID,
		"ticket_number": ticketNumber,
		"status":        "accepted",
	})
}

type adminSupportReplyRequest struct {
	TicketNumber string `json:"ticket_number"`
	RequestID    string `json:"request_id"`
	UserEmail    string `json:"user_email"`
	Reply        string `json:"reply"`
}

// AdminReply POST /api/v1/admin/support/reply — отправить ответ пользователю по обращению (только SMTP).
func (h *SupportHandler) AdminReply(w http.ResponseWriter, r *http.Request) {
	var req adminSupportReplyRequest
	if !readJSONOrRespond(w, r, &req) {
		return
	}

	req.TicketNumber = clampString(strings.TrimSpace(req.TicketNumber), 48)
	req.RequestID = clampString(strings.TrimSpace(req.RequestID), 48)
	req.UserEmail = clampString(strings.TrimSpace(req.UserEmail), supportMaxEmailLen)
	req.Reply = clampString(strings.TrimSpace(req.Reply), supportMaxMessageLen)

	if req.UserEmail == "" {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("user_email is required"))
		return
	}
	if req.Reply == "" {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("reply is required"))
		return
	}
	if req.TicketNumber == "" && req.RequestID == "" {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("ticket_number or request_id is required"))
		return
	}

	if req.RequestID != "" {
		if _, err := uuid.Parse(req.RequestID); err != nil {
			writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("invalid request_id"))
			return
		}
	}

	var markReplyID *uuid.UUID
	if h.tickets != nil {
		row, err := h.resolveTicketRow(r.Context(), req.TicketNumber, req.RequestID)
		if err != nil {
			if errors.Is(err, supportstore.ErrNotFound) {
				http.Error(w, `{"error":"ticket not found"}`, http.StatusNotFound)
				return
			}
			h.log.Warn().Err(err).Msg("support admin reply: resolve ticket")
			writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to load ticket"))
			return
		}
		if !strings.EqualFold(strings.TrimSpace(row.UserEmail), strings.TrimSpace(req.UserEmail)) {
			writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("user_email does not match this ticket"))
			return
		}
		idCopy := row.RequestID
		markReplyID = &idCopy
	}

	refLabel := supportReplyReferenceLabel(req.TicketNumber, req.RequestID)
	if h.mail == nil || !h.mail.Enabled() {
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("support email is not configured"))
		return
	}

	modID := strings.TrimSpace(middleware.UserIDFromCtx(r.Context()))
	subject := fmt.Sprintf("Ответ поддержки · %s", refLabel)
	var body strings.Builder
	body.WriteString("Здравствуйте.\n\n")
	body.WriteString("По вашему обращению ")
	body.WriteString(refLabel)
	body.WriteString(":\n\n")
	body.WriteString(req.Reply)
	body.WriteString("\n\n---\n")
	body.WriteString("Если вопрос остаётся открытым, ответьте на это письмо или снова напишите через раздел «Поддержка» на сайте.\n")
	if modID != "" {
		body.WriteString("\nИдентификатор модератора: ")
		body.WriteString(modID)
		body.WriteString("\n")
	}

	if err := h.mail.SendPlain(r.Context(), []string{req.UserEmail}, subject, body.String()); err != nil {
		h.log.Warn().
			Err(err).
			Str("moderator_id", modID).
			Str("user_email", req.UserEmail).
			Str("ticket_ref", refLabel).
			Msg("support admin reply email failed")
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to send reply"))
		return
	}

	if h.tickets != nil && markReplyID != nil {
		if err := h.tickets.MarkReplied(r.Context(), *markReplyID, modID); err != nil {
			h.log.Warn().Err(err).Str("request_id", markReplyID.String()).Msg("support ticket mark replied failed")
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"status": "sent"})
}

// AdminListTickets GET /api/v1/admin/support/tickets
func (h *SupportHandler) AdminListTickets(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("support ticket storage is not configured"))
		return
	}
	limit, ok := queryInt(w, r, "limit", 50, 1, 100)
	if !ok {
		return
	}
	offset, ok := queryInt(w, r, "offset", 0, 0, 0)
	if !ok {
		return
	}
	rows, total, err := h.tickets.List(r.Context(), limit, offset)
	if err != nil {
		h.log.Warn().Err(err).Msg("support admin list tickets")
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to list tickets"))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, supportTicketToJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": out, "total": total})
}

// AdminGetTicket GET /api/v1/admin/support/tickets/{requestID}
func (h *SupportHandler) AdminGetTicket(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("support ticket storage is not configured"))
		return
	}
	ridStr := strings.TrimSpace(chi.URLParam(r, "requestID"))
	uid, err := uuid.Parse(ridStr)
	if err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("invalid request id"))
		return
	}
	row, err := h.tickets.GetByRequestID(r.Context(), uid)
	if err != nil {
		if errors.Is(err, supportstore.ErrNotFound) {
			http.Error(w, `{"error":"ticket not found"}`, http.StatusNotFound)
			return
		}
		writeCatalog(w, apicatalog.GatewayUpstreamUnavailable.WithMessage("failed to load ticket"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": supportTicketToJSON(*row)})
}

func (h *SupportHandler) resolveTicketRow(ctx context.Context, ticketNumber, requestID string) (*supportstore.Row, error) {
	rid := strings.TrimSpace(requestID)
	if rid != "" {
		uid, err := uuid.Parse(rid)
		if err != nil {
			return nil, err
		}
		return h.tickets.GetByRequestID(ctx, uid)
	}
	return h.tickets.GetByTicketNumber(ctx, ticketNumber)
}

func supportTicketToJSON(r supportstore.Row) map[string]any {
	m := map[string]any{
		"request_id":    r.RequestID.String(),
		"ticket_number": r.TicketNumber,
		"topic":         r.Topic,
		"message":       r.Message,
		"user_email":    r.UserEmail,
		"user_id":       r.UserID,
		"role":          r.Role,
		"booking_id":    r.BookingID,
		"payment_id":    r.PaymentID,
		"source_page":   r.SourcePage,
		"created_at":    r.CreatedAt.UTC().Format(time.RFC3339),
		"replied_by":    r.RepliedBy,
		"notify_status": r.NotifyStatus,
	}
	if r.RepliedAt != nil {
		m["replied_at"] = r.RepliedAt.UTC().Format(time.RFC3339)
	} else {
		m["replied_at"] = nil
	}
	return m
}

func supportReplyReferenceLabel(ticketNumber, requestID string) string {
	tn := strings.TrimSpace(strings.ToUpper(ticketNumber))
	if tn != "" {
		return tn
	}
	return strings.TrimSpace(requestID)
}

func (h *SupportHandler) sendEmail(ctx context.Context, p helpdeskSupportPayload) error {
	subject := fmt.Sprintf("Support ticket: %s", p.Topic)
	var b strings.Builder
	b.WriteString("New support request\n\n")
	b.WriteString("request_id: " + p.RequestID + "\n")
	b.WriteString("created_at: " + p.CreatedAt + "\n")
	b.WriteString("user_id: " + p.UserID + "\n")
	b.WriteString("role: " + p.Role + "\n")
	if p.Email != "" {
		b.WriteString("email: " + p.Email + "\n")
	}
	if p.BookingID != "" {
		b.WriteString("booking_id: " + p.BookingID + "\n")
	}
	if p.PaymentID != "" {
		b.WriteString("payment_id: " + p.PaymentID + "\n")
	}
	if p.SourcePage != "" {
		b.WriteString("source_page: " + p.SourcePage + "\n")
	}
	b.WriteString("\nmessage:\n" + p.Message + "\n")
	return h.mail.SendPlain(ctx, h.recipients, subject, b.String())
}

func (h *SupportHandler) postWebhook(body []byte) error {
	httpReq, err := http.NewRequest(http.MethodPost, h.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.webhookToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.webhookToken)
	}
	resp, err := h.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, supportMaxBodyBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("%w: unexpected helpdesk status: %d", errHelpdeskRequestFailed, resp.StatusCode)
}

// clampString truncates s to at most maxLen Unicode code points.
// Truncation always falls on a valid rune boundary, so the result is always
// well-formed UTF-8 regardless of the input encoding.
func clampString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	// Fast path: byte length fits — no rune counting needed.
	if len(s) <= maxLen {
		return s
	}
	// Walk runes until we've consumed maxLen of them, then slice at the byte
	// position of the next rune.  This is O(maxLen) in the common case where
	// the string is longer than maxLen runes.
	n := 0
	for i := range s {
		if n == maxLen {
			return s[:i]
		}
		n++
	}
	return s
}

func uniqueEmails(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, e := range in {
		v := strings.TrimSpace(strings.ToLower(e))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func supportTicketNumber(requestID string) string {
	id := strings.ReplaceAll(strings.TrimSpace(requestID), "-", "")
	if len(id) >= 12 {
		id = id[:12]
	}
	if id == "" {
		return "SUP-UNKNOWN"
	}
	return "SUP-" + strings.ToUpper(id)
}
