package ui

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/storage"
)

// viewerCache limits how many full screenshots stay decoded in memory while
// scrubbing the timeline.
const viewerCache = 12

// Window is the main viewer window, ManicTime-style: day navigation +
// stacked timelines on top, Details/Summary panels in the middle, the
// hovered screenshot at the bottom.
type Window struct {
	*gtk.ApplicationWindow

	store *Store

	timeline *TimelineStack
	panels   *Panels
	dateLbl  *gtk.Label
	prevBtn  *gtk.Button
	nextBtn  *gtk.Button
	todayBtn *gtk.Button
	viewer   *gtk.Picture
	status   *gtk.Label

	currentDay string // YYYY-MM-DD
	records    []storage.Record
	recSecs    []int // record times-of-day, parallel to records
	appSegs    []Seg
	usageSegs  []Seg

	// viewer texture cache (LRU-ish)
	cacheMu    sync.Mutex
	cache      map[int64]*gdk.Texture
	cacheOrder []int64

	lastShownID int64
}

// NewWindow creates the viewer window and shows today.
func NewWindow(app *gtk.Application, store *Store) *Window {
	w := &Window{
		ApplicationWindow: gtk.NewApplicationWindow(app),
		store:             store,
		cache:             make(map[int64]*gdk.Texture),
	}

	w.SetTitle("Payam Logger")
	w.SetDefaultSize(1200, 860)

	// Header bar with a settings button.
	header := gtk.NewHeaderBar()
	header.SetTitleWidget(gtk.NewLabel("Payam Logger"))
	gear := gtk.NewButtonFromIconName("emblem-system-symbolic")
	gear.SetTooltipText("Settings")
	gear.ConnectClicked(func() { newSettingsDialog(w) })
	header.PackEnd(gear)
	w.SetTitlebar(header)

	// --- top bar: day navigation + zoom ---------------------------------
	w.prevBtn = gtk.NewButtonFromIconName("go-previous-symbolic")
	w.prevBtn.SetTooltipText("Previous day (Left / PgUp)")
	w.prevBtn.ConnectClicked(func() { w.shiftDay(-1) })

	w.dateLbl = gtk.NewLabel("")
	w.dateLbl.SetHExpand(true) // keeps the date centered between the buttons
	w.dateLbl.SetHAlign(gtk.AlignCenter)

	w.nextBtn = gtk.NewButtonFromIconName("go-next-symbolic")
	w.nextBtn.SetTooltipText("Next day (Right / PgDn)")
	w.nextBtn.ConnectClicked(func() { w.shiftDay(+1) })

	w.todayBtn = gtk.NewButtonWithLabel("Today")
	w.todayBtn.SetTooltipText("Jump to today (Home)")
	w.todayBtn.ConnectClicked(func() { w.showDay(time.Now().Format("2006-01-02")) })

	zoomOut := gtk.NewButtonFromIconName("zoom-out-symbolic")
	zoomOut.SetTooltipText("Zoom out (up to 24h)")
	zoomOut.ConnectClicked(func() { w.timeline.ZoomOut() })

	zoomIn := gtk.NewButtonFromIconName("zoom-in-symbolic")
	zoomIn.SetTooltipText("Zoom in (down to 1h)")
	zoomIn.ConnectClicked(func() { w.timeline.ZoomIn() })

	topbar := gtk.NewBox(gtk.OrientationHorizontal, 6)
	topbar.SetMarginTop(6)
	topbar.SetMarginStart(8)
	topbar.SetMarginEnd(8)
	topbar.Append(w.prevBtn)
	topbar.Append(w.dateLbl)
	topbar.Append(w.nextBtn)
	topbar.Append(w.todayBtn)
	topbar.Append(gtk.NewSeparator(gtk.OrientationVertical))
	zoomBox := gtk.NewBox(gtk.OrientationHorizontal, 2)
	zoomBox.SetHAlign(gtk.AlignEnd)
	zoomBox.Append(zoomOut)
	zoomBox.Append(zoomIn)
	topbar.Append(zoomBox)

	// --- stacked timelines ------------------------------------------------
	w.timeline = NewTimelineStack(store.IntervalSec())
	w.timeline.SetMarginStart(8)
	w.timeline.SetMarginEnd(8)
	w.timeline.ConnectHover(w.onTimelineHover)
	w.timeline.ConnectSelection(w.onTimelineSelection)
	w.timeline.ConnectTrack(func(kind TrackKind) { w.refreshPanels() })

	// --- details/summary panels --------------------------------------------
	w.panels = NewPanels()
	w.panels.ConnectSelect(func(start, end float64) { w.timeline.SetSelection(start, end) })

	// --- viewer + status ------------------------------------------------------
	w.viewer = gtk.NewPicture()
	w.viewer.SetHExpand(true)
	w.viewer.SetVExpand(true)
	w.viewer.SetContentFit(gtk.ContentFitContain)
	w.viewer.SetSizeRequest(-1, 240)

	w.status = gtk.NewLabel("")
	w.status.SetHAlign(gtk.AlignStart)
	w.status.SetMarginTop(4)
	w.status.SetMarginBottom(6)
	w.status.SetMarginStart(10)

	root := gtk.NewBox(gtk.OrientationVertical, 4)
	root.Append(topbar)
	root.Append(w.timeline)
	w.panels.Widget().SetMarginStart(8)
	w.panels.Widget().SetMarginEnd(8)
	root.Append(w.panels.Widget())
	root.Append(w.viewer)
	root.Append(w.status)
	w.SetChild(root)

	w.installShortcuts()

	w.showDay(time.Now().Format("2006-01-02"))
	return w
}

