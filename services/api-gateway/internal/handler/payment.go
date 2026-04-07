package handler

import (
	"io"
	"log/slog"
	"net/http"
)

type PaymentHandler struct{}

func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{}
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

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
