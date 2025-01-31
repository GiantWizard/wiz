import json
import requests
from pathlib import Path
from collections import Counter

def load_recipes(directories=["dependencies/items", "exports"]):
    recipes = {}

    for directory in directories:
        path = Path(directory)
        if not path.exists():
            print(f"Warning: Directory '{directory}' does not exist. Skipping.")
            continue

        for file in path.glob('*.json'):
            try:
                with open(file, 'r') as f:
                    data = json.load(f)
                    if 'internalname' in data:
                        if 'recipes' in data:
                            # Filter out forge, katgrade, and trade recipes
                            valid_recipes = [r for r in data['recipes'] 
                                           if not r.get('type') in ['forge', 'katgrade', 'trade']]
                            if valid_recipes:  # Only add if there are valid recipes
                                recipes[data['internalname']] = valid_recipes
                        elif 'recipe' in data:
                            recipe = data['recipe']
                            if not recipe.get('type') in ['forge', 'katgrade', 'trade']:
                                recipes[data['internalname']] = [recipe]
            except Exception as e:
                print(f"Error reading {file}: {e}")
    
    return recipes

# Add to the bazaar price fetching to include fill time metrics
import requests

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
            # Multiple items of same type can be filled simultaneously
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

    # Final coins collected
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
    
    # First, recursively get optimal recipes for all ingredients
    total_cost = 0
    has_all_prices = True
    total_fill_time = 0
    optimal_ingredients = Counter()
    
    for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
        if pos in recipe and recipe[pos]:
            item, count = recipe[pos].split(':')
            count = int(count)
            
            # Recursively get optimal recipe for this ingredient
            sub_recipe = get_optimal_recipe(item, recipes, bazaar_prices, auction_prices, visited)
            
            # Calculate cost and fill time for this ingredient's optimal recipe
            ingredient_cost = 0
            ingredient_fill_time = 0
            for sub_item, sub_count in sub_recipe.items():
                price, fill_time, has_price = get_item_price_and_fill_time(sub_item)
                if has_price:
                    ingredient_cost += price * sub_count * count
                    if bazaar_prices.get(sub_item.replace('-', ':'), {}).get('method') == "Buy Order":
                        ingredient_fill_time = max(ingredient_fill_time, fill_time)
                else:
                    has_all_prices = False
                    
            total_cost += ingredient_cost
            total_fill_time = max(total_fill_time, ingredient_fill_time)
            
            # Add ingredients to optimal recipe
            for sub_item, sub_count in sub_recipe.items():
                optimal_ingredients[sub_item] += sub_count * count

    # Calculate crafting cost per item
    crafting_cost_per_item = total_cost / output_count if has_all_prices else float('inf')
    
    # Compare direct purchase vs crafting
    if has_direct_price and has_all_prices:
        if crafting_cost_per_item >= direct_price:
            return {item_name: 1}
    
    # If we get here, either crafting is cheaper or we have no direct price
    return {item: count/output_count for item, count in optimal_ingredients.items()}

def query_recipe(item_name, recipes, bazaar_prices, auction_prices):
    print(f"Item: {item_name}\n")
    
    def get_price_string(item, count):
        """Get formatted price string for an item"""
        bazaar_item = item.replace('-', ':')
        if bazaar_item in bazaar_prices:
            price_per_unit = bazaar_prices[bazaar_item]['price']
            total_price = price_per_unit * count
            method = bazaar_prices[bazaar_item]['method']
            if method == "Buy Order":
                fill_time = bazaar_prices[bazaar_item]['fill_time']
                fill_time_str = f", fills in {fill_time:.1f}s" if fill_time != float('inf') else ", fill time unknown"
                return f"(@ {price_per_unit:,.1f} each, {method}: {total_price:,.1f} coins total{fill_time_str})"
            else:
                return f"(@ {price_per_unit:,.1f} each, {method}: {total_price:,.1f} coins total)"
        elif item in auction_prices:
            raw_price = auction_prices[item]
            tax_adjusted_price = calculate_bin_tax(raw_price)  # APPLY TAX HERE
            return f"(@ {raw_price:,.1f} each, AH BIN: {tax_adjusted_price:,.1f} coins after tax)"
        return "(No price data)"

    optimal_recipe = get_optimal_recipe(item_name, recipes, bazaar_prices, auction_prices)
    
    if optimal_recipe == {item_name: 1}:
        price_str = get_price_string(item_name, 1)
        print(f"Buy directly: {item_name} {price_str.replace('total', '').replace('each, ', '')}")
        return

    print("Optimal Recipe Breakdown:")
    total_cost = 0
    all_prices_available = True
    total_fill_time = 0
    
    for item, count in optimal_recipe.items():
        bazaar_item = item.replace('-', ':')
        if bazaar_item in bazaar_prices:
            total_cost += bazaar_prices[bazaar_item]['price'] * count
            if bazaar_prices[bazaar_item]['method'] == "Buy Order":
                total_fill_time = max(total_fill_time, bazaar_prices[bazaar_item]['fill_time'])
        elif item in auction_prices:
            total_cost += auction_prices[item] * count
        else:
            all_prices_available = False
        print(f"• {item} x{count:.1f} {get_price_string(item, count)}")
    
    if all_prices_available:
        print(f"\nTotal Crafting Cost: {total_cost:,.1f} coins")
        
        if total_fill_time > 0:
            print(f"Total Fill Time: {total_fill_time:.1f}s")
        
        bazaar_item = item_name.replace('-', ':')
        if bazaar_item in bazaar_prices:
            direct_price = bazaar_prices[bazaar_item]['price']
            method = bazaar_prices[bazaar_item]['method']
            print(f"Direct Purchase Cost: {direct_price:,.1f} coins ({method})")
            print(f"Crafting saves: {direct_price - total_cost:,.1f} coins")
        elif item_name in auction_prices:
            raw_price = auction_prices[item_name]
            direct_price_after_tax = calculate_bin_tax(raw_price)  # APPLY TAX HERE
            print(f"Direct Purchase Cost: {raw_price:,.1f} coins (AH BIN)")
            print(f"After tax: {direct_price_after_tax:,.1f} coins")
            print(f"Crafting saves: {direct_price_after_tax - total_cost:,.1f} coins")
    else:
        print("\nNote: Some items are missing price data, total cost cannot be calculated")

