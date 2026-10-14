// Package daemon wires config, capture, storage and the scheduler together
// into the payam-loggerd background service.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/payamam/payam-logger/backend/internal/capture"
	"github.com/payamam/payam-logger/backend/internal/config"
	"github.com/payamam/payam-logger/backend/internal/scheduler"
	"github.com/payamam/payam-logger/backend/internal/storage"
)

// Daemon is the payam-loggerd background service.
type Daemon struct {
	cfg     *config.Config
	backend capture.Backend
	store   *storage.Store
	log     *slog.Logger
	sched   *scheduler.Scheduler
}

// New assembles the daemon from its parts.
func New(cfg *config.Config, backend capture.Backend, store *storage.Store, log *slog.Logger) *Daemon {
	return &Daemon{
		cfg:     cfg,
		backend: backend,
		store:   store,
		log:     log,
		sched:   scheduler.New(time.Duration(cfg.Capture.IntervalSec)*time.Second, log),
	}
}

// Run blocks until ctx is cancelled (SIGINT/SIGTERM).
func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("payam-loggerd started",
		"backend", d.backend.Name(),
		"interval_sec", d.cfg.Capture.IntervalSec,
		"data_dir", d.cfg.Storage.DataDir,
	)

	err := d.sched.Run(ctx, d.captureOnce)
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return err
}

// captureOnce is the scheduler job: take one screenshot and store it.
func (d *Daemon) captureOnce(ctx context.Context) error {
	shot, err := d.backend.Capture(ctx)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}

	rel, err := d.store.SaveScreenshot(shot)
	if err != nil {
		return err
	}

	d.log.Info("screenshot saved", "path", rel)
	return nil
}
