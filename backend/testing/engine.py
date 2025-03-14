import json
import requests
from pathlib import Path
from collections import defaultdict

def load_recipes(directory="dependencies/items"):
    """Load recipes from JSON files in the specified directory."""
    recipes = {}
    path = Path(directory)
    if not path.exists():
        print(f"Warning: Directory '{directory}' does not exist. Skipping.")
        return recipes

    for file in path.glob('*.json'):
        try:
            with open(file, 'r') as f:
                data = json.load(f)
                if 'internalname' in data:
                    if 'recipes' in data:
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
    """Fetch current prices from the Hypixel Bazaar API."""
    url = 'https://api.hypixel.net/skyblock/bazaar'
    response = requests.get(url).json()
    if "products" not in response:
        raise Exception("Failed to fetch Bazaar data")

    prices = {}
    for item_id, details in response["products"].items():
        buy_summary = details.get("buy_summary", [])
        buy_price = buy_summary[0]["pricePerUnit"] if buy_summary else None

        sell_summary = details.get("sell_summary", [])
        sell_price = sell_summary[0]["pricePerUnit"] if sell_summary else None

        quick_status = details.get("quick_status", {})
        hourly_instasells = quick_status.get("sellMovingWeek", 0) / 168
        hourly_instabuys = quick_status.get("buyMovingWeek", 0) / 168

        if buy_price and sell_price:
            prices[item_id] = {
                "buy_price": buy_price,
                "sell_price": sell_price,
                "hourly_instasells": hourly_instasells,
                "hourly_instabuys": hourly_instabuys
            }

    return prices

def is_valid_recipe(recipe, item_name):
    """
    Check if a recipe is valid for an item (not circular).
    A recipe is invalid if the item appears in its own ingredients.
    """
    for pos in ['A1', 'A2', 'A3', 'B1', 'B2', 'B3', 'C1', 'C2', 'C3']:
        if pos in recipe and recipe[pos]:
            parts = recipe[pos].split(':')
            ing_item = parts[0]
            if ing_item == item_name:
                return False
    return True

def get_recipe_tree(item_name, recipes, visited=None, max_depth=20):
    """
    Recursively builds a recipe tree for an item, with protection against circular dependencies.
    """
    if visited is None:
        visited = set()
    
    # Check for circular dependencies
    if item_name in visited:
        return {
            "item": item_name,
            "is_circular": True,
            "ingredients": []
        }
    
    # Add item to visited set
    visited = visited.copy()
    visited.add(item_name)
    
    # If item doesn't have a recipe or we've reached max depth
    if item_name not in recipes or max_depth <= 0:
        return {
            "item": item_name,
            "is_base": True,
            "ingredients": []
        }
    
    # Filter out recipes where the item is part of its own recipe (circular)
    valid_recipes = [r for r in recipes[item_name] if is_valid_recipe(r, item_name)]
    
    if not valid_recipes:
        return {
            "item": item_name,
            "is_base": True,
            "ingredients": []
        }
    
    # Get the first valid recipe for this item
    recipe = valid_recipes[0]
    output_count = recipe.get('count', 1)
    
    result = {
        "item": item_name,
        "output_count": output_count,
        "ingredients": []
    }
    
    # Process all recipe slots and gather ingredients
    ingredient_counts = defaultdict(int)
    for pos in ['A1', 'A2', 'A3', 'B1', 'B2', 'B3', 'C1', 'C2', 'C3']:
        if pos in recipe and recipe[pos]:
            parts = recipe[pos].split(':')
            ing_item = parts[0]
            count = int(parts[1]) if len(parts) > 1 else 1
            ingredient_counts[ing_item] += count
    
    # Add each unique ingredient with total count
    for ing_item, total_count in ingredient_counts.items():
        # Only recurse if we're not at max depth
        if max_depth > 1:
            sub_tree = get_recipe_tree(ing_item, recipes, visited, max_depth - 1)
        else:
            sub_tree = {"item": ing_item, "is_base": True, "ingredients": []}
            
        result["ingredients"].append({
            "item": ing_item,
            "count": total_count,
            "sub_recipe": sub_tree
        })
    
    return result

