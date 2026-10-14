package capture

import (
	"context"
)

// X11 captures screenshots on X11 sessions, where any client can grab the
// screen directly.
//
// Implementation plan (see docs/capture.md), in order of preference:
//
//  1. exec `maim` (fast, reliable) and read the PNG from stdout.
//  2. exec `scrot` as a fallback.
//  3. Optional later: pure-Go XGetImage via github.com/jezek/xgb to drop the
//     external dependency.
//
// This backend also covers "Ubuntu on Xorg" sessions, which is the
// guaranteed-working fallback if the Wayland permission story blocks us.
type X11 struct{}

func newX11() (Backend, error) { return &X11{}, nil }

func (x *X11) Name() string { return "x11" }

func (x *X11) Capture(ctx context.Context) (*Screenshot, error) {
	return nil, ErrNotImplemented
}
