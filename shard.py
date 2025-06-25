import requests

# --- Data for Ingredient Families ---
# This dictionary maps shards to their family to determine the correct multiplier.
# Extracted from your original table data.
SHARD_FAMILIES = {
    "Megalith": "Reptile and Turtle", "Naga": "Amphibian and Eel", "Tortoise": "Reptile and Turtle",
    "Wyvern": "Reptile and Scaled", "Tiamat": "Croco and Reptile", "Chameleon": "Reptile",
    "Leatherback": "Reptile and Turtle", "Sea Serpent": "Amphibian and Eel and Serpent",
    "Caiman": "Croco and Reptile", "Komodo Dragon": "Reptile and Scaled", "Iguana": "Reptile and Scaled",
    "Moray Eel": "Amphibian and Eel", "Basilisk": "Reptile and Serpent", "Fenlord": "Amphibian and Frog",
    "Alligator": "Croco and Reptile", "Leviathan": "Lizard and Reptile", "Gecko": "Reptile and Scaled",
    "King Cobra": "Reptile and Serpent", "Eel": "Amphibian and Eel", "Crocodile": "Croco and Reptile",
    "Python": "Reptile and Serpent", "Lizard King": "Lizard and Reptile", "Toad": "Amphibian and Frog",
    "Viper": "Reptile and Serpent", "Mossybit": "Amphibian and Frog", "Cuboa": "Reptile and Serpent",
    "Salamander": "Lizard and Reptile", "Newt": "Lizard and Reptile", "Tadgang": "Amphibian and Frog",
    "Fire Eel": "Amphibian and Eel", "Bullfrog": "Amphibian and Frog", "Rana": "Amphibian and Frog"
    # Other shards not in these families will default to the 5x multiplier.
}

# --- Mappings for Special API IDs ---
SPECIAL_ID_MAP = {
    "Loch Emperor": "SHARD_SEA_EMPEROR", "Cyro": "SHARD_CRYO", "Cinderbat": "SHARD_CINDER_BAT",
    "Abyssal Lanternfish": "SHARD_ABYSSAL_LANTERN", "Jormurg": "SHARD_JORMUNG"
}

# --- List of Fusion Recipes ---
RECIPE_LIST = [
    {'result': 'Zealot', 'ingredients': ['Lapis Zombie', 'Tank Zombie']},
    {'result': 'Tank Zombie', 'ingredients': ['Voracious Spider']},
    {'result': 'Power Dragon', 'ingredients': ['Burningsoul']},
    {'result': 'Golden Ghoul', 'ingredients': ['Zealot']},
    {'result': 'Zombie Soldier', 'ingredients': ['Viper']},
    {'result': 'Lava Flame', 'ingredients': ['Eel']},
    {'result': 'Loch Emperor', 'ingredients': ['Water Hydra']},
    {'result': 'Lord Jawbus', 'ingredients': ['Shinyfish']},
    {'result': 'Water Hydra', 'ingredients': ['Fire Eel', 'Cyro']},
    {'result': 'Fire Eel', 'ingredients': ['Eel', 'Moray Eel']},
    {'result': 'Lapis Creeper', 'ingredients': ['Lapis Skeleton', 'Ghost']},
    {'result': 'Stalagmight', 'ingredients': ['Quartzfang', 'Silentdepth']},
    {'result': 'Chill', 'ingredients': ['Lapis Zombie']},
    {'result': 'Salmon', 'ingredients': ['Mossybit']},
    {'result': 'Goldfin', 'ingredients': ['Golden Ghoul']},
    {'result': 'Cod', 'ingredients': ['Mist']},
    {'result': 'Wartybug', 'ingredients': ['Dragonfly']},
    {'result': 'Dragonfly', 'ingredients': ['Firefly']},
    {'result': 'Firefly', 'ingredients': ['Lunar Moth', 'Bezal', 'Cinderbat', 'Flare', 'Lava Flame', 'Fire Eel', 'Bal', 'Flaming Spider']},
    {'result': 'Lunar Moth', 'ingredients': ['Ladybug']},
    {'result': 'Ladybug', 'ingredients': ['Cropeetle']},
    {'result': 'Cropeetle', 'ingredients': ['Invisibug', 'Termite']},
    {'result': 'Termite', 'ingredients': ['Praying Mantis']},
    {'result': 'Praying Mantis', 'ingredients': ['Pest', 'Pest']},
    {'result': 'Spike', 'ingredients': ['Blizzard']},
    {'result': 'Birries', 'ingredients': ['Sea Archer']},
    {'result': 'Bal', 'ingredients': ['Thorn', 'Thorn']},
    {'result': 'Quartzfang', 'ingredients': ['Troglobyte', 'Abyssal Lanternfish']},
    {'result': 'Lapis Skeleton', 'ingredients': ['Lapis Zombie']},
    {'result': 'Etherdrake', 'ingredients': ['Apex Dragon', 'Kraken']},
    {'result': 'Jormurg', 'ingredients': ['Power Dragon', 'Kraken']},
    {'result': 'Galaxy Fish', 'ingredients': ['Sun Fish', 'Sun Fish']},
    {'result': 'Sun Fish', 'ingredients': ['Shinyfish', 'Shinyfish']},
    {'result': 'Molthorn', 'ingredients': ['Etherdrake', 'Jormurg']},
    {'result': 'Moltenfish', 'ingredients': ['Kraken']},
    {'result': 'Daemon', 'ingredients': ['Kraken', 'Kraken']},
    {'result': 'Shellwise', 'ingredients': ['Loch Emperor']},
    {'result': 'Prince', 'ingredients': ['Flare', 'Flare']},
    {'result': 'Draconic', 'ingredients': ['Crocodile']},
    {'result': 'Joydive', 'ingredients': ['Bullfrog']},
    {'result': 'Piranha', 'ingredients': ['Cascade']},
    {'result': 'Yog', 'ingredients': ['Bezal']},
    {'result': 'Tadgang', 'ingredients': ['Birries']},
]

