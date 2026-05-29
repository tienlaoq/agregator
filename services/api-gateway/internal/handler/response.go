package handler

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

// writeJSON serialises v to a buffer first, then writes status + headers + body
// atomically. This prevents the client from receiving a partial JSON body with a
// 200 OK when json.Marshal encounters an un-serialisable value (NaN, cyclic ref).
func writeJSON(w http.ResponseWriter, code int, v any) {
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

// readJSON decodes a JSON request body into v.
//
// The body is capped at limits.JSONMaxBodyBytes (default 64 KiB) via
// http.MaxBytesReader before decoding begins. Requests that exceed the limit
// cause the connection to be closed after the 413 response; the error is
// returned as-is so callers can distinguish it from malformed JSON if needed.
//
// Callers should respond with 413 on *http.MaxBytesError and 400 on any other
// error. The convenience wrapper readJSONOrRespond handles both cases.
// readJSON decodes a JSON request body into v, capping the read at
// limits.JSONMaxBodyBytes (default 64 KiB).
//
// Returns *http.MaxBytesError when the body exceeds the limit, or a JSON
// syntax/type error otherwise. Prefer readJSONOrRespond at call sites.
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limits.JSONMaxBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// readJSONOrRespond decodes the JSON body into v and writes the appropriate
// error response on failure:
//   - 413 Request Entity Too Large when the body exceeds JSONMaxBodyBytes
//   - 400 Bad Request for malformed JSON
//
// Returns false when an error response was written; the caller must return
// immediately in that case.
//
//	if !readJSONOrRespond(w, r, &req) { return }
func readJSONOrRespond(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := readJSON(w, r, v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeCatalog(w, apicatalog.GatewayRequestBodyTooLarge)
			return false
		}
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return false
	}
	return true
}

// queryInt reads a query parameter as an integer.
//
// Behaviour:
//   - Parameter absent or empty string → returns (def, true). No error.
//   - Parameter present but not a valid integer → writes 400 GatewayRequestInvalidQuery and returns (0, false).
//   - Parsed value < min or > max (when max > 0) → same 400.
//
// The caller must return immediately when ok is false (the 400 was already written).
//
//	page, ok := queryInt(w, r, "page", 0, 0, 10000)
//	if !ok { return }
func queryInt(w http.ResponseWriter, r *http.Request, key string, def, min, max int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < min || (max > 0 && v > max) {
		writeCatalog(w, apicatalog.GatewayRequestInvalidQuery.WithMessage(
			"параметр «"+key+"» должен быть целым числом",
		))
		return 0, false
	}
	return v, true
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
