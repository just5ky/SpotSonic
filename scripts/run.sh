#!/usr/bin/env bash
# SpotSonic weekly run script.
# Copy to the project root, fill in the variables below, then set up a cron job.
# See README.md for scheduling instructions.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# ── Configuration ────────────────────────────────────────────────────────────
NAVIDROME_SERVER="${NAVIDROME_SERVER:-http://localhost:4533}"
NAVIDROME_USER="${NAVIDROME_USER:-admin}"
NAVIDROME_PASSWORD="${NAVIDROME_PASSWORD:-}"       # set via env or edit here
INPUT_DIR="${INPUT_DIR:-$PROJECT_DIR/spotify_playlists}"
STATE_FILE="${STATE_FILE:-$PROJECT_DIR/spotsonic-state.json}"
REPORT_FILE="${REPORT_FILE:-$PROJECT_DIR/unmatched.csv}"
THRESHOLD="${THRESHOLD:-0.80}"
LOG_FILE="${LOG_FILE:-$PROJECT_DIR/spotsonic.log}"
# ─────────────────────────────────────────────────────────────────────────────

if [[ -z "$NAVIDROME_PASSWORD" ]]; then
  echo "Error: NAVIDROME_PASSWORD is not set" >&2
  exit 1
fi

BINARY="$PROJECT_DIR/spotsonic"
if [[ ! -x "$BINARY" ]]; then
  echo "Binary not found at $BINARY — building..." >&2
  (cd "$PROJECT_DIR" && go build -o spotsonic .)
fi

echo "=== SpotSonic run: $(date -Iseconds) ===" | tee -a "$LOG_FILE"

"$BINARY" \
  -server   "$NAVIDROME_SERVER" \
  -user     "$NAVIDROME_USER" \
  -password "$NAVIDROME_PASSWORD" \
  -input    "$INPUT_DIR" \
  -state    "$STATE_FILE" \
  -report   "$REPORT_FILE" \
  -threshold "$THRESHOLD" \
  2>&1 | tee -a "$LOG_FILE"

echo "=== Done: $(date -Iseconds) ===" | tee -a "$LOG_FILE"
