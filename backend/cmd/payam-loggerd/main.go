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
	"path/filepath"
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
	// Default config location, used when -config is not passed: pick it up
	// only if it exists so bare invocations still get sane defaults.
	if configPath == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			candidate := filepath.Join(dir, "payam-logger", "config.json")
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
			}
		}
	}

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
	defer d.Close()
	runErr := d.Run(ctx)
	if cerr := store.Close(); cerr != nil {
		logger.Warn("close index db", "err", cerr)
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
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
