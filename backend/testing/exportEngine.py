import json
import requests
from pathlib import Path
from collections import Counter
import os

# ------------------------------------------------------------------------------
# Step 1: Data Loading and Price Fetching Functions
# ------------------------------------------------------------------------------

def load_recipes(directory="dependencies/items"):
    recipes = {}
    path = Path(directory)
    if not path.exists():
        print(f"Warning: Directory '{directory}' does not exist. Skipping.")
        return recipes  # Return empty dict if the folder is missing

    for file in path.glob('*.json'):
        try:
            with open(file, 'r') as f:
                data = json.load(f)
                if 'internalname' in data:
                    if 'recipes' in data:
                        # Filter out forge, katgrade, and trade recipes
                        valid_recipes = [r for r in data['recipes'] 
                                         if not r.get('type') in ['forge', 'katgrade', 'trade']]
                        if valid_recipes:
                            recipes[data['internalname']] = valid_recipes
                    elif 'recipe' in data:
                        recipe = data['recipe']
                        if not recipe.get('type') in ['forge', 'katgrade', 'trade']:
                            recipes[data['internalname']] = [recipe]
        except Exception as e:
            print(f"Error reading {file}: {e}")
    
    return recipes

def fetch_all_bazaar_prices():
    url = 'https://api.hypixel.net/skyblock/bazaar'
    response = requests.get(url).json()
    if "products" not in response:
        raise Exception("Failed to fetch Bazaar data")

    prices = {}
    for item_id, details in response["products"].items():
        # Get highest buy price from buy_summary
        buy_summary = details.get("buy_summary", [])
        buy_price = buy_summary[0]["pricePerUnit"] if buy_summary else None

        # Get lowest sell price from sell_summary
        sell_summary = details.get("sell_summary", [])
        sell_price = sell_summary[0]["pricePerUnit"] if sell_summary else None

        # Get moving week data from quick_status (needed for calculations)
        quick_status = details.get("quick_status", {})
        hourly_instasells = quick_status.get("sellMovingWeek", 0) / 168
        hourly_instabuys = quick_status.get("buyMovingWeek", 0) / 168

        # Calculate fill time in seconds (based on hourly rates)
        fill_time = 3600 / max(hourly_instabuys, hourly_instasells) if max(hourly_instabuys, hourly_instasells) > 0 else float('inf')

        if buy_price and sell_price:
            if hourly_instabuys > hourly_instasells or buy_price > sell_price * hourly_instasells / (hourly_instabuys + 1e-6):
                prices[item_id] = {
                    "price": buy_price,
                    "method": "Instabuy",
                    "hourly_instabuys": hourly_instabuys,
                    "hourly_instasells": hourly_instasells,
                    "fill_time": fill_time
                }
            else:
                prices[item_id] = {
                    "price": sell_price,
                    "method": "Buy Order",
                    "hourly_instabuys": hourly_instabuys,
                    "hourly_instasells": hourly_instasells,
                    "fill_time": fill_time
                }
    
    return prices

def fetch_lbin_prices():
    return requests.get("http://moulberry.codes/lowestbin.json").json()

def calculate_recipe_fill_time(recipe, bazaar_prices):
    """Calculate the time it would take to fill all components of a recipe"""
    max_fill_time = 0
    for item, count in recipe.items():
        bazaar_item = item.replace('-', ':')
        if bazaar_item in bazaar_prices:
            max_fill_time = max(max_fill_time, bazaar_prices[bazaar_item]['fill_time'])
    return max_fill_time

def calculate_bin_tax(price: float) -> float:
    """Calculate BIN tax correctly with separate listing and collection fees."""
    if price < 1_000_000:
        return price  # No tax for amounts below 1M
    
    # BIN listing fee
    if price >= 100_000_000:
        bin_fee = price * 0.025  # 2.5% listing fee
    elif price >= 10_000_000:
        bin_fee = price * 0.02  # 2% listing fee
    else:
        bin_fee = price * 0.01  # 1% listing fee

    # Collection tax (1% capped at 1M minimum collection)
    collection_tax = min(price * 0.01, price - 1_000_000)

    final_amount = price - (bin_fee + collection_tax)
    
    return final_amount

def is_base_item(item_name, recipes):
    if item_name not in recipes:
        return True
    
    for recipe in recipes[item_name]:
        unique_ingredients = set()
        for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
            if pos in recipe and recipe[pos]:
                unique_ingredients.add(recipe[pos].split(':')[0])
                
        if len(unique_ingredients) == 1:
            ingredient = unique_ingredients.pop()
            if ingredient in recipes:
                for ing_recipe in recipes[ingredient]:
                    for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
                        if pos in ing_recipe and ing_recipe[pos]:
                            if ing_recipe[pos].split(':')[0] == item_name:
                                return True
    return False

