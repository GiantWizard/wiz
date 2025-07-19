#!/bin/bash
# file: session-keeper.sh

set -o pipefail

echo "[SESSION-KEEPER] Starting MEGA session keeper."

if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

# --- CRITICAL ADDITION: CLEANUP ---
# Ensure a clean slate on every start/restart.
# mega-quit is the graceful way to stop the server if it's running.
# The pkill and rm are a failsafe for corrupted states.
echo "[SESSION-KEEPER] Performing initial cleanup..."
mega-quit &> /dev/null
sleep 2 # Give it a moment to shut down
pkill mega-cmd-server &> /dev/null
rm -f /home/appuser/.megaCmd/megacmd.lock /home/appuser/.megaCmd/srv_state.db &> /dev/null
echo "[SESSION-KEEPER] Cleanup complete."
# --- END CLEANUP ---

while true; do
  # The rest of your script is already excellent, but we will make one change.
  # We force a login attempt on the first run instead of checking first.
  echo "[SESSION-KEEPER] Establishing MEGA session..."

  if mega-login "$MEGA_EMAIL" "$MEGA_PWD"; then
    echo "[SESSION-KEEPER] Login successful. Session is active."
    # Now that we are logged in, sleep for an hour before the next check.
    echo "[SESSION-KEEPER] Checking again in 1 hour."
    sleep 3600
  else
    # If login fails, log it and retry sooner.
    echo "[SESSION-KEEPER] ERROR: Login FAILED. Retrying in 5 minutes."
    sleep 300
    continue # Retry the loop immediately
  fi
done