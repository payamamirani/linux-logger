// Package capture takes screenshots of the active session.
//
// Capturing is backend-specific because the correct method depends on the
// display server:
//
//   - Wayland (GNOME default on Ubuntu): applications are isolated from each
//     other's pixels, so screenshots must go through a privileged API —
//     either the XDG Desktop Portal or the GNOME Shell D-Bus API.
//   - X11: any client can grab the screen (maim, scrot, XGetImage, ...).
//
// Backends are selected via config ("auto" detects the session type) so the
// rest of the daemon never needs to care about the display server.
package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrNotImplemented marks backend stubs that are pending implementation.
var ErrNotImplemented = errors.New("capture backend not implemented yet")

// Screenshot is a single captured frame.
type Screenshot struct {
	// Data holds the encoded image bytes (JPEG unless noted in MIME).
	Data []byte

	// MIME type of Data, e.g. "image/jpeg".
	MIME string

	// TakenAt is the capture timestamp (UTC).
	TakenAt time.Time
}

// Backend captures a picture of the current screen.
type Backend interface {
	// Name returns the config identifier, e.g. "portal".
	Name() string

	// Capture takes a full-screen screenshot and returns it as encoded
	// bytes. It must honor ctx cancellation.
	Capture(ctx context.Context) (*Screenshot, error)
}

// New returns the named backend. An empty name or "auto" performs
// session-type detection.
func New(name string) (Backend, error) {
	switch name {
	case "", "auto":
		return newAuto()
	case "portal":
		return newPortal()
	case "gnome-shell":
		return newGnomeShell()
	case "extension":
		return newExtension()
	case "x11":
		return newX11()
	default:
		return nil, fmt.Errorf("unknown capture backend %q", name)
	}
}

// newAuto picks a backend based on the session environment.
func newAuto() (Backend, error) {
	switch os.Getenv("XDG_SESSION_TYPE") {
	case "x11":
		return newX11()
	default:
		// Wayland and unknown sessions: try the portal first; it is the
		// sanctioned, compositor-independent way to capture.
		// TODO: probe org.freedesktop.portal.Desktop availability over
		// D-Bus and fall back to gnome-shell if the portal is missing.
		return newPortal()
	}
}