def get_full_recipe(item_name, recipes):
    if item_name not in recipes or is_base_item(item_name, recipes):
        return {item_name: 1}
    
    recipe = next((r for r in recipes[item_name] if r.get('count', 1) == 1), recipes[item_name][0])
    
    result = Counter()
    for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
        if pos in recipe and recipe[pos]:
            ingredient, count = recipe[pos].split(':')
            sub_ingredients = get_full_recipe(ingredient, recipes)
            for sub_item, sub_count in sub_ingredients.items():
                result[sub_item] += sub_count * int(count)
    return result

def get_optimal_recipe(item_name, recipes, bazaar_prices, auction_prices, visited=None):
    if visited is None:
        visited = set()
    if item_name in visited:
        # Circular dependency detected, treat as base item
        return {item_name: 1}
    visited = visited.copy()
    visited.add(item_name)

    def get_item_price_and_fill_time(item):
        bazaar_item = item.replace('-', ':')
        if bazaar_item in bazaar_prices:
            return bazaar_prices[bazaar_item]['price'], bazaar_prices[bazaar_item]['fill_time'], True
        elif item in auction_prices:
            return auction_prices[item], 0, True  # Auction items are assumed instant
        return float('inf'), float('inf'), False

    if item_name not in recipes:
        return {item_name: 1}

    direct_price, direct_fill_time, has_direct_price = get_item_price_and_fill_time(item_name)
    recipe = recipes[item_name][0]
    output_count = recipe.get('count', 1)
    
    total_cost = 0
    has_all_prices = True
    optimal_ingredients = Counter()
    
    for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
        if pos in recipe and recipe[pos]:
            item, count = recipe[pos].split(':')
            count = int(count)
            
            sub_recipe = get_optimal_recipe(item, recipes, bazaar_prices, auction_prices, visited)
            
            ingredient_cost = 0
            for sub_item, sub_count in sub_recipe.items():
                price, fill_time, has_price = get_item_price_and_fill_time(sub_item)
                if has_price:
                    ingredient_cost += price * sub_count * count
                else:
                    has_all_prices = False
                    
            total_cost += ingredient_cost
            
            for sub_item, sub_count in sub_recipe.items():
                optimal_ingredients[sub_item] += sub_count * count

    crafting_cost_per_item = total_cost / output_count if has_all_prices else float('inf')
    
    if has_direct_price and has_all_prices:
        if crafting_cost_per_item >= direct_price:
            return {item_name: 1}
    
    return {item: count/output_count for item, count in optimal_ingredients.items()}

# ------------------------------------------------------------------------------
# Step 2b: Sell Fill Time Calculation (New Helper Function)
# ------------------------------------------------------------------------------

def calculate_sell_fill_time(required_count: float, weekly_rate: float) -> float:
    """
    Calculate the fill time using the formula:
        fill_time = required_count * (604800 / weekly_rate)
    where 604800 is the number of seconds in a week.
    """
    if weekly_rate <= 0:
        return float('inf')
    return required_count * (604800 / weekly_rate)

# ------------------------------------------------------------------------------
# Step 2: Crafting Details Function (Price & Fill Time Logic)
# ------------------------------------------------------------------------------

