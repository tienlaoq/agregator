package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxMasterPhotoBytes is a package-local alias; canonical value in limits.PhotoMaxBytes.
var maxMasterPhotoBytes = limits.PhotoMaxBytes

// resolveOrCreateMasterID returns the caller's master profile id, lazily creating
// the profile when it does not exist yet. This lets a master add photos at any
// time — even before they explicitly fill in and save the profile. On any failure
// it writes the HTTP error response and returns ok=false.
//
// The display name for the implicit profile is taken from user-service; a generic
// fallback is used if it cannot be resolved (the master can rename it later). When
// no user client is wired the handler keeps the legacy "create profile first"
// behaviour.
func (h *MasterHandler) resolveOrCreateMasterID(w http.ResponseWriter, r *http.Request, userID string) (string, bool) {
	prof, err := h.client.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: userID})
	if err == nil {
		if id := prof.GetMaster().GetId(); id != "" {
			return id, true
		}
	} else if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		grpcErrorToHTTP(w, err)
		return "", false
	}

	// No profile yet — create one lazily.
	if h.userClient == nil {
		writeCatalog(w, apicatalog.GatewayMasterNotCreated)
		return "", false
	}
	name := "Мастер"
	if u, uerr := h.userClient.GetUser(r.Context(), &userv1.GetUserRequest{Id: userID}); uerr == nil {
		if n := strings.TrimSpace(u.GetName()); n != "" {
			name = n
		}
	}
	created, cerr := h.client.CreateMyProfile(r.Context(), &masterv1.CreateMyProfileRequest{
		UserId:      userID,
		DisplayName: name,
	})
	if cerr != nil {
		// A concurrent request may have created it between our GET and CREATE.
		if st, ok := status.FromError(cerr); ok && st.Code() == codes.AlreadyExists {
			if again, gerr := h.client.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: userID}); gerr == nil {
				if id := again.GetMaster().GetId(); id != "" {
					return id, true
				}
			}
		}
		grpcErrorToHTTP(w, cerr)
		return "", false
	}
	if id := created.GetMaster().GetId(); id != "" {
		return id, true
	}
	writeCatalog(w, apicatalog.GatewayMasterNotCreated)
	return "", false
}

// UploadMasterPhoto handles POST /owner/master/profile/photos (multipart field "photo").
func (h *MasterHandler) UploadMasterPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}

	masterID, ok := h.resolveOrCreateMasterID(w, r, userID)
	if !ok {
		return // helper already wrote the error response
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
