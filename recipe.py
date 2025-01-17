import json
import requests
from collections import defaultdict
import os
import sys
import traceback

# Load the JSON data from a local file
def load_data():
    with open("data.json", "r") as file:
        return json.load(file)

# Fetch all Bazaar prices in one call and cache them
def fetch_all_bazaar_prices():
    url = 'https://api.hypixel.net/skyblock/bazaar'
    response = requests.get(url).json()
    if "products" not in response:
        raise Exception("Failed to fetch Bazaar data")

    prices = {}
    for item_id, details in response["products"].items():
        quick_status = details.get("quick_status", {})
        buy_price = quick_status.get("buyPrice")
        sell_price = quick_status.get("sellPrice")
        hourly_instabuys = quick_status.get("sellMovingWeek", 0) / 168
        hourly_instasells = quick_status.get("buyMovingWeek", 0) / 168

        if buy_price and sell_price:
            if (hourly_instabuys < hourly_instasells):
                prices[item_id] = {"price": buy_price, "price2": sell_price, "method": "Instabuy", "hourly_instabuys": hourly_instabuys, "hourly_instasells": hourly_instasells}
            elif (buy_price / sell_price > 1.07):
                prices[item_id] = {"price": buy_price, "price2": sell_price, "method": "Buy Order", "hourly_instabuys": hourly_instabuys, "hourly_instasells": hourly_instasells}
            else:
                prices[item_id] = {"price": sell_price, "price2": buy_price, "method": "Buy Order", "hourly_instabuys": hourly_instabuys, "hourly_instasells": hourly_instasells}

    return prices

# Fetch all items and prices from Moulberry's Lowest BIN JSON
def fetch_lbin_prices():
    url = "http://moulberry.codes/lowestbin.json"
    response = requests.get(url).json()
    return response

# Fetch the lowest BIN price for an item
def fetch_lowest_auction_price(item_name, lbin_data):
    return lbin_data.get(item_name.upper(), None)

# Search for itemID by name in the loaded data
def get_item_id(data, item_name):
    for item_id, details in data.items():
        if details.get("name") == item_name:
            return item_id
    return None

