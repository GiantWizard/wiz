#!/bin/bash
# file: session-keeper.sh

set -o pipefail

READY_FILE="/tmp/mega.ready"

echo "[SESSION-KEEPER] Starting MEGA session keeper."

# Ensure credentials are set
if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

# Infinite loop to keep session alive
while true; do
  # --- CLEANUP ---
  echo "[SESSION-KEEPER] Cleaning up old session…"
  rm -f "$READY_FILE"
  mega-quit &> /dev/null
  pkill mega-cmd-server &> /dev/null
  rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db
  sleep 2

  # --- LOGIN ---
  echo "[SESSION-KEEPER] Logging in…"
  if mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
    echo "[SESSION-KEEPER] SUCCESS: Logged in."

    # --- LAUNCH DAEMON ---
    echo "[SESSION-KEEPER] Starting MEGAcmd server in background…"
    mega-cmd-server &> /home/appuser/.megaCmd/megacmdserver.log &
    
    # Wait briefly for server to finish starting
    echo "[SESSION-KEEPER] Waiting 10s for server startup…"
    sleep 10

    # --- SIGNAL READY ---
    echo "[SESSION-KEEPER] Creating ready file at $READY_FILE."
    touch "$READY_FILE"
    echo "[SESSION-KEEPER] Session active; idling 1 hour before next refresh."
    sleep 3600
  else
    echo "[SESSION-KEEPER] ERROR: Login failed; retrying in 5 minutes."
    sleep 300
  fi
done
