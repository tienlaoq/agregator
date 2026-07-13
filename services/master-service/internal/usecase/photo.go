package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

// masterPhotoKeyRe matches the storage object key for a master photo after the
// key is extracted from the public URL and cleaned.
// Format: masters/<masterID>/<filename>
// Filename may contain letters, digits, dots, underscores and hyphens only —
// no path separators, no percent signs, no unicode tricks.
var masterPhotoKeyRe = regexp.MustCompile(`^masters/[0-9a-f-]{36}/[a-zA-Z0-9._-]+$`)

// extractMasterPhotoKey extracts the storage object key from a public URL
// produced by either DiskUploader or MinioUploader:
//
//	"/api/v1/uploads/masters/<id>/photo.jpg" → "masters/<id>/photo.jpg"
//	"https://cdn.example.com/photos/masters/<id>/photo.jpg" → "masters/<id>/photo.jpg"
//
// The URL path is parsed structurally by splitting on "/" and locating the
// first "masters" segment. This avoids LastIndex ambiguity where a crafted URL
// with multiple "/masters/" occurrences could cause key extraction to start
// from the wrong (attacker-controlled) segment.
//
// Only exactly two segments after "masters" are captured (UUID + filename),
// so any trailing path components are silently dropped rather than passed
// through to the regexp and prefix checks.
//
// Returns ("", false) if the URL is unparseable or does not contain a
// "masters" segment followed by at least two more segments.
func extractMasterPhotoKey(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	// Operate on the URL path only — ignore scheme, host, query, fragment.
	// path.Clean normalises duplicate slashes and dots before splitting.
	segments := strings.Split(strings.Trim(path.Clean(parsed.Path), "/"), "/")
	for i, seg := range segments {
		// Require exactly three trailing segments: "masters", UUID, filename.
		// Allowing extra segments (e.g. "masters/<uuid>/dir/photo.jpg") would
		// silently drop everything past the filename and accept a URL that
		// targets a different storage object than what was provided.
		if seg == "masters" && i+3 == len(segments) {
			return "masters/" + segments[i+1] + "/" + segments[i+2], true
		}
	}
	return "", false
}

// normalizeMasterPhotoURL validates rawURL and returns a canonical URL that
// should be persisted. The caller must use the returned value — not rawURL —
// so that percent-encoding, double-slashes, and query strings are stripped
// before the URL reaches the database.
//
// Canonicalisation steps:
//  1. Parse the URL to separate scheme+host from path.
//  2. Extract the storage key via structural path segment matching
//     (extractMasterPhotoKey), which takes the first "masters" segment and
//     exactly two following segments — preventing double-marker attacks.
//  3. URL-decode the key to catch %2e%2e / %2f / %5c variants.
//  4. path.Clean to collapse any remaining ".." or duplicate slashes.
//  5. Allowlist regexp: ^masters/<uuid>/<safe-filename>$.
//  6. Owner check: cleaned key must start with "masters/<masterID>/".
//  7. Reconstruct a clean URL: scheme://host/<path-prefix>/<cleaned-key>.
//     rawURL is never stored; only the reconstructed canonical form is.
func normalizeMasterPhotoURL(masterID uuid.UUID, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("photo url is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("photo url is not a valid URL")
	}

	key, ok := extractMasterPhotoKey(rawURL)
	if !ok {
		return "", fmt.Errorf("photo url does not contain a master upload path")
	}

	// URL-decode to normalise %2e%2e, %2f, %5c and similar before cleaning.
	decoded, err := url.PathUnescape(key)
	if err != nil {
		return "", fmt.Errorf("photo url contains invalid percent-encoding")
	}

	// path.Clean collapses "..", ".", duplicate slashes so that any residual
	// traversal sequences become a non-matching string before the regexp check.
	cleaned := path.Clean(decoded)

	// Strict allowlist: masters/<uuid>/<safe-filename-no-slashes>
	if !masterPhotoKeyRe.MatchString(cleaned) {
		return "", fmt.Errorf("photo url has invalid format")
	}

	// Verify the key belongs to this master specifically (not another master's directory).
	expectedPrefix := "masters/" + masterID.String() + "/"
	if !strings.HasPrefix(cleaned, expectedPrefix) {
		return "", fmt.Errorf("photo url does not belong to this master")
	}

	// Reconstruct a canonical URL from the verified, cleaned key.
	// This strips query strings, fragments, double-slashes, and any
	// percent-encoding from what gets stored in the database.
	//
	// For relative URLs (no host): path only, e.g. "/api/v1/uploads/masters/<id>/photo.jpg".
	// For absolute URLs (with host): preserve scheme+host, e.g. "https://cdn.example.com/masters/<id>/photo.jpg".
	//
	// Path prefix: everything in the original cleaned path up to (but not
	// including) the "masters/" segment, so the public URL shape is preserved.
	origCleanPath := path.Clean(parsed.Path)
	mastersIdx := strings.Index(origCleanPath, "/masters/")
	var canonicalURL string
	if mastersIdx >= 0 {
		prefix := origCleanPath[:mastersIdx]
		canonicalURL = prefix + "/" + cleaned
	} else {
		canonicalURL = "/" + cleaned
	}
	if parsed.Host != "" {
		canonicalURL = parsed.Scheme + "://" + parsed.Host + canonicalURL
	}
	return canonicalURL, nil
}

func (uc *MasterUseCase) AddMasterPhoto(ctx context.Context, userID uuid.UUID, url string) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	cleanURL, err := normalizeMasterPhotoURL(m.ID, url)
	if err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	// The limit check (MaxMasterPhotos) is enforced inside the repo transaction
	// so concurrent uploads cannot race past it. CountPhotosByMaster is no longer
	// called here — the separate count+check pattern had a TOCTOU window.
	if _, err := uc.repo.AddMasterPhoto(ctx, m.ID, cleanURL); err != nil {
		if errors.Is(err, domain.ErrPhotoLimitReached) {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf("too many photos (max %d)", domain.MaxMasterPhotos))
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
}

func (uc *MasterUseCase) DeleteMasterPhoto(ctx context.Context, userID, photoID uuid.UUID) (string, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", pkgerrors.NotFound("master profile not found")
	}
	u, err := uc.repo.DeleteMasterPhoto(ctx, m.ID, photoID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", pkgerrors.NotFound("photo not found")
		}
		return "", err
	}
	return u, nil
}

func (uc *MasterUseCase) AddMasterVideo(ctx context.Context, userID uuid.UUID, url string) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	// Videos are stored under the same masters/<id>/ prefix as photos, so the
	// photo URL validator applies unchanged (it allowlists the storage path and
	// filename, not the media type).
	cleanURL, err := normalizeMasterPhotoURL(m.ID, url)
	if err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	if _, err := uc.repo.AddMasterVideo(ctx, m.ID, cleanURL); err != nil {
		if errors.Is(err, domain.ErrVideoLimitReached) {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf("too many videos (max %d)", domain.MaxMasterVideos))
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
}

func (uc *MasterUseCase) DeleteMasterVideo(ctx context.Context, userID, videoID uuid.UUID) (string, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", pkgerrors.NotFound("master profile not found")
	}
	u, err := uc.repo.DeleteMasterVideo(ctx, m.ID, videoID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", pkgerrors.NotFound("video not found")
		}
		return "", err
	}
	return u, nil
}

func (uc *MasterUseCase) SetMasterCoverPhoto(ctx context.Context, userID, photoID uuid.UUID) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	if err := uc.repo.SetMasterCoverPhoto(ctx, m.ID, photoID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, pkgerrors.NotFound("photo not found")
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
}
