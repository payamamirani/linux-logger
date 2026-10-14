package ui

import (
	"fmt"
	"hash/fnv"
	"math"
	"time"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/storage"
)

// Layout constants for one day, in seconds.
const (
	daySeconds     = 24 * 3600
	defaultViewSec = 10 * 3600 // default visible window: 10 hours ...
	defaultStart   = 8 * 3600  // ... from 08:00 to 18:00
	minViewSec     = 3600      // max zoom-in: 1 hour
	maxViewSec     = daySeconds

	autoZoomPad  = 30 * 60 // padding around the data span on auto-zoom
	snapRadiusPx = 8.0     // sticky-selection snap radius in pixels
	snapMinutes  = 1       // free-selection granularity (whole minutes)

	rulerH      = 18
	trackShotsH = 34
	trackUsageH = 22
	trackAppsH  = 28
	trackTagsH  = 24
	trackAutoH  = 22
)

// Colors (RGB 0..1).
var (
	colGrid   = [3]float64{0.835, 0.816, 0.784} // #d5d0cc — hour grid
	colText   = [3]float64{0.467, 0.443, 0.400} // #77767b — hour labels
	colHover  = [3]float64{0.149, 0.635, 0.412} // #26a269 — hover (green)
	colNow    = [3]float64{0.878, 0.106, 0.142} // #e01b24 — now (red)
	colBorder = [3]float64{0.922, 0.914, 0.898} // #ebeae4
	colChip   = [3]float64{0.227, 0.223, 0.212} // #3a3a38 — chip background

	colActive = [3]float64{0.149, 0.635, 0.412} // green — active time
	colAway   = [3]float64{0.878, 0.106, 0.142} // red — away time
)

// appPalette holds distinct, calm colors for the Applications track.
var appPalette = [][3]float64{
	{0.208, 0.518, 0.894}, // blue
	{0.612, 0.400, 0.749}, // purple
	{0.827, 0.427, 0.129}, // orange
	{0.145, 0.631, 0.506}, // teal
	{0.757, 0.290, 0.529}, // magenta
	{0.588, 0.545, 0.224}, // olive
	{0.169, 0.533, 0.702}, // steel blue
	{0.831, 0.298, 0.290}, // brick
	{0.400, 0.522, 0.302}, // moss
	{0.553, 0.396, 0.663}, // violet
}

// appColor returns a stable color for an application name.
func appColor(app string) [3]float64 {
	h := fnv.New32a()
	h.Write([]byte(app))
	return appPalette[h.Sum32()%uint32(len(appPalette))]
}

// Seg is one time segment on the usage, applications or tags track
// (seconds of day, end exclusive).
type Seg struct {
	Start, End float64
	App        string // applications track / color key (first-level tag)
	Title      string
	State      string // computer usage track: active|away
	ID         int64  // tags track: tag id
}

// TrackKind identifies a timeline row.
type TrackKind int

const (
	TrackScreenshots TrackKind = iota
	TrackUsage
	TrackApps
	TrackTags
	TrackAutotags
)

// track is one row of the stack.
type track struct {
	kind  TrackKind
	da    *gtk.DrawingArea
	label *gtk.Label
	name  string
	has   []bool // screenshots track: bucket → present
	segs  []Seg  // usage/apps tracks
}

