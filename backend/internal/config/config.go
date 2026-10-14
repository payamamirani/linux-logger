// Package config loads payam-loggerd settings.
//
// Settings are read from a JSON file; every field has a sane default, so an
// empty (or missing) config file is valid.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Built-in defaults.
const (
	DefaultBackend     = "auto" // detect Wayland/X11 at runtime
	DefaultIntervalSec = 60
	DefaultJPEGQuality = 80
)

// Config is the root configuration object.
type Config struct {
	Capture CaptureConfig `json:"capture"`
	Storage StorageConfig `json:"storage"`
	Log     LogConfig     `json:"log"`
}

// CaptureConfig controls how and when screenshots are taken.
type CaptureConfig struct {
	// Backend selects the capture implementation:
	//  auto | portal | gnome-shell | x11
	// See docs/capture.md for the trade-offs.
	Backend string `json:"backend"`

	// IntervalSec is the number of seconds between two captures.
	IntervalSec int `json:"interval_sec"`

	// ImageFormat of stored files; currently only "jpeg".
	ImageFormat string `json:"image_format"`

	// JPEGQuality (1-100) of stored screenshots.
	JPEGQuality int `json:"jpeg_quality"`
}

// StorageConfig controls where data lives on disk.
type StorageConfig struct {
	// DataDir holds screenshots and the index DB.
	// Empty means ~/.local/share/payam-logger (XDG aware).
	DataDir string `json:"data_dir"`

	// DBPath of the SQLite index. Empty means <DataDir>/index.db.
	DBPath string `json:"db_path"`

	// RetentionDays removes older data; 0 keeps everything forever.
	RetentionDays int `json:"retention_days"`
}

// LogConfig controls logging (output goes to stderr, i.e. journald).
type LogConfig struct {
	// Level: debug | info | warn | error
	Level string `json:"level"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		Capture: CaptureConfig{
			Backend:     DefaultBackend,
			IntervalSec: DefaultIntervalSec,
			ImageFormat: "jpeg",
			JPEGQuality: DefaultJPEGQuality,
		},
		Storage: StorageConfig{},
		Log:     LogConfig{Level: "info"},
	}
}

// Load reads path and merges it over the defaults. An empty path returns the
// plain defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		cfg.resolve()
		return cfg, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	cfg.resolve()
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Capture.IntervalSec <= 0 {
		return fmt.Errorf("capture.interval_sec must be > 0")
	}
	if q := c.Capture.JPEGQuality; q < 1 || q > 100 {
		return fmt.Errorf("capture.jpeg_quality must be between 1 and 100")
	}
	return nil
}

// resolve fills empty fields with environment-aware defaults.
func (c *Config) resolve() {
	if c.Storage.DataDir == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err == nil {
				base = filepath.Join(home, ".local", "share")
			}
		}
		c.Storage.DataDir = filepath.Join(base, "payam-logger")
	}
	if c.Storage.DBPath == "" {
		c.Storage.DBPath = filepath.Join(c.Storage.DataDir, "index.db")
	}
}
