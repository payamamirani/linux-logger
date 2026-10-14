package capture

import (
	"context"
)

// Extension captures screenshots through the Payam Logger GNOME Shell
// extension (org.payam.Logger on the session bus), which uses the same
// internal Shell.Screenshot object as gnome-shell itself — fully silent:
// no permission dialog, no flash, no shutter sound (unlike the portal path,
// see docs/capture.md).
//
// Implementation plan:
//
//  1. Call org.payam.Logger.Capture(filepath) on the session bus.
//  2. The extension writes a PNG to filepath (must be under
//     ~/.local/share/payam-logger/).
//  3. Read the PNG, decode, re-encode as JPEG, remove the temporary PNG.
//
// Fallback: if the extension is not installed/enabled, use the portal
// backend (works but flashes and plays a sound on every capture).
type Extension struct{}

func newExtension() (Backend, error) { return &Extension{}, nil }

func (e *Extension) Name() string { return "extension" }

func (e *Extension) Capture(ctx context.Context) (*Screenshot, error) {
	return nil, ErrNotImplemented
}