// TimelineStack is the ManicTime-style stacked day timeline: a shared time
// ruler on top, then the Screenshots, Computer usage and Applications rows.
// Zoom/pan/selection/hover state is shared across all rows.
//
// Interaction:
//   - hover         → green position line + time chip; reports the second
//     under the cursor (onHover)
//   - left drag     → range selection; edges stick to nearby segment
//     boundaries of ANY track, otherwise snap to whole minutes; each new
//     drag replaces the previous selection
//   - double-click  → select the whole run/segment under the cursor
//   - simple click  → clear selection and activate the clicked track
//   - middle drag / horizontal scroll → pan; vertical scroll → zoom
type TimelineStack struct {
	*gtk.Box

	labelCol *gtk.Box
	ruler    *gtk.DrawingArea
	tracks   []*track
	active   int

	intervalSec int
	nBuckets    int

	// visible window, seconds of day
	viewStart float64
	viewSpan  float64

	// hover position (seconds of day), -1 when outside
	hover  float64
	hoverX float64

	// selection range (seconds of day), negative start = none
	selStart, selEnd float64
	selPressX        float64

	onHover      func(sec float64)
	onSelection  func(start, end float64)
	onTrack      func(kind TrackKind)
	onDeleteTag  func(tagID int64)
	onAddAutotag func(app, title string)

	// pan drag state
	panning      bool
	panStartView float64

	// click-vs-drag discrimination for the selection gesture
	pressX, pressY float64

	isToday bool
	nowSec  float64
}

// NewTimelineStack creates the widget for the given capture interval.
func NewTimelineStack(intervalSec int) *TimelineStack {
	t := &TimelineStack{
		Box:         gtk.NewBox(gtk.OrientationHorizontal, 4),
		intervalSec: intervalSec,
		nBuckets:    daySeconds / intervalSec,
		viewStart:   defaultStart,
		viewSpan:    defaultViewSec,
		hover:       -1,
		selStart:    -1,
		active:      int(TrackApps),
	}

	t.labelCol = gtk.NewBox(gtk.OrientationVertical, 0)
	t.Append(t.labelCol)

	rows := gtk.NewBox(gtk.OrientationVertical, 2)
	t.Append(rows)

	// Ruler
	t.ruler = gtk.NewDrawingArea()
	t.ruler.SetHExpand(true)
	t.ruler.SetSizeRequest(-1, rulerH)
	t.ruler.SetDrawFunc(t.drawRuler)
	rows.Append(t.ruler)

	// Track rows
	for _, spec := range []struct {
		kind TrackKind
		name string
		h    int
	}{
		{TrackScreenshots, "Screenshots", trackShotsH},
		{TrackUsage, "Usage", trackUsageH},
		{TrackApps, "Applications", trackAppsH},
		{TrackTags, "Tags", trackTagsH},
		{TrackAutotags, "Autotags", trackAutoH},
	} {
		tr := &track{kind: spec.kind, has: make([]bool, t.nBuckets)}
		tr.da = gtk.NewDrawingArea()
		tr.da.SetHExpand(true)
		tr.da.SetSizeRequest(-1, spec.h)
		tr.da.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
			t.drawTrack(tr, cr, w, h)
		})
		t.attachControllers(tr.da, tr)
		t.tracks = append(t.tracks, tr)
		rows.Append(tr.da)

		lbl := gtk.NewLabel(spec.name)
		lbl.SetHAlign(gtk.AlignStart)
		lbl.SetSizeRequest(84, spec.h)
		lbl.SetVAlign(gtk.AlignCenter)
		t.labelCol.Append(lbl)
		tr.label = lbl
		tr.name = spec.name
	}

	t.restyleLabels()

	// Ruler interactions (hover/pan/zoom without selecting).
	t.attachRulerControllers(t.ruler)

	return t
}

// restyleLabels bolds the active track's label.
func (t *TimelineStack) restyleLabels() {
	for i, tr := range t.tracks {
		if i == t.active {
			tr.label.SetMarkup(fmt.Sprintf("<b>%s</b>", tr.name))
		} else {
			tr.label.SetText(tr.name)
		}
	}
}

// ConnectHover registers the hover callback; sec is the second-of-day under
// the cursor, or -1 when the pointer left the widget.
func (t *TimelineStack) ConnectHover(fn func(sec float64)) { t.onHover = fn }

// ConnectSelection registers the selection callback: start/end in
// seconds-of-day (ordered), or (-1,-1) once the selection is cleared.
func (t *TimelineStack) ConnectSelection(fn func(start, end float64)) { t.onSelection = fn }

