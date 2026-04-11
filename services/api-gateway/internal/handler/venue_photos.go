package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

const maxVenuePhotoBytes = 5 << 20 // 5 MiB

var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// venuePhotoExt picks storage extension from Content-Type and/or raw magic bytes.
// Avoids relying on Seek() on multipart.File (not always supported in browsers).
func venuePhotoExt(contentType string, head []byte) (ext string, ok bool) {
	switch contentType {
	case "image/jpeg", "image/pjpeg":
		return ".jpg", true
	case "image/png", "image/x-png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	}
	if len(head) < 3 {
		return "", false
	}
	if head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF {
		return ".jpg", true
	}
	if len(head) >= len(pngMagic) && bytes.Equal(head[:len(pngMagic)], pngMagic) {
		return ".png", true
	}
	if len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")) {
		return ".webp", true
	}
	return "", false
}

// venueUploadDiskSuffix maps request path to a path under uploadRoot (venues/{venueId}/{file}).
// Accepts full gateway paths and /uploads/... as seen on chi sub-routers.
func venueUploadDiskSuffix(urlPath string) string {
	p := strings.TrimPrefix(strings.TrimSpace(urlPath), "/")
	switch {
	case strings.HasPrefix(p, "api/v1/uploads/"):
		p = strings.TrimPrefix(p, "api/v1/uploads/")
	case strings.HasPrefix(p, "uploads/"):
		p = strings.TrimPrefix(p, "uploads/")
	default:
		return ""
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" || strings.Contains(p, "..") {
		return ""
	}
	return p
}

// ServeVenueUploads serves files from uploadRoot matching URL /api/v1/uploads/venues/{venueId}/{file}.
func ServeVenueUploads(uploadRoot string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		suffix := venueUploadDiskSuffix(r.URL.Path)
		if suffix == "" {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(uploadRoot, filepath.FromSlash(suffix))
		cleanRoot, err := filepath.Abs(uploadRoot)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		cleanFile, err := filepath.Abs(full)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !strings.HasPrefix(cleanFile, cleanRoot+string(os.PathSeparator)) && cleanFile != cleanRoot {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, full)
	})
}

func (h *VenueHandler) absPathFromPublicURL(publicPath string) (string, error) {
	p := strings.TrimSpace(publicPath)
	const prefix = "/api/v1/uploads/venues/"
	if !strings.HasPrefix(p, prefix) {
		return "", fmt.Errorf("invalid public path")
	}
	rest := strings.TrimPrefix(p, prefix)
	parts := strings.Split(rest, "/")
	var rel []string
	switch {
	case len(parts) == 2:
		vid, fname := parts[0], parts[1]
		if _, err := uuid.Parse(vid); err != nil {
			return "", fmt.Errorf("invalid venue id in path")
		}
		if fname == "" || strings.Contains(fname, "..") {
			return "", fmt.Errorf("invalid filename")
		}
		rel = []string{"venues", vid, fname}
	case len(parts) == 4 && parts[1] == "halls":
		vid, hid, fname := parts[0], parts[2], parts[3]
		if _, err := uuid.Parse(vid); err != nil {
			return "", fmt.Errorf("invalid venue id in path")
		}
		if _, err := uuid.Parse(hid); err != nil {
			return "", fmt.Errorf("invalid hall id in path")
		}
		if fname == "" || strings.Contains(fname, "..") {
			return "", fmt.Errorf("invalid filename")
		}
		rel = []string{"venues", vid, "halls", hid, fname}
	default:
		return "", fmt.Errorf("invalid public path")
	}
	full := filepath.Join(h.uploadRoot, filepath.Join(rel...))
	cleanRoot, err := filepath.Abs(h.uploadRoot)
	if err != nil {
		return "", err
	}
	cleanFile, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(cleanFile, cleanRoot+string(os.PathSeparator)) && cleanFile != cleanRoot {
		return "", fmt.Errorf("path traversal")
	}
	return full, nil
}

func (h *VenueHandler) removeStoredUpload(publicPath string) {
	abs, err := h.absPathFromPublicURL(publicPath)
	if err != nil {
		return
	}
	_ = os.Remove(abs)
}

// UploadVenuePhoto expects multipart field "photo" (JPEG, PNG or WebP).
func (h *VenueHandler) UploadVenuePhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(venueID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid venue id"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxVenuePhotoBytes+1024)
	if err := r.ParseMultipartForm(maxVenuePhotoBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form or file too large"})
		return
	}
	file, _, err := r.FormFile("photo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "field \"photo\" is required"})
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read file"})
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty file"})
		return
	}
	ct := http.DetectContentType(head[:n])
	ext, ok := venuePhotoExt(ct, head[:n])
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only JPEG, PNG and WebP are allowed"})
		return
	}

	body := io.MultiReader(bytes.NewReader(head[:n]), file)

	fname := uuid.NewString() + ext
	dir := filepath.Join(h.uploadRoot, "venues", venueID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store file"})
		return
	}
	fullPath := filepath.Join(dir, fname)
	dst, err := os.Create(fullPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store file"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, body); err != nil {
		_ = os.Remove(fullPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store file"})
		return
	}

	publicURL := "/api/v1/uploads/venues/" + venueID + "/" + fname
	resp, err := h.client.AddVenuePhoto(r.Context(), &venuev1.AddVenuePhotoRequest{
		VenueId: venueID,
		OwnerId: userID,
		Url:     publicURL,
	})
	if err != nil {
		_ = os.Remove(fullPath)
		grpcErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, venueToJSON(resp, true))
}

// DeleteVenuePhoto removes DB row and file on disk, returns updated venue (owner view).
func (h *VenueHandler) DeleteVenuePhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid venue id"})
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid photo id"})
		return
	}

	delResp, err := h.client.DeleteVenuePhoto(r.Context(), &venuev1.DeleteVenuePhotoRequest{
		VenueId: venueID,
		OwnerId: userID,
		PhotoId: photoID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	if u := delResp.GetUrl(); u != "" {
		h.removeStoredUpload(u)
	}

	v, err := h.client.GetVenue(r.Context(), &venuev1.GetVenueRequest{Id: venueID})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, venueToJSON(v, true))
}

// SetVenueCoverPhoto marks one photo as cover (others cleared).
func (h *VenueHandler) SetVenueCoverPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid venue id"})
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid photo id"})
		return
	}

	resp, err := h.client.SetVenueCoverPhoto(r.Context(), &venuev1.SetVenueCoverPhotoRequest{
		VenueId: venueID,
		OwnerId: userID,
		PhotoId: photoID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, venueToJSON(resp, true))
}
