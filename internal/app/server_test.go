package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dave/choresy/internal/config"
)

func TestNewServerHealthEndpoint(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	server := NewServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewServerReadyEndpoint(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	server := NewServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewServerServesIndex(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	server := NewServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected HTML response body")
	}
	if !strings.Contains(body, "Choresy") {
		t.Fatal("expected Choresy in response body")
	}
}

func TestNewServerReturns404ForUnknownAPI(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	server := NewServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestBuildServerWithoutDBReturnsInMemory(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	handler, closer, err := BuildServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildServer returned error: %v", err)
	}
	if closer == nil {
		t.Fatal("expected non-nil closer")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("closer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMethodWrapperEnforcesHTTPVerb(t *testing.T) {
	handler := method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestBuildVersionedSWInjectsVersion(t *testing.T) {
	// Use a MapFS to simulate the embedded service-worker.js without relying
	// on the real //go:embed FS (which only works at build time).
	swSource := []byte(`const CACHE_NAME = "choresy-static-v1";`)
	fsys := fstest.MapFS{
		"service-worker.js": {Data: swSource},
	}

	// buildVersionedSW should replace "v1" with the given version.
	result := buildVersionedSW(fsys, "0.1.99")
	expected := `const CACHE_NAME = "choresy-static-0.1.99";`
	if string(result) != expected {
		t.Fatalf("got  %q\nwant %q", string(result), expected)
	}
}

func TestBuildVersionedSWHandlesArbitraryBaseVersion(t *testing.T) {
	// The regex must match any numeric base version (v1, v2, v99, etc.)
	// so a future source change doesn't silently break version injection.
	swSource := []byte(`const CACHE_NAME = "choresy-static-v99";`)
	fsys := fstest.MapFS{
		"service-worker.js": {Data: swSource},
	}
	result := buildVersionedSW(fsys, "2.0.0")
	expected := `const CACHE_NAME = "choresy-static-2.0.0";`
	if string(result) != expected {
		t.Fatalf("got  %q\nwant %q", string(result), expected)
	}
}

func TestMethodWrapperAllowsMatchingVerb(t *testing.T) {
	handler := method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