# Build recipe tree
def build_recipe_tree(data, item_id, prices, lbin_data, visited=None):
    if visited is None:
        visited = set()

    if item_id in visited:
        price_info = prices.get(item_id, {"price": 0})
        return {
            "name": item_id,
            "count": 1,
            "note": "base item (cycle detected)",
            "cost": price_info.get("price", 0)
        }

    if item_id not in data or "recipe" not in data[item_id]:
        price_info = prices.get(item_id, {"price": 0})
        if price_info["price"] == 0:  # If not found in Bazaar, check Auctions
            auction_price = fetch_lowest_auction_price(item_id, lbin_data) or 0
            return {
                "name": item_id, 
                "count": 1,
                "note": "base item (from auction)" if auction_price else "base item (no price)",
                "cost": auction_price
            }
        else:
            return {"name": item_id, "count": 1, "note": "base item", "cost": price_info["price"]}

    recipe = data[item_id]["recipe"]
    output_count = int(recipe.get("count", 1))
    
    # Skip crafting if recipe produces multiple items
    if output_count > 1:
        price_info = prices.get(item_id, {"price": 0})
        return {"name": item_id, "count": 1, "note": "base item (multiple output)", "cost": price_info["price"]}

    merged_ingredients = defaultdict(int)
    visited.add(item_id)

    for ingredient in recipe.values():
        if isinstance(ingredient, str) and ":" in ingredient:
            name, _, count = ingredient.partition(":")
            count = int(count) if count.isdigit() else 1
            merged_ingredients[name] += count

    # Check if all components have no price
    all_components_no_price = True
    for name in merged_ingredients:
        price = prices.get(name, {}).get("price", 0)
        auction_price = fetch_lowest_auction_price(name, lbin_data) or 0
        if price > 1 or auction_price > 1:
            all_components_no_price = False
            break

    # If all components have no price but the item itself has a price, return it as base item
    if all_components_no_price:
        price_info = prices.get(item_id, {})
        bazaar_price = price_info.get("price", 0)
        auction_price = fetch_lowest_auction_price(item_id, lbin_data) or 0
        if bazaar_price > 1 or auction_price > 1:
            visited.remove(item_id)
            return {
                "name": item_id,
                "count": 1,
                "note": "base item (components have no price)",
                "cost": bazaar_price if bazaar_price > 1 else auction_price
            }

    tree = {"name": item_id, "children": [], "count": 1}
    total_craft_cost = 0

    # Check if the final item has a price
    final_price = prices.get(item_id, {}).get("price", 0)
    if final_price <= 1:
        auction_price = fetch_lowest_auction_price(item_id, lbin_data) or 0
        if auction_price <= 1:
            # Try to find the highest level component with a price
            highest_priced_component = None
            highest_price = 0
            
            for name, count in merged_ingredients.items():
                child = build_recipe_tree(data, name, prices, lbin_data, visited)
                if child and child.get("cost", 0) > 1:  # Found a component with price
                    if child["cost"] > highest_price:
                        highest_price = child["cost"]
                        highest_priced_component = child
                tree["children"].append(child)

            visited.remove(item_id)
            if highest_priced_component:
                return highest_priced_component  # Return the highest-priced component as base item
            return {"name": item_id, "count": 1, "note": "base item (no price)", "cost": 0}

    # Calculate normal crafting costs
    for name, count in merged_ingredients.items():
        child = build_recipe_tree(data, name, prices, lbin_data, visited)
        if child:  # Make sure child exists
            child["count"] = count
            total_craft_cost += child.get("cost", 0) * count
            tree["children"].append(child)

    bazaar_price = prices.get(item_id, {}).get("price", 0)
    
    # Calculate fill time for buying directly
    hourly_instasells = prices.get(item_id, {}).get("hourly_instasells", 0)
    direct_fill_time = 1 / hourly_instasells if hourly_instasells > 0 else float('inf')

    # Compare costs and decide whether to craft or buy
    if bazaar_price > 0:
        if output_count > 1:
            if total_craft_cost < bazaar_price:
                tree["cost"] = total_craft_cost
                tree["note"] = f"crafting ({output_count} outputs)"
            else:
                tree = {
                    "name": item_id,
                    "count": 1,
                    "note": "base item (multiple output)",
                    "cost": bazaar_price
                }
        else:
            price_difference = (total_craft_cost - bazaar_price) / bazaar_price
            total_items = sum(count for _, count in merged_ingredients.items())
            
            # For items under 1000 coins, only allow "price close" if less than 80 items
            if bazaar_price < 1000 and total_items >= 80:
                # Always buy directly if crafting cost is higher
                if total_craft_cost >= bazaar_price:
                    tree = {
                        "name": item_id,
                        "count": 1,
                        "note": "purchased directly",
                        "cost": bazaar_price
                    }
                else:
                    tree["cost"] = total_craft_cost
                    tree["note"] = "crafting"
            else:
                # For expensive items or small recipes
                if total_craft_cost >= bazaar_price:
                    # If crafting is more expensive, always buy directly
                    tree = {
                        "name": item_id,
                        "count": 1,
                        "note": "purchased directly",
                        "cost": bazaar_price
                    }
                else:
                    # Only show crafting if it's cheaper
                    tree["cost"] = total_craft_cost
                    tree["note"] = "crafting"
    else:
        tree["cost"] = total_craft_cost
        tree["note"] = "crafting (no bazaar price)"

    visited.remove(item_id)
    return tree


# Print the recipe tree with multipliers, prices, and formatting
def print_recipe_tree(tree, prices):
    output = {
        "recipe_tree": _build_tree_structure(tree, prices),
        "raw_materials": [],
        "longest_fill": None,
        "financials": {}
    }

    # Collect raw items
    raw_items = collect_raw_items(tree)
    total_price = 0
    longest_item = None
    longest_time = 0
    
    # Process raw items and calculate total cost
    for item, quantity in raw_items.items():
        price_info = prices.get(item, {})
        price = price_info.get("price", 0)
        method = price_info.get("method", "N/A")
        hourly_instasells = price_info.get("hourly_instasells", 0)
        
        # Calculate fill time for bazaar items
        if method in ["Buy Order", "Instabuy"]:
            fill_time = quantity / hourly_instasells if hourly_instasells > 0 else float('inf')
            if method != "Instabuy" and fill_time > longest_time:
                longest_time = fill_time
                longest_item = {
                    "name": item,
                    "quantity": quantity,
                    "price": price,
                    "time": fill_time,
                    "method": method,
                    "hourly_rate": hourly_instasells
                }
        
        # Add raw material to output
        material = {
            "item": item,
            "quantity": quantity,
            "price": price,
            "total_cost": price * quantity if price > 0 else 0,
            "method": method
        }
        output["raw_materials"].append(material)
        if price > 0:
            total_price += price * quantity

    # Add longest fill information
    if longest_item:
        output["longest_fill"] = {
            "item": longest_item["name"],
            "quantity": longest_item["quantity"],
            "time": longest_item["time"],
            "method": longest_item["method"]
        }

    # Add financial information
    sell_price = prices.get(tree["name"], {}).get("price", 0)
    output["financials"] = {
        "total_cost": total_price,
        "sell_price": sell_price
    }
    
    if sell_price:
        profit = sell_price - total_price
        output["financials"].update({
            "profit": profit,
            "profit_percentage": (profit/total_price*100) if total_price > 0 else 0
        })
        
        hourly_instabuys = prices.get(tree["name"], {}).get("hourly_instabuys", 0)
        if hourly_instabuys > 0:
            output["financials"]["coins_per_hour"] = profit * hourly_instabuys

    # Print the final JSON
    print(json.dumps(output, indent=2))

