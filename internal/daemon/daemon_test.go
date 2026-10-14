package daemon

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/payamam/payam-logger/internal/capture"
	"github.com/payamam/payam-logger/internal/config"
	"github.com/payamam/payam-logger/internal/storage"
)

// fakeBackend counts Capture calls.
type fakeBackend struct{ calls int }

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Capture(ctx context.Context) (*capture.Screenshot, error) {
	f.calls++
	return &capture.Screenshot{
		Data:    []byte("jpeg"),
		MIME:    "image/jpeg",
		TakenAt: time.Now(),
		Width:   10,
		Height:  10,
	}, nil
}

func newTestDaemon(t *testing.T) (*Daemon, *fakeBackend, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(dir, filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	cfg := config.Default()
	cfg.Capture.IntervalSec = 60
	b := &fakeBackend{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(&cfg, b, store, log)
	t.Cleanup(d.Close)
	return d, b, store
}

func TestCaptureOnceCaptures(t *testing.T) {
	d, b, _ := newTestDaemon(t)
	d.isLocked = func(context.Context) bool { return false }

	if err := d.captureOnce(context.Background()); err != nil {
		t.Fatalf("captureOnce: %v", err)
	}
	if b.calls != 1 {
		t.Errorf("backend calls = %d, want 1", b.calls)
	}
}

func TestCaptureOnceSkipsWhenLocked(t *testing.T) {
	d, b, store := newTestDaemon(t)
	d.isLocked = func(context.Context) bool { return true }

	if err := d.captureOnce(context.Background()); err != nil {
		t.Fatalf("captureOnce: %v", err)
	}
	if b.calls != 0 {
		t.Errorf("backend calls = %d, want 0 while locked", b.calls)
	}

	// Nothing stored either.
	recs, err := store.ScreenshotsForDay(context.Background(), time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Errorf("records = %d, want 0 while locked", len(recs))
	}
}

func TestCaptureOnceWorksWhenLockDetectionUnavailable(t *testing.T) {
	d, b, _ := newTestDaemon(t)
	d.isLocked = nil // bus unavailable path

	if err := d.captureOnce(context.Background()); err != nil {
		t.Fatalf("captureOnce: %v", err)
	}
	if b.calls != 1 {
		t.Errorf("backend calls = %d, want 1", b.calls)
	}
}
