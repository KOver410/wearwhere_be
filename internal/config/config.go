package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App         AppConfig
	HTTP        HTTPConfig
	DB          DBConfig
	Redis       RedisConfig
	JWT         JWTConfig
	SMTP        SMTPConfig
	SMS         SMSConfig
	OAuth       OAuthConfig
	Limit       LimitConfig
	Storage     StorageConfig
	Payos       PayosConfig
	Shipping    ShippingConfig
	Reservation ReservationConfig
}

type AppConfig struct {
	Env string
}

type HTTPConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DBConfig struct {
	URL          string
	MaxOpenConns int
	MaxIdleConns int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type SMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

type SMSConfig struct {
	AccountSID string
	AuthToken  string
	FromNumber string
}

// OAuthConfig — each provider may accept multiple Client IDs because each
// frontend platform (web / iOS / Android) gets a different OAuth audience.
//
//	Google:  web ID (web + Android via serverClientId) + iOS ID
//	Apple:   Services ID (web/Android) + App ID Bundle (iOS native sign-in)
type OAuthConfig struct {
	GoogleClientIDs []string
	AppleClientIDs  []string
}

// AllowsGoogleAud reports whether the given ID-token `aud` matches any
// configured Google Client ID. Returns true when no IDs are configured (dev mode).
func (o OAuthConfig) AllowsGoogleAud(aud string) bool {
	if len(o.GoogleClientIDs) == 0 {
		return true
	}
	for _, id := range o.GoogleClientIDs {
		if id == aud {
			return true
		}
	}
	return false
}

type LimitConfig struct {
	LoginMaxAttempts    int
	LoginLockoutMinutes int
	OTPTTLMinutes       int
	OTPMaxPerHour       int
	RateLimitPerMin     int
}

type StorageConfig struct {
	Driver         string
	LocalDir       string
	BaseURL        string
	GCSBucket      string
	GCSCredentials string
	MaxFileSize    int64
	AllowedMIMEs   []string
}

func Load() (*Config, error) {
	_ = godotenv.Load() // silently ignore if .env missing (e.g. in prod)

	cfg := &Config{
		App: AppConfig{
			Env: getEnv("APP_ENV", "development"),
		},
		HTTP: HTTPConfig{
			Port:         getEnv("HTTP_PORT", "8080"),
			ReadTimeout:  getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout: getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		},
		DB: DBConfig{
			URL:          mustEnv("DATABASE_URL"),
			MaxOpenConns: getInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns: getInt("DB_MAX_IDLE_CONNS", 5),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:     mustEnv("JWT_SECRET"),
			AccessTTL:  getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: getDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		},
		SMTP: SMTPConfig{
			Host:      getEnv("SMTP_HOST", ""),
			Port:      getInt("SMTP_PORT", 587),
			Username:  getEnv("SMTP_USERNAME", ""),
			Password:  getEnv("SMTP_PASSWORD", ""),
			FromEmail: getEnv("SMTP_FROM_EMAIL", "no-reply@wearwhere.app"),
			FromName:  getEnv("SMTP_FROM_NAME", "WearWhere"),
		},
		SMS: SMSConfig{
			AccountSID: getEnv("TWILIO_ACCOUNT_SID", ""),
			AuthToken:  getEnv("TWILIO_AUTH_TOKEN", ""),
			FromNumber: getEnv("TWILIO_FROM_NUMBER", ""),
		},
		OAuth: OAuthConfig{
			// Accept either GOOGLE_CLIENT_IDS (comma-separated) or legacy
			// single GOOGLE_CLIENT_ID. Same for Apple.
			GoogleClientIDs: csvOrSingle("GOOGLE_CLIENT_IDS", "GOOGLE_CLIENT_ID"),
			AppleClientIDs:  csvOrSingle("APPLE_CLIENT_IDS", "APPLE_CLIENT_ID"),
		},
		Limit: LimitConfig{
			LoginMaxAttempts:    getInt("LOGIN_MAX_ATTEMPTS", 5),
			LoginLockoutMinutes: getInt("LOGIN_LOCKOUT_MINUTES", 15),
			OTPTTLMinutes:       getInt("OTP_TTL_MINUTES", 15),
			OTPMaxPerHour:       getInt("OTP_MAX_PER_HOUR", 3),
			RateLimitPerMin:     getInt("RATE_LIMIT_PER_MIN", 100),
		},
		Storage: StorageConfig{
			Driver:         getEnv("STORAGE_DRIVER", "local"),
			LocalDir:       getEnv("STORAGE_LOCAL_DIR", "./uploads"),
			BaseURL:        getEnv("STORAGE_BASE_URL", "http://localhost:8080/uploads"),
			GCSBucket:      getEnv("STORAGE_GCS_BUCKET", ""),
			GCSCredentials: getEnv("STORAGE_GCS_CREDENTIALS", ""),
			MaxFileSize:    int64(getInt("STORAGE_MAX_FILE_SIZE", 5*1024*1024)),
			AllowedMIMEs:   csvOrSingle("STORAGE_ALLOWED_MIMES", ""),
		},
	}
	cfg.Payos = PayosConfig{
		Mode:        getEnv("PAYOS_MODE", "mock"),
		ClientID:    getEnv("PAYOS_CLIENT_ID", ""),
		APIKey:      getEnv("PAYOS_API_KEY", ""),
		ChecksumKey: getEnv("PAYOS_CHECKSUM_KEY", ""),
		ReturnURL:   getEnv("PAYOS_RETURN_URL", "http://localhost:3000/checkout/success"),
		CancelURL:   getEnv("PAYOS_CANCEL_URL", "http://localhost:3000/checkout/cancel"),
		BaseURL:     getEnv("PAYOS_BASE_URL", "http://localhost:8080"),
	}
	cfg.Shipping = ShippingConfig{
		Provider: getEnv("SHIPPING_PROVIDER", "flat"),
	}
	cfg.Reservation = ReservationConfig{
		TimeoutMinutes:  getInt("RESERVATION_TIMEOUT_MINUTES", 30),
		CleanupInterval: getDuration("RESERVATION_CLEANUP_INTERVAL", 5*time.Minute),
	}
	return cfg, nil
}

type PayosConfig struct {
	Mode        string
	ClientID    string
	APIKey      string
	ChecksumKey string
	ReturnURL   string
	CancelURL   string
	BaseURL     string
}

type ShippingConfig struct {
	Provider string
}

type ReservationConfig struct {
	TimeoutMinutes  int
	CleanupInterval time.Duration
}

func (c *Config) IsProduction() bool { return c.App.Env == "production" }

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env %s is not set", key))
	}
	return v
}

func getInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// csvOrSingle reads `primaryKey` as a comma-separated list first; if empty,
// falls back to `legacyKey` (single value). Blanks and whitespace are skipped.
func csvOrSingle(primaryKey, legacyKey string) []string {
	raw := os.Getenv(primaryKey)
	if raw == "" {
		raw = os.Getenv(legacyKey)
	}
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func getDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
