#!/usr/bin/env bash

# === Configuration ===
BACKEND_DIR="bazaar-backend"
FRONTEND_DIR="bazaar-frontend"

# === Script Logic ===

# Function to print messages
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $1"
}

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Start Backend ---
log "Navigating to backend directory: '$BACKEND_DIR'"
cd "$BACKEND_DIR" || { echo "ERROR: Could not cd into '$BACKEND_DIR'"; exit 1; }

log "Starting Go backend server in the background..."
# Start the Go process in its own process group (using setsid if available, or just &)
# Using setsid ensures kill -- targets the whole group. Fallback to just & if setsid isn't common on macOS.
# UPDATE: 'kill -- -PID' on macOS should target the process group created by '&' itself.
go run . &
BACKEND_PID=$! # Get the Process ID (PID) of the background Go process
log "Go backend started with PID: $BACKEND_PID"

cd .. # Go back to the script's directory
log "Navigated back to parent directory."


# --- Cleanup Function ---
# This function will be called when the script exits or receives signals
cleanup() {
    log "Received exit signal. Cleaning up..."
    log "Attempting to stop Go backend (PID: $BACKEND_PID)..."

    # Try killing the process group associated with the PID
    # The '--' prevents kill from interpreting a negative PID as a signal number
    # Adding 'pgid' option to ps to verify the process group later if needed
    if ps -p $BACKEND_PID -o pgid= | grep -q '[0-9]'; then # Check if process exists
        kill -- -$BACKEND_PID 2>/dev/null
        EXIT_STATUS=$?
        if [ $EXIT_STATUS -eq 0 ]; then
            log "Sent kill signal to process group $BACKEND_PID."
            # Optional: wait a moment for cleanup
            sleep 1
        else
            # If killing group failed, try killing just the PID directly
            log "Killing process group failed (status $EXIT_STATUS). Trying PID directly..."
            kill $BACKEND_PID 2>/dev/null || log "Process $BACKEND_PID might have already stopped."
        fi
    else
      log "Go process $BACKEND_PID not found."
    fi
    log "Cleanup finished."
}

# --- Trap Signals ---
# Execute the 'cleanup' function when the script receives:
# EXIT: Normal script termination
# SIGINT: Interrupt signal (usually Ctrl+C)
# SIGTERM: Termination signal
trap cleanup EXIT SIGINT SIGTERM


# --- Start Frontend ---
log "Navigating to frontend directory: '$FRONTEND_DIR'"
cd "$FRONTEND_DIR" || { echo "ERROR: Could not cd into '$FRONTEND_DIR'"; exit 1; }

log "Starting SvelteKit frontend dev server (foreground)..."
# Run the Svelte dev server in the foreground.
# This command will block until it's terminated (e.g., by Ctrl+C).
npm run dev

# --- Script End ---
# When 'npm run dev' finishes (e.g., user presses Ctrl+C), the script reaches its end.
# The 'trap cleanup EXIT' ensures the cleanup function runs automatically here too.
log "Frontend process finished. Script exiting."