import json
import requests
from collections import defaultdict

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
def print_recipe_tree(tree, prices, level=0, multiplier=1):
    indent = "  " * level
    note = f" ({tree['note']})" if "note" in tree else ""
    total_count = tree["count"] * multiplier

    price_info = prices.get(tree["name"], {})
    price = price_info.get("price", 0)
    method = price_info.get("method", None)

    # If it's an auction item (no bazaar price), use the cost from the tree
    if price == 0 and "cost" in tree and tree.get("note", "").startswith("base item (from auction)"):
        price = tree["cost"]
        method = "Auction"

    if price > 0:
        unit_price = f"{price:,.2f} per unit"
        total_price = price * total_count
        price_info = f" ({total_count:,.2f} @ {total_price:,.2f} - {method})"
    else:
        unit_price = "No price"
        price_info = ""

    print(f"{indent}- {tree['name']} x{total_count:,.2f}{note} {unit_price}{price_info}")

    for child in tree.get("children", []):
        print_recipe_tree(child, prices, level + 1, total_count)

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
try:
    data = load_data()
    prices = fetch_all_bazaar_prices()
    lbin_data = fetch_lbin_prices()
    
    while True:
        print("\nOptions:")
        print("1. View recipe tree and craft cost")
        print("2. Exit")
        
        choice = input("Enter your choice (1-2): ")

        if choice == "2":
            break
        
        if choice == "1":
            item_name = input("\nEnter the item name: ")
            item_id = get_item_id(data, item_name)
            if item_id:
                print(f"\nItem ID for '{item_name}': {item_id}\n")
                recipe_tree = build_recipe_tree(data, item_id, prices, lbin_data)
                print("Recipe Tree:")
                print_recipe_tree(recipe_tree, prices)

                raw_items = collect_raw_items(recipe_tree)
                total_price = 0
                
                longitem = find_longest_to_fill(raw_items, prices)

                print("\n--- Raw Items Needed ---")
                for item, quantity in raw_items.items():
                    recipe = data[item_id]["recipe"]
                    output_count = int(recipe.get("count", 1))
                    price_info = prices.get(item, {})
                    price = price_info.get("price", 0)
                    method = price_info.get("method", "N/A")
                    if price == 0:  # Check Auctions if Bazaar price is not found
                        price = fetch_lowest_auction_price(item, lbin_data) or 0
                        method = "Auction" if price > 0 else "N/A"

                    if price > 0:
                        total_price += price * quantity
                        print(f"- {item}: {quantity:,.2f} @ {price:,.2f} each = {price * quantity:,.2f} ({method})")
                    else:
                        print(f"- {item}: {quantity:,.2f} (No price available)")
                
                final = total_price

                # Selling Price from Bazaar or Auction
                sell_price = prices.get(item_id, {}).get("price", 0)
                profit = sell_price - final if sell_price else "N/A"
                profit_percentage = ((sell_price - final) / final) * 100 if isinstance(profit, (int, float)) and final > 0 else "N/A"

                print(f"\nTotal cost of raw items: {final:,.2f}")
                if sell_price:
                    print(f"Selling Price: {sell_price:,.2f}")
                print(f"Profit: {profit:,.2f}" if isinstance(profit, (int, float)) else "Profit: N/A")
                print(f"Profit Percentage: {profit_percentage:,.2f}%" if isinstance(profit_percentage, (int, float)) else "Profit Percentage: N/A")
                
                # Calculate and display coins per hour
                hourly_instabuys = prices.get(item_id, {}).get("hourly_instabuys", 0)
                if isinstance(profit, (int, float)) and hourly_instabuys > 0:
                    coins_per_hour = profit * hourly_instabuys
                    print(f"Coins Per Hour: {coins_per_hour:,.2f}")
                else:
                    print("Coins Per Hour: N/A")

                # Longest to fill calculation
                longest_to_fill = find_longest_to_fill(raw_items, prices)
                if longest_to_fill:
                    print(f"\nSubitem taking the longest to fill: {longest_to_fill['item']}\n  Quantity: {longest_to_fill['quantity']:,.2f}\n  Price: {longest_to_fill['price']:,.2f}\n  Time to fill: {longest_to_fill['time_to_fill']:,.2f} hours\n  Method: {longest_to_fill['method']}")

                # Profit per hour calculation
                if longest_to_fill and "time_to_fill" in longest_to_fill:
                    time_to_fill = longest_to_fill["time_to_fill"]
                    sales_method = prices.get(item_id, {}).get("method", "")
                    hourly_instabuys = prices.get(item_id, {}).get("hourly_instabuys", 0)
                    
                    # Calculate sales per hour based on instabuy rate
                    if hourly_instabuys > 0:
                        sales_per_hour = hourly_instabuys
                        time_to_sell = 1 / hourly_instabuys
                        total_time = time_to_fill + time_to_sell
                        profit_per_hour = profit * sales_per_hour
                    else:
                        sales_per_hour = 0
                        profit_per_hour = 0
                    
                    print(f"\nSales Method: {sales_method}")
                    print(f"Hourly Instabuys: {hourly_instabuys:,.2f}")
                    print(f"Sales Per Hour: {sales_per_hour:,.2f}")
                    print(f"Time to Fill Orders: {time_to_fill:,.2f} hours")
                    print(f"Time to Sell One Item: {time_to_sell:,.2f} hours")
                    print(f"Total Time Per Item: {total_time:,.2f} hours")
                    print(f"Profit Per Hour: {profit_per_hour:,.2f}")

            else:
                print(f"Item '{item_name}' not found in the data.")

except Exception as e:
    print(f"An error occurred: {e}")