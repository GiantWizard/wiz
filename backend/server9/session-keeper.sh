#!/bin/bash
# file: session-keeper.sh

set -o pipefail

echo "[SESSION-KEEPER] Starting MEGA session keeper."

# This script runs as 'appuser', so HOME is automatically set by supervisor's environment.
# Check for credentials passed from supervisor
if [ -z "$MEGA_EMAIL" ] || [ -z "$MEGA_PWD" ]; then
  echo "[SESSION-KEEPER] FATAL: MEGA_EMAIL and/or MEGA_PWD environment variables are not set."
  exit 1
fi

while true; do
  echo "[SESSION-KEEPER] Checking MEGA session status..."

  # 'mega-whoami -l' is a reliable command that exits with a non-zero status if not logged in.
  # This check works for both the initial startup and subsequent renewals.
  if mega-whoami -l > /dev/null 2>&1; then
    echo "[SESSION-KEEPER] Session is active. Checking again in 1 hour."
  else
    echo "[SESSION-KEEPER] Session is down, expired, or not yet started. Attempting to log in..."

    # This command will perform the initial login and all subsequent re-logins.
    if mega-login "$MEGA_EMAIL" "$MEGA_PWD"; then
      echo "[SESSION-KEEPER] Login/Re-login successful."
    else
      # If login fails, log it and retry sooner to unblock the other apps.
      echo "[SESSION-KEEPER] ERROR: Login FAILED. Retrying in 5 minutes."
      sleep 300
      continue # Skip the long sleep and retry the loop immediately
    fi
  fi

  # Sleep for 1 hour (3600 seconds) before the next check.
  sleep 3600
done