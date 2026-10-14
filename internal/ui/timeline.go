package ui

import (
	"fmt"
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

	timelineHeight = 72
	snapRadiusPx   = 8.0 // sticky-selection snap radius in pixels
)

// Colors (RGB 0..1).
var (
	colBlue   = [3]float64{0.208, 0.518, 0.894} // #3584e4 — screenshots
	colGrid   = [3]float64{0.835, 0.816, 0.784} // #d5d0cc — hour grid
	colText   = [3]float64{0.467, 0.443, 0.400} // #77767b — hour labels
	colHover  = [3]float64{0.149, 0.635, 0.412} // #26a269 — hover (green)
	colNow    = [3]float64{0.878, 0.106, 0.142} // #e01b24 — now (red)
	colBorder = [3]float64{0.922, 0.914, 0.898} // #ebeae4
	colChip   = [3]float64{0.227, 0.223, 0.212} // #3a3a38 — chip background
)

// chipBGAlpha / chipFillAlpha for translucent overlays.
const chipFillAlpha = 0.16

// Timeline is a custom drawing widget showing one day (00:00–24:00) with a
// zoomable view window. Buckets — one per capture interval — are drawn blue
// when a screenshot exists in them and left white otherwise (locked screen /
// service gaps).
//
// Interaction:
//   - hover         → green position line + time; reports the second under
//     the cursor (onHover)
//   - left drag     → range selection with sticky edges that snap to the
//     boundaries of screenshot runs; pulling harder than snapRadiusPx breaks
//     the stick. Reports (start,end) seconds, or (-1,-1) when cleared.
//   - middle drag / horizontal scroll → pan
//   - vertical scroll / +− buttons → zoom (1h..24h)
type Timeline struct {
	*gtk.DrawingArea

	intervalSec int
	nBuckets    int

	has     []bool    // bucket → screenshot present
	secs    []int     // record times-of-day, sorted
	snapPts []float64 // sticky snap points (run boundaries), sorted

	// visible window, seconds of day
	viewStart float64
	viewSpan  float64

	// hover position (seconds of day), -1 when outside
	hover  float64
	hoverX float64

	// selection range (seconds of day), negative start = none
	selStart, selEnd float64
	selPressX        float64

	onHover     func(sec float64)
	onSelection func(start, end float64)

	// pan drag state
	panning      bool
	panStartX    float64
	panStartView float64

	// "now" marker (only when the shown day is today)
	isToday bool
	nowSec  float64
}

// NewTimeline creates the widget for the given capture interval.
func NewTimeline(intervalSec int) *Timeline {
	t := &Timeline{
		DrawingArea: gtk.NewDrawingArea(),
		intervalSec: intervalSec,
		nBuckets:    daySeconds / intervalSec,
		viewStart:   defaultStart,
		viewSpan:    defaultViewSec,
		hover:       -1,
		selStart:    -1,
	}

	t.SetHExpand(true)
	t.SetSizeRequest(-1, timelineHeight)
	t.SetDrawFunc(t.draw)

	// --- hover ---------------------------------------------------------
	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) {
		t.hoverX, t.hover = x, t.xToSec(x)
		t.QueueDraw()
		if t.onHover != nil {
			t.onHover(t.hover)
		}
	})
	motion.ConnectLeave(func() {
		t.hover = -1
		t.QueueDraw()
		if t.onHover != nil {
			t.onHover(-1)
		}
	})
	t.AddController(motion)

	// --- zoom + pan via scroll ------------------------------------------
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollBothAxes)
	scroll.ConnectScroll(func(dx, dy float64) bool {
		switch {
		case math.Abs(dx) > math.Abs(dy):
			t.panBy(dx)
		default:
			t.zoomAt(t.hoverX, dy)
		}
		return true
	})
	t.AddController(scroll)

	// --- selection: left drag ---------------------------------------------
	sel := gtk.NewGestureDrag()
	sel.ConnectDragBegin(func(x, y float64) {
		start, _ := t.snap(t.xToSec(x))
		t.selStart, t.selEnd = start, start
		t.QueueDraw()
		t.emitSelection()
	})
	sel.ConnectDragUpdate(func(offsetX, offsetY float64) {
		if t.selStart < 0 {
			return
		}
		end, _ := t.snap(t.xToSec(t.selPressX + offsetX))
		t.selEnd = end
		t.QueueDraw()
		t.emitSelection()
	})
	sel.ConnectDragEnd(func(x, y float64) {
		// A "drag" that never really moved is a click → clear selection.
		if math.Abs(t.selEnd-t.selStart)*t.pxPerSec() < 4 {
			t.ClearSelection()
		}
	})
	t.AddController(sel)

	// --- pan: middle drag ---------------------------------------------------
	pan := gtk.NewGestureDrag()
	pan.SetButton(gdk.BUTTON_MIDDLE)
	pan.ConnectDragBegin(func(x, y float64) {
		t.panning, t.panStartX, t.panStartView = true, x, t.viewStart
	})
	pan.ConnectDragUpdate(func(offsetX, offsetY float64) {
		if !t.panning {
			return
		}
		t.setViewStart(t.panStartView - offsetX/t.pxPerSec())
	})
	pan.ConnectDragEnd(func(x, y float64) { t.panning = false })
	t.AddController(pan)

	return t
}

