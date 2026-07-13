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
// detects its image type, and returns a streaming reader over the full part
// content. See readMediaFromMultipart for streaming/error semantics.
func readPhotoFromMultipart(w http.ResponseWriter, r *http.Request) (res photoReadResult, ok bool) {
	return readMediaFromMultipart(w, r, "photo", venuePhotoExt,
		apicatalog.GatewayRequestPhotoFieldRequired, apicatalog.GatewayRequestInvalidImageType)
}

// readVideoFromMultipart is the video counterpart of readPhotoFromMultipart:
// it locates the part named "video" and validates it as MP4 or WebM.
func readVideoFromMultipart(w http.ResponseWriter, r *http.Request) (res photoReadResult, ok bool) {
	return readMediaFromMultipart(w, r, "video", videoExt,
		apicatalog.GatewayRequestVideoFieldRequired, apicatalog.GatewayRequestInvalidVideoType)
}

// readMediaFromMultipart locates the first multipart part named field in r,
// detects its type from the first 512 bytes via detect, and returns a streaming
// reader over the full part content (head bytes re-prepended).
//
// It uses r.MultipartReader() so the part is streamed directly from the
// network without buffering the whole body in RAM (contrast with
// ParseMultipartForm + FormFile which materialises the file in memory or a
// temp file before returning).
//
// On any error the appropriate 4xx response has already been written to w and
// ok==false is returned; the caller must return immediately. missingField is
// written when no part matches field; invalidType when detect rejects the head.
//
// r.Body must already be wrapped in http.MaxBytesReader by the caller.
func readMediaFromMultipart(
	w http.ResponseWriter, r *http.Request, field string,
	detect func(contentType string, head []byte) (string, bool),
	missingField, invalidType apicatalog.Entry,
) (res photoReadResult, ok bool) {
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
			// Exhausted all parts without finding the target field.
			httpx.WriteCatalog(w, missingField)
			return
		}
		if err != nil {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidMultipart)
			return
		}

		if part.FormName() != field {
			// Skip unrelated fields (e.g. a stray "description" text part).
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
		ext, valid := detect(ct, head[:n])
		if !valid {
			httpx.WriteCatalog(w, invalidType)
			return
		}

		// Re-prepend the head bytes so the storage writer sees the complete file.
		body := io.MultiReader(bytes.NewReader(head[:n]), part)
		return photoReadResult{Body: body, ContentType: ct, Ext: ext}, true
	}
}

// videoExt returns the storage extension for a video part. Both the detected
// Content-Type and the magic bytes are checked (browsers may mislabel or omit
// Content-Type). Only web-playable containers are accepted: MP4 and WebM.
func videoExt(contentType string, head []byte) (ext string, ok bool) {
	switch contentType {
	case "video/mp4":
		return ".mp4", true
	case "video/webm":
		return ".webm", true
	}
	// MP4 / ISO-BMFF: bytes 4..8 are the "ftyp" box type. QuickTime (.mov)
	// shares this box, so inspect the major brand at bytes 8..12 and reject the
	// "qt  " brand — MOV-in-.mp4 is not reliably web-playable.
	if len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) {
		if bytes.Equal(head[8:12], []byte("qt  ")) {
			return "", false
		}
		return ".mp4", true
	}
	// WebM / Matroska: EBML header magic 1A 45 DF A3.
	if len(head) >= 4 && bytes.Equal(head[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return ".webm", true
	}
	return "", false
}
