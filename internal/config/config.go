package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port               string
	AppEnv             string
	AppBaseURL         string
	ServerSecure       bool
	DatabaseURL        string
	RedisURL           string
	SessionSecret      string
	CSRFSecret         string
	SMTPHost           string
	SMTPPort           string
	SMTPUser           string
	SMTPPass           string
	SMTPFrom           string
	GoogleClientID     string
	GoogleClientSecret string
	VAPIDPrivateKey    string
	VAPIDPublicKey     string
	VAPIDSubject       string
	TrustedProxyCIDRs  string
}

func Load() (Config, error) {
	cfg := Config{
		Port:               getenv("PORT", "8080"),
		AppEnv:             getenv("APP_ENV", "development"),
		AppBaseURL:         getenv("APP_BASE_URL", "http://localhost:8080"),
		ServerSecure:       getenv("SERVER_SECURE", "false") == "true",
		DatabaseURL:        getenv("DATABASE_URL", ""),
		RedisURL:           getenv("REDIS_URL", ""),
		SessionSecret:      getenv("SESSION_SECRET", ""),
		CSRFSecret:         getenv("CSRF_SECRET", ""),
		SMTPHost:           getenv("SMTP_HOST", ""),
		SMTPPort:           getenv("SMTP_PORT", "587"),
		SMTPUser:           getenv("SMTP_USER", ""),
		SMTPPass:           getenv("SMTP_PASS", ""),
		SMTPFrom:           getenv("SMTP_FROM", ""),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
		VAPIDPrivateKey:    getenv("VAPID_PRIVATE_KEY", ""),
		VAPIDPublicKey:     getenv("VAPID_PUBLIC_KEY", ""),
		VAPIDSubject:       getenv("VAPID_SUBJECT", ""),
		TrustedProxyCIDRs:  getenv("TRUSTED_PROXY_CIDRS", ""),
	}

	if cfg.Port == "" {
		return Config{}, fmt.Errorf("PORT must not be empty")
	}
	if cfg.AppBaseURL == "" {
		return Config{}, fmt.Errorf("APP_BASE_URL must not be empty")
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
