import json
import math
import json5
import os
import concurrent.futures

# --- Global Constants ---
C_MAX = 800000000  # Capital limit on resting orders
MIN_PROFIT_RATIO = 1.15  # The 15% profit margin requirement
AVG_ORDER_LIFETIME_MINUTES = 2.0 # Assumed average time (in mins) an order sits on the book.
INFO_FOLDER_PATH = "info" # The folder containing your JSON files

# --- A counter to limit the number of "Skipping..." messages ---
filter_print_count = 0
FILTER_PRINT_LIMIT = 10

def sanitize_json_text(text):
    """Cleans raw text to fix common JSON errors."""
    sanitized_text = text
    while ',,' in sanitized_text:
        sanitized_text = sanitized_text.replace(',,', ',null,')
    sanitized_text = sanitized_text.replace('[,', '[null,')
    sanitized_text = sanitized_text.replace(':,', ':null,')
    sanitized_text = sanitized_text.replace(':}', ':null}')
    return sanitized_text

def process_single_file(file_path):
    """
    Worker function: reads, sanitizes, and aggregates data from ONE file.
    This function will be executed in a separate thread.
    """
    stat_keys = [
        "buy_price_average", "sell_price_average", "order_frequency_average",
        "order_size_average", "sell_frequency", "sell_size"
    ]
    local_aggregated_stats = {}
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            raw_text = f.read()
        sanitized_text = sanitize_json_text(raw_text)
        data = json5.loads(sanitized_text)
        
        for item in data:
            if not isinstance(item, dict): continue
            product_id = item.get("product_id")
            if not product_id: continue

            if product_id not in local_aggregated_stats:
                local_aggregated_stats[product_id] = {key: 0.0 for key in stat_keys}
                local_aggregated_stats[product_id]['count'] = 0

            for key in stat_keys:
                local_aggregated_stats[product_id][key] += item.get(key, 0.0)
            local_aggregated_stats[product_id]['count'] += 1
        return local_aggregated_stats
    except Exception as e:
        print(f"     WARNING: Could not process {os.path.basename(file_path)}. Reason: {e}")
        return None # Return None on failure

def calculate_profit_for_item(item):
    """
    Calculates max profit per hour for a single (averaged) item.
    """
    global filter_print_count
    # (This function remains unchanged from the previous version)
    if not item or not isinstance(item, dict): return 0.0, "Invalid item data."
    product_id = item.get("product_id", "Unknown")
    sp = item.get("buy_price_average", 0)
    bp = item.get("sell_price_average", 0)
    sell_size = item.get("sell_size", 0)
    order_size = item.get("order_size_average", 0)
    sell_freq = item.get("sell_frequency", 0)
    order_freq = item.get("order_frequency_average", 0)
    if sell_size <= order_size:
        if filter_print_count < FILTER_PRINT_LIMIT:
            filter_print_count += 1
            return 0.0, f"Sell size ({sell_size:.1f}) not > order size ({order_size:.1f})."
        return 0.0, "Size filter fail."
    if order_freq >= sell_freq:
        if filter_print_count < FILTER_PRINT_LIMIT:
            filter_print_count += 1
            return 0.0, f"Order freq ({order_freq:.2f}) not < sell freq ({sell_freq:.2f})."
        return 0.0, "Frequency filter fail."
    if bp <= 0: return 0.0, "Buy price is zero."
    margin_ratio = sp / bp
    if margin_ratio < MIN_PROFIT_RATIO:
        if filter_print_count < FILTER_PRINT_LIMIT:
            filter_print_count += 1
            return 0.0, f"Margin is {margin_ratio:.2f}x, below the {MIN_PROFIT_RATIO}x minimum."
        return 0.0, "Margin below minimum."
    try:
        tsa = sell_freq * sell_size
        cbo = order_freq
        if AVG_ORDER_LIFETIME_MINUTES <= 0: return 0.0, "Order Lifetime must be > 0."
        mbo = C_MAX / (AVG_ORDER_LIFETIME_MINUTES * bp)
        if (mbo + cbo) == 0: return 0.0, "Total buy pressure is zero."
        acquisition_rate = (mbo / (mbo + cbo)) * tsa
        profit_per_hour = (sp - bp) * acquisition_rate * 60
        return (profit_per_hour, "Success") if math.isfinite(profit_per_hour) else (0.0, "Non-finite profit.")
    except (ValueError, ZeroDivisionError) as e:
        return 0.0, f"Math error: {e}"


