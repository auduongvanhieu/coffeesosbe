// Command api is the CoffeeSOS backend.
//
//	api serve          run the HTTP/WebSocket server (default)
//	api migrate        apply pending migrations
//	api migrate down   roll back the latest migration
//	api seed           create platform admin + demo tenant
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coffeesos/internal/app"
	"coffeesos/internal/config"
	"coffeesos/internal/database"
	"coffeesos/internal/seed"
)

var version = "dev" // overridden with -ldflags "-X main.version=..."

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	if cmd == "version" {
		fmt.Println("coffeesos api", version)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	ctx := context.Background()

	switch cmd {
	case "serve":
		return serve(ctx, cfg, log)
	case "migrate":
		if len(args) > 1 && args[1] == "down" {
			return database.Rollback(cfg.DatabaseURL, log)
		}
		return database.Migrate(cfg.DatabaseURL, log)
	case "seed":
		pool, err := database.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		return seed.Run(ctx, pool, cfg, log)
	default:
		return fmt.Errorf("unknown command %q (expected serve|migrate|seed|version)", cmd)
	}
}

func serve(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if cfg.AutoMigrate {
		if err := database.Migrate(cfg.DatabaseURL, log); err != nil {
			return err
		}
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	a := app.New(cfg, pool, log)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           a.Engine,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	select {
	case err := <-errCh:
		return err
	case <-stop.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	var h slog.Handler
	if cfg.IsDev() {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	return slog.New(h)
}
