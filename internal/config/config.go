// Package config loads runtime configuration from environment variables
// (optionally seeded from a .env file in development).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env            string // development | production
	HTTPAddr       string // e.g. ":8080"
	DatabaseURL    string // postgres://user:pass@host:5432/db?sslmode=disable
	JWTSecret      string
	JWTTTL         time.Duration
	CORSOrigins    []string
	TrustedProxies []string // CIDRs allowed to set X-Forwarded-For (nginx on the host / docker bridge)
	AutoMigrate    bool     // run pending migrations before serving
	LogLevel       string

	// Cloudflare R2 (S3-compatible) for uploaded images. Optional: when any
	// field is empty the upload endpoint answers 503 and the API still runs.
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Bucket          string
	R2PublicBaseURL   string // https://cdn.coffeesos.online

	// Used by `api seed` only.
	SeedAdminEmail    string
	SeedAdminPassword string
	SeedDemo          bool
}

func (c *Config) IsDev() bool { return c.Env != "production" }

// Load reads configuration. A .env file is loaded if present (dev convenience);
// real environment variables always win.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Env:               getenv("APP_ENV", "development"),
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:       getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/coffeesos?sslmode=disable"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		JWTTTL:            getDuration("JWT_TTL", 24*time.Hour),
		CORSOrigins:       splitList(getenv("CORS_ORIGINS", "*")),
		TrustedProxies:    splitList(getenv("TRUSTED_PROXIES", "127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")),
		AutoMigrate:       getBool("AUTO_MIGRATE", false),
		LogLevel:          getenv("LOG_LEVEL", "info"),
		R2AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:          os.Getenv("R2_BUCKET"),
		R2PublicBaseURL:   strings.TrimRight(os.Getenv("R2_PUBLIC_URL"), "/"),
		SeedAdminEmail:    getenv("SEED_ADMIN_EMAIL", "admin@coffeesos.local"),
		SeedAdminPassword: os.Getenv("SEED_ADMIN_PASSWORD"),
		SeedDemo:          getBool("SEED_DEMO", true),
	}

	if cfg.JWTSecret == "" {
		if !cfg.IsDev() {
			return nil, errors.New("JWT_SECRET is required in production")
		}
		cfg.JWTSecret = "dev-only-secret-change-me"
	}
	if len(cfg.JWTSecret) < 16 && !cfg.IsDev() {
		return nil, errors.New("JWT_SECRET must be at least 16 characters")
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: invalid %s=%q, using %s\n", key, v, def)
		return def
	}
	return d
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