// ConnectTrack fires when the active track changes (click on a row).
func (t *TimelineStack) ConnectTrack(fn func(kind TrackKind)) { t.onTrack = fn }

// ConnectDeleteTag fires on right-click over a tag segment.
func (t *TimelineStack) ConnectDeleteTag(fn func(tagID int64)) { t.onDeleteTag = fn }

// ConnectAddAutotag fires on right-click over an applications segment.
func (t *TimelineStack) ConnectAddAutotag(fn func(app, title string)) { t.onAddAutotag = fn }

// ActiveTrack returns the currently selected track kind.
func (t *TimelineStack) ActiveTrack() TrackKind { return t.tracks[t.active].kind }

// SetActiveTrack highlights a track (e.g. from panel interactions).
func (t *TimelineStack) SetActiveTrack(kind TrackKind) {
	for i, tr := range t.tracks {
		if tr.kind == kind {
			t.active = i
		}
	}
	t.restyleLabels()
}

// ClearSelection removes the current selection.
func (t *TimelineStack) ClearSelection() {
	t.selStart, t.selEnd = -1, -1
	t.redraw()
	t.emitSelection()
}

// selection returns the current range (lo, hi) or (-1,-1) when none.
func (t *TimelineStack) selection() (float64, float64) {
	if t.selStart < 0 {
		return -1, -1
	}
	lo, hi := t.selStart, t.selEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

// SetSelection selects a range programmatically (details panel rows).
func (t *TimelineStack) SetSelection(start, end float64) {
	t.selStart, t.selEnd = start, end
	t.redraw()
	t.emitSelection()
}

// SetDay loads one day's data and auto-zooms to its data span (ManicTime
// behavior); an empty day falls back to the default 08:00–18:00 window.
func (t *TimelineStack) SetDay(day string, records []storage.Record, appSegs, usageSegs, tagSegs, autotagSegs []Seg, now time.Time) {
	shots := t.tracks[int(TrackScreenshots)]
	shots.has = make([]bool, t.nBuckets)
	for _, r := range records {
		ts, err := time.Parse(time.RFC3339, r.TakenAt)
		if err != nil {
			continue
		}
		sec := ts.Hour()*3600 + ts.Minute()*60 + ts.Second()
		if b := sec / t.intervalSec; b >= 0 && b < t.nBuckets {
			shots.has[b] = true
		}
	}
	t.tracks[int(TrackUsage)].segs = usageSegs
	t.tracks[int(TrackApps)].segs = appSegs
	t.tracks[int(TrackTags)].segs = tagSegs
	t.tracks[int(TrackAutotags)].segs = autotagSegs

	if day == now.Format("2006-01-02") {
		t.isToday = true
		t.nowSec = float64(now.Hour()*3600 + now.Minute()*60 + now.Second())
	} else {
		t.isToday = false
	}

	t.viewStart, t.viewSpan = t.dataSpanOrDefault()
	t.selStart, t.selEnd = -1, -1
	t.hover = -1
	t.redraw()
	t.emitSelection()
}

// dataSpanOrDefault computes the auto-zoom window from all tracks' data.
func (t *TimelineStack) dataSpanOrDefault() (float64, float64) {
	lo, hi := math.MaxFloat64, -math.MaxFloat64
	consider := func(s, e float64) {
		if e > s {
			lo = math.Min(lo, s)
			hi = math.Max(hi, e)
		}
	}
	shots := t.tracks[int(TrackScreenshots)]
	for b, ok := range shots.has {
		if ok {
			consider(float64(b)*float64(t.intervalSec), float64(b+1)*float64(t.intervalSec))
		}
	}
	for _, tr := range t.tracks {
		for _, s := range tr.segs {
			consider(s.Start, s.End)
		}
	}
	if hi < lo {
		return defaultStart, defaultViewSec
	}
	lo = math.Max(0, lo-autoZoomPad)
	hi = math.Min(daySeconds, hi+autoZoomPad)
	if hi-lo < minViewSec {
		mid := (lo + hi) / 2
		lo, hi = mid-minViewSec/2, mid+minViewSec/2
	}
	return lo, hi - lo
}

// ZoomIn / ZoomOut change the visible window around its center.
func (t *TimelineStack) ZoomIn()  { t.zoomAt(float64(t.width())/2, -1) }
func (t *TimelineStack) ZoomOut() { t.zoomAt(float64(t.width())/2, +1) }

// --- internal --------------------------------------------------------------

func (t *TimelineStack) width() int {
	for _, tr := range t.tracks {
		if w := tr.da.Width(); w > 0 {
			return w
		}
	}
	return t.ruler.Width()
}

func (t *TimelineStack) pxPerSec() float64 {
	w := float64(t.width() - 12)
	if w <= 0 {
		return 1
	}
	return w / t.viewSpan
}

func (t *TimelineStack) setViewStart(s float64) {
	maxStart := float64(daySeconds) - t.viewSpan
	if maxStart < 0 {
		maxStart = 0
	}
	t.viewStart = math.Max(0, math.Min(s, maxStart))
	t.redraw()
}

func (t *TimelineStack) xToSec(x float64) float64 {
	w := float64(t.width())
	if w <= 0 {
		return t.viewStart
	}
	sec := t.viewStart + x/w*t.viewSpan
	return math.Max(t.viewStart, math.Min(sec, t.viewStart+t.viewSpan))
}

// zoomAt zooms around the given x anchor; dy>0 zooms out, dy<0 zooms in.
func (t *TimelineStack) zoomAt(x, dy float64) {
	const factorPerTick = 1.25
	factor := math.Pow(factorPerTick, dy)

	newSpan := math.Max(minViewSec, math.Min(maxViewSec, t.viewSpan*factor))
	anchor := t.xToSec(x)

	rel := (anchor - t.viewStart) / t.viewSpan
	newStart := anchor - rel*newSpan

	t.viewSpan = newSpan
	t.setViewStart(newStart)
}

// snap returns the sticky/snapped second for a raw position: nearest
// segment boundary of any track within the radius, else the whole minute.
func (t *TimelineStack) snap(sec float64) float64 {
	radius := snapRadiusPx / t.pxPerSec()
	best, bestDist := sec, math.MaxFloat64
	for _, tr := range t.tracks {
		for _, s := range tr.segs {
			for _, p := range []float64{s.Start, s.End} {
				if d := math.Abs(sec - p); d < bestDist {
					best, bestDist = p, d
				}
			}
		}
	}
	// Screenshot bucket edges are minute-aligned already (1-min interval).
	if bestDist <= radius {
		return best
	}
	gran := float64(snapMinutes * 60)
	return math.Round(sec/gran) * gran
}

func (t *TimelineStack) emitSelection() {
	if t.onSelection == nil {
		return
	}
	if t.selStart < 0 {
		t.onSelection(-1, -1)
		return
	}
	lo, hi := t.selStart, t.selEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	t.onSelection(lo, hi)
}

// redraw explicitly queues every drawing area — queueing the container
// Box does NOT redraw its children in GTK4.
func (t *TimelineStack) redraw() {
	t.ruler.QueueDraw()
	for _, tr := range t.tracks {
		tr.da.QueueDraw()
	}
}

// --- controllers ------------------------------------------------------------

func (t *TimelineStack) attachControllers(w *gtk.DrawingArea, tr *track) {
	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) {
		t.hoverX, t.hover = x, t.xToSec(x)
		t.redraw()
		if t.onHover != nil {
			t.onHover(t.hover)
		}
	})
	motion.ConnectLeave(func() {
		t.hover = -1
		t.redraw()
		if t.onHover != nil {
			t.onHover(-1)
		}
	})
	w.AddController(motion)

	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollBothAxes)
	scroll.ConnectScroll(func(dx, dy float64) bool {
		switch {
		case math.Abs(dx) > math.Abs(dy):
			t.setViewStart(t.viewStart + dx/t.pxPerSec())
		default:
			t.zoomAt(t.hoverX, dy)
		}
		return true
	})
	w.AddController(scroll)

	sel := gtk.NewGestureDrag()
	sel.ConnectDragBegin(func(x, y float64) {
		t.selPressX, t.pressX, t.pressY = x, x, y
		start := t.snap(t.xToSec(x))
		t.selStart, t.selEnd = start, start
		t.redraw()
		t.emitSelection()
	})
	sel.ConnectDragUpdate(func(offsetX, offsetY float64) {
		if t.selStart < 0 {
			return
		}
		t.selEnd = t.snap(t.xToSec(t.selPressX + offsetX))
		t.redraw()
		t.emitSelection()
	})
	sel.ConnectDragEnd(func(x, y float64) {
		if math.Abs(t.selEnd-t.selStart)*t.pxPerSec() < 4 {
			t.ClearSelection() // it was a click
		}
	})
	w.AddController(sel)

	click := gtk.NewGestureClick()
	click.ConnectPressed(func(n int, x, y float64) {
		if n >= 2 { // double-click → select the whole segment under the cursor
			t.selectRunAt(x)
			return
		}
		if tr != nil { // single click also activates this track
			t.active = -1
			for i := range t.tracks {
				if t.tracks[i] == tr {
					t.active = i
				}
			}
			if t.active >= 0 {
				t.restyleLabels()
				if t.onTrack != nil {
					t.onTrack(tr.kind)
				}
			}
		}
	})
	w.AddController(click)

	pan := gtk.NewGestureDrag()
	pan.SetButton(gdk.BUTTON_MIDDLE)
	pan.ConnectDragBegin(func(x, y float64) { t.panning, t.panStartView = true, t.viewStart })
	pan.ConnectDragUpdate(func(offsetX, offsetY float64) {
		if t.panning {
			t.setViewStart(t.panStartView - offsetX/t.pxPerSec())
		}
	})
	pan.ConnectDragEnd(func(x, y float64) { t.panning = false })
	w.AddController(pan)

	// Right-click on the tags track → delete that tag.
	if tr != nil && tr.kind == TrackTags {
		rc := gtk.NewGestureClick()
		rc.SetButton(gdk.BUTTON_SECONDARY)
		rc.ConnectPressed(func(n int, x, y float64) {
			sec := t.xToSec(x)
			for _, s := range tr.segs {
				if sec >= s.Start && sec < s.End && t.onDeleteTag != nil {
					t.onDeleteTag(s.ID)
					return
				}
			}
		})
		w.AddController(rc)
	}

	// Right-click on the applications track → create an autotag rule.
	// Anywhere on the row opens the dialog; a segment under the cursor
	// pre-fills the patterns (with today's sparse data most of the row
	// is white — a silent no-op there felt broken).
	if tr != nil && tr.kind == TrackApps {
		rc := gtk.NewGestureClick()
		rc.SetButton(gdk.BUTTON_SECONDARY)
		rc.ConnectPressed(func(n int, x, y float64) {
			if t.onAddAutotag == nil {
				return
			}
			sec := t.xToSec(x)
			app, title := "", ""
			for _, s := range tr.segs {
				if sec >= s.Start && sec < s.End {
					app, title = s.App, s.Title
					break
				}
			}
			t.onAddAutotag(app, title)
		})
		w.AddController(rc)
	}
}

