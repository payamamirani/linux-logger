package capture

import (
	"context"
)

// GnomeShell captures screenshots via the GNOME Shell private D-Bus API
// (org.gnome.Shell.Screenshot on /org/gnome/Shell/Screenshot).
//
// Implementation plan (see docs/capture.md):
//
//  1. Connect to the D-Bus session bus.
//  2. Call org.gnome.Shell.Screenshot.Screenshot(includeCursor, flash, filename).
//  3. Shell writes the file; read it and re-encode as JPEG.
//
// Caveats:
//
//   - GNOME-only; breaks silently on other compositors.
//   - Since GNOME 41 parts of this API are restricted to privileged callers;
//     availability on the target GNOME version must be verified.
//   - The disk-write goes through gnome-shell, so a writable temp path is
//     required.
//
// Prefer the portal backend; keep this one as a GNOME-specific fallback.
type GnomeShell struct{}

func newGnomeShell() (Backend, error) { return &GnomeShell{}, nil }

func (g *GnomeShell) Name() string { return "gnome-shell" }

func (g *GnomeShell) Capture(ctx context.Context) (*Screenshot, error) {
	return nil, ErrNotImplemented
}
