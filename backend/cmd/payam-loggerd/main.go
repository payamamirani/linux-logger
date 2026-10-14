// Command payam-loggerd is the background service of Payam Logger.
//
// It captures a screenshot of the active session at a fixed interval
// (default: 60s) and stores it locally together with metadata, similar to
// ManicTime's screenshot tracking. It is designed to run as a systemd user
// service bound to the graphical session.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/payamam/payam-logger/backend/internal/capture"
	"github.com/payamam/payam-logger/backend/internal/config"
	"github.com/payamam/payam-logger/backend/internal/daemon"
	"github.com/payamam/payam-logger/backend/internal/storage"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "",
		"path to config JSON (default: built-in defaults)")
	flag.Parse()

	if err := run(configPath); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// Log to stderr: under systemd this ends up in the journal.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLevel(cfg.Log.Level),
	}))

	backend, err := capture.New(capture.Options{
		Backend:     cfg.Capture.Backend,
		DataDir:     cfg.Storage.DataDir,
		JPEGQuality: cfg.Capture.JPEGQuality,
	}, logger)
	if err != nil {
		return err
	}

	store, err := storage.New(cfg.Storage.DataDir, cfg.Storage.DBPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := daemon.New(&cfg, backend, store, logger)
	if err := d.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("payam-loggerd stopped")
	return nil
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