// ConnectHover registers the hover callback; sec is the second-of-day under
// the cursor, or -1 when the pointer left the widget.
func (t *Timeline) ConnectHover(fn func(sec float64)) { t.onHover = fn }

// ConnectSelection registers the selection callback: start/end in
// seconds-of-day (ordered), or (-1,-1) once the selection is cleared.
func (t *Timeline) ConnectSelection(fn func(start, end float64)) { t.onSelection = fn }

// ClearSelection removes the current selection.
func (t *Timeline) ClearSelection() {
	t.selStart, t.selEnd = -1, -1
	t.QueueDraw()
	t.emitSelection()
}

// SetDay loads one day's records and resets the view to the default window.
// day is the shown date (YYYY-MM-DD); the "now" marker is drawn when it
// equals today.
func (t *Timeline) SetDay(day string, records []storage.Record, now time.Time) {
	t.has = make([]bool, t.nBuckets)
	t.secs = t.secs[:0]
	for _, r := range records {
		ts, err := time.Parse(time.RFC3339, r.TakenAt)
		if err != nil {
			continue
		}
		sec := ts.Hour()*3600 + ts.Minute()*60 + ts.Second()
		t.secs = append(t.secs, sec)
		if b := sec / t.intervalSec; b >= 0 && b < t.nBuckets {
			t.has[b] = true
		}
	}
	t.buildSnapPoints()

	if day == now.Format("2006-01-02") {
		t.isToday = true
		t.nowSec = float64(now.Hour()*3600 + now.Minute()*60 + now.Second())
	} else {
		t.isToday = false
	}

	t.viewStart, t.viewSpan = defaultStart, defaultViewSec
	t.selStart, t.selEnd = -1, -1
	t.hover = -1
	t.QueueDraw()
	t.emitSelection()
}

// ZoomIn / ZoomOut change the visible window around its center.
func (t *Timeline) ZoomIn()  { t.zoomAt(float64(t.Width())/2, -1) }
func (t *Timeline) ZoomOut() { t.zoomAt(float64(t.Width())/2, +1) }

// --- internal --------------------------------------------------------------

// buildSnapPoints collects screenshot-run boundaries — the sticky targets.
func (t *Timeline) buildSnapPoints() {
	t.snapPts = t.snapPts[:0]
	for b := 0; b < t.nBuckets; {
		if !t.has[b] {
			b++
			continue
		}
		end := b
		for end+1 < t.nBuckets && t.has[end+1] {
			end++
		}
		t.snapPts = append(t.snapPts,
			float64(b)*float64(t.intervalSec),     // run start
			float64(end+1)*float64(t.intervalSec)) // run end
		b = end + 1
	}
}

