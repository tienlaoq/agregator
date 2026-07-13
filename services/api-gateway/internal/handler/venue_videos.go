package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// maxVenueVideoBytes is a package-local alias; canonical value in limits.VideoMaxBytes.
var maxVenueVideoBytes = limits.VideoMaxBytes

// UploadVenueVideo handles POST /venues/{id}/videos (multipart field "video").
func (h *VenueHandler) UploadVenueVideo(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, maxVenueVideoBytes+1024)
	video, ok := readVideoFromMultipart(w, r)
	if !ok {
		return
	}

	key := "venues/" + venueID + "/" + uuid.NewString() + video.Ext
	publicURL, err := h.storage.Put(r.Context(), key, video.ContentType, -1, video.Body)
	if err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayStorageFailed)
		return
	}

	resp, err := h.client.AddVenueVideo(r.Context(), &venuev1.AddVenueVideoRequest{
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

// DeleteVenueVideo handles DELETE /venues/{id}/videos/{videoId}.
func (h *VenueHandler) DeleteVenueVideo(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "id")
	videoID := chi.URLParam(r, "videoId")
	if _, err := uuid.Parse(venueID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return
	}
	if _, err := uuid.Parse(videoID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVideoId)
		return
	}

	delResp, err := h.client.DeleteVenueVideo(r.Context(), &venuev1.DeleteVenueVideoRequest{
		VenueId: venueID,
		OwnerId: userID,
		VideoId: videoID,
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
