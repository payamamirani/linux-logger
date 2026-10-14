// Package activity records what the user is doing: the focused window
// (application + title + workspace) and active/away state, reported by the
// payam-logger GNOME Shell extension over D-Bus.
//
// The extension emits FocusChanged(app,title,workspace) on every focus or
// title change and IdleChanged(idle) when the user crosses the idle
// threshold. The tracker keeps the currently open segments in memory and
// writes a row whenever a segment closes (state changed or idle gate
// toggled). The collector owns the D-Bus plumbing and feeds the tracker.
package activity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	extBusName = "org.payam.Logger"
	extObjPath = "/org/payam/Logger"
	extIface   = "org.payam.Logger"

	// state strings recorded in computer_usage.state.
	stateActive = "active"
	stateAway   = "away"
)

// Store is the subset of storage the tracker needs.
type Store interface {
	SaveAppUsage(start, end time.Time, app, title string, workspace int) error
	SaveComputerUsage(start, end time.Time, state string) error
}

// focusSegment is the currently open focused-window segment.
type focusSegment struct {
	start     time.Time
	app       string
	title     string
	workspace int
}

// usageSegment is the currently open active/away segment.
type usageSegment struct {
	start time.Time
	state string
}

// tracker is the pure state machine turning events into stored segments.
type tracker struct {
	store Store
	log   *slog.Logger

	focus *focusSegment
	usage *usageSegment
}

// focusChanged closes the open segment (if the state really changed) and
// opens a new one. Ignored while away.
func (t *tracker) focusChanged(app, title string, workspace int, now time.Time) {
	if t.usage != nil && t.usage.state == stateAway {
		return // no app usage recorded while idle/away
	}
	if t.focus != nil && t.focus.app == app && t.focus.title == title && t.focus.workspace == workspace {
		return // unchanged — the segment just continues
	}
	t.closeFocus(now)
	t.focus = &focusSegment{start: now, app: app, title: title, workspace: workspace}
}

// idleChanged toggles the active/away gate. Focus segments are paused while
// away (matching ManicTime's model: no app usage recorded while idle).
func (t *tracker) idleChanged(idle bool, now time.Time) {
	if t.usage == nil {
		return
	}
	if idle && t.usage.state == stateActive {
		t.closeFocus(now)
		t.closeUsage(now)
		t.usage = &usageSegment{start: now, state: stateAway}
	} else if !idle && t.usage.state == stateAway {
		t.closeUsage(now)
		t.usage = &usageSegment{start: now, state: stateActive}
	}
}

func (t *tracker) closeFocus(now time.Time) {
	if t.focus == nil {
		return
	}
	if err := t.store.SaveAppUsage(t.focus.start, now, t.focus.app, t.focus.title, t.focus.workspace); err != nil {
		t.log.Error("save app usage", "err", err)
	}
	t.focus = nil
}

func (t *tracker) closeUsage(now time.Time) {
	if t.usage == nil {
		return
	}
	if err := t.store.SaveComputerUsage(t.usage.start, now, t.usage.state); err != nil {
		t.log.Error("save computer usage", "err", err)
	}
	t.usage = nil
}

func (t *tracker) flush(now time.Time) {
	t.closeFocus(now)
	t.closeUsage(now)
}

// Collector subscribes to the extension's D-Bus signals and feeds the
// tracker until its context is cancelled.
type Collector struct {
	conn    *dbus.Conn
	tracker *tracker
	log     *slog.Logger
}

// NewCollector connects to the session bus and seeds the initial state. It
// returns an error when the extension (v2+) is unavailable — activity
// tracking is optional; screenshots keep working without it.
//
// The extension can enable a moment after the daemon starts, so GetStatus
// is retried for a short window (aborting early when ctx is cancelled)
// before giving up.
func NewCollector(ctx context.Context, store Store, log *slog.Logger) (*Collector, error) {
	// Private connection (not the godbus shared one): closing it on error
	// or shutdown must not break other users of the session bus.
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}

	const (
		seedWait  = 60 * time.Second
		seedEvery = time.Second
	)

	var app, title string
	var workspace int32
	var idle bool
	deadline := time.Now().Add(seedWait)
	waited := false
	for {
		err = conn.Object(extBusName, extObjPath).
			Call(extIface+".GetStatus", 0).Store(&app, &title, &workspace, &idle)
		if err == nil {
			break
		}
		if !time.Now().Before(deadline) {
			conn.Close()
			return nil, fmt.Errorf("extension v2 not available (GetStatus): %w", err)
		}
		if !waited {
			log.Info("extension not up yet — activity tracking waits up to 60s")
			waited = true
		}
		select {
		case <-ctx.Done():
			conn.Close()
			return nil, ctx.Err()
		case <-time.After(seedEvery):
		}
	}

	tr := &tracker{store: store, log: log}
	now := time.Now()
	tr.usage = &usageSegment{start: now, state: stateActive}
	if idle {
		tr.usage.state = stateAway
	} else {
		tr.focus = &focusSegment{start: now, app: app, title: title, workspace: int(workspace)}
	}

	log.Info("activity tracking started", "app", app, "title", title, "idle", idle)
	return &Collector{conn: conn, tracker: tr, log: log}, nil
}

// Run blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	if err := c.conn.AddMatchSignal(
		dbus.WithMatchInterface(extIface),
	); err != nil {
		return fmt.Errorf("subscribe to activity signals: %w", err)
	}

	sigCh := make(chan *dbus.Signal, 64)
	c.conn.Signal(sigCh)
	defer c.conn.RemoveSignal(sigCh)

	for {
		select {
		case <-ctx.Done():
			c.tracker.flush(time.Now())
			c.conn.Close()
			return ctx.Err()
		case sig := <-sigCh:
			if sig.Path != dbus.ObjectPath(extObjPath) {
				continue
			}
			switch sig.Name {
			case extIface + ".FocusChanged":
				if len(sig.Body) >= 3 {
					app, _ := sig.Body[0].(string)
					title, _ := sig.Body[1].(string)
					ws, _ := sig.Body[2].(int32)
					c.tracker.focusChanged(app, title, int(ws), time.Now())
				}
			case extIface + ".IdleChanged":
				if len(sig.Body) >= 1 {
					idle, _ := sig.Body[0].(bool)
					c.tracker.idleChanged(idle, time.Now())
				}
			}
		}
	}
}