def get_api_id(name):
    """Converts a user-friendly shard name to its API ID, handling special cases."""
    if name in SPECIAL_ID_MAP:
        return SPECIAL_ID_MAP[name]
    return "SHARD_" + name.upper().replace(" ", "_")

def get_bazaar_prices():
    """Fetches all bazaar data and returns a simplified price dictionary."""
    print("Fetching all bazaar data from Hypixel API...")
    try:
        response = requests.get("https://api.hypixel.net/v2/skyblock/bazaar", timeout=10)
        response.raise_for_status()
        api_data = response.json()
        if not api_data.get("success"):
            return None
        
        products = api_data.get("products", {})
        price_dict = {}
        
        for product_id, data in products.items():
            buy_summary = data.get("buy_summary", [])
            sell_summary = data.get("sell_summary", [])
            
            price_dict[product_id] = {
                'instant_buy': sell_summary[0]['pricePerUnit'] if sell_summary else None,
                'instant_sell': buy_summary[0]['pricePerUnit'] if buy_summary else None,
            }
        print("Data received and processed.")
        return price_dict
        
    except requests.exceptions.RequestException as e:
        print(f"API Error: {e}")
        return None

def calculate_fusion_profits(recipes, prices):
    """Calculates profit for each fusion recipe with family-specific multipliers."""
    if not prices:
        return [], []

    profit_results = []
    skipped_recipes = []
    DEFAULT_MULTIPLIER = 5
    SPECIAL_MULTIPLIER = 2

    for recipe in recipes:
        result_name = recipe['result']
        ingredients = recipe['ingredients']
        
        result_id = get_api_id(result_name)
        result_value = prices.get(result_id, {}).get('instant_buy')
        
        if result_value is None:
            skipped_recipes.append({
                'recipe_str': f"{' + '.join(ingredients)} -> {result_name}",
                'missing': [f"{result_name} (Product has no sell offers)"]
            })
            continue

        total_ingredient_value = 0
        for ingredient_name in ingredients:
            ingredient_id = get_api_id(ingredient_name)
            ingredient_data = prices.get(ingredient_id, {})
            ingredient_sell_price = ingredient_data.get('instant_sell')
            
            # --- Determine the correct multiplier for this ingredient ---
            family_string = SHARD_FAMILIES.get(ingredient_name, "")
            current_multiplier = DEFAULT_MULTIPLIER
            if 'Reptile' in family_string or 'Amphibian' in family_string:
                current_multiplier = SPECIAL_MULTIPLIER

            total_ingredient_value += ((ingredient_sell_price or 0) * current_multiplier)
            
        recipe_str = f"{' + '.join(ingredients)} -> {result_name}"
        profit = result_value - total_ingredient_value
        
        profit_results.append({
            'recipe_str': recipe_str,
            'ingredient_value': total_ingredient_value,
            'result_value': result_value,
            'profit': profit
        })
            
    profit_results.sort(key=lambda x: x['profit'], reverse=True)
    
    return profit_results, skipped_recipes

# --- Main script execution ---
if __name__ == "__main__":
    all_prices = get_bazaar_prices()
    
    if all_prices:
        profits, skipped = calculate_fusion_profits(RECIPE_LIST, all_prices)

        print("\n--- Fusion Profitability Report (Using Family-Specific Multipliers) ---")
        
        header = f"{'Fusion Recipe':<65} | {'Ingredients Value':>18} | {'Product Value':>15} | {'Difference':>18}"
        print(header)
        print("-" * len(header))

        for result in profits:
            recipe_str = result['recipe_str']
            cost_str = f"{result['ingredient_value']:>18,.0f}"
            value_str = f"{result['result_value']:>15,.0f}"
            profit_str = f"{result['profit']:>18,.0f}"
            print(f"{recipe_str:<65} | {cost_str} | {value_str} | {profit_str}")

        if skipped:
            print("\n--- Recipes Skipped (Missing Bazaar Data) ---")
            for item in skipped:
                print(f"- {item['recipe_str']}")
                print(f"  Reason: {', '.join(item['missing'])}")
    else:
        print("\nCould not fetch bazaar data. Aborting.")