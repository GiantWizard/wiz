import requests
import concurrent.futures
from typing import List, Dict, Any, Optional

# --- Configuration ---
BAZAAR_API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
TOP_N_RESULTS = 50
# Set the minimum profit margin required to show an item
PROFIT_MARGIN_THRESHOLD = 0.15 # (15%)
# Set a maximum number of active orders to filter out highly competitive items.
# An item with thousands of orders is very volatile. Set to None to disable this filter.
MAX_ACTIVE_ORDERS = 500  # <-- NEW: Filter out items with more than 500 total active orders

# --- Time Period Assumptions ---
SPECIAL_HOURS_IN_PERIOD = 96
SPECIAL_48_HOUR_ITEMS = {
    "SEA_LUMIES", "SHARD_YOG", "SHARD_CROW", "SHARD_CHILL", "SHARD_CARROT_KING", "SHARD_CORALOT",
    "SHARD_WATER_HYDRA", "SHARD_LEATHERBACK", "SHARD_NEWT", "SHARD_POWER_DRAGON", "SHARD_BOREAL_OWL",
    "SHARD_GOLDEN_GHOUL", "SHARD_PHANFLARE", "SHARD_TEMPEST", "SHARD_NIGHT_SQUID",
    "ENCHANTED_FIG_LOG", "SHARD_BIRRIES", "SHARD_INFERNO_KOI", "SHARD_BEZAL", "SHARD_SEA_ARCHER",
    "SHARD_COD", "MOONGLADE_JEWEL", "SHARD_LORD_JAWBUS", "SHARD_VERDANT", "SHARD_GROVE",
    "SHARD_LAPIS_ZOMBIE", "SHARD_HARPY", "ENCHANTED_VINESAP", "FIG_LOG", "SHARD_HIDEONLEAF",
    "SHARD_TANK_ZOMBIE", "MANGROVE_GEM", "VINESAP", "SHARD_TADGANG", "SHARD_SEA_EMPEROR",
    "SHARD_FLASH", "SHARD_MUDWORM", "FIGSTONE", "ENCHANTED_TENDER_WOOD", "SHARD_ZEALOT",
    "SHARD_VORACIOUS_SPIDER", "SHARD_MIST", "SHARD_HIDEONGIFT", "SHARD_PHANPYRE",
    "ENCHANTED_SEA_LUMIES", "TENDER_WOOD", "SHARD_AZURE", "SHARD_SALMON", "SHARD_BAL",
    "SHARD_BITBUG", "SHARD_PEST", "SHARD_LAVA_FLAME", "SHARD_OBSIDIAN_DEFENDER",
    "SHARD_WITHER_SPECTER", "SHARD_PRINCE", "SHARD_TROGLOBYTE", "SHARD_GECKO", "SHARD_BASILISK",
    "SHARD_PYTHON", "SHARD_SKELETOR", "SHARD_THORN", "SHARD_HERON", "SHARD_FLARE",
    "SHARD_KING_COBRA", "SHARD_ENT", "SHARD_AERO", "SHARD_ALLIGATOR", "SHARD_SEA_SERPENT",
    "SHARD_PRAYING_MANTIS", "SHARD_DROWNED", "SHARD_HIDEONDRA", "SHARD_IGUANA", "SHARD_TIDE",
    "SHARD_EEL", "ENCHANTMENT_ABSORB_1", "SHARD_FENLORD", "SHARD_HIDEONSACK",
    "SHARD_BARBARIAN_DUKE_X", "SHARD_RAIN_SLIME", "SHARD_MATCHO", "SHARD_GLACITE_WALKER",
    "SHARD_KADA_KNIGHT", "SHARD_MORAY_EEL", "SHARD_CASCADE", "SHARD_HELLWISP", "SHARD_TOAD",
    "SHARD_MOSSYBIT", "SHARD_MOCHIBEAR", "SHARD_FIRE_EEL", "SHARD_MIMIC", "ENCHANTMENT_ARCANE_5",
    "ENCHANTMENT_ARCANE_3", "ENCHANTMENT_ARCANE_4", "ENCHANTMENT_ARCANE_1", "ENCHANTMENT_ARCANE_2",
    "SHARD_SHELLWISE", "SHARD_STAR_SENTRY", "SHARD_LUMISQUID", "SHARD_LIZARD_KING",
    "SHARD_SOUL_OF_THE_ALPHA", "SHARD_ANANKE", "SHARD_THYST", "SHARD_CAIMAN", "SHARD_VIPER",
    "SHARD_BOLT", "SHARD_QUARTZFANG", "SHARD_HIDEONCAVE", "SHARD_DREADWING", "SHARD_PIRANHA",
    "SHARD_LUNAR_MOTH", "SHARD_HIDEONGEON", "SHARD_CROCODILE"
}
DEFAULT_HOURS_IN_PERIOD = 7 * 24 # 168 hours

# --- Core Logic ---

def get_all_bazaar_data() -> Optional[Dict[str, Any]]:
    """Fetches the entire bazaar product list in a single API call."""
    print("Fetching all bazaar data from Hypixel API...")
    try:
        response = requests.get(BAZAAR_API_URL, timeout=10)
        response.raise_for_status()
        bazaar_data = response.json()
        if bazaar_data.get("success"):
            print(f"✅ Data received. Analyzing items with >{PROFIT_MARGIN_THRESHOLD:.0%} profit margin...")
            if MAX_ACTIVE_ORDERS is not None: # <-- NEW
                print(f"   and fewer than {MAX_ACTIVE_ORDERS} active orders.") # <-- NEW
            return bazaar_data.get("products", {})
        else:
            print("❌ API request was not successful.")
            return None
    except requests.exceptions.RequestException as e:
        print(f"❌ An API error occurred: {e}")
        return None