// installShortcuts adds the day-view key bindings (ManicTime-compatible).
func (w *Window) installShortcuts() {
	ctrl := gtk.NewShortcutController()

	add := func(accel string, fn func()) {
		trigger := gtk.NewShortcutTriggerParseString(accel)
		if trigger == nil {
			log.Printf("bad accelerator %q", accel)
			return
		}
		ctrl.AddShortcut(gtk.NewShortcut(trigger, gtk.NewCallbackAction(
			func(_ gtk.Widgetter, _ *glib.Variant) bool {
				fn()
				return true
			})))
	}

	add("Left", func() { w.shiftDay(-1) })                             // ManicTime: PgUp / F7
	add("Right", func() { w.shiftDay(+1) })                            // ManicTime: PgDown / F8
	add("Home", func() { w.showDay(time.Now().Format("2006-01-02")) }) // F9
	add("F5", func() { w.showDay(w.currentDay) })                      // refresh
	add("Escape", func() { w.timeline.ClearSelection() })
	add("plus", func() { w.timeline.ZoomIn() })
	add("minus", func() { w.timeline.ZoomOut() })

	w.AddController(ctrl)
}

// showDay loads (or empties) one day and redraws everything for it.
func (w *Window) showDay(day string) {
	w.currentDay = day

	if ts, err := time.Parse("2006-01-02", day); err == nil {
		w.dateLbl.SetText(ts.Format("Mon, 02 Jan 2006"))
	} else {
		w.dateLbl.SetText(day)
	}

	ctx := context.Background()
	records, err := w.store.Records(ctx, day)
	if err != nil {
		log.Printf("load day %s: %v", day, err)
		records = nil
	}
	w.records = records

	w.recSecs = w.recSecs[:0]
	for _, r := range records {
		if ts, err := time.Parse(time.RFC3339, r.TakenAt); err == nil {
			w.recSecs = append(w.recSecs, ts.Hour()*3600+ts.Minute()*60+ts.Second())
		} else {
			w.recSecs = append(w.recSecs, 0)
		}
	}

	w.appSegs = w.loadAppSegs(day)
	w.usageSegs = w.loadUsageSegs(day)

	// Viewer starts empty until the user hovers the timeline.
	w.viewer.SetPaintable(nil)
	w.lastShownID = -1
	w.cacheMu.Lock()
	w.cache = make(map[int64]*gdk.Texture)
	w.cacheOrder = nil
	w.cacheMu.Unlock()

	today := time.Now().Format("2006-01-02")
	w.todayBtn.SetSensitive(day != today)
	w.nextBtn.SetSensitive(day != today)

	w.timeline.SetDay(day, records, w.appSegs, w.usageSegs, time.Now())

	w.refreshPanels()
	w.showDayStatus()
}

