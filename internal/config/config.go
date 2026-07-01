package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port               string
	AppEnv             string
	AppBaseURL         string
	ServerSecure       bool
	DatabaseURL        string
	SMTPHost           string
	SMTPPort           string
	SMTPUser           string
	SMTPPass           string
	SMTPFrom           string
	GoogleClientID     string
	GoogleClientSecret string
	TrustedProxyCIDRs  string
	RateLimitAuthMax   int
	RateLimitGlobalMax int
	VAPIDPublicKey     string
	VAPIDPrivateKey    string
	VAPIDSubject       string
}

func Load() (Config, error) {
	cfg := Config{
		Port:               getenv("PORT", "8080"),
		AppEnv:             getenv("APP_ENV", "development"),
		AppBaseURL:         getenv("APP_BASE_URL", "http://localhost:8080"),
		ServerSecure:       getenv("SERVER_SECURE", "false") == "true",
		DatabaseURL:        getenv("DATABASE_URL", ""),
		SMTPHost:           getenv("SMTP_HOST", ""),
		SMTPPort:           getenv("SMTP_PORT", "587"),
		SMTPUser:           getenv("SMTP_USER", ""),
		SMTPPass:           getenv("SMTP_PASS", ""),
		SMTPFrom:           getenv("SMTP_FROM", ""),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
		TrustedProxyCIDRs:  getenv("TRUSTED_PROXY_CIDRS", ""),
		RateLimitAuthMax:   getenvInt("RATE_LIMIT_AUTH_MAX", 5),
		RateLimitGlobalMax: getenvInt("RATE_LIMIT_GLOBAL_MAX", 120),
		VAPIDPublicKey:     getenv("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey:    getenv("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:       getenv("VAPID_SUBJECT", ""),
	}

	if cfg.Port == "" {
		return Config{}, fmt.Errorf("PORT must not be empty")
	}
	if cfg.AppBaseURL == "" {
		return Config{}, fmt.Errorf("APP_BASE_URL must not be empty")
	}
	// Fail fast rather than silently falling back to the ephemeral in-memory
	// store in production: without a database the app loses all data on restart
	// and cannot share state across instances. The in-memory path remains
	// available for development (APP_ENV != production).
	if cfg.IsProduction() && cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must be set when APP_ENV=production")
	}

	return cfg, nil
}

func (c Config) HTTPAddr() string {
	return ":" + c.Port
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