func (t *TimelineStack) attachRulerControllers(w *gtk.DrawingArea) {
	t.attachControllers(w, nil)
}

// selectRunAt double-click handler: select the run covering x. Runs come
// from the active track (screenshots: contiguous blue buckets).
func (t *TimelineStack) selectRunAt(x float64) {
	sec := t.xToSec(x)

	tr := t.tracks[t.active]
	switch tr.kind {
	case TrackScreenshots:
		b := int(sec / float64(t.intervalSec))
		if b < 0 || b >= t.nBuckets || !tr.has[b] {
			return
		}
		lo, hi := b, b
		for lo-1 >= 0 && tr.has[lo-1] {
			lo--
		}
		for hi+1 < t.nBuckets && tr.has[hi+1] {
			hi++
		}
		t.SetSelection(float64(lo)*float64(t.intervalSec), float64(hi+1)*float64(t.intervalSec))
	default:
		for _, s := range tr.segs {
			if sec >= s.Start && sec < s.End {
				t.SetSelection(s.Start, s.End)
				return
			}
		}
	}
}

// --- drawing ----------------------------------------------------------------

func (t *TimelineStack) drawRuler(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
	const padX = 6.0
	stripW := float64(width) - 2*padX
	if stripW <= 0 {
		return
	}
	pxPerSec := stripW / t.viewSpan
	secToX := func(sec float64) float64 { return padX + (sec-t.viewStart)*pxPerSec }

	labelSteps := []float64{1800, 3600, 7200, 10800, 21600, 43200}
	step := labelSteps[len(labelSteps)-1]
	for _, s := range labelSteps {
		if s*pxPerSec >= 52 {
			step = s
			break
		}
	}

	cr.SetFontSize(10)
	start := math.Ceil(t.viewStart/step) * step
	for sec := start; sec <= t.viewStart+t.viewSpan+0.001; sec += step {
		x := secToX(sec)
		if x < padX || x > padX+stripW {
			continue
		}
		cairoSetRGB(cr, colGrid)
		cr.SetLineWidth(1)
		cr.MoveTo(x, 3)
		cr.LineTo(x, float64(height))
		cr.Stroke()

		cairoSetRGB(cr, colText)
		cr.MoveTo(x+3, 12)
		cr.ShowText(fmt.Sprintf("%02d:%02d", int(sec/3600)%24, int(sec)%3600/60))
	}
}

