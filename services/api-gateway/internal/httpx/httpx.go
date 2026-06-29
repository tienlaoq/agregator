// Package httpx holds domain-agnostic HTTP helpers shared by every gateway
// handler: JSON request/response coding, query-param parsing, and gRPC→HTTP
// error mapping.
//
// It is a leaf package — it depends only on apicatalog and limits, never on
// the handler package. This is deliberate: keeping these helpers out of
// package handler lets per-domain handler subpackages import them without
// creating an import cycle back through the handler root (see docs/TECH_DEBT.md,
// the api-gateway god-package entry).
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"google.golang.org/grpc/status"
)

// WriteJSON serialises v to a buffer first, then writes status + headers + body
// atomically. This prevents the client from receiving a partial JSON body with a
// 200 OK when json.Marshal encounters an un-serialisable value (NaN, cyclic ref).
func WriteJSON(w http.ResponseWriter, code int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal serialisation error"}`, http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

// ReadJSON decodes a JSON request body into v, capping the read at
// limits.JSONMaxBodyBytes (default 64 KiB) via http.MaxBytesReader before
// decoding begins.
//
// Returns *http.MaxBytesError when the body exceeds the limit, or a JSON
// syntax/type error otherwise. Prefer ReadJSONOrRespond at call sites.
func ReadJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limits.JSONMaxBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// ReadJSONOrRespond decodes the JSON body into v and writes the appropriate
// error response on failure:
//   - 413 Request Entity Too Large when the body exceeds JSONMaxBodyBytes
//   - 400 Bad Request for malformed JSON
//
// Returns false when an error response was written; the caller must return
// immediately in that case.
//
//	if !httpx.ReadJSONOrRespond(w, r, &req) { return }
func ReadJSONOrRespond(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := ReadJSON(w, r, v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			WriteCatalog(w, apicatalog.GatewayRequestBodyTooLarge)
			return false
		}
		WriteCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return false
	}
	return true
}

// QueryInt reads a query parameter as an integer.
//
// Behaviour:
//   - Parameter absent or empty string → returns (def, true). No error.
//   - Parameter present but not a valid integer → writes 400 GatewayRequestInvalidQuery and returns (0, false).
//   - Parsed value < min or > max (when max > 0) → same 400.
//
// The caller must return immediately when ok is false (the 400 was already written).
//
//	page, ok := httpx.QueryInt(w, r, "page", 0, 0, 10000)
//	if !ok { return }
func QueryInt(w http.ResponseWriter, r *http.Request, key string, def, min, max int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < min || (max > 0 && v > max) {
		WriteCatalog(w, apicatalog.GatewayRequestInvalidQuery.WithMessage(
			"параметр «"+key+"» должен быть целым числом",
		))
		return 0, false
	}
	return v, true
}

// WriteCatalog writes a catalogued API error to w.
func WriteCatalog(w http.ResponseWriter, e apicatalog.Entry) {
	e.Write(w)
}

// GRPCErrorToHTTP maps a gRPC status error onto the matching catalogued HTTP
// response, falling back to GatewayUpstreamUnknown for unmapped codes.
func GRPCErrorToHTTP(w http.ResponseWriter, err error) {
	if err == nil {
		WriteCatalog(w, apicatalog.GatewayUpstreamUnknown.WithMessage("unknown error"))
		return
	}
	st := status.Convert(err)
	if ent, ok := apicatalog.FromGRPC(st.Code()); ok {
		msg := strings.TrimSpace(st.Message())
		if msg == "" {
			msg = strings.TrimSpace(err.Error())
		}
		WriteCatalog(w, ent.WithMessage(msg))
		return
	}
	msg := strings.TrimSpace(st.Message())
	if msg == "" {
		msg = strings.TrimSpace(err.Error())
	}
	WriteCatalog(w, apicatalog.GatewayUpstreamUnknown.WithMessage(msg))
}
