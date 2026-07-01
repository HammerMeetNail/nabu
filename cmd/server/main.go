package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HammerMeetNail/nabu/internal/app"
	"github.com/HammerMeetNail/nabu/internal/config"
)

func main() {
	if err := run(config.Load, app.BuildServer, serve); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// shutdownTimeout bounds how long we wait for in-flight requests to drain.
const shutdownTimeout = 15 * time.Second

// serve runs an HTTP server that shuts down gracefully on SIGINT/SIGTERM,
// draining in-flight requests before returning. When it returns, run's
// deferred closer tears down the scheduler and rate-limiter goroutines.
func serve(addr string, h http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: h}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Printf("shutdown signal received, draining (timeout=%s)", shutdownTimeout)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func run(
	loadConfig func() (config.Config, error),
	buildServer func(context.Context, config.Config) (http.Handler, io.Closer, error),
	serve func(string, http.Handler) error,
) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	server, closer, err := buildServer(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}
	if closer != nil {
		defer closer.Close()
	}

	addr := cfg.HTTPAddr()
	log.Printf("listening on %s", addr)
	if err := serve(addr, server); err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}
