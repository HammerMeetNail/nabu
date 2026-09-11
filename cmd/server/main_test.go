package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestStartupDiagnosticsDoNotPrintConnectionOrProviderValues(t *testing.T) {
	for _, cause := range []error{
		&pgconn.ConnectError{Config: &pgconn.Config{Host: "PRIVATE-HOST", User: "PRIVATE-USER", Database: "PRIVATE-DATABASE"}},
		&pgconn.PgError{Code: "23503", Message: "PRIVATE-EMAIL@example.invalid", Detail: "PRIVATE-HOUSEHOLD"},
	} {
		var output bytes.Buffer
		reportStartupError(log.New(&output, "", 0), fmt.Errorf("build server: %w", cause))
		if strings.Contains(output.String(), "PRIVATE") || !strings.Contains(output.String(), "operation=server error_class=") || !strings.Contains(output.String(), "request_id=") {
			t.Fatal("unsafe or missing startup diagnostic")
		}
	}
}

func TestStartupConfigDiagnosticsIdentifyTheRuleWithoutItsValue(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_BASE_URL", "https://PRIVATE-USER:PRIVATE-PASSWORD@example.invalid")
	_, err := config.Load()
	if err == nil {
		t.Fatal("invalid origin accepted")
	}
	var output bytes.Buffer
	reportStartupError(log.New(&output, "", 0), fmt.Errorf("load config: %w", err))
	if strings.Contains(output.String(), "PRIVATE") || !strings.Contains(output.String(), "APP_BASE_URL") || !strings.Contains(output.String(), "error_class=configuration") {
		t.Fatal("configuration diagnostic omitted the rule or exposed the value")
	}
}

func TestNewHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(":8080", http.NewServeMux())

	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %s, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %s, want 30s", srv.ReadTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %s, want 120s", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want 1<<20", srv.MaxHeaderBytes)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %s, want 30s (bounded exports)", srv.WriteTimeout)
	}
	if srv.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", srv.Addr)
	}
}

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
