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

	// DefaultServerPort for the embedded web UI (localhost only).
	DefaultServerPort = 7530

	// DefaultRetentionDays is how long screenshots are kept before the
	// daily cleanup purges them. 0 means keep forever.
	DefaultRetentionDays = 10
)

// Validation bounds.
const (
	// MinIntervalSec — one screenshot per minute at most (per spec).
	MinIntervalSec = 60

	// MaxRetentionDays caps retention; 0 is the special "keep forever".
	MaxRetentionDays = 60
)

// Config is the root configuration object.
type Config struct {
	Capture CaptureConfig `json:"capture"`
	Storage StorageConfig `json:"storage"`
	Server  ServerConfig  `json:"server"`
	Log     LogConfig     `json:"log"`
}

// ServerConfig controls the embedded web UI.
type ServerConfig struct {
	// Port the UI/API listens on (bound to 127.0.0.1 only).
	Port int `json:"port"`
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
		Storage: StorageConfig{
			DataDir:       "", // resolved by Resolve/DataDir
			DBPath:        "",
			RetentionDays: DefaultRetentionDays,
		},
		Server: ServerConfig{Port: DefaultServerPort},
		Log:    LogConfig{Level: "info"},
	}
}

// DefaultPath returns the default config file location
// (~/.config/payam-logger/config.json). Parent directories are not created.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "payam-logger", "config.json"), nil
}

// Save writes the config to path as pretty JSON, creating parent
// directories as needed. File mode 0600 (config may contain local paths).
func (c Config) Save(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
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
	if err := c.ValidatePublic(); err != nil {
		return err
	}
	return nil
}

// ValidatePublic applies the product rules; used by both Load and the web
// settings API.
func (c *Config) ValidatePublic() error {
	if c.Capture.IntervalSec < MinIntervalSec {
		return fmt.Errorf("capture.interval_sec must be at least %d (one screenshot per minute at most)", MinIntervalSec)
	}
	if c.Capture.IntervalSec%MinIntervalSec != 0 {
		return fmt.Errorf("capture.interval_sec must be a multiple of %d (whole minutes)", MinIntervalSec)
	}
	if q := c.Capture.JPEGQuality; q < 1 || q > 100 {
		return fmt.Errorf("capture.jpeg_quality must be between 1 and 100")
	}
	if r := c.Storage.RetentionDays; r < 0 || r > MaxRetentionDays {
		return fmt.Errorf("storage.retention_days must be between 1 and %d (or 0 to keep forever)", MaxRetentionDays)
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