func (w *Window) loadAppSegs(day string) []Seg {
	recs, err := w.store.AppUsage(context.Background(), day)
	if err != nil {
		log.Printf("load app usage %s: %v", day, err)
		return nil
	}
	var segs []Seg
	for _, r := range recs {
		s, e := rfc3339ToSec(r.Start), rfc3339ToSec(r.End)
		if e > s {
			segs = append(segs, Seg{Start: s, End: e, App: r.App, Title: r.Title})
		}
	}
	return segs
}

func (w *Window) loadUsageSegs(day string) []Seg {
	recs, err := w.store.ComputerUsage(context.Background(), day)
	if err != nil {
		log.Printf("load computer usage %s: %v", day, err)
		return nil
	}
	var segs []Seg
	for _, r := range recs {
		s, e := rfc3339ToSec(r.Start), rfc3339ToSec(r.End)
		if e > s {
			segs = append(segs, Seg{Start: s, End: e, State: r.State})
		}
	}
	return segs
}

// rfc3339ToSec converts an RFC3339 timestamp to seconds of its local day.
func rfc3339ToSec(s string) float64 {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return -1
	}
	return float64(ts.Hour()*3600 + ts.Minute()*60 + ts.Second())
}

func (w *Window) shiftDay(delta int) {
	ts, err := time.Parse("2006-01-02", w.currentDay)
	if err != nil {
		return
	}
	w.showDay(ts.AddDate(0, 0, delta).Format("2006-01-02"))
}

// refreshPanels rebuilds the Details/Summary lists for the active track.
func (w *Window) refreshPanels() {
	kind := w.timeline.ActiveTrack()

	var rows []detailRow
	groups := map[string]float64{}

	switch kind {
	case TrackApps:
		for _, s := range w.appSegs {
			title := s.Title
			if title == "" {
				title = "(desktop)"
			}
			rows = append(rows, detailRow{
				title: title, start: formatClock(s.Start), end: formatClock(s.End),
				duration: formatDuration(s.End - s.Start), selStart: s.Start, selEnd: s.End,
			})
			app := s.App
			if app == "" {
				app = "(desktop)"
			}
			groups[app] += s.End - s.Start
		}
	case TrackUsage:
		for _, s := range w.usageSegs {
			name := "Active"
			if s.State == "away" {
				name = "Away"
			}
			rows = append(rows, detailRow{
				title: name, start: formatClock(s.Start), end: formatClock(s.End),
				duration: formatDuration(s.End - s.Start), selStart: s.Start, selEnd: s.End,
			})
			groups[name] += s.End - s.Start
		}
	case TrackScreenshots:
		for i, r := range w.records {
			rows = append(rows, detailRow{
				title: fmt.Sprintf("Screenshot #%d", r.ID),
				start: shortTime(r.TakenAt), end: "",
				duration: fmt.Sprintf("%.0f KB", float64(r.SizeBytes)/1024),
				selStart: float64(w.recSecs[i]), selEnd: float64(w.recSecs[i] + w.store.IntervalSec()),
			})
		}
	}

	w.panels.SetData(kind, rows, summarizeGroups(groups))
}

