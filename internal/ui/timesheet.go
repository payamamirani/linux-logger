package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// tsLevelForIdx maps the group-by dropdown index to a tag level
// (0 = all levels, 1..3 = that level).
var tsLevelForIdx = []int{0, 1, 2, 3}

// timesheetWindow is the M6-lite report: tag totals over a date range with
// grouping by tag level, CSV export and clipboard copy.
type timesheetWindow struct {
	*gtk.Window

	store *Store

	from, to *gtk.Entry
	groupBy  *gtk.DropDown
	rows     *gtk.ListBox
	result   []*tsRow
}

type tsRow struct {
	Group string
	Total float64 // seconds
	Days  map[string]float64
}

// newTimesheetWindow opens the report for the last 7 days by default.
func newTimesheetWindow(parent *Window) {
	w := &timesheetWindow{store: parent.store}

	from, to := lastNDays(7)
	w.from = gtk.NewEntry()
	w.from.SetText(from)
	w.to = gtk.NewEntry()
	w.to.SetText(to)

	w.groupBy = gtk.NewDropDownFromStrings([]string{
		"All tag levels", "First level", "Second level", "Third level"})

	gen := gtk.NewButtonWithLabel("Generate")
	gen.ConnectClicked(func() { w.generate() })

	export := gtk.NewButtonWithLabel("Export CSV")
	export.ConnectClicked(func() { w.exportCSV(parent) })

	copyBtn := gtk.NewButtonWithLabel("Copy")
	copyBtn.ConnectClicked(func() { w.copyToClipboard() })

	controls := gtk.NewBox(gtk.OrientationHorizontal, 6)
	controls.Append(labeled("From (YYYY-MM-DD)", w.from))
	controls.Append(labeled("To", w.to))
	controls.Append(labeled("Group by", w.groupBy))
	controls.Append(gen)
	controls.Append(export)
	controls.Append(copyBtn)

	w.rows = gtk.NewListBox()
	w.rows.SetVExpand(true)
	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetChild(w.rows)
	scroll.SetVExpand(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(10)
	box.SetMarginBottom(10)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)
	box.Append(controls)
	box.Append(scroll)

	w.Window = gtk.NewWindow()
	w.SetTitle("Timesheet — Payam Logger")
	w.SetTransientFor(&parent.ApplicationWindow.Window)
	w.SetDefaultSize(640, 480)
	w.SetChild(box)
	w.Present()

	w.generate()
}

func labeled(text string, child gtk.Widgetter) *gtk.Box {
	l := gtk.NewLabel(text)
	l.SetHAlign(gtk.AlignStart)
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.Append(l)
	box.Append(child)
	return box
}

func (w *timesheetWindow) generate() {
	from := strings.TrimSpace(w.from.Text())
	to := strings.TrimSpace(w.to.Text())
	if len(from) != 10 || len(to) != 10 {
		w.rows.Append(simpleRow("Dates must be YYYY-MM-DD", "", true))
		return
	}

	raw, err := w.store.TagRowsRange(bgCtx(), from, to)
	if err != nil {
		w.rows.Append(simpleRow(err.Error(), "", true))
		return
	}

	level := tsLevelForIdx[w.groupBy.Selected()]
	agg := map[string]*tsRow{}
	for _, r := range raw {
		key := r.Name
		if level > 0 {
			key = tagLevel(r.Name, level)
			if key == "" {
				continue
			}
		}
		row := agg[key]
		if row == nil {
			row = &tsRow{Group: key, Days: map[string]float64{}}
			agg[key] = row
		}
		d := durationBetween(r.Start, r.End)
		row.Total += d
		row.Days[r.Day] += d
	}

	var list []*tsRow
	for _, r := range agg {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Total > list[j].Total })
	w.result = list

	listBoxClear(w.rows)
	if len(list) == 0 {
		w.rows.Append(simpleRow("No tags in this range", "", true))
		return
	}
	for _, r := range list {
		group := gtk.NewLabel(r.Group)
		group.SetHAlign(gtk.AlignStart)
		group.SetHExpand(true)
		group.SetEllipsize(pango.EllipsizeEnd)
		total := gtk.NewLabel(fmt.Sprintf("%s   •   %d days", formatDuration(r.Total), len(r.Days)))
		total.SetHAlign(gtk.AlignEnd)

		row := gtk.NewBox(gtk.OrientationHorizontal, 8)
		row.Append(group)
		row.Append(total)
		w.rows.Append(simpleRow("", "", false, row))
	}
}

func (w *timesheetWindow) exportCSV(parent *Window) {
	if len(w.result) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("group,total,days\n")
	for _, r := range w.result {
		days := make([]string, 0, len(r.Days))
		for d := range r.Days {
			days = append(days, d)
		}
		sort.Strings(days)
		b.WriteString(fmt.Sprintf("%q,%s,%q\n", r.Group, formatDuration(r.Total), strings.Join(days, " ")))
	}

	fc := gtk.NewFileChooserNative("Export timesheet",
		&parent.ApplicationWindow.Window, gtk.FileChooserActionSave, "Save", "Cancel")
	fc.SetCurrentName("payam-timesheet.csv")
	fc.ConnectResponse(func(resp int) {
		if resp != -5 || fc.File() == nil {
			return
		}
		if path := fc.File().Path(); path != "" {
			if err := writeFile(path, []byte(b.String())); err != nil {
				parent.status.SetText(fmt.Sprintf("export: %v", err))
			} else {
				parent.status.SetText("Exported: " + path)
			}
		}
	})
	fc.Show()
}

func (w *timesheetWindow) copyToClipboard() {
	if len(w.result) == 0 {
		return
	}
	var b strings.Builder
	for _, r := range w.result {
		b.WriteString(fmt.Sprintf("%s\t%s\n", r.Group, formatDuration(r.Total)))
	}
	if disp := gdk.DisplayGetDefault(); disp != nil {
		disp.Clipboard().SetText(b.String())
	}
}

// tagLevel extracts the n-th (1-based) element of a comma hierarchy.
func tagLevel(name string, n int) string {
	parts := strings.Split(name, ",")
	if n > len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[n-1])
}

// durationBetween parses two RFC3339 timestamps and returns their delta.
func durationBetween(start, end string) float64 {
	s := parseRFC3339(start)
	e := parseRFC3339(end)
	if s.IsZero() || e.IsZero() {
		return 0
	}
	return e.Sub(s).Seconds()
}

func lastNDays(n int) (string, string) {
	to := timeNow()
	from := to.AddDate(0, 0, -(n - 1))
	return from.Format("2006-01-02"), to.Format("2006-01-02")
}

// small wrappers keep the gtk/glib imports out of the report logic
func bgCtx() context.Context { return context.Background() }
func timeNow() time.Time     { return time.Now() }
func parseRFC3339(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
