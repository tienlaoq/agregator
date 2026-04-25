package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"google.golang.org/grpc/status"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func writeCatalog(w http.ResponseWriter, e apicatalog.Entry) {
	e.Write(w)
}

func grpcErrorToHTTP(w http.ResponseWriter, err error) {
	if err == nil {
		writeCatalog(w, apicatalog.GatewayUpstreamUnknown.WithMessage("unknown error"))
		return
	}
	st := status.Convert(err)
	if ent, ok := apicatalog.FromGRPC(st.Code()); ok {
		msg := strings.TrimSpace(st.Message())
		if msg == "" {
			msg = strings.TrimSpace(err.Error())
		}
		writeCatalog(w, ent.WithMessage(msg))
		return
	}
	msg := strings.TrimSpace(st.Message())
	if msg == "" {
		msg = strings.TrimSpace(err.Error())
	}
	writeCatalog(w, apicatalog.GatewayUpstreamUnknown.WithMessage(msg))
}
