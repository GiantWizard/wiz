#!/bin/bash

# This script first launches the web server in the background.
# Then, it enters an infinite loop to regenerate data every 20 seconds.
# This ensures the website is always available, even during data generation.

# --- Clean up background processes when the script exits ---
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

# --- Define filenames ---
TEMP_FILE="fusion_recipes_temp.json"
FINAL_FILE="fusion_recipes_with_prices.json"

# --- Launch the Web Server (shard4.py) in the background ---
echo "----------------------------------------------------"
echo "STEP 1: Starting web server (shard4.py) in the background..."
echo "Navigate to http://127.0.0.1:5000"
echo "The server will be available shortly. Press Ctrl+C in this terminal to stop everything."
echo "----------------------------------------------------"
python3 shard4.py &
# Give the server a moment to start
sleep 2

# --- Start the data generation loop ---
while true
do
    echo "----------------------------------------------------"
    echo "LOOP: Running data generator (shard3.py)..."
    
    # Run the generator. It will create fusion_recipes_temp.json
    python3 shard3.py

    # Check if the temporary file was created successfully and is a valid JSON
    if jq '.' "$TEMP_FILE" > /dev/null 2>&1; then
        echo "LOOP: Data generation successful. Moving temp file to final destination."
        # Atomically replace the old data file with the new one.
        # This prevents the web server from reading a half-written file.
        mv "$TEMP_FILE" "$FINAL_FILE"
    else
        echo "LOOP ERROR: Data generation failed or produced an invalid JSON. Old data will be kept."
    fi

    echo "LOOP: Data updated. Next update in 20 seconds."
    echo "----------------------------------------------------"
    sleep 20
done