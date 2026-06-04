package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesDevelopmentCORSAndPayosDefaults(t *testing.T) {
	setConfigTestEnv(t)
	t.Setenv("APP_ENV", "development")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	}, cfg.CORS.AllowedOrigins)
	require.Equal(t, "http://localhost:5173/order/success", cfg.Payos.ReturnURL)
	require.Equal(t, "http://localhost:5173/cart", cfg.Payos.CancelURL)
}

func TestLoadUsesEmptyCORSOriginsInProduction(t *testing.T) {
	setConfigTestEnv(t)
	t.Setenv("APP_ENV", "production")

	cfg, err := Load()

	require.NoError(t, err)
	require.Empty(t, cfg.CORS.AllowedOrigins)
}

func TestLoadUsesTrimmedConfiguredCORSOrigins(t *testing.T) {
	setConfigTestEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://app.example , http://localhost:5173 ")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, []string{
		"https://app.example",
		"http://localhost:5173",
	}, cfg.CORS.AllowedOrigins)
}

func setConfigTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://config-test")
	t.Setenv("JWT_SECRET", "config-test-secret")
	t.Setenv("APP_ENV", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	t.Setenv("PAYOS_RETURN_URL", "")
	t.Setenv("PAYOS_CANCEL_URL", "")
}
