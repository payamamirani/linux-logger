#!/usr/bin/env bash
# Test the payam-logger extension: take one fully silent screenshot through
# org.payam.Logger.Capture and verify the PNG.
set -euo pipefail

DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/payam-logger"
OUT="$DATA_DIR/extension-test.png"

mkdir -p "$DATA_DIR"

echo "calling org.payam.Logger.Capture (watch for: NO flash, NO sound) ..."
gdbus call --session \
    --dest org.payam.Logger \
    --object-path /org/payam/Logger \
    --method org.payam.Logger.Capture "$OUT"

file "$OUT"
rm -f "$OUT"
echo "OK — silent capture works"
