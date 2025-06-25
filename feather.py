import requests
import concurrent.futures

# --- Price Fetching Function (Unchanged) ---
BAZAAR_API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
COFLNET_API_URL = "https://sky.coflnet.com/api/item/price/{item_id}/bin"

def get_lowest_price(item_id: str) -> float | None:
    """Fetches the lowest price for a given SkyBlock item from Bazaar or CoflNet."""
    # 1. Try Hypixel Bazaar
    try:
        response = requests.get(BAZAAR_API_URL, timeout=10)
        response.raise_for_status()
        bazaar_data = response.json()
        if bazaar_data.get("success") and item_id in bazaar_data.get("products", {}):
            product_info = bazaar_data["products"][item_id]
            buy_summary = product_info.get("buy_summary", [])
            if buy_summary:
                return min(order["pricePerUnit"] for order in buy_summary)
    except requests.exceptions.RequestException:
        pass

    # 2. Fallback to CoflNet
    try:
        coflnet_url = COFLNET_API_URL.format(item_id=item_id.upper())
        response = requests.get(coflnet_url, timeout=10)
        response.raise_for_status()
        coflnet_data = response.json()
        if "lowest" in coflnet_data and coflnet_data["lowest"] is not None:
            return float(coflnet_data["lowest"])
    except requests.exceptions.RequestException:
        pass

    return None

# --- Table Formatting Function (Updated for new calculation display) ---
def print_results_table(results_data: list, feather_price: float):
    """Formats and prints the final results in a clean, aligned table."""
    if not results_data:
        print("No data to display.")
        return

    headers = ["Rank", "Item", "Current Price", "Calculation", "Final Value"]

    def format_num(n):
        if not isinstance(n, (int, float)): return str(n)
        sign = "-" if n < 0 else ""
        n = abs(n)
        if n > 1_000_000_000: return f"{sign}{n/1_000_000_000:.2f}b"
        if n > 1_000_000: return f"{sign}{n/1_000_000:.2f}m"
        if n > 1000: return f"{sign}{n/1000:.2f}k"
        return f"{sign}{n:,.2f}"

    printable_data = []
    for i, row in enumerate(results_data):
        price_str = "Not Found"
        calc_str = "N/A"
        final_val_str = "N/A"

        if row["price"] is not None:
            price_str = f"{row['price']:,.0f}"
            ratio = row['num'] / row['den']
            subtraction = row.get('subtraction', 0)
            
            calc_str = f"(({format_num(row['price'])}"
            if subtraction > 0:
                calc_str += f" - {format_num(subtraction)}"
            calc_str += f") / {ratio:.2f}) - {format_num(feather_price)}"
            
            final_val_str = f"{row['final_value']:,.0f}"

        printable_data.append([ f"#{i+1}", row["label"], price_str, calc_str, final_val_str ])

    col_widths = [len(h) for h in headers]
    for row in printable_data:
        for i, cell in enumerate(row):
            col_widths[i] = max(col_widths[i], len(cell))

    header_line = " | ".join(headers[i].ljust(col_widths[i]) for i in range(len(headers)))
    print(header_line)
    separator_line = "-+-".join("-" * width for width in col_widths)
    print(separator_line)
    for row in printable_data:
        data_line = " | ".join(row[i].ljust(col_widths[i]) for i in range(len(row)))
        print(data_line)

# --- Function to process a single item (for concurrency) ---
def process_item(item: dict) -> dict:
    """Fetches price and returns a dictionary with all relevant data for one item."""
    price = get_lowest_price(item["id"])
    return {
        "label": item["label"],
        "id": item["id"],
        "num": item["num"],
        "den": item["den"],
        "subtraction": item.get("subtract", 0),
        "price": price,
    }

