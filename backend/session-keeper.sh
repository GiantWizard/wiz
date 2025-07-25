#!/bin/bash
# file: session-keeper.sh

set -o pipefail
READY_FILE="/tmp/mega.ready"

echo "[SESSION-KEEPER] Starting MEGA session keeper."
[ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ] && { 
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL or MEGA_PWD missing."; exit 1; 
}

while true; do
  echo "[SESSION-KEEPER] Cleaning up old session…"
  rm -f "$READY_FILE"
  mega-quit &> /dev/null
  pkill mega-cmd-server &> /dev/null
  rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db
  sleep 2

  echo "[SESSION-KEEPER] Logging in…"
  if mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
    echo "[SESSION-KEEPER] SUCCESS: Logged in. Waiting 10s for server startup…"
    sleep 10
    echo "[SESSION-KEEPER] Creating ready file at $READY_FILE."
    touch "$READY_FILE"
    echo "[SESSION-KEEPER] Session active; sleeping 1h before refresh."
    sleep 3600
  else
    echo "[SESSION-KEEPER] ERROR: Login failed; retrying in 5m."
    sleep 300
  fi
done
