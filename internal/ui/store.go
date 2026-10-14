// Package ui implements the payam-logger GTK4 viewer.
package ui

import (
	"context"
	"os"
	"path/filepath"
	"time"

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

// TagsForDay returns the manual tags of one day.
func (s *Store) TagsForDay(ctx context.Context, day string) ([]storage.TagRecord, error) {
	return s.st.TagsForDay(ctx, day)
}

// AddTag records a manual tag.
func (s *Store) AddTag(start, end time.Time, name, note string, billable bool) error {
	return s.st.AddTag(start, end, name, note, billable)
}

// DeleteTag removes a tag.
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	return s.st.DeleteTag(ctx, id)
}

// TagRowsRange returns raw tag rows for the timesheet.
func (s *Store) TagRowsRange(ctx context.Context, from, to string) ([]storage.TagRow, error) {
	return s.st.TagRowsRange(ctx, from, to)
}

// DeleteScreenshot removes a screenshot file and row.
func (s *Store) DeleteScreenshot(ctx context.Context, id int64) error {
	return s.st.DeleteScreenshot(ctx, id)
}

// Autotags returns all autotag rules.
func (s *Store) Autotags(ctx context.Context) ([]storage.AutotagRule, error) {
	return s.st.Autotags(ctx)
}

// AddAutotag stores a rule.
func (s *Store) AddAutotag(ctx context.Context, name, appPattern, titlePattern string) error {
	return s.st.AddAutotag(ctx, name, appPattern, titlePattern)
}

// DeleteAutotag removes a rule.
func (s *Store) DeleteAutotag(ctx context.Context, id int64) error {
	return s.st.DeleteAutotag(ctx, id)
}

// ReadScreenshot returns the raw JPEG bytes of a record.
func (s *Store) ReadScreenshot(rec storage.Record) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.dataDir, rec.Path))
}
