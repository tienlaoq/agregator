package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DiskUploader implements Uploader using the local filesystem.
//
// WARNING: not suitable for multi-replica deployments — each gateway instance
// only sees the files it wrote itself. Use MinioUploader in production.
// See docs/TECH_DEBT.md [STORAGE-01].
//
// publicBaseURL is the URL prefix the gateway serves uploads from, e.g.
// "/api/v1/uploads". The public URL of a stored object is:
//
//	publicBaseURL + "/" + key
type DiskUploader struct {
	root          string // absolute path to upload root directory
	publicBaseURL string // no trailing slash
}

// NewDiskUploader returns a DiskUploader that stores files under root and
// serves them under publicBaseURL.
func NewDiskUploader(root, publicBaseURL string) (*DiskUploader, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: disk root %q: %w", root, err)
	}
	return &DiskUploader{
		root:          abs,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}, nil
}

// Put writes r to root/key and returns the public URL.
func (u *DiskUploader) Put(_ context.Context, key, _ string, _ int64, r io.Reader) (string, error) {
	// key uses forward slashes; convert to OS-native separators.
	rel := filepath.FromSlash(key)
	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("storage: key %q contains path traversal", key)
	}
	full := filepath.Join(u.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("storage: mkdir %q: %w", filepath.Dir(full), err)
	}
	f, err := os.Create(full)
	if err != nil {
		return "", fmt.Errorf("storage: create %q: %w", full, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		_ = os.Remove(full)
		return "", fmt.Errorf("storage: write %q: %w", full, err)
	}
	return u.publicBaseURL + "/" + key, nil
}

// Delete removes root/key. Returns nil if the file does not exist.
func (u *DiskUploader) Delete(_ context.Context, key string) error {
	rel := filepath.FromSlash(key)
	if strings.Contains(rel, "..") {
		return nil // silently ignore traversal attempts
	}
	full := filepath.Join(u.root, rel)
	// Verify the resolved path is still under root (belt-and-suspenders).
	clean, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(clean, u.root+string(os.PathSeparator)) {
		return nil
	}
	err = os.Remove(clean)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ServeHTTP serves files stored under root, stripping publicBaseURL from
// the request path. Attach this handler to the /uploads/* route when using
// DiskUploader in development.
func (u *DiskUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Strip the publicBaseURL prefix to get the key.
	prefix := strings.TrimRight(u.publicBaseURL, "/")
	key := strings.TrimPrefix(r.URL.Path, prefix)
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "..") {
		http.NotFound(w, r)
		return
	}
	rel := filepath.FromSlash(key)
	full := filepath.Join(u.root, rel)
	clean, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(clean, u.root+string(os.PathSeparator)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, clean)
}
