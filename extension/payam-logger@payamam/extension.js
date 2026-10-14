// Payam Logger Capture — silent screenshot backend.
//
// Exposes org.payam.Logger.Capture(filepath) on the session bus. It captures
// the whole screen with Shell.Screenshot — the exact same object gnome-shell
// uses internally (js/ui/screenshot.js: new Shell.Screenshot()) — WITHOUT the
// flash and shutter sound that the portal and org.gnome.Shell.Screenshot
// D-Bus paths hardcode (see docs/capture.md).
//
// Security notes:
//  - writes are restricted to ~/.local/share/payam-logger/
//  - refuses when saving to disk is locked down by the administrator
//  - refuses while the session is locked

import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Shell from 'gi://Shell';

import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

Gio._promisify(Shell.Screenshot.prototype, 'screenshot');

const BUS_NAME = 'org.payam.Logger';
const BUS_PATH = '/org/payam/Logger';

const DBUS_INTERFACE = `
<node>
  <interface name="org.payam.Logger">
    <method name="Capture">
      <arg type="s" direction="in" name="filepath"/>
      <arg type="b" direction="out" name="success"/>
      <arg type="s" direction="out" name="error_message"/>
    </method>
    <property name="Version" type="u" access="read"/>
  </interface>
</node>`;

const EXTENSION_VERSION = 1;

export default class PayamLoggerExtension extends Extension {
    enable() {
        this._shooter = new Shell.Screenshot();
        this._lockdown = new Gio.Settings({schema_id: 'org.gnome.desktop.lockdown'});
        this._queue = Promise.resolve();

        this._dbus = Gio.DBusExportedObject.wrapJSObject(DBUS_INTERFACE, this);
        this._dbus.export(Gio.DBus.session, BUS_PATH);
        this._nameOwnerId = Gio.DBus.session.own_name(
            BUS_NAME, Gio.BusNameOwnerFlags.REPLACE, null, null);

        console.log('[payam-logger] extension enabled');
    }

    disable() {
        Gio.DBus.session.unown_name(this._nameOwnerId);
        this._dbus.unexport();
        this._dbus = null;
        this._shooter = null;
        this._lockdown = null;

        console.log('[payam-logger] extension disabled');
    }

    get Version() {
        return EXTENSION_VERSION;
    }

    // Calls are serialized: Shell.Screenshot permits only one operation at a
    // time and the daemon must never receive two replies for one call.
    CaptureAsync(params, invocation) {
        this._queue = this._queue
            .then(() => this._capture(params[0]))
            .then(result => invocation.return_value(
                GLib.Variant.new('(bs)', result)))
            .catch(e => invocation.return_value(
                GLib.Variant.new('(bs)', [false, String(e)])));
    }

    async _capture(filepath) {
        if (typeof filepath !== 'string' || filepath.length === 0)
            return [false, 'filepath must be a non-empty string'];

        if (this._lockdown.get_boolean('disable-save-to-disk'))
            return [false, 'saving to disk is disabled by administrator'];

        if (Main.sessionMode.isLocked)
            return [false, 'session is locked'];

        // Only write inside our own data directory (canonicalized so "../"
        // cannot escape the prefix).
        const dataDir = GLib.build_filenamev(
            [GLib.get_user_data_dir(), 'payam-logger']);
        const canonical = GLib.canonicalize_filename(filepath, null);
        if (!canonical.startsWith(dataDir + GLib.DIR_SEPARATOR_S))
            return [false, `refusing to write outside ${dataDir}`];

        const file = Gio.File.new_for_path(canonical);
        const stream = file.replace(
            null, false, Gio.FileCreateFlags.REPLACE_DESTINATION, null);
        try {
            // include_cursor = false: activity shots don't need the pointer
            await this._shooter.screenshot(false, stream);
        } finally {
            stream.close(null);
        }

        console.log(`[payam-logger] captured ${canonical}`);
        return [true, ''];
    }
}
