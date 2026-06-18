// Package storage provides a pluggable file storage abstraction.
// Local backs onto the host filesystem (dev). GCS backs onto Google Cloud
// Storage (prod).
package storage

import (
	"context"
	"errors"
	"io"
)

var ErrNotFound = errors.New("storage: object not found")

type Object struct {
	Key         string // path-like, e.g. "products/<uuid>/<file>.jpg"
	ContentType string // "image/jpeg" | "image/png" | "image/webp"
	Size        int64  // bytes
}

type Storage interface {
	// Put writes r to backend storage and returns the public URL.
	Put(ctx context.Context, obj Object, r io.Reader) (url string, err error)
	// Delete removes the object by key. Idempotent (returns nil for not-found).
	Delete(ctx context.Context, key string) error
	// URL returns the public URL for an existing key without I/O.
	URL(key string) string
}
