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
rm -f "$READY_FILE"
mega-quit &> /dev/null
sleep 2
pkill mega-cmd-server &> /dev/null
rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db &> /dev/null
echo "[SESSION-KEEPER] Cleanup complete."
# --- END CLEANUP ---

while true; do
  echo "[SESSION-KEEPER] Establishing MEGA session..."

  # --- THIS IS THE CRITICAL FIX ---
  # Redirect stdin from /dev/null to force non-interactive mode and prevent hangs.
  if megalogin "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
    echo "[SESSION-KEEPER] Login command exited successfully."
    echo "[SESSION-KEEPER] Waiting 5 seconds for session server to stabilize..."
    sleep 5
    
    echo "[SESSION-KEEPER] Creating ready file at $READY_FILE to signal other services."
    touch "$READY_FILE"
    
    echo "[SESSION-KEEPER] Initialization complete. Session is active. Will check again in 1 hour."
    sleep 3600
    
    # Clean up for the next loop iteration
    rm -f "$READY_FILE"
  else
    # Capture the exit code for better error reporting
    exit_code=$?
    echo "[SESSION-KEEPER] ERROR: Login FAILED with exit code $exit_code. Retrying in 5 minutes."
    sleep 300
    continue
  fi
done