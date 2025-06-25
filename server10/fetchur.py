import requests
import time
import os
import sys
from typing import Dict, Any

# --- Configuration ---
# Use environment variables for configuration, with sensible defaults.
API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
TRACKING_INTERVAL_SECONDS = int(os.getenv("TRACKING_INTERVAL_SECONDS", 20))
STABILITY_THRESHOLD = float(os.getenv("STABILITY_THRESHOLD", 1.0))
REPORT_INTERVAL_CYCLES = int(os.getenv("REPORT_INTERVAL_CYCLES", 6))

# --- Data Storage (in-memory) ---
item_states: Dict[str, Dict[str, int]] = {}
item_analytics: Dict[str, Dict[str, Any]] = {}

def get_bazaar_data() -> Dict[str, Any]:
    """Fetches bazaar data and handles potential API errors."""
    try:
        response = requests.get(API_URL, timeout=10)
        response.raise_for_status()
        data = response.json()
        if data.get("success"):
            return data.get("products", {})
        else:
            # Use print for logging in a server environment
            print(f"[-] API Error: Request not successful. Reason: {data.get('cause', 'Unknown')}")
            return {}
    except requests.exceptions.RequestException as e:
        print(f"[-] Network Error: Could not connect to the API. {e}")
        return {}

def clean_item_name(product_id: str) -> str:
    """Makes API product IDs more readable."""
    return product_id.replace("_", " ").title()

def update_analytics(product_id: str, change: int):
    """Updates the running average of order changes for an item."""
    if product_id not in item_analytics:
        item_analytics[product_id] = {"total_change": 0, "samples": 0}

    analytics = item_analytics[product_id]
    analytics["total_change"] += change
    analytics["samples"] += 1

    avg_change_per_interval = analytics["total_change"] / analytics["samples"]
    intervals_per_minute = 60 / TRACKING_INTERVAL_SECONDS
    analytics["avg_per_min"] = avg_change_per_interval * intervals_per_minute

def print_stability_report():
    """Filters and prints the list of the most stable items found so far."""
    # In a server environment, don't clear the screen. Just print the report.
    print("\n--- Bazaar Volatility Report ---")
    print(f"Showing items with avg. order changes < {STABILITY_THRESHOLD:.1f}/min")
    print(f"Last update: {time.strftime('%Y-%m-%d %H:%M:%S UTC')}\n")

    stable_items = [
        {"id": product_id, "name": clean_item_name(product_id), **analytics}
        for product_id, analytics in item_analytics.items()
        if analytics.get("avg_per_min", float('inf')) < STABILITY_THRESHOLD
    ]

    if not stable_items:
        print("No stable items found matching the criteria yet. Waiting for more data...")
        return

    stable_items.sort(key=lambda x: x['avg_per_min'])

    print(f"{'Item Name':<30} | {'Avg. Changes/Min':<20} | {'Samples'}")
    print("-" * 60)
    for item in stable_items:
        print(f"{item['name']:<30} | {item['avg_per_min']:<20.2f} | {item['samples']}")
    print("-" * 60)


def main_loop():
    """The main execution loop for the scanner."""
    cycle_count = 0
    print("🚀 Starting Bazaar Volatility Scanner...")
    print(f"Checking for order changes every {TRACKING_INTERVAL_SECONDS} seconds.")
    print(f"A full stability report will be generated every {REPORT_INTERVAL_CYCLES * TRACKING_INTERVAL_SECONDS} seconds.")
    # Koyeb handles stopping the script, so we don't need the Ctrl+C message.

    while True:
        cycle_count += 1
        print(f"\n--- Cycle {cycle_count} ({time.strftime('%Y-%m-%d %H:%M:%S UTC')}) ---")

        current_products = get_bazaar_data()
        if not current_products:
            print("Could not fetch data, waiting for next cycle.")
            time.sleep(TRACKING_INTERVAL_SECONDS)
            continue

        if not item_states:
            print("First run: Initializing item states...")

        changes_detected = 0
        for product_id, product_data in current_products.items():
            status = product_data.get("quick_status", {})
            current_buy_orders = status.get('buyOrders', 0)
            current_sell_orders = status.get('sellOrders', 0)

            if product_id in item_states:
                previous_state = item_states[product_id]
                buy_delta = current_buy_orders - previous_state["buy_orders"]
                sell_delta = current_sell_orders - previous_state["sell_orders"]
                total_change = abs(buy_delta) + abs(sell_delta)
                
                update_analytics(product_id, total_change)

                if total_change > 0:
                    changes_detected += 1
                    name = clean_item_name(product_id)
                    buy_sign = '+' if buy_delta >= 0 else ''
                    sell_sign = '+' if sell_delta >= 0 else ''
                    print(f"  [CHANGE] {name}: Buy Orders: {buy_sign}{buy_delta}, Sell Orders: {sell_sign}{sell_delta}")

            item_states[product_id] = {
                "buy_orders": current_buy_orders,
                "sell_orders": current_sell_orders
            }
        
        # This ensures new items get an initial analytic sample
        for product_id in current_products:
             if product_id not in item_analytics:
                update_analytics(product_id, 0)


        if changes_detected == 0 and cycle_count > 1:
            print("  No significant order changes detected in this cycle.")

        if cycle_count % REPORT_INTERVAL_CYCLES == 0:
            print_stability_report()

        time.sleep(TRACKING_INTERVAL_SECONDS)

if __name__ == "__main__":
    # The try/except KeyboardInterrupt is not necessary on the server, but it's fine to leave it.
    main_loop()