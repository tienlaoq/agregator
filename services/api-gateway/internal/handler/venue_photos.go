package handler

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// maxVenuePhotoBytes is a package-local alias; canonical value in limits.PhotoMaxBytes.
var maxVenuePhotoBytes = limits.PhotoMaxBytes

var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// venuePhotoExt returns the storage extension for the given Content-Type and
// magic bytes. Both signals are checked so that browsers that lie about
// Content-Type (or don't set it) are still handled correctly.
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

// keyFromPublicURL extracts the storage object key from a public URL produced
// by storage.Uploader.Put. Works for both DiskUploader and MinioUploader URLs
// by finding the "venues/" or "masters/" segment and keeping everything from
// there. Example:
//
//	"/api/v1/uploads/venues/abc/photo.jpg" → "venues/abc/photo.jpg"
//	"https://cdn.example.com/photos/venues/abc/photo.jpg" → "venues/abc/photo.jpg"
func keyFromPublicURL(publicURL string) string {
	for _, marker := range []string{"/venues/", "/masters/"} {
		if idx := strings.LastIndex(publicURL, marker); idx >= 0 {
			return publicURL[idx+1:] // "venues/abc/photo.jpg"
		}
	}
	return ""
}

// UploadVenuePhoto handles POST /venues/{id}/photos (multipart field "photo").
func (h *VenueHandler) UploadVenuePhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxVenuePhotoBytes+1024)
	photo, ok := readPhotoFromMultipart(w, r)
	if !ok {
		return
	}

	key := "venues/" + venueID + "/" + uuid.NewString() + photo.Ext
	publicURL, err := h.storage.Put(r.Context(), key, photo.ContentType, -1, photo.Body)
	if err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayStorageFailed)
		return
	}

	resp, err := h.client.AddVenuePhoto(r.Context(), &venuev1.AddVenuePhotoRequest{
		VenueId: venueID,
		OwnerId: userID,
		Url:     publicURL,
	})
	if err != nil {
		// Best-effort cleanup: remove the uploaded object if the DB write fails.
		_ = h.storage.Delete(r.Context(), key)
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, venueToJSON(resp, true))
}

// DeleteVenuePhoto handles DELETE /venues/{id}/photos/{photoId}.
func (h *VenueHandler) DeleteVenuePhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidPhotoId)
		return
	}

	delResp, err := h.client.DeleteVenuePhoto(r.Context(), &venuev1.DeleteVenuePhotoRequest{
		VenueId: venueID,
		OwnerId: userID,
		PhotoId: photoID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	if u := delResp.GetUrl(); u != "" {
		if key := keyFromPublicURL(u); key != "" {
			_ = h.storage.Delete(r.Context(), key)
		}
	}

	v, err := h.client.GetVenue(r.Context(), &venuev1.GetVenueRequest{Id: venueID})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, venueToJSON(v, true))
}

// SetVenueCoverPhoto handles POST /venues/{id}/photos/{photoId}/cover.
func (h *VenueHandler) SetVenueCoverPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidPhotoId)
		return
	}

	resp, err := h.client.SetVenueCoverPhoto(r.Context(), &venuev1.SetVenueCoverPhotoRequest{
		VenueId: venueID,
		OwnerId: userID,
		PhotoId: photoID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, venueToJSON(resp, true))
}
