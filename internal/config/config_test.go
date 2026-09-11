package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Fatalf("AppBaseURL = %q", cfg.AppBaseURL)
	}
	if cfg.DatabaseURL != "" {
		t.Fatalf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.HTTPAddr() != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr())
	}
	if cfg.IsProduction() {
		t.Fatal("expected IsProduction() = false")
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_BASE_URL", "https://example.com")
	t.Setenv("DATABASE_URL", "postgres://localhost/nabu")
	t.Setenv("SMTP_HOST", "mailpit")
	t.Setenv("SMTP_PORT", "25")
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASS", "pass")
	t.Setenv("SMTP_FROM", "noreply@example.com")
	t.Setenv("GOOGLE_CLIENT_ID", "g-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "g-secret")
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	t.Setenv("SERVER_SECURE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.AppEnv != "production" {
		t.Fatalf("AppEnv = %q, want production", cfg.AppEnv)
	}
	if !cfg.IsProduction() {
		t.Fatal("expected IsProduction() = true")
	}
	if !cfg.ServerSecure {
		t.Fatal("expected ServerSecure = true")
	}
	if cfg.DatabaseURL != "postgres://localhost/nabu" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.SMTPHost != "mailpit" || cfg.SMTPPort != "25" || cfg.SMTPUser != "user" || cfg.SMTPPass != "pass" || cfg.SMTPFrom != "noreply@example.com" {
		t.Fatalf("SMTP config = %#v", cfg)
	}
	if cfg.GoogleClientID != "g-client" || cfg.GoogleClientSecret != "g-secret" {
		t.Fatalf("Google config = %#v", cfg)
	}
}

func TestLoadAppliesDefaultsForEmptyEnv(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("APP_BASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Fatalf("AppBaseURL = %q", cfg.AppBaseURL)
	}
}

func TestLoadRequiresDatabaseURLInProduction(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "https://example.com")
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when APP_ENV=production and DATABASE_URL is empty")
	}
}

func TestLoadAllowsEmptyDatabaseURLInDevelopment(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err != nil {
		t.Fatalf("Load returned error in development: %v", err)
	}
}

// TestLoad_RateLimitAuthMaxFromEnv covers the getenvInt success path (return n).
func TestLoad_RateLimitAuthMaxFromEnv(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("RATE_LIMIT_AUTH_MAX", "50")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.RateLimitAuthMax != 50 {
		t.Errorf("RateLimitAuthMax = %d, want 50", cfg.RateLimitAuthMax)
	}
}

func TestLoadRejectsInvalidDeploymentConfig(t *testing.T) {
	for _, tc := range []struct{ key, value, want string }{
		{"PORT", "0", "PORT"}, {"PORT", "65536", "PORT"}, {"PORT", "bad", "PORT"},
		{"SERVER_SECURE", "tru", "SERVER_SECURE"},
		{"APP_BASE_URL", "javascript:alert(1)", "APP_BASE_URL"},
		{"APP_BASE_URL", "https://secret:token@example.com", "APP_BASE_URL"},
		{"APP_BASE_URL", "https://example.com/path", "APP_BASE_URL"},
		{"APP_BASE_URL", "https://example.com?token=SECRET", "APP_BASE_URL"},
		{"APP_BASE_URL", "https://example.com#SECRET", "APP_BASE_URL"},
		{"APP_BASE_URL", "https://example.com:99999", "APP_BASE_URL"},
		{"TRUSTED_PROXY_CIDRS", "127.0.0.1,bad-SECRET", "TRUSTED_PROXY_CIDRS"},
		{"TRUSTED_PROXY_CIDRS", "::/0", "TRUSTED_PROXY_CIDRS"},
		{"RATE_LIMIT_AUTH_MAX", "0", "RATE_LIMIT_AUTH_MAX"},
		{"RATE_LIMIT_GLOBAL_MAX", "typo-SECRET", "RATE_LIMIT_GLOBAL_MAX"},
		{"RATE_LIMIT_JOIN_MAX", "-1", "RATE_LIMIT_JOIN_MAX"},
		{"RATE_LIMIT_MAX_CLIENTS", "1000001", "RATE_LIMIT_MAX_CLIENTS"},
		{"DB_MAX_OPEN_CONNS", "1", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_OPEN_CONNS", "9", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_OPEN_CONNS", "101", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_IDLE_CONNS", "26", "DB_MAX_IDLE_CONNS"},
		{"DB_MAX_IDLE_CONNS", "-1", "DB_MAX_IDLE_CONNS"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			t.Setenv("APP_ENV", "development")
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v", err)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("configuration value leaked in error")
			}
		})
	}
}

func TestProductionRequiresSecureURLCookiesAndProxyTrust(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"APP_BASE_URL", "http://example.com"},
		{"SERVER_SECURE", "false"},
		{"TRUSTED_PROXY_CIDRS", ""},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("DATABASE_URL", "postgres://localhost/nabu")
			t.Setenv("APP_BASE_URL", "https://example.com")
			t.Setenv("SERVER_SECURE", "true")
			t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1,::1")
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("unsafe production configuration accepted")
			}
		})
	}
}
