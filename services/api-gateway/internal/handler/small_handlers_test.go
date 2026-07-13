package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
)

func TestHealthCheck(t *testing.T) {
	rr := httptest.NewRecorder()
	HealthCheck(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q", body["status"])
	}
}

func TestListSBPBanks(t *testing.T) {
	rr := httptest.NewRecorder()
	ListSBPBanks(rr, httptest.NewRequest(http.MethodGet, "/api/v1/sbp/banks", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Banks []sbpBank `json:"banks"`
		Stub  bool      `json:"stub"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Stub {
		t.Fatal("expected stub=true")
	}
	if len(body.Banks) != len(stubSBPBanks) {
		t.Fatalf("banks len = %d, want %d", len(body.Banks), len(stubSBPBanks))
	}
	for _, b := range body.Banks {
		if b.ID == "" || b.Name == "" {
			t.Fatalf("incomplete bank entry: %+v", b)
		}
	}
}

func TestVenuePhotoExt(t *testing.T) {
	pngHead := append([]byte(nil), pngMagic...)
	tests := []struct {
		name        string
		contentType string
		head        []byte
		wantExt     string
		wantOK      bool
	}{
		{"jpeg by content-type", "image/jpeg", nil, ".jpg", true},
		{"pjpeg by content-type", "image/pjpeg", nil, ".jpg", true},
		{"png by content-type", "image/png", nil, ".png", true},
		{"x-png by content-type", "image/x-png", nil, ".png", true},
		{"webp by content-type", "image/webp", nil, ".webp", true},
		{"jpeg by magic", "application/octet-stream", []byte{0xFF, 0xD8, 0xFF, 0x00}, ".jpg", true},
		{"png by magic", "application/octet-stream", pngHead, ".png", true},
		{"webp by magic", "application/octet-stream", append([]byte("RIFF????"), []byte("WEBP")...), ".webp", true},
		{"too short", "application/octet-stream", []byte{0x00}, "", false},
		{"unknown", "text/plain", []byte("hello world here"), "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ext, ok := venuePhotoExt(tc.contentType, tc.head)
			if ext != tc.wantExt || ok != tc.wantOK {
				t.Fatalf("got (%q,%v), want (%q,%v)", ext, ok, tc.wantExt, tc.wantOK)
			}
		})
	}
}

func TestVideoExt(t *testing.T) {
	// ftyp box: 4-byte size, then "ftyp" at offset 4.
	mp4Head := append([]byte{0x00, 0x00, 0x00, 0x18}, []byte("ftypmp42")...)
	// QuickTime .mov: same ftyp box, major brand "qt  ".
	movHead := append([]byte{0x00, 0x00, 0x00, 0x14}, []byte("ftypqt  ")...)
	webmHead := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00}
	tests := []struct {
		name        string
		contentType string
		head        []byte
		wantExt     string
		wantOK      bool
	}{
		{"mp4 by content-type", "video/mp4", nil, ".mp4", true},
		{"webm by content-type", "video/webm", nil, ".webm", true},
		{"mp4 by magic", "application/octet-stream", mp4Head, ".mp4", true},
		{"mov rejected by brand", "application/octet-stream", movHead, "", false},
		{"webm by magic", "application/octet-stream", webmHead, ".webm", true},
		{"image rejected", "image/jpeg", []byte{0xFF, 0xD8, 0xFF, 0x00}, "", false},
		{"too short", "application/octet-stream", []byte{0x00}, "", false},
		{"unknown", "text/plain", []byte("hello world here"), "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ext, ok := videoExt(tc.contentType, tc.head)
			if ext != tc.wantExt || ok != tc.wantOK {
				t.Fatalf("got (%q,%v), want (%q,%v)", ext, ok, tc.wantExt, tc.wantOK)
			}
		})
	}
}

// buildMultipart returns a request body + content-type header carrying the
// given parts (field name -> content bytes).
func buildMultipart(t *testing.T, parts []struct {
	field   string
	content []byte
}) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for _, p := range parts {
		fw, err := mw.CreateFormField(p.field)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := fw.Write(p.content); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return buf, mw.FormDataContentType()
}

func TestReadPhotoFromMultipart_ValidPNG(t *testing.T) {
	png := append(append([]byte(nil), pngMagic...), bytes.Repeat([]byte{0x00}, 32)...)
	buf, ct := buildMultipart(t, []struct {
		field   string
		content []byte
	}{
		{"description", []byte("ignored text part")},
		{"photo", png},
	})
	r := httptest.NewRequest(http.MethodPost, "/upload", buf)
	r.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	res, ok := readPhotoFromMultipart(rr, r)
	if !ok {
		t.Fatalf("expected ok, got status %d body %s", rr.Code, rr.Body.String())
	}
	if res.Ext != ".png" || res.ContentType != "image/png" {
		t.Fatalf("res = %+v", res)
	}
	got, _ := io.ReadAll(res.Body)
	if !bytes.Equal(got, png) {
		t.Fatalf("body mismatch: head bytes not re-prepended (len %d vs %d)", len(got), len(png))
	}
}

func TestReadPhotoFromMultipart_NoBoundary(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader([]byte("x")))
	r.Header.Set("Content-Type", "multipart/form-data") // missing boundary
	rr := httptest.NewRecorder()
	if _, ok := readPhotoFromMultipart(rr, r); ok {
		t.Fatal("expected failure")
	}
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestInvalidMultipart.Code {
		t.Fatalf("code = %s", got)
	}
}

func TestReadPhotoFromMultipart_PhotoFieldMissing(t *testing.T) {
	buf, ct := buildMultipart(t, []struct {
		field   string
		content []byte
	}{
		{"description", []byte("no photo here")},
	})
	r := httptest.NewRequest(http.MethodPost, "/upload", buf)
	r.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	if _, ok := readPhotoFromMultipart(rr, r); ok {
		t.Fatal("expected failure")
	}
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestPhotoFieldRequired.Code {
		t.Fatalf("code = %s", got)
	}
}

func TestReadPhotoFromMultipart_EmptyFile(t *testing.T) {
	buf, ct := buildMultipart(t, []struct {
		field   string
		content []byte
	}{
		{"photo", nil},
	})
	r := httptest.NewRequest(http.MethodPost, "/upload", buf)
	r.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	if _, ok := readPhotoFromMultipart(rr, r); ok {
		t.Fatal("expected failure")
	}
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestEmptyFile.Code {
		t.Fatalf("code = %s", got)
	}
}

func TestReadPhotoFromMultipart_InvalidImageType(t *testing.T) {
	buf, ct := buildMultipart(t, []struct {
		field   string
		content []byte
	}{
		{"photo", []byte("this is plain text, not an image at all")},
	})
	r := httptest.NewRequest(http.MethodPost, "/upload", buf)
	r.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	if _, ok := readPhotoFromMultipart(rr, r); ok {
		t.Fatal("expected failure")
	}
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestInvalidImageType.Code {
		t.Fatalf("code = %s", got)
	}
}
