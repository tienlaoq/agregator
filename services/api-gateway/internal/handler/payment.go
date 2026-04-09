package handler

import (
	"io"
	"log/slog"
	"net/http"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
)

type PaymentHandler struct {
	client paymentv1.PaymentServiceClient
}

func NewPaymentHandler(client paymentv1.PaymentServiceClient) *PaymentHandler {
	return &PaymentHandler{client: client}
}

func (h *PaymentHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	slog.Info("payment webhook received",
		"content_length", len(body),
		"content_type", r.Header.Get("Content-Type"),
	)

	resp, err := h.client.HandleWebhook(r.Context(), &paymentv1.WebhookRequest{
		RawBody: body,
	})
	if err != nil {
		slog.Error("forward webhook to payment-service", "err", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if !resp.Ok {
		slog.Warn("payment-service returned ok=false for webhook")
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
