package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	dir     string
	baseURL string
}

func NewLocal(dir, baseURL string) *LocalStorage {
	return &LocalStorage{
		dir:     filepath.Clean(dir),
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (s *LocalStorage) Put(ctx context.Context, obj Object, r io.Reader) (string, error) {
	if err := safeKey(obj.Key); err != nil {
		return "", err
	}
	target := filepath.Join(s.dir, filepath.FromSlash(obj.Key))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("storage.local: mkdir: %w", err)
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("storage.local: open: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("storage.local: write: %w", err)
	}
	return s.URL(obj.Key), nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	if err := safeKey(key); err != nil {
		return err
	}
	target := filepath.Join(s.dir, filepath.FromSlash(key))
	err := os.Remove(target)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage.local: delete: %w", err)
	}
	return nil
}

func (s *LocalStorage) URL(key string) string {
	return s.baseURL + "/" + strings.TrimLeft(key, "/")
}

// safeKey rejects keys that would escape the base directory or otherwise
// behave dangerously when passed to the filesystem.
func safeKey(key string) error {
	if key == "" {
		return errors.New("storage: empty key")
	}
	if strings.ContainsRune(key, 0) {
		return fmt.Errorf("storage: unsafe key %q", key)
	}
	// Reject absolute paths on both Unix and Windows (including drive letters
	// and UNC prefixes — filepath.IsAbs handles these per-OS).
	if filepath.IsAbs(key) {
		return fmt.Errorf("storage: unsafe key %q", key)
	}
	// Reject keys whose forward-slash form starts with "/" (catches forward-slash
	// absolute on Windows, which filepath.IsAbs may miss).
	normalized := filepath.ToSlash(key)
	if strings.HasPrefix(normalized, "/") {
		return fmt.Errorf("storage: unsafe key %q", key)
	}
	// Reject any path component equal to "..".
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return fmt.Errorf("storage: unsafe key %q", key)
		}
	}
	return nil
}
