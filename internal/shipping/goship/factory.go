package goship

import "fmt"

type Config struct {
	Mode         string // mock | sandbox | production
	Token        string
	ClientSecret string
	BaseURL      string
}

func NewFromConfig(cfg Config) (Service, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(), nil
	case "sandbox", "production":
		if cfg.Token == "" {
			return nil, fmt.Errorf("goship: %s mode requires GOSHIP_TOKEN", cfg.Mode)
		}
		return NewHTTPClient(cfg.Token, cfg.ClientSecret, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("goship: unknown mode %q (want mock|sandbox|production)", cfg.Mode)
	}
}