def analyze_folder(folder_path):
    """
    Main function to concurrently discover, aggregate, average, and analyze data.
    """
    # --- 1. DISCOVER FILES ---
    if not os.path.isdir(folder_path):
        print(f"Error: Folder '{folder_path}' not found. Please create it and place your JSON files inside.")
        return
    file_paths = [os.path.join(folder_path, f) for f in os.listdir(folder_path) if f.endswith('.json')]
    if not file_paths:
        print(f"Error: No .json files found in the '{folder_path}' folder.")
        return
    print(f"Found {len(file_paths)} files. Starting concurrent processing...")

    # --- 2. CONCURRENTLY AGGREGATE DATA ---
    final_aggregated_stats = {}
    with concurrent.futures.ThreadPoolExecutor() as executor:
        # Submit all files to the executor
        future_to_file = {executor.submit(process_single_file, path): path for path in file_paths}
        
        for future in concurrent.futures.as_completed(future_to_file):
            partial_result = future.result()
            if partial_result is None: continue # Skip failed files

            # Merge the partial result from the thread into the main dictionary
            for product_id, stats in partial_result.items():
                if product_id not in final_aggregated_stats:
                    final_aggregated_stats[product_id] = stats
                else:
                    for key, value in stats.items():
                        if key != 'count':
                            final_aggregated_stats[product_id][key] += value
                    final_aggregated_stats[product_id]['count'] += stats['count']

    if not final_aggregated_stats:
        print("Could not aggregate any valid item data from the files.")
        return

    print(f"\nAggregation complete. Found {len(final_aggregated_stats)} unique products.")

    # --- 3. CALCULATE AVERAGES ---
    averaged_items = []
    stat_keys = list(next(iter(final_aggregated_stats.values())).keys()) # Get keys from first item
    stat_keys.remove('count')

    for product_id, stats in final_aggregated_stats.items():
        count = stats['count']
        if count == 0: continue
        avg_item = {"product_id": product_id}
        for key in stat_keys:
            avg_item[key] = stats[key] / count
        averaged_items.append(avg_item)

    # --- 4. ANALYZE AVERAGED DATA ---
    results = []
    skipped_items = {}
    for item in averaged_items:
        pph, status = calculate_profit_for_item(item)
        if pph > 0:
            results.append({"id": item.get("product_id", "Unknown"), "profit_per_hour": pph})
        elif status not in ["Margin below minimum.", "Size filter fail.", "Frequency filter fail.", "Success"]:
            if item.get("product_id"):
                skipped_items[item.get("product_id")] = status
    
    # (The rest of the reporting is the same...)
    print("\n--- Filter Report on Averaged Data ---")
    for pid, reason in list(skipped_items.items()):
        print(f"Skipping {pid}: {reason}")
    if filter_print_count >= FILTER_PRINT_LIMIT:
        print(f"...and more items skipped for failing one of the primary filters.")

    if not results:
        print("\n--- Market Analysis Results ---")
        print(f"\nNo items found that meet all profit criteria based on the averaged data.")
        return

    results.sort(key=lambda x: x["profit_per_hour"], reverse=True)
    best_item = results[0]

    print(f"\n--- Market Analysis Results (Averaged Data) ---")
    print(f"Analysis based on an assumed order lifetime of {AVG_ORDER_LIFETIME_MINUTES} minutes.")
    print(f"\n🥇 Best Profit Opportunity:")
    print(f"   Product ID: {best_item['id']}")
    print(f"   Max Potential Profit: ${best_item['profit_per_hour']:,.2f} / hour")
    
    print("\n--- Full Ranking (Top 20) ---")
    for i, result in enumerate(results[:20]):
        print(f"{i+1}. {result['id']}: ${result['profit_per_hour']:,.2f} / hour")

# --- Main Execution ---
if __name__ == "__main__":
    analyze_folder(INFO_FOLDER_PATH)