def analyze_bazaar_item(product_id: str, data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """
    Calculates profit metrics, filters by profit margin, and checks order competition.
    """
    buy_summary = data.get("buy_summary", [])
    sell_summary = data.get("sell_summary", [])
    status = data.get("quick_status", {}) # <-- MODIFIED: Moved up for early access

    if not buy_summary or not sell_summary or not status:
        return None

    # --- NEW: Filter by active order count (competition level) ---
    active_buy_orders = status.get('buyOrders', 0)   # <-- NEW
    active_sell_orders = status.get('sellOrders', 0) # <-- NEW
    total_active_orders = active_buy_orders + active_sell_orders # <-- NEW

    if MAX_ACTIVE_ORDERS is not None and total_active_orders > MAX_ACTIVE_ORDERS: # <-- NEW
        return None # <-- NEW: Skip item if it's too competitive

    top_buy_order_price = buy_summary[0]['pricePerUnit']
    bottom_sell_order_price = sell_summary[0]['pricePerUnit']

    if bottom_sell_order_price <= 0:
        return None

    profit_per_item = top_buy_order_price - bottom_sell_order_price
    profit_margin = profit_per_item / bottom_sell_order_price

    if profit_margin < PROFIT_MARGIN_THRESHOLD:
        return None

    hours_in_period = SPECIAL_HOURS_IN_PERIOD if product_id in SPECIAL_48_HOUR_ITEMS else DEFAULT_HOURS_IN_PERIOD
    
    buy_volume_week = status.get('buyMovingWeek', 0)
    sell_volume_week = status.get('sellMovingWeek', 0)
    transaction_bottleneck = min(buy_volume_week, sell_volume_week)

    profit_per_hour = (profit_per_item * transaction_bottleneck) / hours_in_period

    return {
        'id': product_id,
        'profit_per_hour': profit_per_hour,
        'profit_margin': profit_margin,
        'profit_per_item': profit_per_item,
        'buy_price': top_buy_order_price,
        'sell_price': bottom_sell_order_price,
        'bottleneck_volume': transaction_bottleneck,
        'period_hours': hours_in_period,
        'active_orders': total_active_orders # <-- NEW: Pass competition metric to be printed
    }

def clean_item_name(product_id: str) -> str:
    """Makes API product IDs more readable."""
    parts = product_id.split(':')
    name = parts[0].replace("_", " ").title()
    if len(parts) > 1:
        return f"{name}:{parts[1]}"
    return name

def print_results_table(results: List[Dict[str, Any]]):
    """Formats and prints the filtered, high-margin analysis."""
    if not results:
        message = f"\nAnalysis complete. No items found with a profit margin > {PROFIT_MARGIN_THRESHOLD:.0%}"
        if MAX_ACTIVE_ORDERS is not None:
             message += f" and < {MAX_ACTIVE_ORDERS} active orders."
        print(message)
        return

    print(f"\n--- Top Bazaar Flips with >{PROFIT_MARGIN_THRESHOLD:.0%} Margin (Sorted by Profit/Hour) ---")
    print("Analysis covers all items. Best opportunities are at the top.\n")

    # <-- MODIFIED: Added "Active Orders" column
    headers = ["Item Name", "Profit/Hour", "Margin %", "Active Orders", "Profit Spread", "Top Buy Order", "Bottom Sell Order", "Bottleneck Vol", "Period (h)"]
    
    printable_data = []
    for item in results[:TOP_N_RESULTS]:
        name_str = clean_item_name(item['id'])
        profit_hr_str = f"{item['profit_per_hour']:,.0f}"
        margin_str = f"{item['profit_margin']:.1%}"
        active_orders_str = f"{item['active_orders']:,}" # <-- NEW
        profit_item_str = f"{item['profit_per_item']:,.2f}"
        buy_price_str = f"{item['buy_price']:,.1f}"
        sell_price_str = f"{item['sell_price']:,.1f}"
        vol_str = f"{item['bottleneck_volume']:,}"
        period_str = str(item['period_hours'])
        # <-- MODIFIED: Added the new metric to the row data
        printable_data.append([name_str, profit_hr_str, margin_str, active_orders_str, profit_item_str, buy_price_str, sell_price_str, vol_str, period_str])

    col_widths = [len(h) for h in headers]
    for row in printable_data:
        for i, cell in enumerate(row):
            col_widths[i] = max(col_widths[i], len(cell))
    
    header_line = " | ".join(headers[i].ljust(col_widths[i]) for i in range(len(headers)))
    print(header_line)
    print("-+-".join("-" * width for width in col_widths))
    
    for row in printable_data:
        data_line = " | ".join(row[i].rjust(col_widths[i]) for i in range(len(row)))
        data_line = f"{row[0].ljust(col_widths[0])}" + data_line[col_widths[0]:]
        print(data_line)

# --- Main Execution Logic ---
if __name__ == "__main__":
    all_products = get_all_bazaar_data()
    
    if all_products:
        analyzed_items = []
        with concurrent.futures.ThreadPoolExecutor(max_workers=20) as executor:
            future_to_item = {
                executor.submit(analyze_bazaar_item, pid, data): pid
                for pid, data in all_products.items()
            }
            
            for future in concurrent.futures.as_completed(future_to_item):
                result = future.result()
                if result:
                    analyzed_items.append(result)
        
        analyzed_items.sort(key=lambda x: x['profit_per_hour'], reverse=True)
        
        print_results_table(analyzed_items)