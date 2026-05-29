package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioConfig holds the connection parameters for a MinIO (S3-compatible) server.
type MinioConfig struct {
	Endpoint  string // e.g. "minio:9000" or "s3.amazonaws.com"
	AccessKey string
	SecretKey string
	Bucket    string
	// PublicBaseURL is the externally-reachable URL prefix for objects in Bucket,
	// e.g. "https://cdn.example.com/photos" or "http://localhost:9000/photos".
	// The public URL of an uploaded object is PublicBaseURL + "/" + key.
	PublicBaseURL string
	UseSSL        bool
}

// MinioUploader implements Uploader backed by a MinIO (S3-compatible) server.
// Files are stored in a single pre-created public bucket; the public URL is
// constructed from PublicBaseURL without any redirect through the gateway.
type MinioUploader struct {
	client        *minio.Client
	bucket        string
	publicBaseURL string // no trailing slash
}

// NewMinioUploader connects to MinIO and ensures the target bucket exists.
// The bucket must be created before calling this function (use mc or the
// MinIO console; bucket creation requires admin credentials and is a one-time
// setup step outside the gateway's responsibility).
func NewMinioUploader(cfg MinioConfig) (*MinioUploader, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: minio client: %w", err)
	}
	return &MinioUploader{
		client:        client,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}, nil
}

// Put uploads r to MinIO at the given key and returns the permanent public URL.
func (u *MinioUploader) Put(ctx context.Context, key, contentType string, size int64, r io.Reader) (string, error) {
	_, err := u.client.PutObject(ctx, u.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("storage: put %q: %w", key, err)
	}
	return u.publicBaseURL + "/" + key, nil
}

// Delete removes the object at key. Returns nil if the object does not exist.
func (u *MinioUploader) Delete(ctx context.Context, key string) error {
	err := u.client.RemoveObject(ctx, u.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		// Treat "not found" as success — idempotent delete.
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" {
			return nil
		}
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}
