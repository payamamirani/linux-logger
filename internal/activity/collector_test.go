package activity

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

type savedApp struct {
	start, end time.Time
	app, title string
	workspace  int
}

type savedUsage struct {
	start, end time.Time
	state      string
}

type fakeStore struct {
	apps  []savedApp
	usage []savedUsage
}

func (f *fakeStore) SaveAppUsage(start, end time.Time, app, title string, workspace int) error {
	f.apps = append(f.apps, savedApp{start, end, app, title, workspace})
	return nil
}

func (f *fakeStore) SaveComputerUsage(start, end time.Time, state string) error {
	f.usage = append(f.usage, savedUsage{start, end, state})
	return nil
}

func newTracker() (*tracker, *fakeStore) {
	fs := &fakeStore{}
	return &tracker{store: fs, log: slog.New(slog.NewTextHandler(io.Discard, nil))}, fs
}

func TestFocusSegmentsWrittenOnChange(t *testing.T) {
	tr, fs := newTracker()
	base := time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)

	tr.focusChanged("firefox", "gmail", 0, base)
	tr.focusChanged("firefox", "gmail", 0, base.Add(time.Minute))  // dedup — no new segment
	tr.focusChanged("firefox", "news", 0, base.Add(2*time.Minute)) // title change
	tr.focusChanged("gedit", "notes.txt", 1, base.Add(5*time.Minute))
	tr.flush(base.Add(7 * time.Minute))

	if len(fs.apps) != 3 {
		t.Fatalf("segments = %d, want 3", len(fs.apps))
	}
	want := []struct {
		app, title string
		dur        time.Duration
	}{
		{"firefox", "gmail", 2 * time.Minute},
		{"firefox", "news", 3 * time.Minute},
		{"gedit", "notes.txt", 2 * time.Minute},
	}
	for i, w := range want {
		got := fs.apps[i]
		if got.app != w.app || got.title != w.title {
			t.Errorf("seg %d = %s/%s, want %s/%s", i, got.app, got.title, w.app, w.title)
		}
		if d := got.end.Sub(got.start); d != w.dur {
			t.Errorf("seg %d duration = %v, want %v", i, d, w.dur)
		}
	}
}

func TestIdleGatesFocusRecording(t *testing.T) {
	tr, fs := newTracker()
	base := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)

	// seed active usage segment + focus
	tr.usage = &usageSegment{start: base, state: stateActive}
	tr.focusChanged("firefox", "gmail", 0, base)

	// user goes away at 10:30
	tr.idleChanged(true, base.Add(30*time.Minute))
	// focus events while away are ignored (tracker.focus is nil)
	tr.focusChanged("vlc", "movie", 0, base.Add(31*time.Minute))
	// back at 11:00
	tr.idleChanged(false, base.Add(time.Hour))
	tr.focusChanged("firefox", "gmail", 0, base.Add(time.Hour))
	tr.flush(base.Add(90 * time.Minute))

	// app usage: firefox 10:00–10:30, then firefox 11:00–11:30 (vlc skipped)
	if len(fs.apps) != 2 {
		t.Fatalf("app segments = %d, want 2: %+v", len(fs.apps), fs.apps)
	}
	if fs.apps[0].end.Sub(fs.apps[0].start) != 30*time.Minute {
		t.Errorf("first focus segment = %v, want 30m", fs.apps[0].end.Sub(fs.apps[0].start))
	}
	if fs.apps[1].app != "firefox" || fs.apps[1].start != base.Add(time.Hour) {
		t.Errorf("second segment = %+v, want firefox@11:00", fs.apps[1])
	}

	// computer usage: active 10:00–10:30, away 10:30–11:00, active 11:00–11:30
	if len(fs.usage) != 3 {
		t.Fatalf("usage segments = %d, want 3: %+v", len(fs.usage), fs.usage)
	}
	if fs.usage[1].state != stateAway || fs.usage[1].end.Sub(fs.usage[1].start) != 30*time.Minute {
		t.Errorf("away segment = %+v, want away 30m", fs.usage[1])
	}
}

func TestIdleEventsWithoutUsageSegmentIgnored(t *testing.T) {
	tr, _ := newTracker()
	tr.idleChanged(true, time.Now()) // no usage segment seeded — must not panic
	if tr.usage != nil {
		t.Fatal("usage should stay nil")
	}
}
