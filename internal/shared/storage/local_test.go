package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocal_PutAndDelete(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "http://localhost:8080/uploads")

	payload := bytes.NewBufferString("hello")
	url, err := s.Put(context.Background(),
		Object{Key: "sub/file.txt", ContentType: "text/plain", Size: 5},
		payload)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8080/uploads/sub/file.txt", url)

	onDisk, err := os.ReadFile(filepath.Join(dir, "sub", "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(onDisk))

	require.NoError(t, s.Delete(context.Background(), "sub/file.txt"))
	_, err = os.Stat(filepath.Join(dir, "sub", "file.txt"))
	require.True(t, os.IsNotExist(err))
}

func TestLocal_DeleteMissing_IsNoop(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "http://localhost:8080/uploads")
	require.NoError(t, s.Delete(context.Background(), "does/not/exist.txt"))
}

func TestLocal_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "http://localhost:8080/uploads")

	_, err := s.Put(context.Background(),
		Object{Key: "../escape.txt", ContentType: "text/plain", Size: 1},
		strings.NewReader("x"))
	require.Error(t, err)
}

func TestLocal_RejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "http://localhost:8080/uploads")

	// Forward-slash absolute (Unix style)
	_, err := s.Put(context.Background(),
		Object{Key: "/etc/passwd", ContentType: "text/plain", Size: 1},
		strings.NewReader("x"))
	require.Error(t, err)

	// Backslash absolute (Windows style) — only rejected via filepath.IsAbs on Windows.
	// On Unix this becomes a relative key with a literal backslash, which is allowed.
	// Skip the Windows-specific subcase if running on Unix.
	if filepath.IsAbs(`C:\Windows\system32`) {
		_, err = s.Put(context.Background(),
			Object{Key: `C:\Windows\system32`, ContentType: "text/plain", Size: 1},
			strings.NewReader("x"))
		require.Error(t, err)
	}
}

func TestLocal_RejectsNullByte(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "http://localhost:8080/uploads")

	_, err := s.Put(context.Background(),
		Object{Key: "products/abc\x00/secret", ContentType: "text/plain", Size: 1},
		strings.NewReader("x"))
	require.Error(t, err)
}

func TestLocal_RejectsEmptyKey(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "http://localhost:8080/uploads")

	_, err := s.Put(context.Background(),
		Object{Key: "", ContentType: "text/plain", Size: 0},
		strings.NewReader(""))
	require.Error(t, err)
}
