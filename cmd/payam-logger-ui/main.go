// Command payam-logger-ui is the GTK4 viewer for Payam Logger — browse the
// recorded days and their screenshot timelines.
package main

import (
	"fmt"
	"os"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/payamam/payam-logger/internal/ui"
)

func main() {
	app := gtk.NewApplication("org.payam.Logger.ui", gio.ApplicationFlagsNone)
	app.ConnectActivate(func() {
		store, err := ui.OpenStore()
		if err != nil {
			fatalDialog(app, err)
			return
		}
		win := ui.NewWindow(app, store)
		win.ConnectCloseRequest(func() bool {
			store.Close()
			return false
		})
		win.Present()
	})
	if code := app.Run(os.Args); code > 0 {
		os.Exit(int(code))
	}
}

// fatalDialog reports a startup error and quits.
func fatalDialog(app *gtk.Application, err error) {
	d := gtk.NewMessageDialog(nil, gtk.DialogModal, gtk.MessageError, gtk.ButtonsClose)
	d.SetMarkup(fmt.Sprintf("Payam Logger could not open its data:\n\n%s", err))
	d.ConnectResponse(func(int) {
		d.Destroy()
		app.Quit()
	})
	d.Present()
}
