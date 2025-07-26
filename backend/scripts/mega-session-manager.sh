#!/bin/bash
# MODIFIED: Use bash and remove pipefail for Debian compatibility.
set -e

# Check that credentials from the environment are set
if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-MANAGER] FATAL: MEGA_EMAIL and/or MEGA_PWD are not set."
  exit 1
fi

# Clean up old state from any previous unclean shutdown
echo "[SESSION-MANAGER] Cleaning up previous session state..."
mega-quit &> /dev/null || true
pkill -f mega-cmd-server &> /dev/null || true
rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db*
sleep 2

# Log in to MEGA. This command also starts the server in the background.
echo "[SESSION-MANAGER] Attempting to log in as $MEGA_EMAIL..."
if ! mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
  echo "[SESSION-MANAGER] FATAL: MEGA login failed."
  exit 1
fi

# The script's only remaining job is to stay alive.
echo "[SESSION-MANAGER] Login successful. Session is now active in the background."
echo "[SESSION-MANAGER] This script will now idle to keep the container running."
tail -f /dev/null