package handler

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
)

// photoReadResult holds the streaming reader and detected metadata for the
// "photo" part returned by readPhotoFromMultipart.
type photoReadResult struct {
	// Body is a streaming io.Reader over the part bytes: head ++ remainder.
	// The caller must fully consume or discard it before the request body is
	// closed. It must NOT be retained past the HTTP handler's return.
	Body        io.Reader
	ContentType string // MIME type detected from magic bytes (e.g. "image/jpeg")
	Ext         string // file extension including dot (e.g. ".jpg")
}

// readPhotoFromMultipart locates the first multipart part named "photo" in r,
// detects its image type from the first 512 bytes, and returns a streaming
// reader over the full part content (head bytes re-prepended).
//
// It uses r.MultipartReader() so the part is streamed directly from the
// network without buffering the whole body in RAM (contrast with
// ParseMultipartForm + FormFile which materialises the file in memory or a
// temp file before returning).
//
// On any error the appropriate 4xx response has already been written to w and
// ok==false is returned; the caller must return immediately.
//
// r.Body must already be wrapped in http.MaxBytesReader by the caller.
func readPhotoFromMultipart(w http.ResponseWriter, r *http.Request) (res photoReadResult, ok bool) {
	// multipart.Reader requires the boundary from Content-Type.
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || params["boundary"] == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidMultipart)
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			// Exhausted all parts without finding "photo".
			httpx.WriteCatalog(w, apicatalog.GatewayRequestPhotoFieldRequired)
			return
		}
		if err != nil {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidMultipart)
			return
		}

		if part.FormName() != "photo" {
			// Skip non-photo fields (e.g. a stray "description" text part).
			// Discard without reading into RAM.
			_, _ = io.Copy(io.Discard, part)
			part.Close()
			continue
		}

		// Read enough bytes for magic-byte detection without buffering the
		// entire file. 512 bytes is what http.DetectContentType requires.
		head := make([]byte, 512)
		n, readErr := io.ReadFull(part, head)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidFileRead)
			return
		}
		if n == 0 {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestEmptyFile)
			return
		}

		ct := http.DetectContentType(head[:n])
		ext, valid := venuePhotoExt(ct, head[:n])
		if !valid {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidImageType)
			return
		}

		// Re-prepend the head bytes so the storage writer sees the complete file.
		body := io.MultiReader(bytes.NewReader(head[:n]), part)
		return photoReadResult{Body: body, ContentType: ct, Ext: ext}, true
	}
}
