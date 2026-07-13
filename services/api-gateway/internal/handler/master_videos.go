package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// maxMasterVideoBytes is a package-local alias; canonical value in limits.VideoMaxBytes.
var maxMasterVideoBytes = limits.VideoMaxBytes

// UploadMasterVideo handles POST /owner/master/profile/videos (multipart field "video").
func (h *MasterHandler) UploadMasterVideo(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	masterID, ok := h.resolveOrCreateMasterID(w, r, userID)
	if !ok {
		return // helper already wrote the error response
	}
	if _, err := uuid.Parse(masterID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayInternalInvalidMasterId)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMasterVideoBytes+1024)
	video, ok := readVideoFromMultipart(w, r)
	if !ok {
		return
	}

	key := "masters/" + masterID + "/" + uuid.NewString() + video.Ext
	publicURL, err := h.storage.Put(r.Context(), key, video.ContentType, -1, video.Body)
	if err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayStorageFailed)
		return
	}

	resp, err := h.client.AddMasterVideo(r.Context(), &masterv1.AddMasterVideoRequest{
		UserId: userID,
		Url:    publicURL,
	})
	if err != nil {
		_ = h.storage.Delete(r.Context(), key)
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, masterProtoToJSON(resp.GetMaster()))
}

// DeleteMasterVideo handles DELETE /owner/master/profile/videos/{videoId}.
func (h *MasterHandler) DeleteMasterVideo(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	videoID := chi.URLParam(r, "videoId")
	if _, err := uuid.Parse(videoID); err != nil {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVideoId)
		return
	}
	del, err := h.client.DeleteMasterVideo(r.Context(), &masterv1.DeleteMasterVideoRequest{
		UserId:  userID,
		VideoId: videoID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	if u := del.GetDeletedUrl(); u != "" {
		if key := keyFromPublicURL(u); key != "" {
			_ = h.storage.Delete(r.Context(), key)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted_url": del.GetDeletedUrl()})
}
