package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/diamondburned/gotk4/pkg/cairo"
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

	timelineHeight = 68
)

// Colors (RGB 0..1).
var (
	colBlue   = [3]float64{0.208, 0.518, 0.894} // #3584e4 — screenshots
	colGrid   = [3]float64{0.835, 0.816, 0.784} // #d5d0cc — hour grid
	colText   = [3]float64{0.467, 0.443, 0.400} // #77767b — hour labels
	colHover  = [3]float64{0.878, 0.106, 0.142} // #e01b24 — hover line
	colNow    = [3]float64{0.149, 0.635, 0.412} // #26a269 — now marker
	colBorder = [3]float64{0.922, 0.914, 0.898} // #ebeae4
)

// Timeline is a custom drawing widget showing one day (00:00–24:00) with a
// zoomable view window. Buckets — one per capture interval — are drawn blue
// when a screenshot exists in them and left white otherwise (locked screen /
// service gaps). Hover, scroll-zoom and drag-pan are built in.
type Timeline struct {
	*gtk.DrawingArea

	intervalSec int
	nBuckets    int

	has  []bool // bucket → screenshot present
	secs []int  // record times-of-day, sorted (kept for callers)

	// visible window, seconds of day
	viewStart float64
	viewSpan  float64

	// hover position (seconds of day), -1 when outside
	hover  float64
	hoverX float64

	onHover func(sec float64)

	// drag panning state
	dragging      bool
	dragStartView float64

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
	}

	t.SetHExpand(true)
	t.SetSizeRequest(-1, timelineHeight)
	t.SetDrawFunc(t.draw)

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

	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollBothAxes)
	scroll.ConnectScroll(func(dx, dy float64) bool {
		t.zoomAt(t.hoverX, dy)
		return true
	})
	t.AddController(scroll)

	drag := gtk.NewGestureDrag()
	drag.ConnectDragBegin(func(x, y float64) {
		t.dragging = true
		t.dragStartView = t.viewStart
	})
	drag.ConnectDragUpdate(func(offsetX, offsetY float64) {
		if !t.dragging {
			return
		}
		width := float64(t.Width())
		if width <= 0 {
			return
		}
		delta := -offsetX / width * t.viewSpan
		t.setViewStart(t.dragStartView + delta)
	})
	drag.ConnectDragEnd(func(x, y float64) { t.dragging = false })
	t.AddController(drag)

	return t
}

// ConnectHover registers the hover callback; sec is the second-of-day under
// the cursor, or -1 when the pointer left the widget.
func (t *Timeline) ConnectHover(fn func(sec float64)) { t.onHover = fn }

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

	if day == now.Format("2006-01-02") {
		t.isToday = true
		t.nowSec = float64(now.Hour()*3600 + now.Minute()*60 + now.Second())
	} else {
		t.isToday = false
	}

	t.viewStart, t.viewSpan = defaultStart, defaultViewSec
	t.hover = -1
	t.QueueDraw()
}

// ZoomIn / ZoomOut change the visible window around its center.
func (t *Timeline) ZoomIn()  { t.zoomAt(float64(t.Width())/2, -1) }
func (t *Timeline) ZoomOut() { t.zoomAt(float64(t.Width())/2, +1) }

// VisibleWindow returns the current window (start, span) in seconds of day.
func (t *Timeline) VisibleWindow() (start, span float64) { return t.viewStart, t.viewSpan }

// --- internal --------------------------------------------------------------

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

	// keep the anchor at the same relative position
	rel := (anchor - t.viewStart) / t.viewSpan
	newStart := anchor - rel*newSpan

	t.viewSpan = newSpan
	t.setViewStart(newStart)
}

// draw renders hour grid + labels, screenshot buckets, now marker and the
// hover indicator.
func (t *Timeline) draw(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
	const padX = 6.0
	const padTop = 6.0
	const labelH = 16.0

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
		label := fmt.Sprintf("%02d:%02d", int(sec/3600)%24, int(sec)%3600/60)
		cr.MoveTo(x+3, stripY+stripH+12)
		cr.ShowText(label)
	}

	// --- "now" marker (today only) -------------------------------------------
	if t.isToday && t.nowSec >= t.viewStart && t.nowSec <= t.viewStart+t.viewSpan {
		x := secToX(t.nowSec)
		cairoSetRGB(cr, colNow)
		cr.SetLineWidth(2)
		cr.MoveTo(x, stripY)
		cr.LineTo(x, stripY+stripH)
		cr.Stroke()
	}

	// --- border ----------------------------------------------------------------
	cairoSetRGB(cr, colBorder)
	cr.SetLineWidth(1)
	cr.Rectangle(padX, stripY, stripW, stripH)
	cr.Stroke()

	// --- hover indicator --------------------------------------------------------
	if t.hover >= 0 {
		x := secToX(t.hover)
		cairoSetRGB(cr, colHover)
		cr.SetLineWidth(1.5)
		cr.MoveTo(x, stripY)
		cr.LineTo(x, stripY+stripH)
		cr.Stroke()

		// time chip above the strip
		label := fmt.Sprintf("%02d:%02d", int(t.hover/3600), int(t.hover)%3600/60)
		cr.SetFontSize(10)
		cairoSetRGB(cr, colHover)
		cr.MoveTo(math.Min(math.Max(x+3, padX), padX+stripW-34), stripY-1)
		cr.ShowText(label)
	}
}

func cairoSetRGB(cr *cairo.Context, c [3]float64) {
	cr.SetSourceRGB(c[0], c[1], c[2])
}
