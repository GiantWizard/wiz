#!/bin/bash
# file: session-keeper.sh

set -o pipefail

echo "[SESSION-KEEPER] Starting MEGA session keeper."

if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

# --- CLEANUP on every start ---
echo "[SESSION-KEEPER] Performing initial cleanup..."
mega-quit &> /dev/null
sleep 2
pkill mega-cmd-server &> /dev/null
rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db &> /dev/null
echo "[SESSION-KEEPER] Cleanup complete."
# --- END CLEANUP ---

while true; do
  echo "[SESSION-KEEPER] Establishing MEGA session..."

  if megalogin "$MEGA_EMAIL" "$MEGA_PWD"; then
    echo "[SESSION-KEEPER] Login successful. Server process is starting."
    # --- CRITICAL ---
    # Add a short delay to allow the background mega-cmd-server to fully initialize
    # before other services attempt to connect to it.
    echo "[SESSION-KEEPER] Waiting 5 seconds for session server to stabilize..."
    sleep 5
    echo "[SESSION-KEEPER] Session should now be stable. Checking again in 1 hour."
    sleep 3600
  else
    echo "[SESSION-KEEPER] ERROR: Login FAILED. Retrying in 5 minutes."
    sleep 300
    continue
  fi
done