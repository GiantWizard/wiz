import requests
import pandas as pd
import datetime
import math
from collections import defaultdict
import sys
import numpy as np
from concurrent.futures import ThreadPoolExecutor, as_completed
from tqdm import tqdm

# --- Mayor Data and Logic (Unchanged) ---
MAYORS_LIST = ["Seraphine","Diana","Marina","Foxy","Cole","Aatrox","Marina","Diana","Paul","Scorpius","Marina","Aatrox","Foxy","Paul","Cole","Foxy","Diana","Derpy","Marina","Aatrox","Paul","Cole","Foxy","Diana","Aatrox","Jerry","Cole","Paul","Marina","Cole","Diana","Marina","Aatrox","Scorpius","Diana","Paul","Foxy","Marina","Paul","Cole","Aatrox","Dante","Technoblade","Diana","Paul","Marina","Foxy","Jerry","Aatrox","Paul","Diana","Foxy","Aatrox","Paul","Diana","Scorpius","Diaz","Marina","Aatrox","Foxy","Diana","Paul","Aatrox","Derpy","Cole","Diana","Marina","Aatrox","Foxy","Paul","Diana","Jerry","Cole","Marina","Diana","Foxy","Marina","Paul","Aatrox","Scorpius","Marina","Diana","Aatrox","Foxy","Paul","Diana","Marina","Derpy","Paul","Diana","Marina","Cole","Aatrox","Marina","Foxy","Jerry","Paul","Diana","Paul","Barry","Aatrox","Barry","Marina","Scorpius","Aatrox","Foxy","Paul","Cole","Diana","Marina","Aatrox","Derpy","Aatrox","Diana","Cole","Aatrox","Diana","Paul","Marina","Jerry","Cole","Foxy","Aatrox","Paul","Diana","Cole","Aatrox","Scorpius","Paul","Marina","Aatrox","Diana","Barry","Marina","Barry","Derpy","Foxy","Aatrox","Diana","Cole","Diana","Marina","Aatrox","Jerry","Diana","Paul","Aatrox","Marina","Paul","Foxy","Diaz","Scorpius","Aatrox","Finnegan","Diana","Aatrox","Foxy","Diana","Aatrox","Derpy","Marina","Paul","Diana","Aatrox","Finnegan","Marina","Aatrox","Jerry","Paul","Foxy","Diaz","Diana","Cole","Diana","Paul","Scorpius","Marina","Aatrox","Diana","Paul","Aatrox","Cole","Finnegan","Derpy","Paul","Aatrox","Diana","Finnegan","Marina","Paul","Aatrox","Jerry","Cole","Aatrox","Marina","Diana","Paul","Diana","Finnegan","Scorpius","Marina","Cole","Diana","Aatrox","Paul","Diana","Cole","Derpy","Finnegan","Diana","Marina","Cole","Paul","Marina","Diana","Jerry","Aatrox","Cole","Marina","Paul","Foxy","Diana","Aatrox","Scorpius","Cole","Finnegan","Marina","Paul","Aatrox","Diana","Marina","Derpy","Foxy","Cole","Aatrox","Paul","Diana","Cole","Paul","Jerry","Aatrox","Marina","Cole","Diana","Paul","Aatrox","Finnegan","Scorpius","Marina","Foxy","Diana","Aatrox","Cole","Marina","Paul","Derpy","Aatrox","Cole","Paul","Diana","Aatrox","Finnegan","Diana","Jerry","Paul","Aatrox","Foxy","Paul","Marina","Diaz","Diana","Scorpius","Foxy","Paul","Diaz","Cole","Aatrox","Marina","Paul","Derpy","Finnegan","Diana","Paul","Cole","Diaz","Aatrox","Finnegan","Jerry","Diana","Paul","Diaz","Aatrox","Diana","Paul","Marina","Scorpius","Finnegan","Aatrox","Diaz","Cole","Aatrox","Paul","Marina","Derpy","Foxy","Aatrox","Cole","Finnegan","Marina","Diaz","Paul","Jerry","Finnegan","Diana","Diaz","Aatrox","Paul","Marina","Diana","Scorpius","Cole","Aatrox","Foxy","Paul","Diaz","Marina","Diana","Derpy","Finnegan","Aatrox","Diaz","Cole","Marina","Aatrox","Diaz","Jerry","Cole"]
ALL_UNIQUE_MAYORS = sorted(list(set(MAYORS_LIST)))
ANCHOR_DATE = datetime.datetime(2025, 6, 16, 14, 15)
ANCHOR_MAYOR_INDEX = 336
TERM_DURATION_HOURS = 124

