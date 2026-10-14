package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupRemovesOnlyExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	old1 := time.Now().AddDate(0, 0, -40)
	old2 := old1.Add(time.Hour)
	recent := time.Now().Add(-time.Hour)

	saveAt(t, s, old1)
	saveAt(t, s, old2)
	saveAt(t, s, recent)

	removed, err := s.Cleanup(ctx, time.Now().AddDate(0, 0, -30)) // keep last 30 days
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}

	// Index rows gone for old days, recent kept.
	days, err := s.Days(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Count != 1 {
		t.Fatalf("days after cleanup = %+v, want single recent day with 1 shot", days)
	}

	// Old day directory pruned, recent file still on disk.
	oldDir := filepath.Join(s.DataDir(), "screenshots",
		fmt.Sprintf("%04d/%02d/%02d", old1.Year(), int(old1.Month()), old1.Day()))
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old day dir still exists: %v", err)
	}
	recentRel := saveAt(t, s, recent.Add(time.Minute)) // ensure dir alive
	if _, err := os.Stat(filepath.Join(s.DataDir(), recentRel)); err != nil {
		t.Errorf("recent file missing: %v", err)
	}
}

func TestCleanupNoopWhenNothingExpired(t *testing.T) {
	s := newTestStore(t)
	saveAt(t, s, time.Now())

	removed, err := s.Cleanup(context.Background(), time.Now().AddDate(0, 0, -365))
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}
