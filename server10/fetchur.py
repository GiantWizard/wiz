# api_on_koyeb.py - Your new script to deploy on Koyeb
import requests
import time
import os
import sys
import threading
from flask import Flask, jsonify

# --- Configuration ---
API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
TRACKING_INTERVAL_SECONDS = 30  # Increased interval slightly for a web service
STABILITY_THRESHOLD = 1.0
REPORT_INTERVAL_CYCLES = 4 # Report every 2 minutes (4 * 30s)

# --- Shared Data (Thread-Safe) ---
# This dictionary will be shared between the analysis thread and the web server thread.
# The lock ensures we don't try to read the data while it's being written.
stable_item_data = {
    "last_updated_utc": None,
    "stable_items": []
}
data_lock = threading.Lock()

# --- Background Volatility Analysis Logic ---

def run_volatility_analysis():
    """This function runs in a background thread, continuously analyzing the bazaar."""
    global stable_item_data
    
    item_states = {}
    item_analytics = {}
    cycle_count = 0
    
    print("🚀 Background analysis thread started.")

    while True:
        cycle_count += 1
        print(f"BACKGROUND: Analyzing cycle {cycle_count}...")
        
        try:
            response = requests.get(API_URL, timeout=10)
            response.raise_for_status()
            data = response.json()
            current_products = data.get("products", {}) if data.get("success") else {}
        except requests.exceptions.RequestException as e:
            print(f"BACKGROUND ERROR: Could not fetch data: {e}")
            time.sleep(TRACKING_INTERVAL_SECONDS)
            continue
            
        # --- (This is the same logic from your original volatility scanner) ---
        for product_id, product_data in current_products.items():
            status = product_data.get("quick_status", {})
            current_buy_orders = status.get('buyOrders', 0)
            current_sell_orders = status.get('sellOrders', 0)

            if product_id in item_states:
                previous_state = item_states[product_id]
                total_change = abs(current_buy_orders - previous_state["buy_orders"]) + abs(current_sell_orders - previous_state["sell_orders"])
                
                # Update analytics
                if product_id not in item_analytics:
                    item_analytics[product_id] = {"total_change": 0, "samples": 0}
                analytics = item_analytics[product_id]
                analytics["total_change"] += total_change
                analytics["samples"] += 1
                avg_change_per_interval = analytics["total_change"] / analytics["samples"]
                intervals_per_minute = 60 / TRACKING_INTERVAL_SECONDS
                analytics["avg_per_min"] = avg_change_per_interval * intervals_per_minute

            item_states[product_id] = {"buy_orders": current_buy_orders, "sell_orders": current_sell_orders}
        # --- (End of reused logic) ---

        # Every few cycles, update the shared data with the latest list of stable items
        if cycle_count % REPORT_INTERVAL_CYCLES == 0:
            print("BACKGROUND: Generating and storing new stability report...")
            
            latest_stable_items = [
                pid for pid, analytics in item_analytics.items()
                if "avg_per_min" in analytics and analytics["avg_per_min"] < STABILITY_THRESHOLD
            ]
            
            # Use the lock to safely update the shared data
            with data_lock:
                stable_item_data["stable_items"] = latest_stable_items
                stable_item_data["last_updated_utc"] = time.strftime('%Y-%m-%d %H:%M:%S')
            
            print(f"BACKGROUND: Stored {len(latest_stable_items)} stable items.")

        time.sleep(TRACKING_INTERVAL_SECONDS)


# --- Flask API Setup ---
app = Flask(__name__)

@app.route('/')
def get_stable_items():
    """The main API endpoint that returns the list of stable items."""
    with data_lock:
        # Return a copy of the data
        return jsonify(stable_item_data)

@app.route('/health')
def health_check():
    """
    A simple health check endpoint for Koyeb to monitor.
    If this returns a 200 OK, Koyeb knows the service is running.
    """
    return "OK", 200


if __name__ == "__main__":
    # Start the background thread for bazaar analysis
    analysis_thread = threading.Thread(target=run_volatility_analysis, daemon=True)
    analysis_thread.start()

    # Start the Flask web server
    # Koyeb provides the PORT environment variable.
    port = int(os.environ.get("PORT", 8080))
    app.run(host='0.0.0.0', port=port)