func (t *TimelineStack) drawTrack(tr *track, cr *cairo.Context, width, height int) {
	const padX = 6.0
	stripW := float64(width) - 2*padX
	if stripW <= 0 {
		return
	}
	pxPerSec := stripW / t.viewSpan
	secToX := func(sec float64) float64 { return padX + (sec-t.viewStart)*pxPerSec }

	// background
	cairoSetRGB(cr, [3]float64{1, 1, 1})
	cr.Rectangle(padX, 0, stripW, float64(height))
	cr.Fill()

	switch tr.kind {
	case TrackScreenshots:
		cairoSetRGB(cr, [3]float64{0.208, 0.518, 0.894}) // blue
		b0 := int(math.Max(0, math.Floor(t.viewStart/float64(t.intervalSec))))
		b1 := int(math.Min(float64(t.nBuckets-1), math.Floor((t.viewStart+t.viewSpan)/float64(t.intervalSec))))
		for b := b0; b <= b1; {
			if !tr.has[b] {
				b++
				continue
			}
			end := b
			for end+1 <= b1 && tr.has[end+1] {
				end++
			}
			x0 := secToX(float64(b) * float64(t.intervalSec))
			x1 := secToX(float64(end+1) * float64(t.intervalSec))
			if x1-x0 < 1 {
				x1 = x0 + 1
			}
			cr.Rectangle(x0, 2, x1-x0, float64(height)-4)
			cr.Fill()
			b = end + 1
		}

	case TrackUsage:
		for _, s := range tr.segs {
			x0, x1 := secToX(s.Start), secToX(s.End)
			if x1 < padX || x0 > padX+stripW {
				continue
			}
			if s.State == "away" {
				cairoSetRGB(cr, colAway)
			} else {
				cairoSetRGB(cr, colActive)
			}
			cr.Rectangle(math.Max(x0, padX), 2, math.Min(x1, padX+stripW)-math.Max(x0, padX), float64(height)-4)
			cr.Fill()
		}

	case TrackApps, TrackTags, TrackAutotags:
		cr.SetFontSize(10)
		for _, s := range tr.segs {
			x0, x1 := secToX(s.Start), secToX(s.End)
			if x1 < padX || x0 > padX+stripW {
				continue
			}
			cx0, cx1 := math.Max(x0, padX), math.Min(x1, padX+stripW)
			cairoSetRGB(cr, appColor(s.App))
			cr.Rectangle(cx0, 2, cx1-cx0, float64(height)-4)
			cr.Fill()

			// label when the segment is wide enough
			if cx1-cx0 > 40 {
				label := s.App
				if label == "" {
					label = "(desktop)"
				}
				cr.SetSourceRGB(1, 1, 1)
				cr.MoveTo(cx0+4, float64(height)/2+3.5)
				maxw := int(cx1 - cx0 - 8)
				cr.ShowText(clipText(cr, label, maxw))
			}
		}
	}

	// selection overlay
	if t.selStart >= 0 {
		lo, hi := t.selStart, t.selEnd
		if lo > hi {
			lo, hi = hi, lo
		}
		x0, x1 := math.Max(padX, secToX(lo)), math.Min(padX+stripW, secToX(hi))
		if x1 > x0 {
			cr.SetSourceRGBA(0.2, 0.2, 0.2, 0.16)
			cr.Rectangle(x0, 0, x1-x0, float64(height))
			cr.Fill()
			cairoSetRGB(cr, colChip)
			cr.SetLineWidth(2)
			cr.MoveTo(x0, 0)
			cr.LineTo(x0, float64(height))
			cr.MoveTo(x1, 0)
			cr.LineTo(x1, float64(height))
			cr.Stroke()
		}
	}

	// "now" marker
	if t.isToday && t.nowSec >= t.viewStart && t.nowSec <= t.viewStart+t.viewSpan {
		x := secToX(t.nowSec)
		cairoSetRGB(cr, colNow)
		cr.SetLineWidth(2.5)
		cr.MoveTo(x, 0)
		cr.LineTo(x, float64(height))
		cr.Stroke()
	}

	// border
	cairoSetRGB(cr, colBorder)
	cr.SetLineWidth(1)
	cr.Rectangle(padX, 0, stripW, float64(height))
	cr.Stroke()

	// hover indicator
	if t.hover >= 0 {
		x := secToX(t.hover)
		cairoSetRGB(cr, colHover)
		cr.SetLineWidth(1.5)
		cr.MoveTo(x, 0)
		cr.LineTo(x, float64(height))
		cr.Stroke()
	}
}

// clipText truncates s with an ellipsis so its rendered width fits maxw.
func clipText(cr *cairo.Context, s string, maxw int) string {
	if int(cr.TextExtents(s).XAdvance) <= maxw {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if int(cr.TextExtents(string(runes)+"…").XAdvance) <= maxw {
			return string(runes) + "…"
		}
	}
	return "…"
}

// formatDuration renders a duration as "2h 13m", "13m 45s" or "45s".
func formatDuration(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	s := int(math.Round(sec))
	h, m, secPart := s/3600, (s%3600)/60, s%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, secPart)
	default:
		return fmt.Sprintf("%ds", secPart)
	}
}

func cairoSetRGB(cr *cairo.Context, c [3]float64) {
	cr.SetSourceRGB(c[0], c[1], c[2])
}

// formatClock renders seconds-of-day as HH:MM.
func formatClock(sec float64) string {
	return fmt.Sprintf("%02d:%02d", int(sec)/3600, int(sec)%3600/60)
}
