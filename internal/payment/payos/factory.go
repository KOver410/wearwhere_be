// internal/payment/payos/factory.go
package payos

import "fmt"

type Config struct {
	Mode        string // "mock" | "production"
	ClientID    string
	APIKey      string
	ChecksumKey string
	BaseURL     string // for mock checkout URL (defaults to http://localhost:8080)
	ReturnURL   string
	CancelURL   string
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(cfg.BaseURL), nil
	case "production":
		if cfg.ClientID == "" || cfg.APIKey == "" || cfg.ChecksumKey == "" {
			return nil, fmt.Errorf("payos: production mode requires PAYOS_CLIENT_ID, PAYOS_API_KEY, PAYOS_CHECKSUM_KEY")
		}
		return NewHTTPClient(cfg.ClientID, cfg.APIKey, cfg.ChecksumKey), nil
	default:
		return nil, fmt.Errorf("payos: unknown mode %q (want mock|production)", cfg.Mode)
	}
}
