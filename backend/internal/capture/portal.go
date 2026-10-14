package capture

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	portalBusName = "org.freedesktop.portal.Desktop"
	portalObjPath = "/org/freedesktop/portal/desktop"
	portalIface   = "org.freedesktop.portal.Screenshot"
	requestIface  = "org.freedesktop.portal.Request"

	portalTimeout = 20 * time.Second
)

// Portal captures screenshots through the XDG Desktop Portal
// (org.freedesktop.portal.Screenshot) — the sanctioned Wayland method that
// works across compositors.
//
// Caveats (verified in Phase 0, see docs/capture.md):
//   - requires a one-time permission grant in the portal permission store
//     (scripts/install.sh does this);
//   - every capture flashes the screen and plays a shutter sound — this is
//     hardcoded in xdg-desktop-portal-gnome (flash=TRUE). The extension
//     backend exists precisely to avoid this; the portal is a fallback.
//
// Protocol: Screenshot() returns a request object path; the result arrives
// asynchronously as org.freedesktop.portal.Request::Response(code, results)
// carrying a file:// URI to a PNG, which we re-encode as JPEG and delete.
type Portal struct {
	conn    *dbus.Conn
	quality int
}

func newPortal(opts Options) (Backend, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}
	p := &Portal{conn: conn, quality: opts.JPEGQuality}

	// Subscribe once for portal request responses; Capture filters by the
	// per-call request path.
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(requestIface),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return nil, fmt.Errorf("subscribe to portal responses: %w", err)
	}
	return p, nil
}

func (p *Portal) Name() string { return "portal" }

func (p *Portal) Capture(ctx context.Context) (*Screenshot, error) {
	token := fmt.Sprintf("payam%d", time.Now().UnixNano())

	sigCh := make(chan *dbus.Signal, 8)
	p.conn.Signal(sigCh)
	defer p.conn.RemoveSignal(sigCh)

	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
	}
	var reqPath dbus.ObjectPath
	err := p.conn.Object(portalBusName, portalObjPath).
		CallWithContext(ctx, portalIface+".Screenshot", 0, "", opts).
		Store(&reqPath)
	if err != nil {
		return nil, fmt.Errorf("portal call: %w", err)
	}

	uri, err := p.awaitResponse(ctx, sigCh, reqPath)
	if err != nil {
		return nil, err
	}

	path, err := uriToPath(uri)
	if err != nil {
		return nil, err
	}
	defer os.Remove(path)

	return pngFileToJPEG(path, p.quality)
}

// awaitResponse waits for the Response signal matching our request path.
func (p *Portal) awaitResponse(ctx context.Context, sigCh <-chan *dbus.Signal, reqPath dbus.ObjectPath) (string, error) {
	timeout := time.After(portalTimeout)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("portal response timeout after %s", portalTimeout)
		case sig := <-sigCh:
			if sig.Path != reqPath || sig.Name != requestIface+".Response" {
				continue
			}
			if len(sig.Body) < 2 {
				return "", errors.New("malformed portal response")
			}
			code, _ := sig.Body[0].(uint32)
			if code != 0 {
				return "", fmt.Errorf("portal response code %d (1=cancelled, 2=error) — is the screenshot permission granted?", code)
			}
			results, _ := sig.Body[1].(map[string]dbus.Variant)
			v, ok := results["uri"]
			if !ok {
				return "", errors.New("portal response has no uri")
			}
			uri, _ := v.Value().(string)
			if uri == "" {
				return "", errors.New("portal returned empty uri")
			}
			return uri, nil
		}
	}
}

// uriToPath converts a file:// URI into a filesystem path.
func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse uri %q: %w", uri, err)
	}
	if u.Scheme != "file" || u.Path == "" {
		return "", fmt.Errorf("unsupported uri %q", uri)
	}
	return strings.TrimPrefix(u.Path, "//"), nil
}
