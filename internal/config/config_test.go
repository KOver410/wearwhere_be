package config

import (
	"os"
	"testing"
)

func TestLoad_GoshipDefaults(t *testing.T) {
	os.Clearenv()
	// satisfy mustEnv calls in Load()
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Goship.Mode != "mock" {
		t.Errorf("Goship.Mode = %q, want mock", cfg.Goship.Mode)
	}
	if cfg.Goship.DefaultItemWeightG != 500 {
		t.Errorf("DefaultItemWeightG = %d, want 500", cfg.Goship.DefaultItemWeightG)
	}
	if cfg.Shipping.Provider != "flat" {
		t.Errorf("Shipping.Provider default = %q, want flat", cfg.Shipping.Provider)
	}
}

func TestLoad_GoongDefaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Goong.Mode != "mock" {
		t.Errorf("Goong.Mode = %q, want mock", cfg.Goong.Mode)
	}
	if cfg.Goong.BaseURL != "https://rsapi.goong.io" {
		t.Errorf("Goong.BaseURL = %q, want https://rsapi.goong.io", cfg.Goong.BaseURL)
	}
}
