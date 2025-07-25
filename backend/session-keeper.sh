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
  if ! mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null; then
    echo "[SESSION-KEEPER] ERROR: mega-login command failed to start. Retrying in 5 minutes."
    sleep 300
    continue # Restart the while loop
  fi

  # --- STEP 2: VERIFY READINESS ---
  echo "[SESSION-KEEPER] Login command sent. Now verifying that the server is fully operational..."
  
  server_ready=false
  # Try for up to 2 minutes (12 attempts * 10 seconds)
  for i in {1..12}; do
    # We run 'mega-ls' on the root. When it stops returning the banner, the server is ready.
    ls_output=$(mega-ls -q / 2>&1)

    # The most reliable check is to see if the output contains the welcome banner.
    if ! echo "$ls_output" | grep -q 'Welcome to MEGAcmd'; then
      echo "[SESSION-KEEPER] SUCCESS: Server is responsive and ready."
      server_ready=true
      break # Exit the 'for' loop
    fi
    echo "[SESSION-KEEPER] Server not ready yet (check $i/12). Retrying in 10 seconds..."
    sleep 10
  done

  # --- STEP 3: SIGNAL AND WAIT ---
  if [ "$server_ready" = true ]; then
    echo "[SESSION-KEEPER] Creating ready file at $READY_FILE to signal other services."
    touch "$READY_FILE"
    echo "[SESSION-KEEPER] Initialization complete. Session is active. Idling for 1 hour."
    sleep 3600
  else
    echo "[SESSION-KEEPER] ERROR: Server did not become ready after 2 minutes. Restarting the entire process."
    sleep 60 # Wait a minute before a full restart to avoid hammering the service.
  fi

done