def get_crafting_details(item_name, recipes, bazaar_prices, auction_prices):
    """
    Return a dict of crafting details for an item if crafting is profitable.
    Otherwise, return None.
    The overall sell-side fill time is computed via calculate_sell_fill_time():
      sell_fill_time = output_count * (604800 / weekly_rate)
    """
    if item_name not in recipes:
        return None

    recipe = recipes[item_name][0]
    output_count = recipe.get('count', 1)
    
    total_cost = 0
    ingredients = []
    
    # Process recipe ingredients
    for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
        if pos in recipe and recipe[pos]:
            parts = recipe[pos].split(':')
            ing_item = parts[0]
            count = int(parts[1]) if len(parts) > 1 else 1
            
            bazaar_item = ing_item.replace('-', ':')
            buy_method = "Not Available"
            cost_per_unit = 0
            individual_fill_time = 0
            weekly_rate = None
            
            if bazaar_item in bazaar_prices:
                bp = bazaar_prices[bazaar_item]
                cost_per_unit = bp['price']
                buy_method = f"Bazaar {bp['method']}"
                if bp['method'] == "Buy Order":
                    weekly_rate = bp['hourly_instasells'] * 168
                    # Use the total count (as given in the recipe) for fill time calculation
                    ingredient_required = count
                    individual_fill_time = calculate_sell_fill_time(ingredient_required, weekly_rate)
                else:  # Instabuy is considered instant
                    individual_fill_time = 0
            elif ing_item in auction_prices:
                cost_per_unit = auction_prices[ing_item]
                buy_method = "AH BIN (before tax)"
                individual_fill_time = 0
            else:
                return None
                    
            total_cost += cost_per_unit * count
            ingredients.append({
                "item": ing_item,
                "total_needed": count,         
                "per_item": count / output_count,
                "buy_method": buy_method,
                "cost_per_unit": cost_per_unit,
                "fill_time": individual_fill_time,
                "weekly_rate": weekly_rate
            })
    
    sell_price = None
    sell_method = None
    hourly_volume = None
    bazaar_item = item_name.replace('-', ':')
    sell_fill_time = 0
    weekly_instabuys = None
    weekly_instasells = None

    if bazaar_item in bazaar_prices:
        bp = bazaar_prices[bazaar_item]
        sell_price = bp['price']
        sell_method = f"Bazaar {bp['method']}"
        if bp['method'] == "Buy Order":
            weekly_sell_rate = bp['hourly_instasells'] * 168
            hourly_volume = bp['hourly_instasells']
            weekly_instasells = weekly_sell_rate
        else:  # Instabuy
            weekly_sell_rate = bp['hourly_instabuys'] * 168
            hourly_volume = bp['hourly_instabuys']
            weekly_instabuys = weekly_sell_rate

        sell_fill_time = calculate_sell_fill_time(output_count, weekly_sell_rate)
    elif item_name in auction_prices:
        raw_price = auction_prices[item_name]
        sell_price = calculate_bin_tax(raw_price)
        sell_method = "AH BIN (after tax)"
        hourly_volume = float('inf')
        sell_fill_time = 0
    else:
        return None
    
    crafting_cost_per_item = total_cost / output_count
    crafting_savings = sell_price - crafting_cost_per_item
    if crafting_savings <= 0:
        return None

    base_profit_per_hour = (3600 / (sell_fill_time)) * crafting_savings if sell_fill_time > 0 else 0
    max_possible_profit = crafting_savings * hourly_volume
    profit_per_hour = min(base_profit_per_hour, max_possible_profit)
    
    return {
        'item': item_name,
        'profit_per_hour': round(profit_per_hour, 2),
        'crafting_savings': round(crafting_savings, 2),
        'sell_fill_time': round(sell_fill_time, 7),
        'weekly_instabuys': weekly_instabuys,
        'weekly_instasells': weekly_instasells,
        'sell_price': round(sell_price, 2),
        'sell_method': sell_method,
        'crafting_cost': round(crafting_cost_per_item, 2),
        'ingredients': ingredients
    }

# ------------------------------------------------------------------------------
# Step 3: Query Function (Aggregates Ingredients & Displays Fill Times and Weekly Data)
# ------------------------------------------------------------------------------

