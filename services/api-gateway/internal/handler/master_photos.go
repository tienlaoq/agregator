package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxMasterPhotoBytes is a package-local alias; canonical value in limits.PhotoMaxBytes.
var maxMasterPhotoBytes = limits.PhotoMaxBytes

// UploadMasterPhoto handles POST /owner/master/profile/photos (multipart field "photo").
func (h *MasterHandler) UploadMasterPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	prof, err := h.client.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: userID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			writeCatalog(w, apicatalog.GatewayMasterNotCreated)
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}
	masterID := prof.GetMaster().GetId()
	if masterID == "" {
		writeCatalog(w, apicatalog.GatewayMasterNotCreated)
		return
	}
	if _, err := uuid.Parse(masterID); err != nil {
		writeCatalog(w, apicatalog.GatewayInternalInvalidMasterId)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMasterPhotoBytes+1024)
	photo, ok := readPhotoFromMultipart(w, r)
	if !ok {
		return
	}

	key := "masters/" + masterID + "/" + uuid.NewString() + photo.Ext
	publicURL, err := h.storage.Put(r.Context(), key, photo.ContentType, -1, photo.Body)
	if err != nil {
		writeCatalog(w, apicatalog.GatewayStorageFailed)
		return
	}

	resp, err := h.client.AddMasterPhoto(r.Context(), &masterv1.AddMasterPhotoRequest{
		UserId: userID,
		Url:    publicURL,
	})
	if err != nil {
		_ = h.storage.Delete(r.Context(), key)
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, masterProtoToJSON(resp.GetMaster()))
}

// DeleteMasterPhoto handles DELETE /owner/master/profile/photos/{photoId}.
func (h *MasterHandler) DeleteMasterPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(photoID); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidPhotoId)
		return
	}
	del, err := h.client.DeleteMasterPhoto(r.Context(), &masterv1.DeleteMasterPhotoRequest{
		UserId:  userID,
		PhotoId: photoID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	if u := del.GetDeletedUrl(); u != "" {
		if key := keyFromPublicURL(u); key != "" {
			_ = h.storage.Delete(r.Context(), key)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted_url": del.GetDeletedUrl()})
}

// SetMasterCoverPhoto handles POST /owner/master/profile/photos/{photoId}/cover.
func (h *MasterHandler) SetMasterCoverPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(photoID); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidPhotoId)
		return
	}
	resp, err := h.client.SetMasterCoverPhoto(r.Context(), &masterv1.SetMasterCoverPhotoRequest{
		UserId:  userID,
		PhotoId: photoID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, masterProtoToJSON(resp.GetMaster()))
}
