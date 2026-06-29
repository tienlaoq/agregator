package handler

import (
	"net/http"

	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
