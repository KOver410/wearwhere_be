package goong

import "fmt"

type Config struct {
	Mode    string // mock | production
	APIKey  string
	BaseURL string
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(), nil
	case "production":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("goong: production mode requires GOONG_API_KEY")
		}
		base := cfg.BaseURL
		if base == "" {
			base = "https://rsapi.goong.io"
		}
		return NewHTTPClient(cfg.APIKey, base), nil
	default:
		return nil, fmt.Errorf("goong: unknown mode %q (want mock|production)", cfg.Mode)
	}
}
