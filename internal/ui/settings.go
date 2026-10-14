package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os/exec"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/config"
)

// intervalChoices are the preset capture intervals offered in the UI.
var intervalChoices = []struct {
	label string
	secs  int
}{
	{"Every 1 minute", 60},
	{"Every 2 minutes", 120},
	{"Every 5 minutes", 300},
	{"Every 10 minutes", 600},
	{"Every 15 minutes", 900},
	{"Every 30 minutes", 1800},
	{"Every 60 minutes", 3600},
}

// settingsDialog edits the capture interval and retention, saves the config
// file and restarts the user service so changes take effect immediately.
type settingsDialog struct {
	*gtk.Window

	w       *Window
	cfgPath string
	cfg     config.Config

	interval  *gtk.DropDown
	retention *gtk.SpinButton
	forever   *gtk.CheckButton
	note      *gtk.Label

	// customSecs is set when the current interval is not one of the
	// presets and was appended to the dropdown dynamically.
	customSecs int
}

func newSettingsDialog(w *Window) {
	d := &settingsDialog{w: w}

	path, err := config.DefaultPath()
	if err != nil {
		log.Printf("settings: config path: %v", err)
		return
	}
	d.cfgPath = path

	cfg, err := config.Load(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("settings: load config: %v — using defaults", err)
		}
		cfg, err = config.Load("")
		if err != nil {
			log.Printf("settings: defaults: %v", err)
			return
		}
	}
	d.cfg = cfg

	d.Window = gtk.NewWindow()
	d.SetTitle("Settings — Payam Logger")
	d.SetTransientFor(&w.ApplicationWindow.Window)
	d.SetModal(true)
	d.SetDefaultSize(420, 260)

	d.build()
	d.Present()
}

func (d *settingsDialog) build() {
	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.SetMarginTop(14)
	box.SetMarginBottom(14)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	// --- capture interval -------------------------------------------
	lblInterval := gtk.NewLabel("Screenshot interval:")
	lblInterval.SetHAlign(gtk.AlignStart)
	d.interval = d.buildIntervalDropdown()
	box.Append(lblInterval)
	box.Append(d.interval)

	// --- retention ---------------------------------------------------
	lblRetention := gtk.NewLabel("Keep screenshots for (days):")
	lblRetention.SetHAlign(gtk.AlignStart)
	d.retention = gtk.NewSpinButtonWithRange(1, float64(config.MaxRetentionDays), 1)
	d.retention.SetValue(float64(retentionOrDefault(d.cfg)))

	d.forever = gtk.NewCheckButtonWithLabel("Keep forever (no automatic deletion)")
	if d.cfg.Storage.RetentionDays == 0 {
		d.forever.SetActive(true)
		d.retention.SetSensitive(false)
	}
	d.forever.ConnectToggled(func() {
		d.retention.SetSensitive(!d.forever.Active())
	})

	box.Append(lblRetention)
	box.Append(d.retention)
	box.Append(d.forever)

	// --- feedback + buttons ------------------------------------------
	d.note = gtk.NewLabel("")
	d.note.SetHAlign(gtk.AlignStart)
	box.Append(d.note)

	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.ConnectClicked(func() { d.Close() })

	apply := gtk.NewButtonWithLabel("Apply")
	apply.ConnectClicked(d.onApply)

	btns := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btns.SetHAlign(gtk.AlignEnd)
	btns.Append(cancel)
	btns.Append(apply)
	box.Append(btns)

	d.SetChild(box)
}

// buildIntervalDropdown builds the preset list; if the configured interval
// is not a preset it is appended so the current value is always visible.
func (d *settingsDialog) buildIntervalDropdown() *gtk.DropDown {
	labels := make([]string, len(intervalChoices))
	selected := 0
	for i, c := range intervalChoices {
		labels[i] = c.label
		if c.secs == d.cfg.Capture.IntervalSec {
			selected = i
		}
	}
	if d.cfg.Capture.IntervalSec%config.MinIntervalSec == 0 {
		found := false
		for _, c := range intervalChoices {
			if c.secs == d.cfg.Capture.IntervalSec {
				found = true
				break
			}
		}
		if !found {
			d.customSecs = d.cfg.Capture.IntervalSec
			labels = append(labels,
				fmt.Sprintf("Every %d minutes", d.cfg.Capture.IntervalSec/config.MinIntervalSec))
			selected = len(labels) - 1
		}
	}

	dd := gtk.NewDropDownFromStrings(labels)
	dd.SetSelected(uint(selected))
	return dd
}

func (d *settingsDialog) onApply() {
	cfg := d.cfg

	if idx := int(d.interval.Selected()); idx < len(intervalChoices) {
		cfg.Capture.IntervalSec = intervalChoices[idx].secs
	} else if d.customSecs > 0 {
		cfg.Capture.IntervalSec = d.customSecs
	}

	if d.forever.Active() {
		cfg.Storage.RetentionDays = 0
	} else {
		cfg.Storage.RetentionDays = int(d.retention.Value())
	}

	if err := cfg.Save(d.cfgPath); err != nil {
		d.setNote(fmt.Sprintf("Save failed: %v", err), true)
		return
	}
	if err := restartService(); err != nil {
		d.setNote(fmt.Sprintf("Saved, but restart failed: %v", err), true)
		return
	}

	d.setNote("✓ Applied — service restarted with the new settings", false)
	d.w.status.SetText(fmt.Sprintf(
		"settings applied: every %d min • keep %s",
		cfg.Capture.IntervalSec/60,
		retentionLabel(cfg.Storage.RetentionDays)))
}

func (d *settingsDialog) setNote(text string, isErr bool) {
	if isErr {
		d.note.SetMarkup(fmt.Sprintf("<span foreground=\"#e01b24\">%s</span>", text))
	} else {
		d.note.SetMarkup(fmt.Sprintf("<span foreground=\"#26a269\">%s</span>", text))
	}
}

// restartService restarts the payam-logger user service (no root needed).
func restartService() error {
	out, err := exec.Command("systemctl", "--user", "restart", "payam-logger").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func retentionOrDefault(cfg config.Config) int {
	if cfg.Storage.RetentionDays == 0 {
		return config.DefaultRetentionDays
	}
	return cfg.Storage.RetentionDays
}

func retentionLabel(days int) string {
	if days == 0 {
		return "forever"
	}
	return fmt.Sprintf("%d days", days)
}
