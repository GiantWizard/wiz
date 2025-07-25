#!/bin/bash
# file: session-keeper.sh

set -o pipefail

READY_FILE="/tmp/mega.ready"

echo "[SESSION-KEEPER] Starting MEGA session keeper."

if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

# This loop will run forever, ensuring the session stays active.
while true; do
  # --- CLEANUP on every start of the loop ---
  echo "[SESSION-KEEPER] Performing cleanup..."
  rm -f "$READY_FILE"
  mega-quit &> /dev/null
  sleep 2
  pkill mega-cmd-server &> /dev/null
  rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db &> /dev/null
  echo "[SESSION-KEEPER] Cleanup complete."

  # --- STEP 1: LOGIN ---
  echo "[SESSION-KEEPER] Establishing MEGA session..."
  if mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
    echo "[SESSION-KEEPER] SUCCESS: Logged in. Waiting for MEGAcmd to become responsive…"

    # --- STEP 2: WAIT FOR SERVER READINESS ---
    for i in {1..6}; do
      if mega-cmd --non-interactive ls / &> /dev/null; then
        echo "[SESSION-KEEPER] MEGAcmd is ready (checked after $((i*5))s)."
        break
      fi
      echo "[SESSION-KEEPER] Server not ready yet (attempt $i/6). Sleeping 5s…"
      sleep 5
    done

    # --- STEP 3: SIGNAL AND WAIT ---
    echo "[SESSION-KEEPER] Creating ready file at $READY_FILE."
    touch "$READY_FILE"
    echo "[SESSION-KEEPER] Initialization complete. Session is active. Idling for 1 hour."
    sleep 3600
  else
    echo "[SESSION-KEEPER] ERROR: mega-login failed. Retrying in 5 minutes."
    sleep 300
  fi

done
