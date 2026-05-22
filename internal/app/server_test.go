package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
