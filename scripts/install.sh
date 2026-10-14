#!/usr/bin/env bash
# Install payam-loggerd as a systemd *user* service.
# Run `make build` first. Safe to re-run (config is never overwritten).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_SRC="$ROOT/backend/bin/payam-loggerd"
BIN_DST="$HOME/.local/bin/payam-loggerd"
UNIT_SRC="$ROOT/systemd/payam-logger.service"
UNIT_DST="$HOME/.config/systemd/user/payam-logger.service"
CONF_SRC="$ROOT/config/payam-logger.example.json"
CONF_DST="$HOME/.config/payam-logger/config.json"

[ -x "$BIN_SRC" ] || { echo "error: binary not found; run: make build" >&2; exit 1; }

mkdir -p "$(dirname "$BIN_DST")" "$(dirname "$UNIT_DST")" "$(dirname "$CONF_DST")"
install -m 0755 "$BIN_SRC" "$BIN_DST"
install -m 0644 "$UNIT_SRC" "$UNIT_DST"
[ -f "$CONF_DST" ] || install -m 0644 "$CONF_SRC" "$CONF_DST"

systemctl --user daemon-reload
systemctl --user enable --now payam-logger.service

echo "Installed."
echo "  logs:   journalctl --user -u payam-logger -f"
echo "  status: systemctl --user status payam-logger"
echo "  data:   ~/.local/share/payam-logger"
