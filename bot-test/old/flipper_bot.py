import os
import requests
import csv
import re
import discord
from discord.ext import commands, tasks
from discord.ui import View, Select
from dotenv import load_dotenv

# --- Load Environment Variables ---
load_dotenv()
TOKEN = os.getenv("DISCORD_TOKEN")
CSV_FILENAME = "fusion_list.csv" # The name of your recipe file

# --- Bot Setup ---
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents)

# --- Global Cache for Profit Report ---
profit_report_cache = None
parsed_recipes = [] # This will hold all recipes from the CSV

# --- HEADERS for Discord UI (Simplified for the new logic) ---
HEADERS = {
    "ib_is": "1. Insta-Buy Inputs -> Insta-Sell Outputs",
    "bo_is": "2. Buy-Order Inputs -> Insta-Sell Outputs",
    "ib_so": "3. Insta-Buy Inputs -> Sell-Order Outputs",
    "bo_so": "4. Buy-Order Inputs -> Sell-Order Outputs",
}

MANUAL_ID_OVERRIDES = {
    "Stridersurfer":       "SHARD_STRIDER_SURFER",
    "Abyssal Lanternfish": "SHARD_ABYSSAL_LANTERN",
    "Cinderbat":           "SHARD_CINDER_BAT",
}

def get_bazaar_id_from_name(name):
    if name in MANUAL_ID_OVERRIDES:
        return MANUAL_ID_OVERRIDES[name]
    return f"SHARD_{name.upper().replace(' ', '_')}"

# --- CSV Parsing Logic ---
def parse_fusion_csv(filename):
    """
    Reads the provided CSV. Each row can represent multiple recipes, as one
    set of inputs can have several mutually exclusive outputs. This function
    unpacks them into a flat list of one-to-one recipes and normalizes the
    input order to prevent duplicates.
    """
    recipes = []
    pattern = re.compile(r"(\d+)x\s+([\w\s'-]+)")

    try:
        with open(filename, mode='r', encoding='utf-8') as infile:
            reader = csv.reader(infile)
            next(reader) # Skip header row 1
            next(reader) # Skip header row 2

            for row in reader:
                parsed_inputs = []
                for cell in row[:2]:
                    match = pattern.search(cell)
                    if match:
                        quantity = int(match.group(1))
                        name = match.group(2).strip()
                        parsed_inputs.append({'name': name, 'quantity': quantity})
                
                if len(parsed_inputs) != 2:
                    continue
                
                # Sort the inputs alphabetically by name to create a canonical order.
                parsed_inputs.sort(key=lambda item: item['name'])

                for output_cell in row[2:5]:
                    if not output_cell.strip():
                        continue
                    
                    match = pattern.search(output_cell)
                    if match:
                        quantity = int(match.group(1))
                        name = match.group(2).strip()
                        parsed_output = {'name': name, 'quantity': quantity}
                        
                        input_str = " + ".join([f"{i['quantity']}x {i['name']}" for i in parsed_inputs])
                        output_str = f"{parsed_output['quantity']}x {parsed_output['name']}"
                        
                        recipes.append({
                            'inputs': parsed_inputs,
                            'outputs': [parsed_output],
                            'recipe_str': f"{input_str} → {output_str}"
                        })

    except FileNotFoundError:
        print(f"CRITICAL ERROR: The recipe file '{filename}' was not found.")
        return []
    except Exception as e:
        print(f"An error occurred while parsing the CSV: {e}")
        return []
        
    return recipes

def get_all_shard_names_from_recipes(recipes):
    """Creates a set of all unique shard names from the parsed recipes."""
    all_names = set()
    for recipe in recipes:
        for shard in recipe['inputs']:
            all_names.add(shard['name'])
        for shard in recipe['outputs']:
            all_names.add(shard['name'])
    return list(all_names)

# --- API and Pricing Logic ---
def fetch_raw_bazaar_data():
    url = "https://api.hypixel.net/v2/skyblock/bazaar"
    try:
        resp = requests.get(url, timeout=10)
        resp.raise_for_status()
        data = resp.json()
        if not data.get("success"):
            print(f"API Error: {data.get('cause')}")
            return None
        return data["products"]
    except Exception as e:
        print(f"Fetch error: {e}")
        return None

def process_shard_prices(shard_names, raw_data):
    if not raw_data: return {}
    out = {}
    for name in shard_names:
        api_id = get_bazaar_id_from_name(name)
        prod = raw_data.get(api_id, {})
        sell, buy = prod.get("sell_summary", []), prod.get("buy_summary", [])
        bo = sell[0]["pricePerUnit"] if sell else None
        ib = buy[0]["pricePerUnit"]  if buy  else None
        out[name] = {"buy_order_cost": bo, "insta_buy_cost": ib}
    return out

