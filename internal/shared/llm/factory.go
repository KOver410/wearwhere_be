package llm

import (
	"fmt"
	"time"
)

type Config struct {
	Provider string
	APIKey   string
	Model    string
	BaseURL  string
	Timeout  time.Duration
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Provider {
	case "mock", "":
		return NewMockClient(), nil
	case "gemini":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("llm: gemini provider requires GEMINI_API_KEY")
		}
		return NewGeminiClient(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Timeout), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (want mock|gemini)", cfg.Provider)
	}
}
