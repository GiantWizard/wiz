#!/usr/bin/env bash

# === Configuration ===
BACKEND_DIR="bazaar-backend"
FRONTEND_DIR="bazaar-frontend"

# === Helpers ===
log() { echo "[$(date +'%Y-%m-%d %H:%M:%S')] $1"; }
die() { echo "ERROR: $1"; exit 1; }

set -e

# === Cleanup ===
cleanup() {
    log "Cleaning up…"
    if [ -n "$FRONTEND_PID" ] && ps -p $FRONTEND_PID &>/dev/null; then
        log " ➔ Stopping frontend (PID $FRONTEND_PID)"
        kill $FRONTEND_PID || true
    fi
    if [ -n "$BACKEND_PID" ] && ps -p $BACKEND_PID &>/dev/null; then
        log " ➔ Stopping backend (PID $BACKEND_PID)"
        if command -v setsid &>/dev/null; then
            kill -- -$BACKEND_PID 2>/dev/null || kill $BACKEND_PID || true
        else
            kill $BACKEND_PID || true
        fi
    fi
    log "Done."
}
trap cleanup EXIT SIGINT SIGTERM

# --- Start Backend ---
log "👉 Starting Go backend…"
pushd "$BACKEND_DIR" >/dev/null

if command -v setsid &>/dev/null; then
    setsid go run . &
else
    go run . &
fi
BACKEND_PID=$!
popd >/dev/null
log "   Backend PID: $BACKEND_PID"

# --- Prepare & Start Frontend ---
pushd "$FRONTEND_DIR" >/dev/null

# Only install if package.json exists and node_modules is missing
if [ -f package.json ] && [ ! -d node_modules ]; then
    log "➔ Installing frontend dependencies…"
    npm install
fi

log "👉 Starting Svelte frontend…"
npm run dev &
FRONTEND_PID=$!
popd >/dev/null
log "   Frontend PID: $FRONTEND_PID"

# --- Wait for Both ---
wait $BACKEND_PID $FRONTEND_PID
