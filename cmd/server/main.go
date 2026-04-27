package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/dave/choresy/internal/app"
	"github.com/dave/choresy/internal/config"
)

func main() {
	if err := run(config.Load, app.BuildServer, http.ListenAndServe); err != nil {
		log.Fatalf("serve: %v", err)
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