def flatten_recipe(recipe_tree):
    """Flatten a recipe into basic ingredient counts, handling circular dependencies."""
    result = defaultdict(int)
    
    def process_ingredient(tree, multiplier=1):
        # Skip circular dependencies
        if tree.get("is_circular", False):
            result[tree["item"]] += multiplier
            return
        
        # Process base items (no recipe or max depth reached)
        if tree.get("is_base", False) or not tree.get("ingredients"):
            result[tree["item"]] += multiplier
            return
        
        # Process ingredients recursively
        for ing in tree["ingredients"]:
            item = ing["item"]
            count = ing["count"]
            sub_recipe = ing["sub_recipe"]
            
            # If sub_recipe has ingredients, process them
            if sub_recipe.get("ingredients") and not sub_recipe.get("is_circular", False) and not sub_recipe.get("is_base", False):
                output_count = sub_recipe.get("output_count", 1)
                new_multiplier = multiplier * count / output_count
                process_ingredient(sub_recipe, new_multiplier)
            else:
                # Base item or circular dependency
                result[item] += multiplier * count
    
    process_ingredient(recipe_tree)
    return dict(result)

def format_price(price):
    """Format price for display."""
    if price >= 1000000000:
        return f"{price/1000000000:.2f}B coins"
    elif price >= 1000000:
        return f"{price/1000000:.2f}M coins"
    elif price >= 1000:
        return f"{price/1000:.2f}K coins"
    else:
        return f"{price:.2f} coins"

def print_flattened_recipe_with_prices(flattened_recipe, bazaar_prices):
    """Print a flattened recipe with prices from bazaar data."""
    total_cost = 0
    
    print("\nIngredients with prices:")
    for item, count in sorted(flattened_recipe.items()):
        price_info = bazaar_prices.get(item, {})
        buy_price = price_info.get("buy_price", 0)
        
        item_total = buy_price * count
        total_cost += item_total
        
        if count % 1 == 0:  # If it's a whole number
            count_display = int(count)
        else:
            count_display = f"{count:.2f}"
            
        price_display = format_price(buy_price)
        total_display = format_price(item_total)
        
        print(f"• {item} x{count_display} - {price_display} each = {total_display}")
    
    print(f"\nEstimated total cost: {format_price(total_cost)}")
    return total_cost

def query_recipe_with_prices(item_name, recipes, bazaar_prices, depth=3):
    """
    Query and display a recipe with bazaar prices for the specified item.
    
    Args:
        item_name (str): The name of the item to query
        recipes (dict): Dictionary of all available recipes
        bazaar_prices (dict): Dictionary of bazaar prices
        depth (int): Maximum recursion depth for recipe calculation
    """
    print(f"\nRecipe for: {item_name}")
    
    if item_name not in recipes:
        print(f"No recipe found for {item_name}")
        return {}
    
    # Filter out recipes where the item is in its own recipe (circular)
    valid_recipes = [r for r in recipes[item_name] if is_valid_recipe(r, item_name)]
    
    if not valid_recipes:
        print(f"No valid recipe found for {item_name} (may be circular)")
        return {}
    
    # Get direct price from bazaar if available
    if item_name in bazaar_prices:
        direct_price = bazaar_prices[item_name].get("buy_price", 0)
        print(f"Direct purchase price: {format_price(direct_price)}")
    
    recipe_tree = get_recipe_tree(item_name, recipes, max_depth=depth)
    flattened = flatten_recipe(recipe_tree)
    total_cost = print_flattened_recipe_with_prices(flattened, bazaar_prices)
    
    # Compare with direct purchase if available
    if item_name in bazaar_prices:
        direct_price = bazaar_prices[item_name].get("buy_price", 0)
        if direct_price > 0:
            savings = direct_price - total_cost
            if savings > 0:
                print(f"\nSavings by crafting: {format_price(savings)} ({(savings/direct_price)*100:.2f}%)")
            else:
                print(f"\nCheaper to buy directly by: {format_price(-savings)} ({(-savings/direct_price)*100:.2f}%)")
    
    return flattened

if __name__ == "__main__":
    recipes = load_recipes("dependencies/items")
    bazaar_prices = fetch_all_bazaar_prices()
    
    # Example usage
    item_to_query = input("Enter item name to query: ")
    flattened_recipe = query_recipe_with_prices(item_to_query, recipes, bazaar_prices, depth=3)