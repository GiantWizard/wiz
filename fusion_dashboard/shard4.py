# shard4.py

import json
from flask import Flask, render_template

app = Flask(__name__)

FINAL_DATA_FILE = "fusion_recipes_with_prices.json"

def load_recipe_data():
    """Loads the final, validated recipe data file."""
    try:
        with open(FINAL_DATA_FILE, 'r') as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return None

def analyze_and_categorize_strategies(all_recipe_data):
    """Analyzes the data, now accounting for variable output quantities."""
    if not all_recipe_data: return {}
    
    strategy_keys = [
        "ib_both_is_prod", "ib_c1_bo_c2_is_prod", "bo_c1_ib_c2_is_prod", "bo_both_is_prod",
        "ib_both_so_prod", "ib_c1_bo_c2_so_prod", "bo_c1_ib_c2_so_prod", "bo_both_so_prod"
    ]
    categorized_profits = {key: [] for key in strategy_keys}
    def safe_subtract(a, b): return a - b if a is not None and b is not None else None
    
    for target_key, item_data in all_recipe_data.items():
        market_price = item_data.get("market_price", {})
        # These are the base prices for ONE shard
        insta_sell_price = market_price.get("insta_sell_revenue")
        sell_order_price = market_price.get("insta_buy_cost")

        for recipe in item_data.get("recipes", []):
            # --- CHANGE: START ---
            # Get the output quantity for THIS specific recipe (defaults to 1.0 if not present)
            output_quantity = recipe.get("produces", {}).get("quantity", 1.0)
            
            # Calculate the total revenue for this fusion by multiplying price by quantity
            total_insta_sell_revenue = insta_sell_price * output_quantity if insta_sell_price is not None else None
            total_sell_order_revenue = sell_order_price * output_quantity if sell_order_price is not None else None
            # --- CHANGE: END ---
            
            craft_costs = recipe.get("cost_summary", {})
            
            # --- CHANGE: START ---
            # Use the new total revenue variables for profit calculation
            profits = {
                "ib_both_is_prod": safe_subtract(total_insta_sell_revenue, craft_costs.get("cost_instabuy_both")),
                "bo_c1_ib_c2_is_prod": safe_subtract(total_insta_sell_revenue, craft_costs.get("cost_instabuy_c1_buy_order_c2")),
                "ib_c1_bo_c2_is_prod": safe_subtract(total_insta_sell_revenue, craft_costs.get("cost_buy_order_c1_instabuy_c2")),
                "bo_both_is_prod": safe_subtract(total_insta_sell_revenue, craft_costs.get("cost_buy_order_both")),
                "ib_both_so_prod": safe_subtract(total_sell_order_revenue, craft_costs.get("cost_instabuy_both")),
                "bo_c1_ib_c2_so_prod": safe_subtract(total_sell_order_revenue, craft_costs.get("cost_instabuy_c1_buy_order_c2")),
                "ib_c1_bo_c2_so_prod": safe_subtract(total_sell_order_revenue, craft_costs.get("cost_buy_order_c1_instabuy_c2")),
                "bo_both_so_prod": safe_subtract(total_sell_order_revenue, craft_costs.get("cost_buy_order_both")),
            }
            # --- CHANGE: END ---

            for key, profit in profits.items():
                if profit is not None and profit > 0:
                    c1, c2 = recipe["recipe_components"]
                    recipe_str = f"{c1['quantity']}x {c1['name']} + {c2['quantity']}x {c2['name']}"
                    categorized_profits[key].append({"target": target_key, "recipe_str": recipe_str, "profit": profit})

    final_report = {}
    for key, profit_list in categorized_profits.items():
        profit_list.sort(key=lambda x: x['profit'], reverse=True)
        unique_top_items = []
        seen_targets = set()
        for item in profit_list:
            if item['target'] not in seen_targets:
                item['profit_str'] = f"{item['profit']:,.0f}"
                unique_top_items.append(item)
                seen_targets.add(item['target'])
        final_report[key] = unique_top_items
    return final_report

@app.route('/')
def index():
    """The main page of the web app."""
    HEADERS = {
        "ib_both_is_prod": "1. Insta-Buy Comps -> Insta-Sell Product", "bo_c1_ib_c2_is_prod": "2. Mixed (BO/IB) -> IS", "ib_c1_bo_c2_is_prod": "3. Mixed (IB/BO) -> IS",
        "bo_both_is_prod": "4. Buy-Order Comps -> Insta-Sell Product", "ib_both_so_prod": "5. Insta-Buy Comps -> Sell-Order Product", "bo_c1_ib_c2_so_prod": "6. Mixed (BO/IB) -> SO",
        "ib_c1_bo_c2_so_prod": "7. Mixed (IB/BO) -> SO", "bo_both_so_prod": "8. Buy-Order Comps -> Sell-Order Product",
    }
    full_data = load_recipe_data()
    if not full_data:
        return "<h1>Waiting for initial data... The page will refresh automatically.</h1>", 202
    
    categorized_data = analyze_and_categorize_strategies(full_data)
    return render_template('index.html', report_data=categorized_data, headers=HEADERS, num_items_to_show=10)

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5001, debug=False) # Changed port to 5000 for consistency