// onTimelineSelection shows the selected range duration and its content
// stats in the status line; a cleared selection restores the day summary.
func (w *Window) onTimelineSelection(start, end float64) {
	if start < 0 {
		w.showDayStatus()
		return
	}

	// count screenshots + app time inside [start, end]
	n := 0
	for _, s := range w.recSecs {
		if float64(s) >= start && float64(s) <= end {
			n++
		}
	}
	var appTime float64
	for _, s := range w.appSegs {
		lo, hi := math01(mathMax(start, s.Start)), math01(mathMin(end, s.End))
		if hi > lo {
			appTime += hi - lo
		}
	}
	w.status.SetText(fmt.Sprintf("Selected %s → %s • %s • %d screenshots • active %s",
		formatClock(start), formatClock(end), formatDuration(end-start), n, formatDuration(appTime)))
}

// tiny math helpers (kept local to avoid importing math everywhere).
func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func math01(v float64) float64 { return v }

func (w *Window) showDayStatus() {
	if n := len(w.records); n > 0 {
		var total int64
		for _, r := range w.records {
			total += r.SizeBytes
		}
		w.status.SetText(fmt.Sprintf("%s — %d screenshots • %.1f MB",
			w.currentDay, n, float64(total)/1024/1024))
	} else {
		w.status.SetText(fmt.Sprintf("%s — no screenshots recorded this day", w.currentDay))
	}
}

// onTimelineHover reacts to the pointer position on any timeline: shows the
// screenshot valid at that moment below, or nothing in white (gap) areas.
func (w *Window) onTimelineHover(sec float64) {
	if sec < 0 {
		return // pointer left the widget — keep the last frame
	}

	idx, valid := w.recordAt(sec)
	if !valid {
		return // hovering a white gap — keep the last frame
	}

	rec := w.records[idx]
	if hh, mm := int(sec/3600), int(sec)%3600/60; hh >= 0 {
		w.status.SetText(fmt.Sprintf("%02d:%02d — shot at %s • %dx%d",
			hh, mm, shortTime(rec.TakenAt), rec.Width, rec.Height))
	}

	if rec.ID == w.lastShownID {
		return
	}

	tex := w.cachedTexture(idx)
	if tex == nil {
		return
	}
	w.lastShownID = rec.ID
	glib.IdleAdd(func() bool {
		w.viewer.SetPaintable(tex)
		return false
	})
}

// recordAt returns the index of the last record at or before sec; valid is
// false when sec falls into a gap (more than one interval before that
// record or before the first record of the day).
func (w *Window) recordAt(sec float64) (int, bool) {
	if len(w.recSecs) == 0 {
		return 0, false
	}
	idx := sort.Search(len(w.recSecs), func(i int) bool { return float64(w.recSecs[i]) > sec }) - 1
	if idx < 0 {
		return 0, false
	}
	if sec-float64(w.recSecs[idx]) > float64(w.store.IntervalSec()) {
		return idx, false // white gap (locked / service off)
	}
	return idx, true
}

// cachedTexture returns (and memoizes) the full-size texture of a record.
func (w *Window) cachedTexture(idx int) *gdk.Texture {
	rec := w.records[idx]

	w.cacheMu.Lock()
	if tex, ok := w.cache[rec.ID]; ok {
		w.cacheMu.Unlock()
		return tex
	}
	w.cacheMu.Unlock()

	data, err := w.store.ReadScreenshot(rec)
	if err != nil {
		log.Printf("read %s: %v", rec.Path, err)
		return nil
	}
	tex, err := gdk.NewTextureFromBytes(glib.NewBytes(data))
	if err != nil {
		log.Printf("texture %s: %v", rec.Path, err)
		return nil
	}

	w.cacheMu.Lock()
	w.cache[rec.ID] = tex
	w.cacheOrder = append(w.cacheOrder, rec.ID)
	for len(w.cacheOrder) > viewerCache {
		delete(w.cache, w.cacheOrder[0])
		w.cacheOrder = w.cacheOrder[1:]
	}
	w.cacheMu.Unlock()
	return tex
}

// shortTime renders an RFC3339 timestamp as HH:MM:SS.
func shortTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("15:04:05")
}
