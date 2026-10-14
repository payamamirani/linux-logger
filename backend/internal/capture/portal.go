package capture

import (
	"context"
)

// Portal captures screenshots through the XDG Desktop Portal
// (org.freedesktop.portal.Screenshot) — the sanctioned method on Wayland,
// works across compositors (GNOME, KDE, wlroots, ...).
//
// Implementation plan (see docs/capture.md):
//
//  1. Connect to the D-Bus session bus.
//  2. Call org.freedesktop.portal.Screenshot on
//     /org/freedesktop/portal/desktop with a handle token.
//  3. Wait for the org.freedesktop.portal.Request::Response signal.
//  4. Read the returned file:// URI, decode the PNG, re-encode as JPEG
//     (stdlib image + image/jpeg + image/png).
//
// NOTE: the first capture may trigger a permission dialog. Whether the
// decision persists (so subsequent captures are silent) must be verified on
// the target GNOME — this is the main open question for Wayland support.
type Portal struct{}

func newPortal() (Backend, error) { return &Portal{}, nil }

func (p *Portal) Name() string { return "portal" }

func (p *Portal) Capture(ctx context.Context) (*Screenshot, error) {
	return nil, ErrNotImplemented
}