def calculate_profit_per_hour(recipes, bazaar_prices, auction_prices):
    
    def get_crafting_details(item_name, recipes, bazaar_prices, auction_prices):
        if item_name not in recipes:
            return None
            
        recipe = recipes[item_name][0]
        output_count = recipe.get('count', 1)
        
        total_cost = 0
        total_fill_time = 0
        ingredients = []
        
        # Process recipe ingredients
        for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
            if pos in recipe and recipe[pos]:
                parts = recipe[pos].split(':')
                item = parts[0]
                count = int(parts[1]) if len(parts) > 1 else 1
                
                # Get ingredient price details
                bazaar_item = item.replace('-', ':')
                buy_method = "Not Available"
                cost_per_unit = 0
                ingredient_fill_time = 0
                
                if bazaar_item in bazaar_prices:
                    bp = bazaar_prices[bazaar_item]
                    cost_per_unit = bp['price']
                    buy_method = f"Bazaar {bp['method']}"
                    if bp['method'] == "Buy Order":
                        ingredient_fill_time = bp['fill_time']
                elif item in auction_prices:
                    cost_per_unit = auction_prices[item]
                    buy_method = "AH BIN (before tax)"
                else:
                    return None  # Skip items with missing price data
                    
                total_cost += cost_per_unit * count
                total_fill_time = max(total_fill_time, ingredient_fill_time)
                
                ingredients.append({
                    "item": item,
                    "total_needed": count,
                    "count_per_item": count / output_count,
                    "buy_method": buy_method,
                    "cost_per_unit": cost_per_unit
                })
        
        # Get final item price details
        sell_price = None
        sell_method = None
        hourly_volume = 0
        bazaar_item = item_name.replace('-', ':')
        
        if bazaar_item in bazaar_prices:
            bp = bazaar_prices[bazaar_item]
            sell_price = bp['price']
            sell_method = f"Bazaar {bp['method']}"
            hourly_volume = bp['hourly_instabuys']
        elif item_name in auction_prices:
            raw_price = auction_prices[item_name]
            sell_price = calculate_bin_tax(raw_price)
            sell_method = "AH BIN (after tax)"
            hourly_volume = float('inf')
        else:
            return None
        
        crafting_cost_per_item = total_cost / output_count
        crafting_savings = sell_price - crafting_cost_per_item
        
        if crafting_savings <= 0:
            return None
        
        fill_time = max(total_fill_time, 0.1)
        
        # Calculate profit per hour
        base_profit_per_hour = (3600 / fill_time) * crafting_savings
        max_possible_profit = crafting_savings * hourly_volume
        profit_per_hour = min(base_profit_per_hour, max_possible_profit)
        
        return {
            'item': item_name,
            'profit_per_hour': round(profit_per_hour, 2),
            'crafting_savings': round(crafting_savings, 2),
            'fill_time': round(fill_time, 2),
            'sell_price': round(sell_price, 2),
            'sell_method': sell_method,
            'crafting_cost': round(crafting_cost_per_item, 2),
            'ingredients': [{
                "item": ing['item'],
                "count_per_item": round(ing['count_per_item'], 2),
                "buy_method": ing['buy_method'],
                "cost_per_unit": round(ing['cost_per_unit'], 2)
            } for ing in ingredients]
        }
        if item_name not in recipes:
            return None
            
        recipe = recipes[item_name][0]
        output_count = recipe.get('count', 1)
        
        total_cost = 0
        total_fill_time = 0
        ingredients = Counter()
        
        # Process recipe ingredients
        for pos in ['A1','A2','A3','B1','B2','B3','C1','C2','C3']:
            if pos in recipe and recipe[pos]:
                parts = recipe[pos].split(':')
                item = parts[0]
                count = int(parts[1]) if len(parts) > 1 else 1
                ingredients[item] += count
        
        # Calculate total cost and fill time
        for item, count in ingredients.items():
            bazaar_item = item.replace('-', ':')
            if bazaar_item in bazaar_prices:
                total_cost += bazaar_prices[bazaar_item]['price'] * count
                if bazaar_prices[bazaar_item]['method'] == "Buy Order":
                    total_fill_time = max(total_fill_time, bazaar_prices[bazaar_item]['fill_time'])
            elif item in auction_prices:
                total_cost += auction_prices[item] * count
            else:
                return None  # Missing data, skip item
                
        # Get final item price and hourly volume
        sell_price = None
        sell_method = None
        hourly_volume = 0
        bazaar_item = item_name.replace('-', ':')
        if bazaar_item in bazaar_prices:
            sell_price = bazaar_prices[bazaar_item]['price']
            sell_method = f"Bazaar {bazaar_prices[bazaar_item]['method']}"
            hourly_volume = bazaar_prices[bazaar_item]['hourly_instabuys']
        elif item_name in auction_prices:
            raw_price = auction_prices[item_name]
            sell_price = calculate_bin_tax(raw_price)
            sell_method = "AH BIN (after tax)"
            hourly_volume = float('inf')
        else:
            return None  # Skip if no price available
        
        crafting_cost_per_item = total_cost / output_count
        crafting_savings = sell_price - crafting_cost_per_item
        if crafting_savings <= 0:
            return None  # Not profitable, skip
        
        fill_time = max(total_fill_time, 0.1)  # Minimum 0.1s
        
        # Calculate profit per hour
        base_profit_per_hour = (3600 / fill_time) * crafting_savings
        max_possible_profit = crafting_savings * hourly_volume
        profit_per_hour = min(base_profit_per_hour, max_possible_profit)
        
        # Prepare ingredients list
        ingredients_list = []
        for item, count in ingredients.items():
            count_per_item = count / output_count
            ingredients_list.append({
                "item": item,
                "count_per_item": round(count_per_item, 2)
            })
        
        return {
            'item': item_name,
            'profit_per_hour': round(profit_per_hour, 2),
            'crafting_savings': round(crafting_savings, 2),
            'fill_time': round(fill_time, 2),
            'sell_price': round(sell_price, 2),
            'sell_method': sell_method,
            'crafting_cost': round(crafting_cost_per_item, 2),
            'ingredients': ingredients_list
        }

    print("Calculating profitable crafting recipes (with AH BIN tax)...")
    
    profitable_items = []
    for item in recipes:
        result = get_crafting_details(item, recipes, bazaar_prices, auction_prices)
        if result:
            profitable_items.append(result)
    
    # Sort by profit per hour descending
    profitable_items.sort(key=lambda x: x['profit_per_hour'], reverse=True)
    return profitable_items[:40]
        
