// Command emailvetter runs the email-vetting service: an HTTP API (Huma v2)
// over a rqlite-backed blocklist of disposable email domains.
//
//	go run ./cmd/emailvetter
//
// Huma serves OpenAPI 3.1 and 3.0 contracts at /openapi(.json|.yaml) and
// /openapi-3.0(.json|.yaml), and interactive docs at /docs.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/miguelpragier/emailvetter/internal/api"
	"github.com/miguelpragier/emailvetter/internal/config"
	"github.com/miguelpragier/emailvetter/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})))

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	st := store.New(cfg.RQLiteAddr, cfg.RQLiteUser, cfg.RQLitePass, cfg.RQLiteTimeout)
	if err := st.EnsureSchema(ctx); err != nil {
		return err
	}

	srv := api.New(api.Deps{Cfg: cfg, Store: st})
	slog.Info("starting emailvetter", "addr", cfg.Addr)
	return srv.Start(ctx)
}