def _build_tree_structure(tree, prices, level=0):
    """Helper function to build tree structure as JSON"""
    node = {
        "name": tree["name"],
        "count": tree["count"]
    }
    
    if "note" in tree:
        node["note"] = tree["note"]
    
    price_info = prices.get(tree["name"], {})
    price = price_info.get("price", 0)
    method = price_info.get("method", None)

    if price > 0:
        node.update({
            "unit_price": price,
            "total_price": price * tree["count"],
            "method": method
        })

    if "children" in tree and tree["children"]:
        node["children"] = [_build_tree_structure(child, prices, level + 1) for child in tree["children"]]

    return node

# Collect raw items recursively
def collect_raw_items(tree, multiplier=1, raw_items=None):
    if raw_items is None:
        raw_items = defaultdict(float)

    if "children" not in tree or not tree["children"] or tree.get("note") == "purchased directly":
        if "cycle detected" in str(tree.get("note", "")):
            raw_items[tree["name"]] += 1  # Always add just 1 for cycle-detected items
        else:
            raw_items[tree["name"]] += tree["count"] * multiplier
        return raw_items

    for child in tree.get("children", []):
        collect_raw_items(child, multiplier * tree["count"], raw_items)

    return raw_items

# Find the subitem that takes the longest to fill
def find_longest_to_fill(raw_items, prices):
    longest_item = None
    longest_time = 0

    for item, quantity in raw_items.items():
        method = prices.get(item, {}).get("method", "")
        if method == "Instabuy":
            continue  # Skip items bought via instabuy

        hourly_instasells = prices.get(item, {}).get("hourly_instasells", 0)
        if hourly_instasells > 0:
            # Use quantity 1 for cycle-resolved items
            actual_quantity = 1 if prices.get(item, {}).get("note") == "base item (cycle resolved)" else quantity
            time_to_fill = actual_quantity / hourly_instasells
            if time_to_fill > longest_time:
                longest_time = time_to_fill
                longest_item = {
                    "item": item,
                    "quantity": actual_quantity,
                    "price": prices.get(item, {}).get("price", 0),
                    "time_to_fill": longest_time,
                    "method": method
                }

    return longest_item


# Main execution
if __name__ == "__main__":
    try:
        item_id = os.getenv('ITEM_ID')
        if not item_id:
            print(json.dumps({"error": "Waiting for item ID..."}))
            sys.exit(0)  # Exit cleanly, not an error

        print(f"Debug: Processing item ID: {item_id}", file=sys.stderr)
        
        data = load_data()
        prices = fetch_all_bazaar_prices()
        lbin_data = fetch_lbin_prices()
        
        recipe_tree = build_recipe_tree(data, item_id, prices, lbin_data)
        output = {
            "recipe_tree": recipe_tree,
            "raw_materials": [],
            "longest_fill": None,
            "financials": {}
        }
        
        # Process raw materials
        raw_items = collect_raw_items(recipe_tree)
        total_cost = 0
        
        for item, quantity in raw_items.items():
            price_info = prices.get(item, {})
            price = price_info.get("price", 0)
            method = price_info.get("method", "N/A")
            
            material = {
                "item": item,
                "quantity": quantity,
                "price": price,
                "total_cost": price * quantity if price > 0 else 0,
                "method": method
            }
            output["raw_materials"].append(material)
            if price > 0:
                total_cost += price * quantity
        
        # Find longest fill time
        longest = find_longest_to_fill(raw_items, prices)
        if longest:
            output["longest_fill"] = longest
        
        # Add financial information
        sell_price = prices.get(item_id, {}).get("price", 0)
        output["financials"] = {
            "total_cost": total_cost,
            "sell_price": sell_price
        }
        
        if sell_price:
            profit = sell_price - total_cost
            output["financials"].update({
                "profit": profit,
                "profit_percentage": (profit/total_cost*100) if total_cost > 0 else 0
            })
            
            hourly_instabuys = prices.get(item_id, {}).get("hourly_instabuys", 0)
            if hourly_instabuys > 0:
                output["financials"]["coins_per_hour"] = profit * hourly_instabuys
        
        print(json.dumps(output))

    except Exception as e:
        print(json.dumps({
            "error": str(e),
            "traceback": traceback.format_exc()
        }), file=sys.stderr)
        sys.exit(1)