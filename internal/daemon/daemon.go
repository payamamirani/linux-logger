// Package daemon wires config, capture, storage and the scheduler together
// into the payam-loggerd background service.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/payamam/payam-logger/internal/activity"
	"github.com/payamam/payam-logger/internal/capture"
	"github.com/payamam/payam-logger/internal/config"
	"github.com/payamam/payam-logger/internal/scheduler"
	"github.com/payamam/payam-logger/internal/storage"
)

const (
	screensaverName  = "org.gnome.ScreenSaver"
	screensaverIface = "org.gnome.ScreenSaver"

	// cleanupInterval is how often expired screenshots are purged (also
	// runs once at startup, catching up after downtime).
	cleanupInterval = 24 * time.Hour
)

// Daemon is the payam-loggerd background service.
type Daemon struct {
	cfg       *config.Config
	backend   capture.Backend
	store     *storage.Store
	log       *slog.Logger
	sched     *scheduler.Scheduler
	cleanup   *scheduler.Scheduler // nil when retention is disabled
	collector *activity.Collector
	bus       *dbus.Conn
	wg        sync.WaitGroup

	// isLocked reports whether the session is currently locked. Injectable
	// for tests; nil means lock detection is unavailable.
	isLocked func(ctx context.Context) bool
}

// New assembles the daemon from its parts.
func New(cfg *config.Config, backend capture.Backend, store *storage.Store, log *slog.Logger) *Daemon {
	d := &Daemon{
		cfg:     cfg,
		backend: backend,
		store:   store,
		log:     log,
		sched:   scheduler.New(time.Duration(cfg.Capture.IntervalSec)*time.Second, log),
	}

	// Lock detection (GNOME screensaver). When unavailable we simply
	// keep capturing; the extension itself also refuses while locked.
	// Private connection so Close() never hits the shared godbus one.
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Warn("session bus unavailable — lock detection disabled", "err", err)
	} else {
		d.bus = bus
		d.isLocked = d.sessionLocked
	}

	if cfg.Storage.RetentionDays > 0 {
		d.cleanup = scheduler.New(cleanupInterval, log)
	}

	// Activity tracking (M1): focused window + active/away segments from
	// the GNOME Shell extension. Optional — screenshots work without it.
	if collector, err := activity.NewCollector(store, log); err != nil {
		log.Warn("activity tracking disabled", "err", err)
	} else {
		d.collector = collector
	}

	return d
}

// Close releases daemon resources (the index DB stays with the caller).
func (d *Daemon) Close() {
	if d.bus != nil {
		d.bus.Close()
	}
}

// Run blocks until ctx is cancelled (SIGINT/SIGTERM).
func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("payam-loggerd started",
		"backend", d.backend.Name(),
		"interval_sec", d.cfg.Capture.IntervalSec,
		"data_dir", d.cfg.Storage.DataDir,
		"retention_days", d.cfg.Storage.RetentionDays,
	)

	if d.cleanup != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			if err := d.cleanup.Run(ctx, d.cleanupOnce); err != nil &&
				!errors.Is(err, context.Canceled) {
				d.log.Error("cleanup scheduler failed", "err", err)
			}
		}()
	}

	if d.collector != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			if err := d.collector.Run(ctx); err != nil &&
				!errors.Is(err, context.Canceled) {
				d.log.Error("activity collector failed", "err", err)
			}
		}()
	}

	err := d.sched.Run(ctx, d.captureOnce)
	d.wg.Wait()

	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return err
}

// captureOnce is the scheduler job: take one screenshot and store it.
func (d *Daemon) captureOnce(ctx context.Context) error {
	// Privacy + noise: skip while the session is locked (also covers the
	// lock screen shown right after resume from suspend).
	if d.isLocked != nil && d.isLocked(ctx) {
		d.log.Info("skipping capture: session locked")
		return nil
	}

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

// cleanupOnce is the retention job: purge screenshots older than the
// configured retention window (files + index rows).
func (d *Daemon) cleanupOnce(ctx context.Context) error {
	before := time.Now().AddDate(0, 0, -d.cfg.Storage.RetentionDays)
	removed, err := d.store.Cleanup(ctx, before)
	if err != nil {
		return err
	}
	if removed > 0 {
		d.log.Info("retention cleanup", "removed", removed, "older_than", before.Format("2006-01-02"))
	}
	return nil
}

// sessionLocked asks the GNOME screensaver whether the session is locked.
// Unavailable interface (non-GNOME desktop) is treated as unlocked.
func (d *Daemon) sessionLocked(ctx context.Context) bool {
	var active bool
	err := d.bus.Object(screensaverName, "/org/gnome/ScreenSaver").
		CallWithContext(ctx, screensaverIface+".GetActive", 0).Store(&active)
	if err != nil {
		return false
	}
	return active
}
