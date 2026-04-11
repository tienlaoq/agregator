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
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxMasterPhotoBytes = 5 << 20 // 5 MiB

func absMasterUploadPath(uploadRoot, publicPath string) (string, error) {
	p := strings.TrimSpace(publicPath)
	const prefix = "/api/v1/uploads/masters/"
	if !strings.HasPrefix(p, prefix) {
		return "", fmt.Errorf("invalid path")
	}
	rest := strings.TrimPrefix(p, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid path")
	}
	mid, fname := parts[0], parts[1]
	if _, err := uuid.Parse(mid); err != nil {
		return "", fmt.Errorf("invalid path")
	}
	if fname == "" || strings.Contains(fname, "..") {
		return "", fmt.Errorf("invalid path")
	}
	full := filepath.Join(uploadRoot, "masters", mid, fname)
	cleanRoot, err := filepath.Abs(uploadRoot)
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

func (h *MasterHandler) removeMasterStoredUpload(publicPath string) {
	abs, err := absMasterUploadPath(h.uploadRoot, publicPath)
	if err != nil {
		return
	}
	_ = os.Remove(abs)
}

// UploadMasterPhoto POST /api/v1/owner/master/profile/photos — multipart field "photo".
func (h *MasterHandler) UploadMasterPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	prof, err := h.client.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: userID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "сначала создайте профиль мастера"})
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}
	masterID := prof.GetMaster().GetId()
	if masterID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "сначала создайте профиль мастера"})
		return
	}
	if _, err := uuid.Parse(masterID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid master id"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMasterPhotoBytes+1024)
	if err := r.ParseMultipartForm(maxMasterPhotoBytes); err != nil {
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
	dir := filepath.Join(h.uploadRoot, "masters", masterID)
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

	publicURL := "/api/v1/uploads/masters/" + masterID + "/" + fname
	resp, err := h.client.AddMasterPhoto(r.Context(), &masterv1.AddMasterPhotoRequest{
		UserId: userID,
		Url:    publicURL,
	})
	if err != nil {
		_ = os.Remove(fullPath)
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, masterProtoToJSON(resp.GetMaster()))
}

// DeleteMasterPhoto DELETE /api/v1/owner/master/profile/photos/{photoId}
func (h *MasterHandler) DeleteMasterPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(photoID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid photo id"})
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
		h.removeMasterStoredUpload(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted_url": del.GetDeletedUrl()})
}

// SetMasterCoverPhoto POST /api/v1/owner/master/profile/photos/{photoId}/cover
func (h *MasterHandler) SetMasterCoverPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	photoID := chi.URLParam(r, "photoId")
	if _, err := uuid.Parse(photoID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid photo id"})
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
