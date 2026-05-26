package storage

import (
	"context"
	"io"
)

// GCSStorage is a stub for Sprint 1. All methods return ErrNotImplemented
// except URL which produces the canonical GCS public URL format so callers
// in dev paths don't crash if they accidentally request a URL.
type GCSStorage struct {
	bucket   string
	credPath string
}

func NewGCS(bucket, credentialsPath string) *GCSStorage {
	return &GCSStorage{bucket: bucket, credPath: credentialsPath}
}

func (s *GCSStorage) Put(ctx context.Context, obj Object, r io.Reader) (string, error) {
	return "", ErrNotImplemented
}

func (s *GCSStorage) Delete(ctx context.Context, key string) error {
	return ErrNotImplemented
}

func (s *GCSStorage) URL(key string) string {
	return "https://storage.googleapis.com/" + s.bucket + "/" + key
}
