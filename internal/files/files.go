// Package files stores uploaded file contents in object storage.
package files

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/bjschafer/print-dis/internal/config"
)

// ErrNotFound is returned by Get when the key does not exist.
var ErrNotFound = errors.New("files: not found")

// ErrBadKey is returned for keys that are not safe relative paths.
var ErrBadKey = errors.New("files: invalid key")

// Blobs is a minimal object store. Keys look like "requests/{id}/{random}".
type Blobs interface {
	// Put stores r under key, replacing any existing object. size is the
	// number of bytes r will yield, or -1 when unknown. Nothing is left
	// behind under key if Put fails.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get opens the object and reports its size. The error is ErrNotFound
	// when there is no such object. The reader also implements io.ReaderAt,
	// so a zip-based model can be measured without buffering it.
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	// Delete removes the object. Deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}

// New builds the backend named by cfg.FilesBackend, or returns nil when
// uploads are disabled.
func New(cfg *config.Config) (Blobs, error) {
	switch cfg.FilesBackend {
	case "":
		return nil, nil
	case "dir":
		return NewDir(cfg.FilesDir)
	case "s3":
		return NewS3(S3Options{
			Endpoint:        cfg.S3Endpoint,
			Bucket:          cfg.S3Bucket,
			AccessKeyID:     cfg.S3AccessKeyID,
			SecretAccessKey: cfg.S3SecretAccessKey,
			Region:          cfg.S3Region,
		})
	}
	return nil, fmt.Errorf("files: unknown backend %q", cfg.FilesBackend)
}
