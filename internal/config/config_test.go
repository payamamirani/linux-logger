package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Capture.IntervalSec != DefaultIntervalSec {
		t.Errorf("default interval = %d, want %d", cfg.Capture.IntervalSec, DefaultIntervalSec)
	}
	if cfg.Storage.RetentionDays != DefaultRetentionDays {
		t.Errorf("default retention = %d, want %d", cfg.Storage.RetentionDays, DefaultRetentionDays)
	}
}

func TestLoadEmptyPathGivesResolvedDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Storage.DataDir == "" || cfg.Storage.DBPath == "" {
		t.Errorf("data dir not resolved: %+v", cfg.Storage)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidCustom(t *testing.T) {
	path := writeConfig(t, `{
		"capture": {"interval_sec": 600},
		"storage": {"retention_days": 30}
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Capture.IntervalSec != 600 {
		t.Errorf("interval = %d, want 600", cfg.Capture.IntervalSec)
	}
	if cfg.Storage.RetentionDays != 30 {
		t.Errorf("retention = %d, want 30", cfg.Storage.RetentionDays)
	}
}

func TestValidationBounds(t *testing.T) {
	cases := []struct {
		name, json string
	}{
		{"interval below minimum", `{"capture": {"interval_sec": 30}}`},
		{"interval not whole minutes", `{"capture": {"interval_sec": 90}}`},
		{"retention too large", `{"storage": {"retention_days": 61}}`},
		{"retention negative", `{"storage": {"retention_days": -1}}`},
		{"jpeg quality out of range", `{"capture": {"interval_sec": 60, "jpeg_quality": 0}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, c.json)); err == nil {
				t.Fatalf("expected validation error for %s", c.json)
			}
		})
	}
}

func TestZeroRetentionMeansForever(t *testing.T) {
	path := writeConfig(t, `{"storage": {"retention_days": 0}}`)
	if _, err := Load(path); err != nil {
		t.Fatalf("retention 0 must be valid (keep forever): %v", err)
	}
}
