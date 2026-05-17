package storage

import "fmt"

type Config struct {
	Driver         string // "local" | "gcs"
	LocalDir       string
	BaseURL        string
	GCSBucket      string
	GCSCredentials string
	MaxFileSize    int64
	AllowedMIMEs   []string
}

func New(cfg Config) (Storage, error) {
	if len(cfg.AllowedMIMEs) == 0 {
		cfg.AllowedMIMEs = []string{"image/jpeg", "image/png", "image/webp"}
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 5 * 1024 * 1024
	}
	switch cfg.Driver {
	case "local", "":
		return NewLocal(cfg.LocalDir, cfg.BaseURL), nil
	case "gcs":
		return NewGCS(cfg.GCSBucket, cfg.GCSCredentials), nil
	default:
		return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
	}
}
