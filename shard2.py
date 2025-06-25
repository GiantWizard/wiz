import requests
import concurrent.futures
from typing import List, Dict, Any, Optional
import math

# --- Configuration ---
BAZAAR_API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
TOP_N_RESULTS = 25  # Show the top 25 scored results
PROFIT_MARGIN_THRESHOLD = 0.15  # (15%)
MAX_ACTIVE_ORDERS = 750  # Filter out items with more than this many total active orders

# --- NEW: Scoring Weights (Must add up to 1.0) ---
# Define what makes an item "the best" for you.
# - profit_per_hour: How much money you make over time.
# - profit_margin: How safe the flip is (higher margin is safer).
# - low_competition: How easy it is to flip without being undercut.
SCORE_WEIGHTS = {
    "profit_per_hour": 0.5, # 50% of the score
    "profit_margin": 0.3,   # 30% of the score
    "low_competition": 0.2  # 20% of the score
}

# --- Time Period Assumptions (No changes needed here) ---
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
            print(f"✅ Data received. Analyzing items that meet requirements...")
            return bazaar_data.get("products", {})
        else:
            print("❌ API request was not successful.")
            return None
    except requests.exceptions.RequestException as e:
        print(f"❌ An API error occurred: {e}")
        return None

def analyze_bazaar_item(product_id: str, data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """
    Calculates profit metrics and filters out items that do not meet the baseline requirements.
    """
    buy_summary = data.get("buy_summary", [])
    sell_summary = data.get("sell_summary", [])
    status = data.get("quick_status", {})

    if not buy_summary or not sell_summary or not status:
        return None

    # --- Filter by active order count (competition level) ---
    active_buy_orders = status.get('buyOrders', 0)
    active_sell_orders = status.get('sellOrders', 0)
    total_active_orders = active_buy_orders + active_sell_orders

    if MAX_ACTIVE_ORDERS is not None and total_active_orders > MAX_ACTIVE_ORDERS:
        return None # Skip item if it's too competitive

    top_buy_order_price = buy_summary[0]['pricePerUnit']
    bottom_sell_order_price = sell_summary[0]['pricePerUnit']

    if bottom_sell_order_price <= 0: return None

    profit_per_item = top_buy_order_price - bottom_sell_order_price
    profit_margin = profit_per_item / bottom_sell_order_price

    if profit_margin < PROFIT_MARGIN_THRESHOLD:
        return None # Skip item if margin is too low

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
        'active_orders': total_active_orders
    }

def add_flip_scores(items: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    """
    NEW: Calculates a weighted score for each item to find the "best" flips.
    """
    if not items:
        return []

    print(f"\n✅ Found {len(items)} potential flips. Calculating scores to find the best...")

    # Find the maximum values for normalization (so big numbers don't dominate the score)
    max_profit_hr = max(item['profit_per_hour'] for item in items)
    max_margin = max(item['profit_margin'] for item in items)
    # For competition, we find the minimum non-zero value to properly score "low is good"
    min_orders = min((item['active_orders'] for item in items if item['active_orders'] > 0), default=1)

    scored_items = []
    for item in items:
        # Normalize each metric from 0 to 1
        norm_profit_hr = item['profit_per_hour'] / max_profit_hr if max_profit_hr > 0 else 0
        norm_margin = item['profit_margin'] / max_margin if max_margin > 0 else 0
        # For low competition, a lower order count is better, so we invert the score
        norm_competition = min_orders / item['active_orders'] if item['active_orders'] > 0 else 1

        # Calculate the weighted score
        score = (norm_profit_hr * SCORE_WEIGHTS['profit_per_hour'] +
                 norm_margin * SCORE_WEIGHTS['profit_margin'] +
                 norm_competition * SCORE_WEIGHTS['low_competition'])
        
        item['score'] = score * 100  # Multiply by 100 for a more readable score
        scored_items.append(item)
    
    return scored_items

def clean_item_name(product_id: str) -> str:
    """Makes API product IDs more readable."""
    return product_id.replace("_", " ").title()

def print_results_table(results: List[Dict[str, Any]]):
    """Formats and prints the filtered and scored analysis."""
    if not results:
        print(f"\nAnalysis complete. No items found that meet your criteria.")
        return

    print(f"\n--- Top {TOP_N_RESULTS} Bazaar Flips (Sorted by Weighted Score) ---")
    print("Score balances profit/hr, margin, and low competition. Best opportunities are at the top.\n")

    # MODIFIED: Added "Score" column and re-ordered for importance
    headers = ["Item Name", "Score", "Profit/Hour", "Margin %", "Active Orders", "Profit Spread"]
    
    printable_data = []
    for item in results[:TOP_N_RESULTS]:
        printable_data.append([
            clean_item_name(item['id']),
            f"{item['score']:.1f}",
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
        # Justify the first column to the left, and the rest to the right
        data_line = row[0].ljust(col_widths[0])
        for i in range(1, len(row)):
            data_line += " | " + row[i].rjust(col_widths[i])
        print(data_line)

# --- Main Execution Logic ---
if __name__ == "__main__":
    all_products = get_all_bazaar_data()
    
    if all_products:
        # Step 1: Filter items based on your baseline requirements (margin, orders)
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
        
        # Step 2: NEW - Score the filtered items to find the best overall opportunities
        scored_items = add_flip_scores(analyzed_items)
        
        # Step 3: Sort the results by the new score
        scored_items.sort(key=lambda x: x['score'], reverse=True)
        
        # Step 4: Print the final, ranked list
        print_results_table(scored_items)