def get_mayor_for_date(target_date):
    if not isinstance(target_date, datetime.datetime):
        target_date = pd.to_datetime(target_date).to_pydatetime()
    target_date = target_date.replace(tzinfo=None)
    time_difference = ANCHOR_DATE - target_date
    diff_in_hours = time_difference.total_seconds() / 3600
    terms_to_go_back = math.ceil(diff_in_hours / TERM_DURATION_HOURS)
    target_mayor_index = ANCHOR_MAYOR_INDEX - terms_to_go_back
    if 0 <= target_mayor_index < len(MAYORS_LIST):
        return MAYORS_LIST[target_mayor_index]
    else:
        return "Unknown"

def calculate_mayor_influence_for_item(item_id):
    """
    Analyzes an item using a full suite of filters: item age, mayor recency,
    dynamic window, and result consistency.
    """
    smooth_window = '60D'
    api_url = f"https://sky.coflnet.com/api/bazaar/{item_id}/history/"
    
    try:
        response = requests.get(api_url)
        response.raise_for_status()
        data = response.json()
        points = [{'timestamp': p['timestamp'], 'avg_price': (p['buy'] + p['sell']) / 2}
                  for p in data if p.get('buy') and p.get('sell') and p.get('timestamp')]

        if not points: return None

        df_all_history = pd.DataFrame(points)
        df_all_history['timestamp'] = pd.to_datetime(df_all_history['timestamp'], format='ISO8601')
        df_all_history = df_all_history.set_index('timestamp').sort_index()
        
        if df_all_history.empty: return None

        one_and_a_half_years_ago = datetime.datetime.now() - datetime.timedelta(days=548)
        if df_all_history.index[0].to_pydatetime().replace(tzinfo=None) > one_and_a_half_years_ago:
            return None

        df_all_history['smooth_trend'] = df_all_history['avg_price'].rolling(window=smooth_window, center=True, min_periods=1).mean()
        df_all_history['mayor'] = df_all_history.index.to_series().apply(get_mayor_for_date)
        df_all_history['relative_deviation'] = (df_all_history['avg_price'] - df_all_history['smooth_trend']) / df_all_history['smooth_trend']
        
        robust_influence_scores = {}
        one_year_ago_dt = datetime.datetime.now() - datetime.timedelta(days=365)
        mayors_in_history = df_all_history['mayor'].unique()

        for mayor in mayors_in_history:
            if mayor == "Unknown": continue

            mayor_df_full = df_all_history[df_all_history['mayor'] == mayor]
            if mayor_df_full.empty or mayor_df_full.index.max().to_pydatetime().replace(tzinfo=None) < one_year_ago_dt:
                continue

            df_one_year = df_all_history.loc[df_all_history.index >= one_year_ago_dt]
            one_year_deviations = df_one_year[df_one_year['mayor'] == mayor]['relative_deviation'].tolist()

            term_starts = df_all_history[df_all_history['mayor'] != df_all_history['mayor'].shift(1)]
            mayor_term_starts = term_starts[term_starts['mayor'] == mayor]
            
            five_term_deviations = []
            if not mayor_term_starts.empty:
                start_date_5_terms = mayor_term_starts.index[-5] if len(mayor_term_starts) >= 5 else mayor_term_starts.index[0]
                df_five_terms = df_all_history.loc[df_all_history.index >= start_date_5_terms]
                five_term_deviations = df_five_terms[df_five_terms['mayor'] == mayor]['relative_deviation'].tolist()

            final_deviations = one_year_deviations if len(one_year_deviations) > len(five_term_deviations) else five_term_deviations
            
            if not final_deviations: continue

            if len(final_deviations) > 1:
                overall_sign = np.sign(np.mean(final_deviations))
                if overall_sign == 0: continue
                num_consistent_points = sum(1 for dev in final_deviations if np.sign(dev) == overall_sign)
                if (num_consistent_points / len(final_deviations)) < 0.9: continue
            
            robust_influence_scores[mayor] = np.mean(final_deviations)

        if not robust_influence_scores: return None
            
        most_influential_mayor = max(robust_influence_scores, key=lambda m: abs(robust_influence_scores[m]))
        avg_term_deviation = robust_influence_scores[most_influential_mayor]
        
        if abs(avg_term_deviation) > 0.02:
            return (item_id, most_influential_mayor, avg_term_deviation)
        else:
            return None
            
    except (requests.exceptions.RequestException, Exception):
        return None

