# profit_flipper.py - Your local script for finding profitable flips from stable items.

import requests
import concurrent.futures
import sys
from typing import List, Dict, Any, Optional, Set

# ==============================================================================
# --- CONFIGURATION ---
# ==============================================================================

# PASTE THE PUBLIC URL OF YOUR KOYEB API SERVICE HERE
STABILITY_API_URL = "https://political-constancy-giantwizard-e6c5747e.koyeb.app/"

# This is the Hypixel API for fetching bazaar data
BAZAAR_API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"

# How many of the best results you want to see in the final table
TOP_N_RESULTS = 50

# The minimum profit margin you're willing to accept (e.g., 0.15 = 15%)
PROFIT_MARGIN_THRESHOLD = 0.15

# A filter to ignore items that are extremely competitive. Set to None to disable.
MAX_ACTIVE_ORDERS = 750

# ==============================================================================
# --- API COMMUNICATION ---
# ==============================================================================

def fetch_stable_items_from_api() -> Set[str]:
    """
    Downloads the list of stable item IDs from our personal Koyeb API.
    This is the crucial first step.
    """
    if "YOUR-APP-NAME" in STABILITY_API_URL:
        print("!!! CONFIGURATION ERROR !!!")
        print("Please paste your actual Koyeb app URL into the STABILITY_API_URL variable.")
        return set()

    print(f"[*] Fetching list of stable items from your API: {STABILITY_API_URL}")
    try:
        response = requests.get(STABILITY_API_URL, timeout=10)
        response.raise_for_status() # Raises an error for bad responses like 404 or 500
        data = response.json()
        
        stable_ids = data.get("stable_items", [])
        last_updated = data.get("last_updated_utc", "N/A")
        
        if not stable_ids:
            print("⚠️ API returned an empty list of stable items.")
            return set()

        print(f"✅ Successfully fetched {len(stable_ids)} stable item IDs (API last updated: {last_updated} UTC).")
        return set(stable_ids) # Using a Set for very fast lookups later

    except requests.exceptions.RequestException as e:
        print(f"❌ Failed to fetch data from your API: {e}")
        return set()

def get_all_bazaar_data() -> Optional[Dict[str, Any]]:
    """Fetches the entire bazaar product list from the official Hypixel API."""
    print("[*] Fetching all bazaar data from Hypixel API...")
    try:
        response = requests.get(BAZAAR_API_URL, timeout=10)
        response.raise_for_status()
        bazaar_data = response.json()
        if bazaar_data.get("success"):
            print("✅ Hypixel data received.")
            return bazaar_data.get("products", {})
        else:
            print("❌ Hypixel API request was not successful.")
            return None
    except requests.exceptions.RequestException as e:
        print(f"❌ An error occurred connecting to the Hypixel API: {e}")
        return None

# ==============================================================================
# --- DATA ANALYSIS & PRESENTATION ---
# ==============================================================================

def analyze_bazaar_item(product_id: str, data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """
    Calculates profit metrics for a single item.
    This function is only called for items that are already confirmed to be stable.
    """
    buy_summary = data.get("buy_summary", [])
    sell_summary = data.get("sell_summary", [])
    status = data.get("quick_status", {})

    if not buy_summary or not sell_summary or not status:
        return None

    # Filter by active order count (competition level)
    total_active_orders = status.get('buyOrders', 0) + status.get('sellOrders', 0)
    if MAX_ACTIVE_ORDERS is not None and total_active_orders > MAX_ACTIVE_ORDERS:
        return None

    # Calculate profit margin
    top_buy_order_price = buy_summary[0]['pricePerUnit']
    bottom_sell_order_price = sell_summary[0]['pricePerUnit']

    if bottom_sell_order_price <= 0: return None
    profit_margin = (top_buy_order_price - bottom_sell_order_price) / bottom_sell_order_price

    if profit_margin < PROFIT_MARGIN_THRESHOLD:
        return None # Skip item if margin is too low

    # Calculate profit per hour
    profit_per_item = top_buy_order_price - bottom_sell_order_price
    transaction_bottleneck = min(status.get('buyMovingWeek', 0), status.get('sellMovingWeek', 0))
    # Note: We assume a 7-day period for profit/hr calculation
    profit_per_hour = (profit_per_item * transaction_bottleneck) / (7 * 24)

    return {
        'id': product_id,
        'profit_per_hour': profit_per_hour,
        'profit_margin': profit_margin,
        'profit_per_item': profit_per_item,
        'active_orders': total_active_orders,
    }

def clean_item_name(product_id: str) -> str:
    """Makes API product IDs more readable."""
    return product_id.replace("_", " ").title()

def print_results_table(results: List[Dict[str, Any]]):
    """Formats and prints the final analysis in a clean table."""
    if not results:
        print("\nAnalysis complete. No stable items were found that also met your profit criteria.")
        return

    print(f"\n--- Top Profitable & Stable Bazaar Flips (Sorted by Profit/Hour) ---")

    headers = ["Item Name", "Profit/Hour", "Margin %", "Active Orders", "Profit Per Item"]
    
    printable_data = []
    for item in results[:TOP_N_RESULTS]:
        printable_data.append([
            clean_item_name(item['id']),
            f"{item['profit_per_hour']:,.0f}",
            f"{item['profit_margin']:.1%}",
            f"{item['active_orders']:,}",
            f"{item['profit_per_item']:,.2f}",
        ])

    col_widths = [len(h) for h in headers]
    for row in printable_data:
        for i, cell in enumerate(row):
            col_widths[i] = max(col_widths[i], len(cell))
    
    header_line = " | ".join(headers[i].ljust(col_widths[i]) for i in range(len(headers)))
    print(header_line)
    print("-+-".join("-" * width for width in col_widths))
    
    for row in printable_data:
        data_line = row[0].ljust(col_widths[0])
        for i in range(1, len(row)):
            data_line += " | " + row[i].rjust(col_widths[i])
        print(data_line)

# ==============================================================================
# --- MAIN EXECUTION BLOCK ---
# ==============================================================================

if __name__ == "__main__":
    # Step 1: Fetch the curated list of stable items from our Koyeb API.
    stable_item_ids = fetch_stable_items_from_api()

    # Step 2: CRITICAL CHECK - If the list is empty, there is no point in continuing.
    # This prevents the script from analyzing every item if the API is down or still starting.
    if not stable_item_ids:
        print("\n❌ API did not provide any stable items to analyze. Aborting.")
        print("   (This is normal if the API has just started. Wait a few minutes and try again.)")
        sys.exit(1) # This safely exits the script.

    # Step 3: Fetch all raw data from the official Hypixel Bazaar API.
    all_products = get_all_bazaar_data()
    
    # Step 4: Process the data.
    if all_products:
        analyzed_items = []
        # Use a thread pool to analyze multiple items at once for speed.
        with concurrent.futures.ThreadPoolExecutor(max_workers=20) as executor:
            future_to_item = {}
            
            # This is the core filtering logic:
            # We only submit an item for profit analysis if its ID is in our stable list.
            for pid, data in all_products.items():
                if pid in stable_item_ids:
                    future_to_item[executor.submit(analyze_bazaar_item, pid, data)] = pid

            print(f"\n[*] Analyzing {len(future_to_item)} stable items for profit...")
            for future in concurrent.futures.as_completed(future_to_item):
                result = future.result()
                if result:
                    analyzed_items.append(result)
        
        # Sort the profitable items by the most important metric.
        analyzed_items.sort(key=lambda x: x['profit_per_hour'], reverse=True)
        
        # Display the final results.
        print_results_table(analyzed_items)