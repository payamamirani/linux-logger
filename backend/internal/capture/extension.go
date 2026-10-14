package capture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	extBusName = "org.payam.Logger"
	extObjPath = "/org/payam/Logger"
)

// Extension captures screenshots through the payam-logger GNOME Shell
// extension (org.payam.Logger), which uses gnome-shell's internal
// Shell.Screenshot object — fully silent: no permission dialog, no flash,
// no shutter sound. See extension/payam-logger@payamam/ and docs/capture.md.
//
// Protocol: the caller passes a PNG path (must be under the payam-logger
// data dir — the extension enforces this), the extension writes the PNG
// synchronously, then we re-encode it as JPEG and delete the temporary PNG.
type Extension struct {
	conn    *dbus.Conn
	dataDir string
	quality int
}

func newExtension(opts Options) (Backend, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Extension{conn: conn, dataDir: opts.DataDir, quality: opts.JPEGQuality}, nil
}

func (e *Extension) Name() string { return "extension" }

func (e *Extension) Capture(ctx context.Context) (*Screenshot, error) {
	tmp := filepath.Join(e.dataDir, fmt.Sprintf(".pending-%d.png", time.Now().UnixNano()))

	var ok bool
	var msg string
	err := e.conn.Object(extBusName, extObjPath).
		CallWithContext(ctx, extBusName+".Capture", 0, tmp).
		Store(&ok, &msg)
	if err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("extension dbus call: %w", err)
	}
	if !ok {
		os.Remove(tmp)
		return nil, fmt.Errorf("extension refused: %s", msg)
	}
	defer os.Remove(tmp)

	return pngFileToJPEG(tmp, e.quality)
}
