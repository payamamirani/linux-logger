// Payam Logger Capture — silent screenshot backend + activity reporter.
//
// D-Bus interface org.payam.Logger on the session bus:
//
//   Methods:
//     Capture(filepath) → (success, error_message)
//         Fully silent screenshot via gnome-shell's internal
//         Shell.Screenshot object (no flash/sound/dialog).
//     GetStatus() → (app, title, workspace, idle)
//         Current focused-window state + idle flag.
//
//   Signals:
//     FocusChanged(app, title, workspace)
//         Emitted when the focused window, its title or the workspace
//         changes (deduplicated + throttled).
//     IdleChanged(idle)
//         Emitted when the user crosses the idle threshold (true) or
//         becomes active again (false).
//
// Security notes (Capture):
//  - writes are restricted to ~/.local/share/payam-logger/
//  - refuses when saving to disk is locked down by the administrator
//  - refuses while the session is locked

import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import Shell from 'gi://Shell';
import St from 'gi://St';

import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as PanelMenu from 'resource:///org/gnome/shell/ui/panelMenu.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';
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
    <method name="GetStatus">
      <arg type="s" direction="out" name="app"/>
      <arg type="s" direction="out" name="title"/>
      <arg type="i" direction="out" name="workspace"/>
      <arg type="b" direction="out" name="idle"/>
    </method>
    <signal name="FocusChanged">
      <arg type="s" name="app"/>
      <arg type="s" name="title"/>
      <arg type="i" name="workspace"/>
    </signal>
    <signal name="IdleChanged">
      <arg type="b" name="idle"/>
    </signal>
    <property name="Version" type="u" access="read"/>
  </interface>
</node>`;

const EXTENSION_VERSION = 4;

// Default web UI port (overridden by ~/.config/payam-logger/config.json).
const DEFAULT_UI_PORT = 7530;

const PayamIndicator = GObject.registerClass(
class PayamIndicator extends PanelMenu.Button {
    _init(extPath, port) {
        super._init(0.0, 'Payam Logger');

        this._url = `http://127.0.0.1:${port}`;

        const iconFile = Gio.File.new_for_path(
            GLib.build_filenamev([extPath, 'icon.svg']));
        this.add_child(new St.Icon({
            gicon: new Gio.FileIcon({file: iconFile}),
            style_class: 'system-status-icon',
        }));

        const open = new PopupMenu.PopupMenuItem('Open Payam Logger');
        open.connect('activate', () => this._openUI());
        this.menu.addMenuItem(open);

        const shots = new PopupMenu.PopupMenuItem('Open screenshots folder');
        shots.connect('activate', () => this._openScreenshots());
        this.menu.addMenuItem(shots);

        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        const about = new PopupMenu.PopupMenuItem(`Payam Logger v${EXTENSION_VERSION} — ${this._url}`);
        about.sensitive = false;
        this.menu.addMenuItem(about);
    }

    _openUI() {
        Gio.AppInfo.launch_default_for_uri(this._url, null)
            .catch(e => console.error(`[payam-logger] open UI: ${e.message}`));
    }

    _openScreenshots() {
        const dir = GLib.build_filenamev(
            [GLib.get_user_data_dir(), 'payam-logger', 'screenshots']);
        GLib.mkdir_with_parents(dir, 0o700);
        try {
            Gio.AppInfo.launch_default_for_uri(GLib.filename_to_uri(dir, null), null)
                .catch(e => console.error(`[payam-logger] open folder: ${e.message}`));
        } catch (e) {
            console.error(`[payam-logger] folder URI: ${e.message}`);
        }
    }
});

// readUIPort reads the daemon's web port from its config, if present.
function readUIPort() {
    try {
        const path = GLib.build_filenamev(
            [GLib.get_user_config_dir(), 'payam-logger', 'config.json']);
        const [ok, bytes] = GLib.file_get_contents(path);
        if (!ok)
            return DEFAULT_UI_PORT;
        const cfg = JSON.parse(new TextDecoder().decode(bytes));
        return (cfg.server && cfg.server.port) || DEFAULT_UI_PORT;
    } catch {
        return DEFAULT_UI_PORT;
    }
}

// After this many ms without input the user counts as idle/away.
const IDLE_THRESHOLD_MS = 5 * 60 * 1000;

// Minimum ms between two FocusChanged emissions for the same state
// (guards against hyperactive title changes, e.g. terminals/clocks).
const FOCUS_EMIT_MIN_INTERVAL_MS = 1000;

