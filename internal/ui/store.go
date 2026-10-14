// Package ui implements the payam-logger GTK4 viewer.
package ui

import (
	"context"
	"os"
	"path/filepath"

	"github.com/payamam/payam-logger/internal/config"
	"github.com/payamam/payam-logger/internal/storage"
)

// Store gives the UI read access to the index and the screenshot files.
// It shares the SQLite database with the daemon (WAL mode: concurrent
// readers are fine).
type Store struct {
	st          *storage.Store
	dataDir     string
	intervalSec int
}

// OpenStore opens the default data location (~/.local/share/payam-logger).
func OpenStore() (*Store, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	st, err := storage.New(cfg.Storage.DataDir, cfg.Storage.DBPath)
	if err != nil {
		return nil, err
	}
	return &Store{
		st:          st,
		dataDir:     cfg.Storage.DataDir,
		intervalSec: cfg.Capture.IntervalSec,
	}, nil
}

// IntervalSec returns the configured capture interval — the timeline draws
// one bucket per interval.
func (s *Store) IntervalSec() int { return s.intervalSec }

// Close closes the underlying database.
func (s *Store) Close() error { return s.st.Close() }

// Days returns one summary per recorded day, newest first.
func (s *Store) Days(ctx context.Context) ([]storage.DaySummary, error) {
	return s.st.Days(ctx)
}

// Records returns all screenshots of one day ordered by time.
func (s *Store) Records(ctx context.Context, day string) ([]storage.Record, error) {
	return s.st.ScreenshotsForDay(ctx, day)
}

// AppUsage returns the focused-window segments of one day.
func (s *Store) AppUsage(ctx context.Context, day string) ([]storage.AppUsageRecord, error) {
	return s.st.AppUsageForDay(ctx, day)
}

// ComputerUsage returns the active/away segments of one day.
func (s *Store) ComputerUsage(ctx context.Context, day string) ([]storage.ComputerUsageRecord, error) {
	return s.st.ComputerUsageForDay(ctx, day)
}

// ReadScreenshot returns the raw JPEG bytes of a record.
func (s *Store) ReadScreenshot(rec storage.Record) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.dataDir, rec.Path))
}
