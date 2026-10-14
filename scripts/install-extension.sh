#!/usr/bin/env bash
# Install the Payam Logger GNOME Shell extension (silent capture backend).
# GNOME Shell scans extensions at startup: after the FIRST install you must
# log out and back in once, then re-run this script to enable it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UUID="payam-logger@payamam"
SRC="$ROOT/extension/$UUID"
DST="$HOME/.local/share/gnome-shell/extensions/$UUID"

command -v gnome-extensions >/dev/null || {
    echo "error: gnome-extensions not found" >&2; exit 1;
}

mkdir -p "$(dirname "$DST")"
rm -rf "$DST"
cp -r "$SRC" "$DST"
echo "extension files installed: $DST"

if gnome-extensions list 2>/dev/null | grep -qx "$UUID"; then
    if gnome-extensions show "$UUID" 2>/dev/null | grep -q "State: ACTIVE"; then
        echo "extension already active."
    else
        gnome-extensions enable "$UUID"
        echo "extension enabled."
    fi
    echo "test: scripts/extension-capture-test.sh"
else
    cat <<'EOF'
NOTE: GNOME Shell discovers extensions at startup.
      1. Log out and log back in (once).
      2. Run this script again to enable the extension.
EOF
fi