export default class PayamLoggerExtension extends Extension {
    enable() {
        this._shooter = new Shell.Screenshot();
        this._lockdown = new Gio.Settings({schema_id: 'org.gnome.desktop.lockdown'});
        this._queue = Promise.resolve();

        this._dbus = Gio.DBusExportedObject.wrapJSObject(DBUS_INTERFACE, this);
        this._dbus.export(Gio.DBus.session, BUS_PATH);
        this._nameOwnerId = Gio.DBus.session.own_name(
            BUS_NAME, Gio.BusNameOwnerFlags.REPLACE, null, null);

        this._idle = false;
        this._lastEmit = 0;
        this._watchIds = [];

        // Activity tracking must never take Capture down with it: if any
        // gnome-shell API moves again, log it and keep screenshots alive.
        try {
            this._setupActivityTracking();
            console.log('[payam-logger] extension enabled (v3: activity tracking)');
        } catch (e) {
            console.error(`[payam-logger] activity tracking failed: ${e.message}`);
            console.log('[payam-logger] extension enabled (capture only)');
        }

        // Panel indicator (tray): Open UI menu entry.
        try {
            this._indicator = new PayamIndicator(this.path, readUIPort());
            Main.panel.addToStatusArea('payam-logger', this._indicator);
        } catch (e) {
            console.error(`[payam-logger] panel indicator failed: ${e.message}`);
        }
    }

    disable() {
        if (this._indicator) {
            this._indicator.destroy();
            this._indicator = null;
        }
        this._teardownActivityTracking();

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

    // --- activity tracking ------------------------------------------------

    _setupActivityTracking() {
        this._watchIds.push(
            global.display.connect('notify::focus-window',
                () => this._onFocusChanged()));

        this._watchIds.push(
            global.workspace_manager.connect('workspace-switched',
                () => this._onFocusChanged()));

        // Track the initial state too.
        this._lastFocus = null;
        this._onFocusChanged();

        // Idle/away detection via Mutter's IdleMonitor.
        // (mutter 18 / GNOME 50: Meta.get_backend() and get_idle_monitor()
        // are gone — the core monitor hangs off global.backend.)
        const monitor = global.backend.get_core_idle_monitor();
        this._idleWatchId = monitor.add_idle_watch(IDLE_THRESHOLD_MS, () => {
            // Fires each time the user *has been* idle for the threshold;
            // only report the transition once until activity resumes.
            if (!this._idle) {
                this._idle = true;
                this._emitIdle(true);
            }
        });
        this._activeWatchId = monitor.add_user_active_watch(() => {
            if (this._idle) {
                this._idle = false;
                this._emitIdle(false);
            }
        });
    }

    _teardownActivityTracking() {
        for (const id of this._watchIds)
            global.display.disconnect(id);
        this._watchIds = [];

        const monitor = global.backend.get_core_idle_monitor();
        if (this._idleWatchId) {
            monitor.remove_watch(this._idleWatchId);
            this._idleWatchId = 0;
        }
        if (this._activeWatchId) {
            monitor.remove_watch(this._activeWatchId);
            this._activeWatchId = 0;
        }
    }

    _onFocusChanged() {
        const state = this._focusState();

        // Suppress no-op repeats (same app+title+workspace).
        if (this._lastFocus &&
            this._lastFocus.app === state.app &&
            this._lastFocus.title === state.title &&
            this._lastFocus.workspace === state.workspace)
            return;

        // Follow title changes of the currently focused window too.
        this._trackTitleChanges();

        // Throttle: never more than one signal per
        // FOCUS_EMIT_MIN_INTERVAL_MS (spammy titles are deduped above,
        // this only rate-limits genuinely alternating titles).
        const now = GLib.get_monotonic_time() / 1000;
        if (this._lastFocus && now - this._lastEmit < FOCUS_EMIT_MIN_INTERVAL_MS / 1000)
            return;
        this._lastEmit = now;
        this._lastFocus = state;

        this._dbus.emit_signal('FocusChanged',
            GLib.Variant.new('(ssi)', [state.app, state.title, state.workspace]));
    }

    // Watch title changes of the focused window.
    _trackTitleChanges() {
        if (this._titleWatchId) {
            global.display.disconnect(this._titleWatchId);
            this._titleWatchId = 0;
        }
        const win = global.display.focus_window;
        if (win) {
            this._titleWatchId = win.connect('notify::title',
                () => this._onFocusChanged());
        }
    }

    _focusState() {
        const win = global.display.focus_window;
        let app = '';
        let title = '';
        if (win) {
            // GTK apps report a proper application id; fall back to wm_class.
            app = win.get_gtk_application_id() || win.get_wm_class() || '';
            title = win.get_title() || '';
        }
        const workspace = global.workspace_manager.get_active_workspace_index();
        return {app, title, workspace};
    }

    _emitIdle(idle) {
        this._dbus.emit_signal('IdleChanged', GLib.Variant.new('(b)', [idle]));
        console.log(`[payam-logger] idle=${idle}`);
    }

    // --- D-Bus methods ------------------------------------------------------

    GetStatus() {
        const s = this._focusState();
        return [s.app, s.title, s.workspace, this._idle];
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