// snap returns the nearest sticky point within the snap radius, or the raw
// second when the pointer pulled harder than the stickiness.
func (t *Timeline) snap(sec float64) (float64, bool) {
	if len(t.snapPts) == 0 {
		return sec, false
	}
	radius := snapRadiusPx / t.pxPerSec()

	best, bestDist := sec, math.MaxFloat64
	for _, p := range t.snapPts {
		if d := math.Abs(sec - p); d < bestDist {
			best, bestDist = p, d
		}
	}
	if bestDist <= radius {
		return best, true
	}
	return sec, false
}

func (t *Timeline) pxPerSec() float64 {
	w := float64(t.Width() - 12)
	if w <= 0 {
		return 1
	}
	return w / t.viewSpan
}

func (t *Timeline) panBy(dxCursor float64) {
	t.setViewStart(t.viewStart + dxCursor/t.pxPerSec())
}

func (t *Timeline) setViewStart(s float64) {
	maxStart := float64(daySeconds) - t.viewSpan
	if maxStart < 0 {
		maxStart = 0
	}
	t.viewStart = math.Max(0, math.Min(s, maxStart))
	t.QueueDraw()
}

func (t *Timeline) xToSec(x float64) float64 {
	w := float64(t.Width())
	if w <= 0 {
		return t.viewStart
	}
	sec := t.viewStart + x/w*t.viewSpan
	return math.Max(t.viewStart, math.Min(sec, t.viewStart+t.viewSpan))
}

// zoomAt zooms around the given x anchor; dy>0 zooms out, dy<0 zooms in.
func (t *Timeline) zoomAt(x, dy float64) {
	const factorPerTick = 1.25
	factor := math.Pow(factorPerTick, dy)

	newSpan := math.Max(minViewSec, math.Min(maxViewSec, t.viewSpan*factor))
	anchor := t.xToSec(x)

	rel := (anchor - t.viewStart) / t.viewSpan
	newStart := anchor - rel*newSpan

	t.viewSpan = newSpan
	t.setViewStart(newStart)
}

