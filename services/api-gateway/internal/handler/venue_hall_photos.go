package handler

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// UploadVenueHallPhoto stores under venues/{venueId}/halls/{hallId}/ and registers the row.
func (h *VenueHandler) UploadVenueHallPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")
	hallID := chi.URLParam(r, "hallId")
	if _, err := uuid.Parse(venueID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid venue id"})
		return
	}
	if _, err := uuid.Parse(hallID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid hall id"})
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
	dir := filepath.Join(h.uploadRoot, "venues", venueID, "halls", hallID)
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

	publicURL := "/api/v1/uploads/venues/" + venueID + "/halls/" + hallID + "/" + fname
	resp, err := h.client.AddVenueHallPhoto(r.Context(), &venuev1.AddVenueHallPhotoRequest{
		VenueId: venueID,
		HallId:  hallID,
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

func (h *VenueHandler) DeleteVenueHallPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")
	hallID := chi.URLParam(r, "hallId")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid venue id"})
		return
	}
	if _, err := uuid.Parse(hallID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid hall id"})
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid photo id"})
		return
	}

	delResp, err := h.client.DeleteVenueHallPhoto(r.Context(), &venuev1.DeleteVenueHallPhotoRequest{
		VenueId: venueID,
		HallId:  hallID,
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

func (h *VenueHandler) SetVenueHallCoverPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	venueID := chi.URLParam(r, "id")
	hallID := chi.URLParam(r, "hallId")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid venue id"})
		return
	}
	if _, err := uuid.Parse(hallID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid hall id"})
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid photo id"})
		return
	}

	resp, err := h.client.SetVenueHallCoverPhoto(r.Context(), &venuev1.SetVenueHallCoverPhotoRequest{
		VenueId: venueID,
		HallId:  hallID,
		OwnerId: userID,
		PhotoId: photoID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, venueToJSON(resp, true))
}
