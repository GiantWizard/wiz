import json
import math
import json5

# --- Global Constants ---
C_MAX = 800000000  # Capital limit
MIN_PROFIT_RATIO = 1.15  # The 15% profit margin requirement (Sell Price / Buy Price)

# --- A counter to limit the number of "Skipping..." messages ---
filter_print_count = 0
FILTER_PRINT_LIMIT = 10 # Set how many skipped items to report on

def sanitize_json_text(text):
    """Cleans raw text to fix common JSON errors."""
    sanitized_text = text
    while ',,' in sanitized_text:
        sanitized_text = sanitized_text.replace(',,', ',null,')
    sanitized_text = sanitized_text.replace('[,', '[null,')
    sanitized_text = sanitized_text.replace(':,', ':null,')
    sanitized_text = sanitized_text.replace(':}', ':null}')
    return sanitized_text

def calculate_profit_for_item(item):
    """
    Calculates max profit per hour for an item if it meets all criteria.
    Returns the profit and a status message.
    """
    global filter_print_count

    if not item or not isinstance(item, dict):
        return 0.0, "Invalid item data (was null)."

    product_id = item.get("product_id", "Unknown")
    sp = item.get("buy_price_average", 0)
    bp = item.get("sell_price_average", 0)

    # --- 1. APPLY FILTERS ---

    # Filter 1: Buy price must be positive to calculate a valid ratio.
    if bp <= 0:
        return 0.0, "Buy price is zero or negative."

    # Filter 2: Apply the user-suggested 1.15 sell/buy ratio check.
    margin_ratio = sp / bp
    if margin_ratio < MIN_PROFIT_RATIO:
        # Only print the reason for the first few filtered items to avoid spam.
        if filter_print_count < FILTER_PRINT_LIMIT:
            filter_print_count += 1
            return 0.0, f"Margin is {margin_ratio:.2f}x, which is below the {MIN_PROFIT_RATIO}x minimum."
        return 0.0, "Margin below minimum."

    # --- 2. CALCULATION (only for items that passed the filter) ---
    try:
        sell_freq = item.get("sell_frequency", 0)
        sell_size = item.get("sell_size", 0)
        tsa = sell_freq * sell_size

        buy_freq = item.get("order_frequency_average", 0)
        buy_size = item.get("order_size_average", 0)
        tba = buy_freq * buy_size
        
        cbo = buy_freq
        cso = sell_freq

        if abs(tba - tsa) < 1e-9:
            return 0.0, "Identical buy/sell volume, invalidates formula."

        a = bp * (tba - tsa)
        b = (bp * tba * cbo) + (sp * cso * tsa) - (C_MAX * (tba - tsa))
        c = -C_MAX * tba * cbo

        if abs(a) < 1e-9:
             return 0.0, "Calculation error (coefficient 'a' is zero)."

        discriminant = b**2 - 4 * a * c
        if discriminant < 0:
            return 0.0, "Math error (negative discriminant)."

        mbo = (-b + math.sqrt(discriminant)) / (2 * a)
        if mbo <= 0:
            return 0.0, "No profitable buy order rate possible."

        if (mbo + cbo) == 0:
            return 0.0, "Total buy pressure is zero."
        
        acquisition_rate = (mbo / (mbo + cbo)) * tsa
        profit_per_hour = (sp - bp) * acquisition_rate * 60

        return profit_per_hour, "Success"

    except (ValueError, ZeroDivisionError) as e:
        return 0.0, f"Unexpected math error: {e}"

def analyze_best_profit(file_path):
    """
    Reads, sanitizes, and analyzes item data from a JSON file to find the
    best profit opportunity, with detailed feedback.
    """
    try:
        print("Attempting to read and sanitize the JSON file...")
        with open(file_path, 'r', encoding='utf-8') as f:
            raw_text = f.read()
        sanitized_text = sanitize_json_text(raw_text)
        data = json5.loads(sanitized_text)
        print(f"File successfully parsed. Analyzing {len(data)} items...")
        
    except FileNotFoundError:
        print(f"Error: The file '{file_path}' was not found.")
        return
    except Exception as e:
        print(f"Error: Could not parse the file even after sanitation. Reason: {e}")
        return

    results = []
    skipped_items = {}
    
    for item in data:
        pph, status = calculate_profit_for_item(item)
        if pph > 0:
            results.append({"id": item.get("product_id", "Unknown"), "profit_per_hour": pph})
        # Keep track of why items were skipped for the summary
        elif status not in ["Margin below minimum.", "Success"]:
            if item.get("product_id"):
                skipped_items[item.get("product_id")] = status

    print("\n--- Filter Report ---")
    for pid, reason in list(skipped_items.items()):
        print(f"Skipping {pid}: {reason}")
    if filter_print_count >= FILTER_PRINT_LIMIT:
        print(f"...and more items skipped due to low margin.")

    if not results:
        print("\n--- Market Analysis Results ---")
        print(f"\nNo items found with a profit margin ratio of {MIN_PROFIT_RATIO}x or higher.")
        return

    results.sort(key=lambda x: x["profit_per_hour"], reverse=True)
    best_item = results[0]

    print("\n--- Market Analysis Results (>= 15% Margin) ---")
    print(f"\n🥇 Best Profit Opportunity:")
    print(f"   Product ID: {best_item['id']}")
    print(f"   Max Potential Profit: ${best_item['profit_per_hour']:,.2f} / hour")
    
    print("\n--- Full Ranking ---")
    for i, result in enumerate(results):
        print(f"{i+1}. {result['id']}: ${result['profit_per_hour']:,.2f} / hour")

# --- Main Execution ---
if __name__ == "__main__":
    analyze_best_profit("info.json")