func (t *Timeline) emitSelection() {
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

// --- drawing ----------------------------------------------------------------

func (t *Timeline) draw(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
	const padX = 6.0
	const padTop = 6.0
	const labelH = 18.0

	stripW := float64(width) - 2*padX
	stripH := float64(height) - padTop - labelH
	stripY := padTop
	if stripW <= 0 || stripH <= 0 {
		return
	}
	pxPerSec := stripW / t.viewSpan

	secToX := func(sec float64) float64 { return padX + (sec-t.viewStart)*pxPerSec }

	// --- strip background (white = no screenshot / locked) -----------------
	cairoSetRGB(cr, [3]float64{1, 1, 1})
	cr.Rectangle(padX, stripY, stripW, stripH)
	cr.Fill()

	// --- blue screenshot runs ----------------------------------------------
	cairoSetRGB(cr, colBlue)
	b0 := int(math.Max(0, math.Floor(t.viewStart/float64(t.intervalSec))))
	b1 := int(math.Min(float64(t.nBuckets-1), math.Floor((t.viewStart+t.viewSpan)/float64(t.intervalSec))))
	for b := b0; b <= b1; {
		if !t.has[b] {
			b++
			continue
		}
		end := b
		for end+1 <= b1 && t.has[end+1] {
			end++
		}
		x0 := secToX(float64(b) * float64(t.intervalSec))
		x1 := secToX(float64(end+1) * float64(t.intervalSec))
		if x1-x0 < 1 { // never fully invisible
			x1 = x0 + 1
		}
		cr.Rectangle(x0, stripY, x1-x0, stripH)
		cr.Fill()
		b = end + 1
	}

	// --- hour grid + labels --------------------------------------------------
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
		cr.MoveTo(x, stripY)
		cr.LineTo(x, stripY+stripH)
		cr.Stroke()

		cairoSetRGB(cr, colText)
		cr.MoveTo(x+3, stripY+stripH+13)
		cr.ShowText(fmt.Sprintf("%02d:%02d", int(sec/3600)%24, int(sec)%3600/60))
	}

	// --- selection overlay -----------------------------------------------------
	if t.selStart >= 0 {
		lo, hi := t.selStart, t.selEnd
		if lo > hi {
			lo, hi = hi, lo
		}
		x0, x1 := math.Max(padX, secToX(lo)), math.Min(padX+stripW, secToX(hi))
		if x1 > x0 {
			cr.SetSourceRGBA(0.2, 0.2, 0.2, chipFillAlpha)
			cr.Rectangle(x0, stripY, x1-x0, stripH)
			cr.Fill()

			cairoSetRGB(cr, colChip)
			cr.SetLineWidth(2)
			cr.MoveTo(x0, stripY)
			cr.LineTo(x0, stripY+stripH)
			cr.MoveTo(x1, stripY)
			cr.LineTo(x1, stripY+stripH)
			cr.Stroke()

			// duration chip below the strip, centered on the selection
			label := formatDuration(hi - lo)
			cr.SetFontSize(10)
			tw := textWidth(cr, label)
			cx := math.Min(math.Max((x0+x1)/2-tw/2, padX), padX+stripW-tw)
			drawChip(cr, cx, stripY+stripH+1, tw+10, 15, label)
		}
	}

	// --- "now" marker (today only) — red, on top of everything ----------------
	if t.isToday && t.nowSec >= t.viewStart && t.nowSec <= t.viewStart+t.viewSpan {
		x := secToX(t.nowSec)
		cairoSetRGB(cr, colNow)
		cr.SetLineWidth(2.5)
		cr.MoveTo(x, stripY)
		cr.LineTo(x, stripY+stripH)
		cr.Stroke()
	}

	// --- border ------------------------------------------------------------------
	cairoSetRGB(cr, colBorder)
	cr.SetLineWidth(1)
	cr.Rectangle(padX, stripY, stripW, stripH)
	cr.Stroke()

	// --- hover indicator (green) ---------------------------------------------------
	if t.hover >= 0 {
		x := secToX(t.hover)

		cairoSetRGB(cr, colHover)
		cr.SetLineWidth(1.5)
		cr.MoveTo(x, stripY)
		cr.LineTo(x, stripY+stripH)
		cr.Stroke()

		// time chip below the strip (in the label row, above any hour label)
		label := fmt.Sprintf("%02d:%02d", int(t.hover/3600), int(t.hover)%3600/60)
		cr.SetFontSize(10)
		tw := textWidth(cr, label)
		cx := math.Min(math.Max(x-tw/2, padX), padX+stripW-tw)
		drawChipColored(cr, cx, stripY+stripH+1, tw+10, 15, label, colHover)
	}
}

// drawChip paints a dark rounded chip with white text.
func drawChip(cr *cairo.Context, x, y, w, h float64, label string) {
	drawChipColored(cr, x, y, w, h, label, colChip)
}

func drawChipColored(cr *cairo.Context, x, y, w, h float64, label string, c [3]float64) {
	cairoSetRGB(cr, c)
	cr.Rectangle(x, y, w, h)
	cr.Fill()
	cr.SetSourceRGB(1, 1, 1)
	cr.MoveTo(x+5, y+11)
	cr.ShowText(label)
}

// textWidth approximates the rendered width of s at the current font size.
func textWidth(cr *cairo.Context, s string) float64 {
	e := cr.TextExtents(s)
	return e.XAdvance
}

// formatDuration renders a duration as "2h 13m", "45m" or "1h".
func formatDuration(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func cairoSetRGB(cr *cairo.Context, c [3]float64) {
	cr.SetSourceRGB(c[0], c[1], c[2])
}