def query_recipe(item_name, recipes, bazaar_prices, auction_prices):
    details = get_crafting_details(item_name, recipes, bazaar_prices, auction_prices)
    
    if details is None:
        print(f"\nNo crafting details available for '{item_name}'.")
        bazaar_item = item_name.replace('-', ':')
        if bazaar_item in bazaar_prices:
            bp = bazaar_prices[bazaar_item]
            print(f"Direct Purchase: {item_name} @ {bp['price']:,.1f} coins ({bp['method']})")
        elif item_name in auction_prices:
            raw_price = auction_prices[item_name]
            print(f"Direct Purchase: {item_name} @ {raw_price:,.1f} coins (AH BIN, after tax: {calculate_bin_tax(raw_price):,.1f})")
        else:
            print(f"No price data available for '{item_name}'.")
        return

    # Aggregate ingredient data to compute total fill times per ingredient
    aggregated = {}
    for ing in details['ingredients']:
        key = ing['item']
        if key not in aggregated:
            aggregated[key] = {
                "total_needed": 0,
                "per_item": ing['per_item'],
                "buy_method": ing['buy_method'],
                "cost_per_unit": ing['cost_per_unit'],
                "weekly_rate": ing.get("weekly_rate")
            }
        aggregated[key]["total_needed"] += ing['total_needed']
    
    # Calculate total fill time for each aggregated ingredient
    for item, data in aggregated.items():
        if data["weekly_rate"] is not None and data["weekly_rate"] > 0:
            data["total_fill_time"] = data["total_needed"] * (604800 / data["weekly_rate"])
        else:
            data["total_fill_time"] = 0
    
    # For an overall ingredient fill time, we take the maximum total fill time among ingredients.
    overall_aggregated_fill_time = max((data["total_fill_time"] for data in aggregated.values()), default=0)

    print(f"\nCrafting Details for '{details['item']}':")
    print(f"  Sell Price       : {details['sell_price']:,.1f} coins via {details['sell_method']}")
    print(f"  Crafting Cost    : {details['crafting_cost']:,.1f} coins")
    print(f"  Crafting Savings : {details['crafting_savings']:,.1f} coins")
    if details.get('weekly_instabuys') is not None:
        print(f"  Instabuys per Week: {details['weekly_instabuys']:.1f}")
    elif details.get('weekly_instasells') is not None:
        print(f"  Instasells per Week: {details['weekly_instasells']:.1f}")
    # Use overall_aggregated_fill_time instead of a separate buy_fill_time
    print(f"  Time to Make Sale: {overall_aggregated_fill_time:.2f}s to fill ingredient orders, {details['sell_fill_time']:.7f}s sell order fill")
    print(f"  Profit per Hour  : {details['profit_per_hour']:,.1f} coins (Effective Cycle Time: {overall_aggregated_fill_time + details['sell_fill_time']:.2f}s)")
    
    print("  Ingredients:")
    for item, data in aggregated.items():
        print(f"    - {item}: {data['per_item']:.1f} per final item (total {data['total_needed']:.0f}), {data['buy_method']} @ {data['cost_per_unit']:,.1f} coins each, total fill time: {data['total_fill_time']:.1f} seconds")

# ------------------------------------------------------------------------------
# Step 4: Export Function - Top 40 Bazaar Only Crafts to JSON
# ------------------------------------------------------------------------------

def export_top_40_bazaar_crafts(recipes, bazaar_prices, auction_prices, filename="top_40_bazaar_crafts.json"):
    crafts = []
    for item in recipes:
        details = get_crafting_details(item, recipes, bazaar_prices, auction_prices)
        if details is not None:
            # Only include crafts that use Bazaar pricing for the sell side
            if details['sell_method'].startswith("Bazaar"):
                # Aggregate ingredient data as in the query function
                aggregated = {}
                for ing in details['ingredients']:
                    key = ing['item']
                    if key not in aggregated:
                        aggregated[key] = {
                            "total_needed": 0,
                            "per_item": ing['per_item'],
                            "buy_method": ing['buy_method'],
                            "cost_per_unit": ing['cost_per_unit'],
                            "weekly_rate": ing.get("weekly_rate")
                        }
                    aggregated[key]["total_needed"] += ing['total_needed']
                for key, data in aggregated.items():
                    if data["weekly_rate"] is not None and data["weekly_rate"] > 0:
                        data["total_fill_time"] = data["total_needed"] * (604800 / data["weekly_rate"])
                    else:
                        data["total_fill_time"] = 0
                overall_aggregated_fill_time = max((data["total_fill_time"] for data in aggregated.values()), default=0)
                # Add aggregated data and effective cycle time to details
                details['aggregated_ingredient_fill_time'] = overall_aggregated_fill_time
                details['ingredients_aggregated'] = aggregated
                details['effective_cycle_time'] = overall_aggregated_fill_time + details['sell_fill_time']
                crafts.append(details)
    # Sort by profit_per_hour in descending order
    crafts.sort(key=lambda x: x['profit_per_hour'], reverse=True)
    top_40 = crafts[:40]
    with open(filename, "w") as f:
        json.dump(top_40, f, indent=4)
    print(f"Exported top 40 bazaar-only crafts to {filename}")

# ------------------------------------------------------------------------------
# Step 5: Main Routine (User Query & Export)
# ------------------------------------------------------------------------------

if __name__ == "__main__":
    recipes = load_recipes("dependencies/items")
    bazaar_prices = fetch_all_bazaar_prices()
    auction_prices = fetch_lbin_prices()

    # Export top 40 bazaar-only crafts to JSON
    export_top_40_bazaar_crafts(recipes, bazaar_prices, auction_prices)

    print("Welcome to the Skyblock Crafting Query Tool!")
    print("Type the item name you want to query (or 'exit' to quit).")
    while True:
        item = input("\nEnter item name: ").strip()
        if item.lower() in ["exit", "quit"]:
            print("Exiting the query tool. Goodbye!")
            break
        query_recipe(item, recipes, bazaar_prices, auction_prices)
