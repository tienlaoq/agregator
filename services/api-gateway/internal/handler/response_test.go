package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── readJSONOrRespond ────────────────────────────────────────────────────────

func TestReadJSONOrRespond_Valid(t *testing.T) {
	var dst struct{ Name string }
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alice"}`))
	rec := httptest.NewRecorder()

	ok := readJSONOrRespond(rec, req, &dst)

	require.True(t, ok)
	assert.Equal(t, "alice", dst.Name)
	assert.Equal(t, http.StatusOK, rec.Code) // no response written
}

func TestReadJSONOrRespond_MalformedJSON_400(t *testing.T) {
	var dst struct{ Name string }
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{not valid json`))
	rec := httptest.NewRecorder()

	ok := readJSONOrRespond(rec, req, &dst)

	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "GATEWAY.REQUEST.INVALID_BODY")
}

func TestReadJSONOrRespond_BodyTooLarge_413(t *testing.T) {
	var dst struct{ Name string }
	// Build a body larger than JSONMaxBodyBytes (64 KiB)
	large := `{"name":"` + strings.Repeat("x", 70*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(large))
	rec := httptest.NewRecorder()

	ok := readJSONOrRespond(rec, req, &dst)

	assert.False(t, ok)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "GATEWAY.REQUEST.BODY_TOO_LARGE")
}

func TestReadJSONOrRespond_ExactLimit_OK(t *testing.T) {
	// A valid JSON body just at the limit should succeed.
	// Build payload exactly at 64 KiB by padding a string field.
	// JSONMaxBodyBytes = 64*1024 = 65536
	const limit = 64 * 1024
	prefix := `{"name":"`
	suffix := `"}`
	padding := strings.Repeat("a", limit-len(prefix)-len(suffix))
	body := prefix + padding + suffix

	var dst struct{ Name string }
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	ok := readJSONOrRespond(rec, req, &dst)

	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, rec.Code)
}