# --- Main Execution Logic ---
if __name__ == "__main__":
    # Define the items, their fractions, and any subtraction amounts
    ITEMS_TO_CALCULATE = [
        {"id": "NECRON_HANDLE",             "label": "Necron's Handle",           "num": 275000/1.1, "den": 7000,   "subtract": 100_000_000},
        {"id": "DIVAN_ALLOY",               "label": "Divan Alloy",               "num": 1000000/1.1, "den": 17000},
        {"id": "WITHER_CHESTPLATE",         "label": "Wither Chestplate",         "num": 52000/1.1, "den": 7000,    "subtract": 10_000_000},
        {"id": "RECOMBOBULATOR_3000",       "label": "Recombobulator 3000 (M3)",  "num": 6000/1.1, "den": 12000,   "subtract": 6_000_000},
        {"id": "ENCHANTMENT_LOOTING_5",     "label": "Enchantment Looting 5",     "num": 500000/1.1, "den": 100000},
        {"id": "VAMPIRE_DENTIST_RELIC",     "label": "Vampire Dentist Relic",     "num": 18450/1.1, "den": 8000},
        {"id": "WARDEN_HEART",              "label": "Warden Heart",              "num": 3600000/1.1, "den": 500000},
        {"id": "TOXIC_ARROW_POISON",        "label": "Toxic Arrow Poison",        "num": 3300/1.1, "den": 400000},
        {"id": "OVERFLUX_CAPACITOR",        "label": "Overflux Capacitor",        "num": 1200000/1.1, "den": 300000},
        {"id": "HAMSTER_WHEEL",             "label": "Hamster Wheel",             "num": 3000/1.1, "den": 300000},
        {"id": "JUDGEMENT_CORE",            "label": "Judgement Core",            "num": 885000/1.1, "den": 70000},
        {"id": "CRUDE_GABAGOOL_DISTILLATE", "label": "Crude Gabagool Distillate", "num": 10649/1.1, "den": 85000},
        {"id": "HIGH_CLASS_ARCHFIEND_DICE", "label": "High Class Archfiend Dice", "num": 195000/1.1, "den": 85000},
        {"id": "FIRST_MASTER_STAR",         "label": "First Master Star",         "num": 11850/1.1, "den": 12000,   "subtract": 5_000_000},
        {"id": "SECOND_MASTER_STAR",        "label": "Second Master Star",        "num": 10950/1.1, "den": 12000,   "subtract": 6_000_000},
        {"id": "THIRD_MASTER_STAR",         "label": "Third Master Star",         "num": 48750/1.1, "den": 20000,   "subtract": 7_000_000},
        {"id": "FOURTH_MASTER_STAR",        "label": "Fourth Master Star",        "num": 81240/1.1, "den": 20000,   "subtract": 8_000_000},
        {"id": "FIFTH_MASTER_STAR",         "label": "Fifth Master Star",         "num": 94200/1.1, "den": 7000,    "subtract": 9_000_000},
        {"id": "GIANTS_SWORD",              "label": "Giant's Sword",             "num": 160020/1.1, "den": 20000,   "subtract": 25_000_000}
    ]

    # 1. Fetch the Ananke Feather price first, as it's needed for all calculations.
    print("Fetching price for ANANKE_FEATHER...")
    ananke_feather_price = get_lowest_price("ANANKE_FEATHER")
    if ananke_feather_price is None:
        print("❌ CRITICAL: Could not fetch the price for ANANKE_FEATHER. Aborting.")
        exit()
    print(f"✅ Ananke Feather price: {ananke_feather_price:,.0f}\n")

    # 2. Use a ThreadPoolExecutor to fetch all other prices concurrently.
    print("Concurrently fetching prices for all other items...")
    with concurrent.futures.ThreadPoolExecutor(max_workers=10) as executor:
        # executor.map applies the process_item function to every item in the list
        results_with_prices = list(executor.map(process_item, ITEMS_TO_CALCULATE))
    print("✅ All prices fetched. Calculating final values...\n")

    # 3. Perform the final calculation on the fetched data.
    final_results = []
    for result in results_with_prices:
        result["final_value"] = None # Default value
        if result["price"] is not None and result["den"] != 0:
            price_after_subtraction = result["price"] - result["subtraction"]
            ratio = result["num"] / result["den"]
            if ratio != 0:
                intermediate_value = price_after_subtraction / ratio
                result["final_value"] = intermediate_value - ananke_feather_price
        final_results.append(result)

    # 4. Sort the final results by the calculated 'final_value'.
    final_results.sort(key=lambda x: x["final_value"] if x["final_value"] is not None else -float('inf'), reverse=True)

    # 5. Print the results in a formatted table.
    print_results_table(final_results, ananke_feather_price)