package ui

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/storage"
)

// newAddAutotagDialog opens the "Add to autotag" window, prefilled from the
// right-clicked application segment (ManicTime's right-click → Add to
// autotag).
func newAddAutotagDialog(parent *Window, app, title string) {
	d := &struct{ w *gtk.Window }{}

	name := gtk.NewEntry()
	name.SetPlaceholderText("Tag, e.g. Browsing or Client 1, Support")

	appPat := gtk.NewEntry()
	appPat.SetText(app)
	appPat.SetPlaceholderText("Application contains… (empty = any)")

	titlePat := gtk.NewEntry()
	titlePat.SetText(title)
	titlePat.SetPlaceholderText("Title contains… (empty = any)")

	errLbl := gtk.NewLabel("")

	make := func(label string, e *gtk.Entry) *gtk.Box { return labeled(label, e) }

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(14)
	box.SetMarginBottom(14)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)
	box.Append(make("Tag name", name))
	box.Append(make("Rule — application", appPat))
	box.Append(make("Rule — window title", titlePat))
	box.Append(errLbl)

	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.ConnectClicked(func() { d.w.Close() })

	ok := gtk.NewButtonWithLabel("Add rule")
	ok.ConnectClicked(func() {
		n := strings.TrimSpace(name.Text())
		if n == "" {
			errLbl.SetText("Tag name is required")
			return
		}
		if err := parent.store.AddAutotag(parent.ctx(), n,
			strings.TrimSpace(appPat.Text()), strings.TrimSpace(titlePat.Text())); err != nil {
			errLbl.SetText(err.Error())
			return
		}
		d.w.Close()
		parent.reloadAutotags()
	})

	btns := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btns.SetHAlign(gtk.AlignEnd)
	btns.Append(cancel)
	btns.Append(ok)
	box.Append(btns)

	d.w = gtk.NewWindow()
	d.w.SetTitle("Add to autotag — Payam Logger")
	d.w.SetTransientFor(&parent.ApplicationWindow.Window)
	d.w.SetModal(true)
	d.w.SetDefaultSize(380, 260)
	d.w.SetChild(box)
	d.w.Present()
}

// autotagEditor lists all rules with delete buttons (the M4 rule editor).
func autotagEditor(parent *Window) {
	win := gtk.NewWindow()
	win.SetTitle("Autotag rules — Payam Logger")
	win.SetTransientFor(&parent.ApplicationWindow.Window)
	win.SetDefaultSize(560, 380)

	var refresh func() *gtk.Box
	refresh = func() *gtk.Box {
		box := gtk.NewBox(gtk.OrientationVertical, 4)
		box.SetMarginTop(10)
		box.SetMarginBottom(10)
		box.SetMarginStart(12)
		box.SetMarginEnd(12)

		rules, err := parent.store.Autotags(parent.ctx())
		if err != nil {
			box.Append(gtk.NewLabel(err.Error()))
			return box
		}
		if len(rules) == 0 {
			lbl := gtk.NewLabel("No rules yet — right-click an application segment on the timeline.")
			lbl.SetSensitive(false)
			box.Append(lbl)
			return box
		}
		for _, r := range rules {
			rule := storage.AutotagRule(r)
			desc := fmt.Sprintf("%s\n   app ~ %q   title ~ %q",
				rule.Name, rule.AppPattern, rule.TitlePattern)
			if rule.AppPattern == "" && rule.TitlePattern == "" {
				desc += "  (matches everything)"
			}
			lbl := gtk.NewLabel(desc)
			lbl.SetHAlign(gtk.AlignStart)
			lbl.SetHExpand(true)

			del := gtk.NewButtonFromIconName("user-trash-symbolic")
			del.SetTooltipText("Delete rule")
			del.ConnectClicked(func() {
				_ = parent.store.DeleteAutotag(parent.ctx(), rule.ID)
				win.SetChild(refresh())
				parent.reloadAutotags()
			})

			row := gtk.NewBox(gtk.OrientationHorizontal, 6)
			row.Append(lbl)
			row.Append(del)
			box.Append(row)
		}
		return box
	}

	win.SetChild(refresh())
	win.Present()
}
