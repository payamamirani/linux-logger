#!/usr/bin/env bash
# Full setup of payam-loggerd as a systemd *user* service:
#   1. binary        -> ~/.local/bin/payam-loggerd
#   2. systemd unit  -> ~/.config/systemd/user/payam-logger.service
#   3. config        -> ~/.config/payam-logger/config.json (never overwritten)
#   4. GNOME Shell extension (silent capture) — needs one logout/login the
#      very first time; the script says so when applicable
#   5. portal screenshot permission (fallback backend) — idempotent
#   6. enable + start the service
#
# Run `make build` first. Safe to re-run.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_SRC="$ROOT/bin/payam-loggerd"
BIN_DST="$HOME/.local/bin/payam-loggerd"
UNIT_SRC="$ROOT/systemd/payam-logger.service"
UNIT_DST="$HOME/.config/systemd/user/payam-logger.service"
CONF_SRC="$ROOT/config/payam-logger.example.json"
CONF_DST="$HOME/.config/payam-logger/config.json"

[ -x "$BIN_SRC" ] || { echo "error: binary not found; run: make build" >&2; exit 1; }

echo "==> binary"
mkdir -p "$(dirname "$BIN_DST")"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "==> systemd unit"
mkdir -p "$(dirname "$UNIT_DST")"
install -m 0644 "$UNIT_SRC" "$UNIT_DST"

echo "==> config"
mkdir -p "$(dirname "$CONF_DST")"
if [ -f "$CONF_DST" ]; then
    echo "    config already exists, keeping it: $CONF_DST"
else
    install -m 0644 "$CONF_SRC" "$CONF_DST"
    echo "    installed default config: $CONF_DST"
fi

echo "==> GNOME Shell extension (silent capture)"
"$ROOT/scripts/install-extension.sh" || true

echo "==> portal screenshot permission (fallback backend)"
gdbus call --session \
    --dest org.freedesktop.impl.portal.PermissionStore \
    --object-path /org/freedesktop/impl/portal/PermissionStore \
    --method org.freedesktop.impl.portal.PermissionStore.SetPermission \
    screenshot true screenshot '' "['yes']" >/dev/null 2>&1 \
    && echo "    granted" || echo "    skipped (portal unavailable)"

echo "==> systemd user service"
systemctl --user daemon-reload
systemctl --user enable --now payam-logger.service

sleep 1
systemctl --user --no-pager status payam-logger.service | head -5 || true

cat <<EOF

Installed.
  logs:   journalctl --user -u payam-logger -f
  stop:   systemctl --user stop payam-logger
  data:   ~/.local/share/payam-logger
  config: ~/.config/payam-logger/config.json
EOF
