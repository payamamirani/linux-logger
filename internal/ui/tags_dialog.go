package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/storage"
)

// addTagDialog is the ManicTime-style "Add tag" window: assigns a tag to a
// time range (prefilled from the current timeline selection).
type addTagDialog struct {
	*gtk.Window

	store *Store
	day   string // YYYY-MM-DD being viewed

	name     *gtk.Entry
	note     *gtk.Entry
	billable *gtk.CheckButton
	start    *gtk.Entry
	end      *gtk.Entry
	err      *gtk.Label
}

// newAddTagDialog opens the dialog; startSec/endSec preselect the range
// (pass -1,-1 to default to the last hour).
func newAddTagDialog(parent *Window, startSec, endSec float64) {
	d := &addTagDialog{
		store: parent.store,
		day:   parent.currentDay,
	}

	dayTs, err := time.Parse("2006-01-02", parent.currentDay)
	if err != nil {
		return
	}
	if startSec < 0 || endSec < startSec {
		now := time.Now()
		startSec = float64(now.Hour()*3600+now.Minute()*60) - 3600
		endSec = float64(now.Hour()*3600 + now.Minute()*60)
		if startSec < 0 {
			startSec = 0
		}
	}

	d.Window = gtk.NewWindow()
	d.SetTitle("Add tag — Payam Logger")
	d.SetTransientFor(&parent.ApplicationWindow.Window)
	d.SetModal(true)
	d.SetDefaultSize(380, 320)

	d.name = gtk.NewEntry()
	d.name.SetPlaceholderText("Client, Project, Task  (comma = hierarchy)")
	d.name.SetActivatesDefault(true)

	d.note = gtk.NewEntry()
	d.note.SetPlaceholderText("Note (optional)")

	d.billable = gtk.NewCheckButtonWithLabel("Billable")

	d.start = gtk.NewEntry()
	d.start.SetText(formatClock(startSec))
	d.end = gtk.NewEntry()
	d.end.SetText(formatClock(endSec))

	d.err = gtk.NewLabel("")

	form := func(label string, entry *gtk.Entry) *gtk.Box {
		l := gtk.NewLabel(label)
		l.SetHAlign(gtk.AlignStart)
		box := gtk.NewBox(gtk.OrientationVertical, 2)
		box.Append(l)
		box.Append(entry)
		return box
	}

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(14)
	box.SetMarginBottom(14)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)
	box.Append(form("Tag", d.name))
	box.Append(form("Note", d.note))
	box.Append(d.billable)

	times := gtk.NewBox(gtk.OrientationHorizontal, 8)
	times.Append(form("Start (HH:MM)", d.start))
	times.Append(form("End (HH:MM)", d.end))
	box.Append(times)

	// recent tags as quick picks
	if names := parent.recentTagNames(); len(names) > 0 {
		lbl := gtk.NewLabel("Recent:")
		lbl.SetHAlign(gtk.AlignStart)
		box.Append(lbl)
		row := gtk.NewBox(gtk.OrientationHorizontal, 4)
		for _, n := range names {
			n := n
			btn := gtk.NewButtonWithLabel(firstLevel(n))
			btn.SetTooltipText(n)
			btn.ConnectClicked(func() { d.name.SetText(n) })
			row.Append(btn)
		}
		box.Append(row)
	}

	box.Append(d.err)

	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.ConnectClicked(func() { d.Close() })

	ok := gtk.NewButtonWithLabel("Add tag")
	ok.ConnectClicked(func() { d.save(dayTs) })

	btns := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btns.SetHAlign(gtk.AlignEnd)
	btns.Append(cancel)
	btns.Append(ok)
	box.Append(btns)

	d.SetChild(box)
	d.Present()
}

func (d *addTagDialog) save(dayTs time.Time) {
	name := strings.TrimSpace(d.name.Text())
	if name == "" {
		d.err.SetText("Tag name is required")
		return
	}
	startSec, ok1 := parseClock(d.start.Text())
	endSec, ok2 := parseClock(d.end.Text())
	if !ok1 || !ok2 {
		d.err.SetText("Times must be HH:MM (e.g. 09:30)")
		return
	}
	if endSec <= startSec {
		d.err.SetText("End must be after start")
		return
	}

	start := time.Date(dayTs.Year(), dayTs.Month(), dayTs.Day(),
		int(startSec)/3600, (int(startSec)%3600)/60, 0, 0, time.Local)
	end := start.Add(time.Duration((endSec - startSec) * float64(time.Second)))

	if err := d.store.AddTag(start, end, name, d.note.Text(), d.billable.Active()); err != nil {
		d.err.SetText(err.Error())
		return
	}
	d.Close()
}

// firstLevel returns the first element of a comma-separated tag hierarchy.
func firstLevel(name string) string {
	if i := strings.Index(name, ","); i >= 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

// parseClock parses "HH:MM" into seconds of day.
func parseClock(s string) (float64, bool) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return float64(h*3600 + m*60), true
}

// confirmDeleteTag asks and deletes a tag.
func confirmDeleteTag(parent *Window, tagID int64) {
	d := gtk.NewMessageDialog(&parent.ApplicationWindow.Window, gtk.DialogModal,
		gtk.MessageQuestion, gtk.ButtonsYesNo)
	d.SetMarkup("Delete this tag?")
	d.ConnectResponse(func(resp int) {
		d.Destroy()
		if resp == -8 { // GTK_RESPONSE_YES
			if err := parent.store.DeleteTag(parent.ctx(), tagID); err != nil {
				parent.status.SetText(fmt.Sprintf("delete tag: %v", err))
				return
			}
			parent.showDay(parent.currentDay)
		}
	})
	d.Present()
}

// confirmDeleteScreenshot asks and deletes a screenshot (file + row).
func confirmDeleteScreenshot(parent *Window, id int64) {
	d := gtk.NewMessageDialog(&parent.ApplicationWindow.Window, gtk.DialogModal,
		gtk.MessageQuestion, gtk.ButtonsYesNo)
	d.SetMarkup("Delete this screenshot permanently?")
	d.ConnectResponse(func(resp int) {
		d.Destroy()
		if resp == -8 {
			if err := parent.store.DeleteScreenshot(parent.ctx(), id); err != nil {
				parent.status.SetText(fmt.Sprintf("delete screenshot: %v", err))
				return
			}
			parent.showDay(parent.currentDay)
		}
	})
	d.Present()
}

// saveScreenshotAs opens a save dialog and writes the screenshot bytes.
func saveScreenshotAs(parent *Window, rec storage.Record) {
	data, err := parent.store.ReadScreenshot(rec)
	if err != nil {
		parent.status.SetText(fmt.Sprintf("read: %v", err))
		return
	}

	fc := gtk.NewFileChooserNative("Save screenshot as",
		&parent.ApplicationWindow.Window, gtk.FileChooserActionSave,
		"Save", "Cancel")
	fc.SetCurrentName(fmt.Sprintf("payam-%s.jpg", strings.ReplaceAll(rec.TakenAt, ":", "")))
	fc.ConnectResponse(func(resp int) {
		if resp != -5 { // accept
			return
		}
		file := fc.File()
		if file == nil {
			return
		}
		path := file.Path()
		if err := writeFile(path, data); err != nil {
			parent.status.SetText(fmt.Sprintf("save: %v", err))
			return
		}
		parent.status.SetText("Saved: " + path)
	})
	fc.Show()
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
