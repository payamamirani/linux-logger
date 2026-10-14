package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/payamam/payam-logger/backend/internal/capture"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir, filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func saveAt(t *testing.T, s *Store, ts time.Time) string {
	t.Helper()
	shot := &capture.Screenshot{
		Data:    []byte("jpeg-bytes"),
		MIME:    "image/jpeg",
		TakenAt: ts,
		Width:   1920,
		Height:  1080,
	}
	rel, err := s.SaveScreenshot(shot)
	if err != nil {
		t.Fatalf("SaveScreenshot(%v): %v", ts, err)
	}
	return rel
}

func TestSaveScreenshotLayout(t *testing.T) {
	s := newTestStore(t)
	ts := time.Date(2026, 10, 14, 9, 5, 3, 0, time.Local)

	rel := saveAt(t, s, ts)

	want := filepath.Join("screenshots", "2026", "10", "14", "090503.jpg")
	if rel != want {
		t.Errorf("rel = %q, want %q", rel, want)
	}
	if _, err := os.Stat(filepath.Join(s.DataDir(), want)); err != nil {
		t.Errorf("file not written: %v", err)
	}
}

func TestDaysAndQueries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	day14 := time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)
	day13 := time.Date(2026, 10, 13, 18, 30, 0, 0, time.Local)

	for i := 0; i < 3; i++ { // three shots on the 14th
		saveAt(t, s, day14.Add(time.Duration(i)*time.Minute))
	}
	saveAt(t, s, day13) // one shot on the 13th

	days, err := s.Days(ctx)
	if err != nil {
		t.Fatalf("Days: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("len(days) = %d, want 2", len(days))
	}
	// Newest day first.
	if days[0].Day != "2026-10-14" || days[1].Day != "2026-10-13" {
		t.Errorf("day order = %s, %s — want 2026-10-14 first", days[0].Day, days[1].Day)
	}
	if days[0].Count != 3 {
		t.Errorf("day 14 count = %d, want 3", days[0].Count)
	}
	if want := int64(3 * len("jpeg-bytes")); days[0].TotalSize != want {
		t.Errorf("day 14 total size = %d, want %d", days[0].TotalSize, want)
	}

	recs, err := s.ScreenshotsForDay(ctx, "2026-10-14")
	if err != nil {
		t.Fatalf("ScreenshotsForDay: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("len(recs) = %d, want 3", len(recs))
	}
	// Ordered by time, fields populated.
	first := recs[0]
	if first.Path == "" || first.Width != 1920 || first.Height != 1080 {
		t.Errorf("record fields incomplete: %+v", first)
	}
	if recs[0].TakenAt == recs[1].TakenAt || recs[1].TakenAt == recs[2].TakenAt {
		t.Error("records not ordered or duplicated timestamps")
	}

	// Unknown day → empty, no error.
	recs, err = s.ScreenshotsForDay(ctx, "2020-01-01")
	if err != nil || len(recs) != 0 {
		t.Errorf("unknown day: recs=%d err=%v, want 0/nil", len(recs), err)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	s.Close() // close and reopen the same DB — schema must already be current
	reopened, err := New(s.dataDir, s.dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
}
