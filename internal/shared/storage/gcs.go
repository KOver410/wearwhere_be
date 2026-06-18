package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCSStorage backs onto a Google Cloud Storage bucket (prod). Objects are
// written with their declared ContentType and addressed by the canonical
// public URL (https://storage.googleapis.com/<bucket>/<key>), so the bucket
// must grant public read access for those URLs to resolve.
type GCSStorage struct {
	client *gcs.Client
	bucket string
}

// NewGCS builds a GCSStorage backed by a real GCS client. When credentialsPath
// is non-empty the service-account JSON at that path is used; otherwise the
// client falls back to Application Default Credentials (e.g. the VM's attached
// service account in production).
func NewGCS(bucket, credentialsPath string) (*GCSStorage, error) {
	if bucket == "" {
		return nil, errors.New("storage.gcs: bucket is required")
	}
	var opts []option.ClientOption
	if credentialsPath != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsPath))
	}
	client, err := gcs.NewClient(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("storage.gcs: new client: %w", err)
	}
	return newGCSWithClient(client, bucket), nil
}

// newGCSWithClient wires a GCSStorage to a pre-built client. Used by NewGCS and
// by tests that inject a client pointed at a fake GCS server.
func newGCSWithClient(client *gcs.Client, bucket string) *GCSStorage {
	return &GCSStorage{client: client, bucket: bucket}
}

func (s *GCSStorage) Put(ctx context.Context, obj Object, r io.Reader) (string, error) {
	if err := safeKey(obj.Key); err != nil {
		return "", err
	}
	w := s.client.Bucket(s.bucket).Object(obj.Key).NewWriter(ctx)
	w.ContentType = obj.ContentType
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("storage.gcs: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("storage.gcs: close: %w", err)
	}
	return s.URL(obj.Key), nil
}

func (s *GCSStorage) Delete(ctx context.Context, key string) error {
	if err := safeKey(key); err != nil {
		return err
	}
	err := s.client.Bucket(s.bucket).Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, gcs.ErrObjectNotExist) {
		return fmt.Errorf("storage.gcs: delete: %w", err)
	}
	return nil
}

func (s *GCSStorage) URL(key string) string {
	return "https://storage.googleapis.com/" + s.bucket + "/" + key
}
