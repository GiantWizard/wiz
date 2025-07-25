#!/bin/bash
# file: session-keeper.sh

set -o pipefail

READY_FILE="/tmp/mega.ready"

echo "[SESSION-KEEPER] Starting MEGA session keeper."

if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

# --- CLEANUP on every start ---
echo "[SESSION-KEEPER] Performing initial cleanup..."
# Ensure the ready file does not exist on startup
rm -f "$READY_FILE"
mega-quit &> /dev/null
sleep 2
pkill mega-cmd-server &> /dev/null
rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db &> /dev/null
echo "[SESSION-KEEPER] Cleanup complete."
# --- END CLEANUP ---

while true; do
  echo "[SESSION-KEEPER] Establishing MEGA session..."

  if megalogin "$MEGA_EMAIL" "$MEGA_PWD"; then
    echo "[SESSION-KEEPER] Login command sent successfully."
    echo "[SESSION-KEEPER] Waiting 5 seconds for session server to stabilize..."
    sleep 5
    
    # --- THIS IS THE KEY ---
    # Create the ready file to signal that the session is active.
    echo "[SESSION-KEEPER] Creating ready file at $READY_FILE. Session is active."
    touch "$READY_FILE"
    
    echo "[SESSION-KEEPER] Initialization complete. Checking again in 1 hour."
    sleep 3600
    
    # In case of a loop, remove the ready file before trying again.
    rm -f "$READY_FILE"
  else
    echo "[SESSION-KEEPER] ERROR: Login FAILED. Retrying in 5 minutes."
    sleep 300
    continue
  fi
done