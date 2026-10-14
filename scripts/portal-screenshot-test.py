#!/usr/bin/env python3
"""فاز ۰ — تست پورتال اسکرین‌شات (xdg-desktop-portal).

Calls org.freedesktop.portal.Screenshot.Screenshot on the session bus and
waits for the Request::Response signal. Prints the response code, the
result URI and the captured file size.

Usage:
    python3 scripts/portal-screenshot-test.py [--interactive] [--timeout N]

Exit codes: 0=success, 1=timeout, 2=synchronous error
"""
import argparse
import os
import secrets
import sys

import gi

gi.require_version("Gio", "2.0")
from gi.repository import Gio, GLib

PORTAL = "org.freedesktop.portal.Desktop"
DESKTOP_PATH = "/org/freedesktop/portal/desktop"
IFACE = "org.freedesktop.portal.Screenshot"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--interactive", action="store_true",
                    help="pass interactive=true (forces the permission dialog)")
    ap.add_argument("--timeout", type=int, default=60)
    args = ap.parse_args()

    bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    loop = GLib.MainLoop.new(None, False)
    state = {"handle": None, "done": False}

    def on_response(conn, sender, path, iface, signal, params, user_data):
        if path != state["handle"]:
            return
        code, results = params.unpack()
        state["done"] = True
        print(f"response_code = {code}  (0=success 1=cancelled 2=error)")
        uri = results.get("uri")
        if uri:
            p = uri.removeprefix("file://")
            size = os.path.getsize(p) if os.path.exists(p) else -1
            print(f"uri         = {uri}")
            print(f"file_size   = {size}")
        loop.quit()

    bus.signal_subscribe(
        PORTAL, "org.freedesktop.portal.Request", "Response",
        None, None, Gio.DBusSignalFlags.NONE, on_response, None,
    )

    token = "payam" + secrets.token_hex(4)
    opts = {"handle_token": GLib.Variant("s", token)}
    if args.interactive:
        opts["interactive"] = GLib.Variant("b", True)

    print(f"calling Screenshot(interactive={args.interactive}) ...")
    try:
        res = bus.call_sync(
            PORTAL, DESKTOP_PATH, IFACE, "Screenshot",
            GLib.Variant("(sa{sv})", ("", opts)),
            GLib.VariantType("(o)"),
            Gio.DBusCallFlags.NONE, -1, None,
        )
    except GLib.Error as e:
        print(f"SYNC_ERROR: {e.message}")
        return 2
    state["handle"] = res.unpack()[0]
    print(f"request_handle = {state['handle']}")

    GLib.timeout_add_seconds(args.timeout, loop.quit)
    loop.run()

    if not state["done"]:
        print(f"TIMEOUT after {args.timeout}s (dialog ignored?)")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
