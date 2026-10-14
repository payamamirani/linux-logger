package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// detailRow is one row of the Details panel.
type detailRow struct {
	title    string
	start    string // HH:MM
	end      string
	duration string
	// selection range on the timeline (seconds of day)
	selStart, selEnd float64
}

// summaryRow is one row of the Summary panel.
type summaryRow struct {
	group            string
	duration         string
	share            string // "37%"
	selStart, selEnd float64
}

// Panels hosts the Details (left) and Summary (right) lists.
type Panels struct {
	root     *gtk.Paned
	details  *gtk.ListBox
	summary  *gtk.ListBox
	filter   *gtk.Entry
	trackLbl *gtk.Label

	filterText string
	rows       []detailRow
	sumRows    []summaryRow

	onSelect func(start, end float64) // timeline selection request
}

// NewPanels builds the bottom section.
func NewPanels() *Panels {
	p := &Panels{}

	p.trackLbl = gtk.NewLabel("")
	p.trackLbl.SetHAlign(gtk.AlignStart)
	p.trackLbl.SetMarkup("<b>Applications</b>")

	p.filter = gtk.NewEntry()
	p.filter.SetPlaceholderText("Filter…")
	p.filter.ConnectChanged(func() {
		p.filterText = strings.ToLower(strings.TrimSpace(p.filter.Text()))
		p.rebuildDetails()
	})

	p.details = gtk.NewListBox()
	p.details.SetHExpand(true)
	p.details.SetVExpand(true)
	p.details.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(p.rows) && p.onSelect != nil {
			r := p.rows[idx]
			p.onSelect(r.selStart, r.selEnd)
		}
	})

	detailsBox := gtk.NewBox(gtk.OrientationVertical, 4)
	detailsBox.Append(p.trackLbl)
	detailsBox.Append(p.filter)
	detailsScroll := gtk.NewScrolledWindow()
	detailsScroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	detailsScroll.SetChild(p.details)
	detailsScroll.SetVExpand(true)
	detailsBox.Append(detailsScroll)

	p.summary = gtk.NewListBox()
	p.summary.SetHExpand(true)
	p.summary.SetVExpand(true)
	p.summary.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(p.sumRows) && p.onSelect != nil {
			r := p.sumRows[idx]
			if r.selStart >= 0 {
				p.onSelect(r.selStart, r.selEnd)
			}
		}
	})

	sumLbl := gtk.NewLabel("")
	sumLbl.SetHAlign(gtk.AlignStart)
	sumLbl.SetMarkup("<b>Summary</b>")
	summaryScroll := gtk.NewScrolledWindow()
	summaryScroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	summaryScroll.SetChild(p.summary)
	summaryScroll.SetVExpand(true)
	summaryBox := gtk.NewBox(gtk.OrientationVertical, 4)
	summaryBox.Append(sumLbl)
	summaryBox.Append(summaryScroll)

	p.root = gtk.NewPaned(gtk.OrientationHorizontal)
	p.root.SetStartChild(detailsBox)
	p.root.SetEndChild(summaryBox)
	p.root.SetPosition(560)
	p.root.SetHExpand(true)
	return p
}

// Widget returns the panels root widget.
func (p *Panels) Widget() *gtk.Paned { return p.root }

// ConnectSelect registers the timeline-selection callback for row clicks.
func (p *Panels) ConnectSelect(fn func(start, end float64)) { p.onSelect = fn }

// SetTrack updates the panel header for the active timeline.
func (p *Panels) SetTrack(kind TrackKind) {
	names := map[TrackKind]string{
		TrackScreenshots: "Screenshots",
		TrackUsage:       "Computer usage",
		TrackApps:        "Applications",
	}
	p.trackLbl.SetMarkup(fmt.Sprintf("<b>%s</b>", names[kind]))
}

// SetData fills both panels for the active track.
func (p *Panels) SetData(kind TrackKind, rows []detailRow, sums []summaryRow) {
	p.rows = rows
	p.sumRows = sums
	p.SetTrack(kind)
	p.rebuildDetails()
	p.rebuildSummary()
}

// listBoxClear removes all rows of a ListBox (GTK4 has no remove_all).
func listBoxClear(lb *gtk.ListBox) {
	for {
		child := lb.FirstChild()
		if child == nil {
			break
		}
		lb.Remove(child)
	}
}

func (p *Panels) rebuildDetails() {
	listBoxClear(p.details)

	var visible []detailRow
	for _, r := range p.rows {
		if p.filterText != "" && !strings.Contains(strings.ToLower(r.title), p.filterText) {
			continue
		}
		visible = append(visible, r)
	}
	if len(visible) == 0 {
		p.details.Append(simpleRow("—", "", true))
		return
	}
	for _, r := range visible {
		title := gtk.NewLabel(r.title)
		title.SetHAlign(gtk.AlignStart)
		title.SetHExpand(true)
		title.SetEllipsize(pango.EllipsizeEnd)
		time := gtk.NewLabel(fmt.Sprintf("%s – %s  •  %s", r.start, r.end, r.duration))
		time.SetHAlign(gtk.AlignEnd)

		box := gtk.NewBox(gtk.OrientationHorizontal, 8)
		box.Append(title)
		box.Append(time)
		p.details.Append(simpleRow("", "", false, box))
	}
}

func (p *Panels) rebuildSummary() {
	listBoxClear(p.summary)
	if len(p.sumRows) == 0 {
		p.summary.Append(simpleRow("—", "", true))
		return
	}
	for _, r := range p.sumRows {
		group := gtk.NewLabel(r.group)
		group.SetHAlign(gtk.AlignStart)
		group.SetHExpand(true)
		dur := gtk.NewLabel(fmt.Sprintf("%s  •  %s", r.duration, r.share))
		dur.SetHAlign(gtk.AlignEnd)

		box := gtk.NewBox(gtk.OrientationHorizontal, 8)
		box.Append(group)
		box.Append(dur)
		p.summary.Append(simpleRow("", "", false, box))
	}
}

// simpleRow builds a plain ListBoxRow; with a text row when text != "".
func simpleRow(text, _ string, disabled bool, child ...gtk.Widgetter) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if len(child) > 0 && child[0] != nil {
		row.SetChild(child[0])
	} else if text != "" {
		lbl := gtk.NewLabel(text)
		if disabled {
			lbl.SetSensitive(false)
		}
		lbl.SetHAlign(gtk.AlignStart)
		row.SetChild(lbl)
	}
	return row
}

// summarizeGroups aggregates detail durations by group (app or state),
// sorted by total time — the Summary panel data.
func summarizeGroups(groups map[string]float64) []summaryRow {
	type kv struct {
		k string
		v float64
	}
	var list []kv
	for k, v := range groups {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })

	var total float64
	for _, x := range list {
		total += x.v
	}
	var out []summaryRow
	for _, x := range list {
		share := 0.0
		if total > 0 {
			share = x.v / total * 100
		}
		out = append(out, summaryRow{
			group:    x.k,
			duration: formatDuration(x.v),
			share:    fmt.Sprintf("%.0f%%", share),
		})
	}
	return out
}
