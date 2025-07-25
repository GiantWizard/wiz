#!/bin/bash
# file: session-keeper.sh

set -o pipefail

READY_FILE="/tmp/mega.ready"

echo "[SESSION-KEEPER] Starting MEGA session keeper."

if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

while true; do
  echo "[SESSION-KEEPER] Performing cleanup..."
  rm -f "$READY_FILE"
  mega-quit &> /dev/null
  sleep 2
  pkill mega-cmd-server &> /dev/null
  rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db &> /dev/null
  echo "[SESSION-KEEPER] Cleanup complete."

  echo "[SESSION-KEEPER] Establishing MEGA session..."
  if mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
    echo "[SESSION-KEEPER] SUCCESS: Logged in. Creating ready file at $READY_FILE."
    touch "$READY_FILE"
    echo "[SESSION-KEEPER] Initialization complete. Session is active. Idling for 1 hour."
    sleep 3600
  else
    echo "[SESSION-KEEPER] ERROR: mega-login failed. Retrying in 5 minutes."
    sleep 300
  fi
done
