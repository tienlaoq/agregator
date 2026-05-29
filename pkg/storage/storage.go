// Package storage provides a file storage abstraction used by the API gateway
// for uploading and deleting user-generated content (venue photos, master photos).
//
// Two implementations are provided:
//   - MinioUploader: stores files in MinIO (S3-compatible). Use in production.
//     Files are served directly from MinIO via a public bucket URL.
//   - DiskUploader: stores files on the local filesystem. Use in development
//     or single-replica deployments only — files are NOT replicated across
//     gateway instances (see docs/TECH_DEBT.md STORAGE-01).
//
// Usage: accept the Uploader interface in handlers; build the concrete type in
// deps.go based on MINIO_ENDPOINT presence in config.
package storage

import (
	"context"
	"io"
)

// Uploader is the file storage interface accepted by VenueHandler and
// MasterHandler. Keep it minimal — two operations cover all current needs.
type Uploader interface {
	// Put stores r under the given object key and returns the public URL
	// the client should use to retrieve the file.
	// contentType must be a valid MIME type (e.g. "image/jpeg").
	// size is the byte length of r; pass -1 if unknown (MinIO will buffer).
	Put(ctx context.Context, key, contentType string, size int64, r io.Reader) (publicURL string, err error)

	// Delete removes the object identified by key. Implementations must treat
	// "not found" as a no-op (idempotent delete).
	Delete(ctx context.Context, key string) error
}
