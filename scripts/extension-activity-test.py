#!/usr/bin/env python3
"""فاز M1 — تست سیگنال‌های فعالیت اکستنشن v2.

Subscribes to org.payam.Logger FocusChanged/IdleChanged signals and prints
them live for N seconds. Call GetStatus first. Switch windows / change
titles while it runs to see focus events (idle events need IDLE_THRESHOLD).

Usage: python3 scripts/extension-activity-test.py [--seconds 30]
"""
import argparse
import sys

import gi

gi.require_version("Gio", "2.0")
from gi.repository import Gio, GLib

BUS = "org.payam.Logger"
PATH = "/org/payam/Logger"
IFACE = "org.payam.Logger"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--seconds", type=int, default=30)
    args = ap.parse_args()

    bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)

    # Initial status
    try:
        res = bus.call_sync(
            BUS, PATH, IFACE, "GetStatus", None,
            GLib.VariantType.new("(ssib)"), Gio.DBusCallFlags.NONE, -1, None)
        app, title, ws, idle = res.unpack()
        print(f"status: app={app!r} title={title!r} workspace={ws} idle={idle}")
    except GLib.Error as e:
        print(f"GetStatus failed: {e.message}")
        print("→ extension v2 is not active. Update it (make install-extension) "
              "and log out/in once.")
        return 2

    def on_signal(conn, sender, path, iface, signal, params):
        print(f"signal: {signal}{params.unpack()}")

    bus.signal_subscribe(BUS, IFACE, None, PATH, None,
                         Gio.DBusSignalFlags.NONE, on_signal)

    loop = GLib.MainLoop.new(None, False)
    GLib.timeout_add_seconds(args.seconds, loop.quit)
    print(f"listening for {args.seconds}s — switch windows / titles ...")
    loop.run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
