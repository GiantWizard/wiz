import requests
import json

# --- Helper Data ---
# This dictionary is crucial for handling known API ID inconsistencies.
MANUAL_ID_OVERRIDES = {
    "Stridersurfer": "SHARD_STRIDER_SURFER",
    "Abyssal Lanternfish": "SHARD_ABYSSAL_LANTERN",
    "Cinderbat": "SHARD_CINDER_BAT"
    # Add any other exceptions you find here in the future.
}

# --- Static Shard Data ---
# You MUST use your full, original list of shards here for the script to work correctly.
# This is a small sample for demonstration.
shards_data = [
    {'id': 'C1', 'name': 'Grove'},
    {'id': 'C2', 'name': 'Mist'},
    {'id': 'U38', 'name': 'Stridersurfer'},
    {'id': 'R23', 'name': 'Abyssal Lanternfish'},
    {'id': 'L28', 'name': 'Cinderbat'},
    # ... and so on for all your shards.
]

def get_bazaar_id_from_name(name):
    """
    Constructs the correct Bazaar API ID from a shard name, handling known exceptions.
    """
    # First, check if there's a manual override for this name.
    if name in MANUAL_ID_OVERRIDES:
        return MANUAL_ID_OVERRIDES[name]
    
    # If no override exists, use the standard generation rule.
    return f"SHARD_{name.upper().replace(' ', '_')}"

def fetch_raw_bazaar_data():
    """
    Fetches the complete, raw product data from the Hypixel Bazaar API.
    This data is keyed by the official API product IDs.

    Returns:
        dict: A dictionary of all products from the API, or None on failure.
    """
    api_url = "https://api.hypixel.net/v2/skyblock/bazaar"
    print("Fetching raw bazaar data from Hypixel API...")
    try:
        response = requests.get(api_url, timeout=10)
        response.raise_for_status()
        api_data = response.json()
        if not api_data.get("success"):
            print(f"API returned an error: {api_data.get('cause', 'Unknown reason')}")
            return None
        print("Raw data received successfully.")
        return api_data.get("products", {})
    except Exception as e:
        print(f"An error occurred while fetching raw data: {e}")
        return None

def process_shard_prices(static_shard_list, raw_bazaar_data):
    """
    Processes raw bazaar data, re-keying it with human-readable names
    from the static shard list.

    Args:
        static_shard_list (list): The complete list of shard dictionaries.
        raw_bazaar_data (dict): The raw product data from the API.

    Returns:
        dict: A dictionary where keys are human-readable shard names and
              values contain their price info.
    """
    if not raw_bazaar_data:
        print("Cannot process prices without raw bazaar data.")
        return None

    print("\nProcessing raw data and mapping prices to shard names...")
    processed_prices = {}
    
    for shard in static_shard_list:
        shard_name = shard['name']
        api_id = get_bazaar_id_from_name(shard_name)
        
        product_data = raw_bazaar_data.get(api_id)
        
        if product_data:
            sell_summary = product_data.get("sell_summary", [])
            buy_summary = product_data.get("buy_summary", [])
            
            cost_to_buy = sell_summary[0]['pricePerUnit'] if sell_summary else None
            value_to_sell = buy_summary[0]['pricePerUnit'] if buy_summary else None
            
            processed_prices[shard_name] = {
                'cost_to_buy': cost_to_buy,
                'value_to_sell': value_to_sell,
            }
        else:
            # This is expected for items not currently on the Bazaar.
            print(f"  [!] Note: No active market for '{shard_name}' (API ID: {api_id})")
            processed_prices[shard_name] = {
                'cost_to_buy': None,
                'value_to_sell': None,
            }
            
    print(f"Finished processing prices for {len(static_shard_list)} shards.")
    return processed_prices

# --- Main Execution Block ---
if __name__ == "__main__":
    # Step 1: Fetch the raw data from the API
    raw_bazaar_data = fetch_raw_bazaar_data()
    
    # Proceed only if the raw data was fetched successfully
    if raw_bazaar_data:
        # Step 2: Process the raw data to create our name-keyed price list
        final_shard_prices = process_shard_prices(shards_data, raw_bazaar_data)
        
        if final_shard_prices:
            output_filename = "bazaar_prices_by_name.json"
            print(f"\nSaving processed price data to '{output_filename}'...")
            
            try:
                with open(output_filename, "w") as f:
                    json.dump(final_shard_prices, f, indent=2)
                
                print(f"Successfully saved data to '{output_filename}'.")
                
            except IOError as e:
                print(f"\nError: Could not write data to file: {e}")
                
    else:
        print("\nProcessing failed. No raw data was fetched, so no file was created.")