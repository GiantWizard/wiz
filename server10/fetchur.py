# all_in_one_flipper.py - Deploy this single file to Koyeb

import requests
import time
import os
import threading
from flask import Flask, Response

# --- Main Configuration ---
HYPIXEL_API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
STABILITY_THRESHOLD = 3  # Avg changes per minute < 1.0
PROFIT_MARGIN_THRESHOLD = 0.15 # 15% minimum profit
MAX_ACTIVE_ORDERS = 750

# --- Background Analysis Configuration ---
TRACKING_INTERVAL_SECONDS = 30
REPORT_INTERVAL_CYCLES = 4 # Update stable list every 2 minutes

# --- Shared Data (Thread-Safe) ---
# This dictionary is shared between the background thread and the web server.
# The lock prevents data corruption.
shared_data = {
    "last_updated_utc": None,
    "stable_items": [],
    "is_analyzing": True
}
data_lock = threading.Lock()

# ==============================================================================
# 1. BACKGROUND THREAD: Continuously finds stable items
# ==============================================================================
def run_volatility_analysis():
    """This function runs in the background 24/7 to find stable items."""
    global shared_data
    item_states, item_analytics, cycle_count = {}, {}, 0
    print("🚀 Background volatility analysis thread started.")

    while True:
        cycle_count += 1
        try:
            response = requests.get(HYPIXEL_API_URL, timeout=10)
            current_products = response.json().get("products", {})
        except Exception as e:
            print(f"BACKGROUND ERROR: Could not fetch data: {e}")
            time.sleep(TRACKING_INTERVAL_SECONDS)
            continue

        for pid, data in current_products.items():
            status = data.get("quick_status", {})
            orders = (status.get('buyOrders', 0), status.get('sellOrders', 0))
            if pid in item_states:
                change = abs(orders[0] - item_states[pid][0]) + abs(orders[1] - item_states[pid][1])
                if pid not in item_analytics: item_analytics[pid] = {"total_change": 0, "samples": 0}
                analytics = item_analytics[pid]
                analytics["total_change"] += change
                analytics["samples"] += 1
                analytics["avg_per_min"] = (analytics["total_change"] / analytics["samples"]) * (60 / TRACKING_INTERVAL_SECONDS)
            item_states[pid] = orders
            
        if cycle_count % REPORT_INTERVAL_CYCLES == 0:
            latest_stable = [pid for pid, a in item_analytics.items() if a.get("avg_per_min", 999) < STABILITY_THRESHOLD]
            with data_lock:
                shared_data["stable_items"] = latest_stable
                shared_data["last_updated_utc"] = time.strftime('%Y-%m-%d %H:%M:%S')
                shared_data["is_analyzing"] = False # Mark initial analysis as complete
            print(f"BACKGROUND: Stability report generated. Found {len(latest_stable)} stable items.")
        time.sleep(TRACKING_INTERVAL_SECONDS)

# ==============================================================================
# 2. ON-DEMAND ANALYSIS: Runs only when the /flipper URL is visited
# ==============================================================================
def analyze_for_profit(stable_item_ids):
    """Fetches Hypixel data and analyzes the provided stable items for profit."""
    if not stable_item_ids:
        return "No stable items available to analyze yet."

    try:
        print("FLIPPER: Fetching Hypixel data for profit analysis...")
        response = requests.get(HYPIXEL_API_URL, timeout=10)
        all_products = response.json().get("products", {})
        print("FLIPPER: Data received. Analyzing for profit...")
    except Exception as e:
        return f"Error fetching data from Hypixel API: {e}"

    results = []
    for pid in stable_item_ids:
        data = all_products.get(pid)
        if not data: continue
        
        buy_summary = data.get("buy_summary", [])
        sell_summary = data.get("sell_summary", [])
        status = data.get("quick_status", {})
        if not buy_summary or not sell_summary or not status: continue

        total_orders = status.get('buyOrders', 0) + status.get('sellOrders', 0)
        if total_orders > MAX_ACTIVE_ORDERS: continue

        top_buy = buy_summary[0]['pricePerUnit']
        bot_sell = sell_summary[0]['pricePerUnit']
        if bot_sell <= 0: continue
        
        margin = (top_buy - bot_sell) / bot_sell
        if margin < PROFIT_MARGIN_THRESHOLD: continue

        profit_per_item = top_buy - bot_sell
        volume = min(status.get('buyMovingWeek', 0), status.get('sellMovingWeek', 0))
        profit_per_hour = (profit_per_item * volume) / 168 # 168 hours in a week
        results.append({
            'name': pid.replace("_", " ").title(),
            'profit_hr': f"{profit_per_hour:,.0f}",
            'margin': f"{margin:.1%}",
            'orders': f"{total_orders:,}",
            'spread': f"{profit_per_item:,.2f}"
        })

    if not results:
        return "Analysis complete. No stable items were found that also met your profit criteria."

    results.sort(key=lambda x: float(x['profit_hr'].replace(",", "")), reverse=True)
    
    # Format results into a clean text table
    headers = ["Item Name", "Profit/Hour", "Margin %", "Active Orders", "Profit Spread"]
    col_widths = [len(h) for h in headers]
    for row in results:
        col_widths = [max(col_widths[i], len(row[k])) for i, k in enumerate(['name', 'profit_hr', 'margin', 'orders', 'spread'])]

    header_line = " | ".join(h.ljust(w) for h, w in zip(headers, col_widths))
    divider = "-+-".join("-" * w for w in col_widths)
    
    data_lines = []
    for row in results[:50]: # Show top 50
        line = row['name'].ljust(col_widths[0])
        line += " | " + row['profit_hr'].rjust(col_widths[1])
        line += " | " + row['margin'].rjust(col_widths[2])
        line += " | " + row['orders'].rjust(col_widths[3])
        line += " | " + row['spread'].rjust(col_widths[4])
        data_lines.append(line)
        
    return f"--- Top Profitable & Stable Bazaar Flips ---\n\n{header_line}\n{divider}\n" + "\n".join(data_lines)

# ==============================================================================
# 3. FLASK WEB SERVER: Serves the results to your browser
# ==============================================================================
app = Flask(__name__)

@app.route('/')
def home():
    """A simple homepage."""
    return '<h1>Volatility and Flipper API</h1><p>Visit <a href="/flipper">/flipper</a> to get the latest profit analysis.</p>'

@app.route('/flipper')
def get_flipper_results():
    """The main endpoint to trigger the analysis and show results."""
    with data_lock:
        is_analyzing = shared_data["is_analyzing"]
        stable_items = shared_data["stable_items"]

    if is_analyzing:
        return Response("The server has just started. Please wait 2-3 minutes for the first stability analysis to complete, then refresh this page.", mimetype='text/plain')

    # Run the analysis and get the formatted string
    result_string = analyze_for_profit(stable_items)
    
    # Return the string as pre-formatted text, which looks great in a browser
    return Response(f"<pre>{result_string}</pre>")

@app.route('/health')
def health_check():
    """Health check for Koyeb to confirm the service is running."""
    return "OK", 200

if __name__ == "__main__":
    # Start the background thread for bazaar analysis
    analysis_thread = threading.Thread(target=run_volatility_analysis, daemon=True)
    analysis_thread.start()

    # Start the Flask web server
    port = int(os.environ.get("PORT", 8080))
    app.run(host='0.0.0.0', port=port)