def query_items():
    while True:
        print("\nWould you like to see:")
        print("1. Only Bazaar craftable items")
        print("2. All craftable items (including Auction House with tax)")
        try:
            choice = int(input("Enter your choice (1 or 2): "))
            if choice in [1, 2]:
                break
            print("Please enter 1 or 2")
        except ValueError:
            print("Please enter 1 or 2")
    
    bazaar_only = choice == 1
    # First display the profit list based on choice
    calculate_profit_per_hour(recipes, bazaar_prices, auction_prices if not bazaar_only else {})
    
    # Then start querying for specific items
    while True:
        item_name = input("\nEnter item name to see recipe (or 'quit' to exit): ").strip().upper()
        if item_name.lower() == 'quit':
            break
        query_recipe(item_name, recipes, bazaar_prices, auction_prices)  # ✅ FIXED!

# Usage:
if __name__ == "__main__":
    # Load data
    recipes = load_recipes()
    bazaar_prices = fetch_all_bazaar_prices()
    auction_prices = fetch_lbin_prices()

    # Generate Bazaar-only data
    bazaar_only = calculate_profit_per_hour(recipes, bazaar_prices, {})
    
    # Generate AH-included data
    ah_included = calculate_profit_per_hour(recipes, bazaar_prices, auction_prices)

    # Save results to JSON files
    with open('bazaar_profitable_items.json', 'w') as f:
        json.dump(bazaar_only, f, indent=2)
    
    with open('ah_included_profitable_items.json', 'w') as f:
        json.dump(ah_included, f, indent=2)