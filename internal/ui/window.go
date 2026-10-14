package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // JPEG decoder for the full viewer
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/storage"
)

const thumbHeight = 96

// Window is the main viewer window.
type Window struct {
	*gtk.ApplicationWindow

	store *Store

	daysList *gtk.ListBox
	viewer   *gtk.Picture
	tlScroll *gtk.ScrolledWindow
	status   *gtk.Label

	// loadState guards the async day-loading (generation counter + cancel).
	gen    atomic.Int64
	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewWindow creates the viewer window and starts loading recorded days.
func NewWindow(app *gtk.Application, store *Store) *Window {
	w := &Window{ApplicationWindow: gtk.NewApplicationWindow(app), store: store}

	w.SetTitle("Payam Logger")
	w.SetDefaultSize(1150, 720)

	// Header bar with a settings button.
	header := gtk.NewHeaderBar()
	header.SetTitleWidget(gtk.NewLabel("Payam Logger"))
	gear := gtk.NewButtonFromIconName("emblem-system-symbolic")
	gear.SetTooltipText("Settings")
	gear.ConnectClicked(func() { newSettingsDialog(w) })
	header.PackEnd(gear)
	w.SetTitlebar(header)

	// --- left: recorded days ------------------------------------------
	w.daysList = gtk.NewListBox()
	w.daysList.SetVExpand(true)
	w.daysList.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row != nil {
			w.loadDay(row.Name())
		}
	})

	daysScroll := gtk.NewScrolledWindow()
	daysScroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	daysScroll.SetChild(w.daysList)
	daysScroll.SetSizeRequest(215, -1)

	// --- right: viewer + timeline + status -----------------------------
	w.viewer = gtk.NewPicture()
	w.viewer.SetHExpand(true)
	w.viewer.SetVExpand(true)
	w.viewer.SetContentFit(gtk.ContentFitContain)

	tlBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	w.tlScroll = gtk.NewScrolledWindow()
	w.tlScroll.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyNever)
	w.tlScroll.SetChild(tlBox)

	w.status = gtk.NewLabel("")
	w.status.SetHAlign(gtk.AlignStart)

	right := gtk.NewBox(gtk.OrientationVertical, 4)
	right.Append(w.viewer)
	right.Append(w.tlScroll)
	right.Append(w.status)

	paned := gtk.NewPaned(gtk.OrientationHorizontal)
	paned.SetStartChild(daysScroll)
	paned.SetEndChild(right)
	paned.SetPosition(220)
	paned.SetHExpand(true)
	paned.SetVExpand(true)

	w.SetChild(paned)

	w.loadDays()
	return w
}

// loadDays fills the day list and selects the most recent day.
func (w *Window) loadDays() {
	days, err := w.store.Days(context.Background())
	if err != nil {
		log.Printf("load days: %v", err)
	}

	if len(days) == 0 {
		w.status.SetText("No screenshots recorded yet — is payam-loggerd running?")
		return
	}

	var first *gtk.ListBoxRow
	for _, d := range days {
		row := gtk.NewListBoxRow()
		row.SetName(d.Day)
		label := gtk.NewLabel(fmt.Sprintf("%s\n%d screenshots", d.Day, d.Count))
		label.SetHAlign(gtk.AlignStart)
		row.SetChild(label)
		w.daysList.Append(row)
		if first == nil {
			first = row
		}
	}
	w.daysList.SelectRow(first)
}

// loadDay starts an async load of one day's timeline.
func (w *Window) loadDay(day string) {
	gen := w.gen.Add(1)

	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	var ctx context.Context
	ctx, w.cancel = context.WithCancel(context.Background())
	w.mu.Unlock()

	// Fresh (empty) timeline strip for this day.
	tlBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	w.tlScroll.SetChild(tlBox)
	w.viewer.SetPaintable(nil)
	w.status.SetText(fmt.Sprintf("loading %s …", day))

	go w.loadDayAsync(ctx, gen, day, tlBox)
}

func (w *Window) loadDayAsync(ctx context.Context, gen int64, day string, tlBox *gtk.Box) {
	recs, err := w.store.Records(ctx, day)
	if err != nil {
		glib.IdleAdd(func() bool {
			w.status.SetText(fmt.Sprintf("error loading %s: %v", day, err))
			return false
		})
		return
	}

	total := int64(0)
	for _, r := range recs {
		total += r.SizeBytes
	}

	firstShown := false
	const batch = 12
	var batchBox *gtk.Box

	for i, rec := range recs {
		if ctx.Err() != nil { // day changed, abort stale load
			return
		}

		data, err := w.store.ReadScreenshot(rec)
		if err != nil {
			log.Printf("read %s: %v", rec.Path, err)
			continue
		}

		th, err := thumbnail(data, thumbHeight)
		if err != nil {
			log.Printf("thumbnail %s: %v", rec.Path, err)
			continue
		}

		idx, r := i, rec
		full := data
		glib.IdleAdd(func() bool {
			if w.gen.Load() != gen {
				return false // stale
			}
			if batchBox == nil {
				batchBox = gtk.NewBox(gtk.OrientationHorizontal, 4)
				tlBox.Append(batchBox)
			}
			batchBox.Append(w.newThumbButton(idx, r, th, full))
			if !firstShown {
				firstShown = true
				w.showFull(r, full)
				w.status.SetText(fmt.Sprintf("%s — %d screenshots • %.1f MB",
					day, len(recs), float64(total)/1024/1024))
			}
			return false
		})

		if (i+1)%batch == 0 {
			// Start a new visual batch on the next iteration so IdleAdd
			// batches stay small and the UI stays responsive.
			batchBox = nil
		}
	}

	glib.IdleAdd(func() bool {
		if w.gen.Load() != gen {
			return false
		}
		if !firstShown {
			w.status.SetText(fmt.Sprintf("%s — no readable screenshots (%d records)", day, len(recs)))
		}
		return false
	})
}

// newThumbButton builds one clickable timeline thumbnail.
func (w *Window) newThumbButton(idx int, rec storage.Record, thumbPNG, fullJPEG []byte) gtk.Widgetter {
	pic := gtk.NewPicture()
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetCanShrink(false)
	pic.SetPaintable(textureFromBytes(thumbPNG))
	pic.SetSizeRequest(-1, thumbHeight)

	btn := gtk.NewButton()
	btn.SetChild(pic)
	btn.SetHasFrame(false)
	btn.SetTooltipText(shortTime(rec.TakenAt))
	btn.ConnectClicked(func() { w.showFull(rec, fullJPEG) })
	return btn
}

// showFull shows the full-resolution screenshot in the viewer.
func (w *Window) showFull(rec storage.Record, jpegBytes []byte) {
	w.viewer.SetPaintable(textureFromBytes(jpegBytes))
	w.viewer.SetTooltipText(fmt.Sprintf("%s — %dx%d", shortTime(rec.TakenAt), rec.Width, rec.Height))
}

// textureFromBytes builds a GdkTexture from encoded image bytes.
func textureFromBytes(encoded []byte) *gdk.Texture {
	t, err := gdk.NewTextureFromBytes(glib.NewBytes(encoded))
	if err != nil {
		log.Printf("texture: %v", err)
		return nil
	}
	return t
}

// shortTime renders an RFC3339 timestamp as HH:MM:SS.
func shortTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("15:04:05")
}

// imageDims decodes image dimensions (used in tests).
func imageDims(b []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