def run_full_analysis():
    """Performs a full market analysis for high-end, high-velocity items."""
    print("Step 1: Discovering all tradable items...")
    BAZAAR_API_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
    try:
        response = requests.get(BAZAAR_API_URL)
        response.raise_for_status()
        bazaar_data = response.json()
        all_products = bazaar_data.get('products', {})
        print(f"Found {len(all_products)} items.")
    except requests.exceptions.RequestException as e:
        print(f"FATAL: Could not fetch item list from Hypixel API: {e}", file=sys.stderr)
        return

    # ***MODIFICATION: Re-calibrating filters for high-end, high-velocity items ***
    print("\nStep 2: Filtering for elite high-end, high-velocity items.")
    MIN_PRICE = 100_000
    MIN_MARKET_VELOCITY = 30_000_000_000 # 30 Billion

    filtered_products_with_specs = {}
    for item_id, data in all_products.items():
        status = data.get('quick_status', {})
        buy_price = status.get('buyPrice', 0)
        
        buy_volume = status.get('buyMovingWeek', 0)
        sell_volume = status.get('sellMovingWeek', 0)
        market_velocity = (buy_volume + sell_volume) * buy_price

        # The 'if' condition now checks for high price AND very high velocity.
        if buy_price > MIN_PRICE and market_velocity >= MIN_MARKET_VELOCITY:
            filtered_products_with_specs[item_id] = {
                'buy_price': buy_price,
                'market_velocity': market_velocity
            }
    
    print(f"Filtered down to {len(filtered_products_with_specs)} elite items to analyze.")

    print("\nStep 3 & 4: Analyzing items... (Applying 1.5yr age, recency, dynamic window, and 90% consistency filters)")
    mayor_results = defaultdict(list)
    
    with ThreadPoolExecutor(max_workers=10) as executor:
        future_to_item = {
            executor.submit(calculate_mayor_influence_for_item, item_id): item_id 
            for item_id in filtered_products_with_specs
        }
        
        for future in tqdm(as_completed(future_to_item), total=len(filtered_products_with_specs), desc="Analyzing Items"):
            result = future.result()
            if result:
                item_name, mayor, score = result
                specs = filtered_products_with_specs[item_name]
                mayor_results[mayor].append({'item': item_name, 'score': score, 'price': specs['buy_price'], 'velocity': specs['market_velocity']})

    print("\n--- Analysis Complete: Top Elite Items With Consistent Mayor Influence ---")
    for mayor in ALL_UNIQUE_MAYORS:
        items = mayor_results.get(mayor, [])
        print("-" * 75)
        print(f"Mayor: {mayor}")
        print("-" * 75)
        if not items:
            print("  No items found with a highly consistent influence pattern for this mayor.")
        else:
            sorted_items = sorted(items, key=lambda x: abs(x['score']), reverse=True)
            for i, item_data in enumerate(sorted_items[:3]):
                sign = '+' if item_data['score'] >= 0 else ''
                price = item_data['price']
                velocity = item_data['velocity']
                velocity_str = f"{velocity / 1_000_000_000:.1f}B" if velocity >= 1_000_000_000 else f"{velocity / 1_000_000:.1f}M"
                print(f"  {i+1}. {item_data['item']:<35} | Score: {sign}{item_data['score']*100:5.2f}%")
                # Using {:, .0f} for price formatting as we are dealing with whole numbers over 100k
                print(f"     (Price: {price:,.0f} | 7d Velocity: {velocity_str})")
        print()


if __name__ == "__main__":
    run_full_analysis()