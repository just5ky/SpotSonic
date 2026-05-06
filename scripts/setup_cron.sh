#!/usr/bin/env bash
# Installs a weekly WSL cron job that runs SpotSonic every Monday at 09:00.
# Run once: bash scripts/setup_cron.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_SCRIPT="$SCRIPT_DIR/run.sh"
chmod +x "$RUN_SCRIPT"

# Cron expression: minute hour day-of-month month day-of-week
CRON_EXPR="0 9 * * 1"

CRON_LINE="$CRON_EXPR NAVIDROME_PASSWORD='YOUR_PASSWORD_HERE' $RUN_SCRIPT"

# Check if already installed
if crontab -l 2>/dev/null | grep -qF "$RUN_SCRIPT"; then
  echo "Cron job already installed."
  crontab -l | grep "$RUN_SCRIPT"
  exit 0
fi

# Append to existing crontab
(crontab -l 2>/dev/null; echo "$CRON_LINE") | crontab -

echo "Cron job installed:"
echo "  $CRON_LINE"
echo ""
echo "IMPORTANT: edit the crontab to set your real password:"
echo "  crontab -e"
echo ""
echo "To verify: crontab -l"
echo "To remove: crontab -e  (delete the SpotSonic line)"
