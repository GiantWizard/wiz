#!/bin/sh
set -e # Exit immediately if a command exits with a non-zero status.

# Start the TCP health listener in the background
./health_listener.sh &
LISTENER_PID=$!
echo "Health listener PID: $LISTENER_PID"

# Start the main application
echo "Starting timestamp_generator..."
./timestamp_generator

# If timestamp_generator exits, kill the listener
# (This part is tricky with simple shell scripts if timestamp_generator daemonizes or forks)
echo "timestamp_generator exited. Killing listener."
kill $LISTENER_PID
wait $LISTENER_PID 2>/dev/null # Wait for listener to exit
exit $? # Exit with the status of timestamp_generator