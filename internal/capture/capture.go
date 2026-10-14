// Package capture takes screenshots of the active session.
//
// Capturing is backend-specific because the correct method depends on the
// display server and desktop environment:
//
//   - "extension": calls the payam-logger GNOME Shell extension
//     (org.payam.Logger), which uses gnome-shell's internal Shell.Screenshot
//     object. Fully silent: no dialog, no flash, no shutter sound.
//     Primary backend on GNOME Wayland (see docs/capture.md).
//   - "portal": XDG Desktop Portal screenshot — works everywhere on Wayland
//     but flashes and plays a sound on every capture (hardcoded in
//     xdg-desktop-portal-gnome). Fallback.
//   - "x11": direct X11 grab for Xorg sessions (maim/scrot). Fallback.
//
// "auto" probes the session bus and picks the best available backend.
package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/godbus/dbus/v5"
)

// ErrNotImplemented marks backend stubs that are pending implementation.
var ErrNotImplemented = errors.New("capture backend not implemented yet")

// Screenshot is a single captured frame, already re-encoded as JPEG.
type Screenshot struct {
	// Data holds the encoded JPEG bytes.
	Data []byte

	// MIME type of Data ("image/jpeg").
	MIME string

	// TakenAt is the capture timestamp in local wall-clock time — the
	// storage layer uses it for the on-disk YYYY/MM/DD layout.
	TakenAt time.Time

	// Width and Height of the captured image in pixels.
	Width, Height int
}

// Backend captures a picture of the current screen.
type Backend interface {
	// Name returns the config identifier, e.g. "extension".
	Name() string

	// Capture takes a full-screen screenshot and returns it as encoded
	// JPEG bytes. It must honor ctx cancellation.
	Capture(ctx context.Context) (*Screenshot, error)
}

// Options configures backends.
type Options struct {
	// Backend selects the implementation: auto | extension | portal |
	// gnome-shell | x11.
	Backend string

	// DataDir is the payam-logger data directory. Temporary PNGs are
	// written there (the extension refuses to write anywhere else).
	DataDir string

	// JPEGQuality (1-100) of re-encoded screenshots.
	JPEGQuality int
}

// New returns the configured backend. An empty name or "auto" probes the
// session bus and picks the best available backend.
func New(opts Options, log *slog.Logger) (Backend, error) {
	switch opts.Backend {
	case "", "auto":
		return newAuto(opts, log)
	case "extension":
		return newExtension(opts)
	case "portal":
		return newPortal(opts)
	case "gnome-shell":
		return newGnomeShell()
	case "x11":
		return newX11()
	default:
		return nil, fmt.Errorf("unknown capture backend %q", opts.Backend)
	}
}

// newAuto picks a backend based on what is available on the session bus:
// the extension (silent) first, then the portal, then X11 tools.
func newAuto(opts Options, log *slog.Logger) (Backend, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}

	if nameHasOwner(conn, "org.payam.Logger") {
		log.Info("auto: extension backend available")
		return newExtension(opts)
	}
	if nameHasOwner(conn, portalBusName) {
		log.Info("auto: portal backend available (extension not found — install it with: make install-extension)")
		return newPortal(opts)
	}
	log.Info("auto: falling back to x11 backend")
	return newX11()
}

// nameHasOwner reports whether name is currently owned on the bus.
func nameHasOwner(conn *dbus.Conn, name string) bool {
	var owned bool
	err := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&owned)
	return err == nil && owned
}
