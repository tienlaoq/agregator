package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// UploadVenueHallPhoto handles POST /venues/{id}/halls/{hallId}/photos.
func (h *VenueHandler) UploadVenueHallPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	hallID := chi.URLParam(r, "hallId")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}
	if _, err := uuid.Parse(hallID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidHallId)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxVenuePhotoBytes+1024)
	photo, ok := readPhotoFromMultipart(w, r)
	if !ok {
		return
	}

	key := "venues/" + venueID + "/halls/" + hallID + "/" + uuid.NewString() + photo.Ext
	publicURL, err := h.storage.Put(r.Context(), key, photo.ContentType, -1, photo.Body)
	if err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayStorageFailed)
		return
	}

	resp, err := h.client.AddVenueHallPhoto(r.Context(), &venuev1.AddVenueHallPhotoRequest{
		VenueId: venueID,
		HallId:  hallID,
		OwnerId: userID,
		Url:     publicURL,
	})
	if err != nil {
		_ = h.storage.Delete(r.Context(), key)
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, venueToJSON(resp, true))
}

// DeleteVenueHallPhoto handles DELETE /venues/{id}/halls/{hallId}/photos/{photoId}.
func (h *VenueHandler) DeleteVenueHallPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	hallID := chi.URLParam(r, "hallId")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}
	if _, err := uuid.Parse(hallID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidHallId)
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidPhotoId)
		return
	}

	delResp, err := h.client.DeleteVenueHallPhoto(r.Context(), &venuev1.DeleteVenueHallPhotoRequest{
		VenueId: venueID,
		HallId:  hallID,
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

// SetVenueHallCoverPhoto handles POST /venues/{id}/halls/{hallId}/photos/{photoId}/cover.
func (h *VenueHandler) SetVenueHallCoverPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	hallID := chi.URLParam(r, "hallId")
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}
	if _, err := uuid.Parse(hallID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidHallId)
		return
	}
	if _, err := uuid.Parse(photoID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidPhotoId)
		return
	}

	resp, err := h.client.SetVenueHallCoverPhoto(r.Context(), &venuev1.SetVenueHallCoverPhotoRequest{
		VenueId: venueID,
		HallId:  hallID,
		OwnerId: userID,
		PhotoId: photoID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, venueToJSON(resp, true))
}
