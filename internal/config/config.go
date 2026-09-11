package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/clientip"
)

// ValidationError contains only controlled field/rule descriptions, never env values.
type ValidationError struct{ message string }

func (e *ValidationError) Error() string { return e.message }
func invalidConfig(format string, args ...any) error {
	return &ValidationError{message: fmt.Sprintf(format, args...)}
}

type Config struct {
	Port               string
	AppEnv             string
	AppBaseURL         string
	ServerSecure       bool
	DatabaseURL        string
	DBMaxOpenConns     int
	DBMaxIdleConns     int
	SMTPHost           string
	SMTPPort           string
	SMTPUser           string
	SMTPPass           string
	SMTPFrom           string
	GoogleClientID     string
	GoogleClientSecret string
	// AppleClientIDs are the audiences accepted on Sign in with Apple
	// identity tokens: the iOS bundle ID and, if the web flow is added
	// later, the Services ID. Comma-separated; empty disables the endpoint.
	AppleClientIDs string
	// AppleWebClientID is the Services ID used by the web Sign in with
	// Apple flow (it must have AppBaseURL's domain and the
	// /api/auth/apple/web/callback return URL registered in the Apple
	// developer portal). Empty disables the web flow; it is always
	// accepted as an identity-token audience when set.
	AppleWebClientID    string
	TrustedProxyCIDRs   string
	RateLimitAuthMax    int
	RateLimitGlobalMax  int
	RateLimitJoinMax    int
	RateLimitMaxClients int
	VAPIDPublicKey      string
	VAPIDPrivateKey     string
	VAPIDSubject        string
	// APNs (native iOS push). All four must be set to enable the sender;
	// unset leaves APNs a graceful no-op like the VAPID signer.
	APNSAuthKeyP8 string // contents of the .p8 auth key (PEM, or base64 of it)
	APNSKeyID     string
	APNSTeamID    string
	APNSBundleID  string
}

func Load() (Config, error) {
	cfg := Config{
		Port:               getenv("PORT", "8080"),
		AppEnv:             getenv("APP_ENV", "development"),
		AppBaseURL:         getenv("APP_BASE_URL", "http://localhost:8080"),
		DatabaseURL:        getenv("DATABASE_URL", ""),
		SMTPHost:           getenv("SMTP_HOST", ""),
		SMTPPort:           getenv("SMTP_PORT", "587"),
		SMTPUser:           getenv("SMTP_USER", ""),
		SMTPPass:           getenv("SMTP_PASS", ""),
		SMTPFrom:           getenv("SMTP_FROM", ""),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
		AppleClientIDs:     getenv("APPLE_CLIENT_IDS", ""),
		AppleWebClientID:   getenv("APPLE_WEB_CLIENT_ID", ""),
		TrustedProxyCIDRs:  getenv("TRUSTED_PROXY_CIDRS", ""),
		VAPIDPublicKey:     getenv("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey:    getenv("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:       getenv("VAPID_SUBJECT", ""),
		APNSAuthKeyP8:      getenv("APNS_AUTH_KEY_P8", ""),
		APNSKeyID:          getenv("APNS_KEY_ID", ""),
		APNSTeamID:         getenv("APNS_TEAM_ID", ""),
		APNSBundleID:       getenv("APNS_BUNDLE_ID", ""),
	}

	var err error
	cfg.ServerSecure, err = strconv.ParseBool(getenv("SERVER_SECURE", "false"))
	if err != nil {
		return Config{}, invalidConfig("SERVER_SECURE must be a boolean")
	}
	for _, value := range []struct {
		key      string
		fallback int
		target   *int
	}{
		{"RATE_LIMIT_AUTH_MAX", 5, &cfg.RateLimitAuthMax},
		{"RATE_LIMIT_GLOBAL_MAX", 120, &cfg.RateLimitGlobalMax},
		{"RATE_LIMIT_JOIN_MAX", 10, &cfg.RateLimitJoinMax},
		{"RATE_LIMIT_MAX_CLIENTS", 4096, &cfg.RateLimitMaxClients},
		{"DB_MAX_OPEN_CONNS", 25, &cfg.DBMaxOpenConns},
		{"DB_MAX_IDLE_CONNS", 5, &cfg.DBMaxIdleConns},
	} {
		*value.target, err = getenvInt(value.key, value.fallback)
		if err != nil {
			return Config{}, err
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate also covers callers constructing a Config without Load. It runs
// before opening dependencies or starting background workers.
func (c Config) Validate() error {
	// Fail fast rather than silently falling back to the ephemeral in-memory
	// store in production: without a database the app loses all data on restart
	// and cannot share state across instances. The in-memory path remains
	// available for development (APP_ENV != production).
	if c.IsProduction() && strings.TrimSpace(c.DatabaseURL) == "" {
		return invalidConfig("DATABASE_URL must be set when APP_ENV=production")
	}
	if port, err := strconv.Atoi(c.Port); err != nil || port < 1 || port > 65535 {
		return invalidConfig("PORT must be an integer from 1 to 65535")
	}
	if c.DBMaxOpenConns < 10 || c.DBMaxOpenConns > 100 || c.DBMaxIdleConns < 0 || c.DBMaxIdleConns > c.DBMaxOpenConns {
		return invalidConfig("DB_MAX_OPEN_CONNS must be 10..100 (8 reserved for delivery) and DB_MAX_IDLE_CONNS must be 0..DB_MAX_OPEN_CONNS")
	}
	u, err := url.Parse(c.AppBaseURL)
	if err != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return invalidConfig("APP_BASE_URL must be an HTTP(S) origin without credentials, query, or fragment")
	}
	if u.Port() != "" {
		if port, err := strconv.Atoi(u.Port()); err != nil || port < 1 || port > 65535 {
			return invalidConfig("APP_BASE_URL port must be an integer from 1 to 65535")
		}
	}
	if c.IsProduction() && (u.Scheme != "https" || !c.ServerSecure) {
		return invalidConfig("production requires an HTTPS APP_BASE_URL and SERVER_SECURE=true")
	}
	prefixes, err := clientip.ParseTrustedProxies(c.TrustedProxyCIDRs)
	if err != nil {
		return invalidConfig("%s", err)
	}
	if c.IsProduction() && len(prefixes) == 0 {
		return invalidConfig("TRUSTED_PROXY_CIDRS must be set when APP_ENV=production")
	}
	for _, value := range []struct {
		key string
		n   int
	}{
		{"RATE_LIMIT_AUTH_MAX", c.RateLimitAuthMax},
		{"RATE_LIMIT_GLOBAL_MAX", c.RateLimitGlobalMax},
		{"RATE_LIMIT_JOIN_MAX", c.RateLimitJoinMax},
		{"RATE_LIMIT_MAX_CLIENTS", c.RateLimitMaxClients},
	} {
		if value.n < 1 || value.n > 1000000 {
			return invalidConfig("%s must be an integer from 1 to 1000000", value.key)
		}
	}
	return nil
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

func getenvInt(key string, fallback int) (int, error) {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n, nil
		}
		return 0, invalidConfig("%s must be an integer", key)
	}
	return fallback, nil
}
