package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/require"
)

const testBucket = "wearwhere-test"

func newFakeGCS(t *testing.T) *GCSStorage {
	t.Helper()
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{})
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: testBucket})
	return newGCSWithClient(server.Client(), testBucket)
}

func TestGCS_PutAndReadBack(t *testing.T) {
	s := newFakeGCS(t)
	ctx := context.Background()

	url, err := s.Put(ctx,
		Object{Key: "products/abc/photo.jpg", ContentType: "image/jpeg", Size: 5},
		bytes.NewBufferString("hello"))
	require.NoError(t, err)
	require.Equal(t, "https://storage.googleapis.com/"+testBucket+"/products/abc/photo.jpg", url)

	// Read the object back through the GCS client to prove it was really stored.
	r, err := s.client.Bucket(testBucket).Object("products/abc/photo.jpg").NewReader(ctx)
	require.NoError(t, err)
	defer r.Close()
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
	require.Equal(t, "image/jpeg", r.Attrs.ContentType)
}

func TestGCS_Delete(t *testing.T) {
	s := newFakeGCS(t)
	ctx := context.Background()

	_, err := s.Put(ctx,
		Object{Key: "products/abc/photo.jpg", ContentType: "image/jpeg", Size: 1},
		strings.NewReader("x"))
	require.NoError(t, err)

	require.NoError(t, s.Delete(ctx, "products/abc/photo.jpg"))

	_, err = s.client.Bucket(testBucket).Object("products/abc/photo.jpg").NewReader(ctx)
	require.Error(t, err) // object no longer exists
}

func TestGCS_DeleteMissing_IsNoop(t *testing.T) {
	s := newFakeGCS(t)
	require.NoError(t, s.Delete(context.Background(), "does/not/exist.jpg"))
}

func TestGCS_RejectsEmptyKey(t *testing.T) {
	s := newFakeGCS(t)
	_, err := s.Put(context.Background(),
		Object{Key: "", ContentType: "image/jpeg", Size: 0},
		strings.NewReader(""))
	require.Error(t, err)
}

func TestGCS_RejectsPathTraversalKey(t *testing.T) {
	s := newFakeGCS(t)
	_, err := s.Put(context.Background(),
		Object{Key: "../escape.jpg", ContentType: "image/jpeg", Size: 1},
		strings.NewReader("x"))
	require.Error(t, err)
}

func TestGCS_URL(t *testing.T) {
	s := newGCSWithClient(nil, "my-bucket")
	require.Equal(t, "https://storage.googleapis.com/my-bucket/k/e/y.jpg", s.URL("k/e/y.jpg"))
}