# --- Profit Calculation Logic ---
def calculate_all_profits(recipes, prices):
    """
    Calculates profits for all parsed recipes and then filters the results to
    show only the single most profitable recipe for each unique output shard.
    """
    profs = {k: [] for k in HEADERS.keys()}

    for recipe in recipes:
        total_ib_cost, total_bo_cost = 0, 0
        valid_cost = True
        for shard in recipe['inputs']:
            price_data = prices.get(shard['name'])
            if not price_data or price_data['insta_buy_cost'] is None or price_data['buy_order_cost'] is None:
                valid_cost = False
                break
            total_ib_cost += price_data['insta_buy_cost'] * shard['quantity']
            total_bo_cost += price_data['buy_order_cost'] * shard['quantity']
        if not valid_cost: continue

        output_shard = recipe['outputs'][0]
        output_name = output_shard['name']
        price_data = prices.get(output_name)
        if not price_data or price_data['insta_buy_cost'] is None or price_data['buy_order_cost'] is None:
            continue
            
        total_is_rev = price_data['buy_order_cost'] * output_shard['quantity']
        total_so_rev = price_data['insta_buy_cost'] * output_shard['quantity']

        profits = {
            "ib_is": total_is_rev - total_ib_cost,
            "bo_is": total_is_rev - total_bo_cost,
            "ib_so": total_so_rev - total_ib_cost,
            "bo_so": total_so_rev - total_bo_cost,
        }
        
        for key, p in profits.items():
            if p > 0:
                profs[key].append({
                    "recipe_str": recipe['recipe_str'],
                    "profit": p,
                    "output_name": output_name
                })

    final_report = {}
    for key, profit_list in profs.items():
        profit_list.sort(key=lambda x: x['profit'], reverse=True)
        
        unique_top_items = []
        seen_outputs = set()
        
        for item in profit_list:
            if item['output_name'] not in seen_outputs:
                unique_top_items.append(item)
                seen_outputs.add(item['output_name'])
        
        final_report[key] = unique_top_items
        
    return final_report

# --- BACKGROUND TASK ---
@tasks.loop(minutes=5)
async def update_fusion_data():
    global profit_report_cache, parsed_recipes
    
    if not parsed_recipes:
        parsed_recipes = parse_fusion_csv(CSV_FILENAME)
        if not parsed_recipes:
            print("Stopping task because recipe list is empty or file not found.")
            update_fusion_data.stop()
            return

    all_shard_names = get_all_shard_names_from_recipes(parsed_recipes)
    raw_api_data = fetch_raw_bazaar_data()
    if not raw_api_data:
        print("Failed to fetch bazaar data, skipping cache update.")
        return

    prices = process_shard_prices(all_shard_names, raw_api_data)
    profit_report_cache = calculate_all_profits(parsed_recipes, prices)
    print("Cache updated successfully.")

# --- UI COMPONENTS ---
class StrategySelect(Select):
    def __init__(self):
        opts = [discord.SelectOption(label=hdr, value=key) for key, hdr in HEADERS.items()]
        super().__init__(placeholder="Select a flipping strategy…", min_values=1, max_values=1, options=opts)

    async def callback(self, interaction: discord.Interaction):
        await interaction.response.defer(ephemeral=True)
        if profit_report_cache is None:
            return await interaction.followup.send("Data is still loading, please wait a moment...", ephemeral=True)

        key  = self.values[0]
        top10 = profit_report_cache.get(key, [])[:10]
        embed = discord.Embed(title=HEADERS[key], color=discord.Color.green())

        if not top10:
            embed.description = "No profitable flips found for this strategy right now."
        else:
            lines = []
            for i, f in enumerate(top10, 1):
                profit_line = f"**{i}. Profit: {f['profit']:,.0f} coins**"
                recipe_line = f"`{f['recipe_str']}`"
                lines.append(f"{profit_line}\n{recipe_line}")
            embed.description = "\n\n".join(lines)
            
        await interaction.followup.send(embed=embed, ephemeral=True)

class FlipperView(View):
    def __init__(self):
        super().__init__(timeout=None)
        self.add_item(StrategySelect())

@bot.event
async def on_ready():
    print(f"Logged in as {bot.user}")
    if not update_fusion_data.is_running():
        update_fusion_data.start()

@bot.command(name="flips")
@commands.has_role("totally not biased by wiz")
async def get_flips(ctx):
    await ctx.send(
        "**Shard Fusion Profit Finder (CSV Version)**\n"
        "This bot now reads its recipes directly from `fusion_list.csv`.\n"
        "Choose your strategy to see the most profitable flips.",
        view=FlipperView()
    )

if __name__ == "__main__":
    if not TOKEN:
        print("CRITICAL: DISCORD_TOKEN not set in .env file")
    else:
        bot.run(TOKEN)