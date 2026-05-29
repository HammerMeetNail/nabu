package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dave/choresy/internal/config"
)

func TestRunServesOnPort(t *testing.T) {
	loadConfig := func() (config.Config, error) {
		return config.Config{}, errors.New("load failed")
	}
	buildServer := func(_ context.Context, _ config.Config) (http.Handler, io.Closer, error) {
		return nil, nil, nil
	}
	serve := func(_ string, _ http.Handler) error { return nil }

	err := run(loadConfig, buildServer, serve)
	if err == nil {
		t.Fatal("expected error from loadConfig failure")
	}
}

func TestRunBuildsAndServes(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	loadConfig := func() (config.Config, error) { return cfg, nil }
	buildServer := func(_ context.Context, _ config.Config) (http.Handler, io.Closer, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		return mux, io.NopCloser(nil), nil
	}
	serve := func(addr string, h http.Handler) error {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		return nil
	}

	if err := run(loadConfig, buildServer, serve); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestRunBuildServerError(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	loadConfig := func() (config.Config, error) { return cfg, nil }
	buildServer := func(_ context.Context, _ config.Config) (http.Handler, io.Closer, error) {
		return nil, nil, errors.New("db failure")
	}
	serve := func(_ string, _ http.Handler) error { return nil }

	err = run(loadConfig, buildServer, serve)
	if err == nil {
		t.Fatal("expected error from buildServer failure")
	}
	if err.Error()[:13] != "build server:" {
		t.Errorf("wrong error prefix: %v", err)
	}
}

func TestRunServeError(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	loadConfig := func() (config.Config, error) { return cfg, nil }
	buildServer := func(_ context.Context, _ config.Config) (http.Handler, io.Closer, error) {
		return http.NewServeMux(), io.NopCloser(nil), nil
	}
	serve := func(_ string, _ http.Handler) error {
		return errors.New("port busy")
	}

	err = run(loadConfig, buildServer, serve)
	if err == nil {
		t.Fatal("expected error from serve failure")
	}
	if err.Error()[:17] != "listen and serve:" {
		t.Errorf("wrong error prefix: %v", err)
	}
}
