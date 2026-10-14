#!/usr/bin/env bash
# Build and run the daemon in the foreground with debug logging (dev loop).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

make build
exec ./backend/bin/payam-loggerd -config config/payam-logger.example.json
