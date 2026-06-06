package config

import "testing"

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
