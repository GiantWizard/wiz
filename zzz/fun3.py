import requests
import pandas as pd
import datetime
import math
from collections import Counter
import sys
import numpy as np

# --- Mayor Data and Logic ---
MAYORS_LIST = ["Seraphine","Diana","Marina","Foxy","Cole","Aatrox","Marina","Diana","Paul","Scorpius","Marina","Aatrox","Foxy","Paul","Cole","Foxy","Diana","Derpy","Marina","Aatrox","Paul","Cole","Foxy","Diana","Aatrox","Jerry","Cole","Paul","Marina","Cole","Diana","Marina","Aatrox","Scorpius","Diana","Paul","Foxy","Marina","Paul","Cole","Aatrox","Dante","Technoblade","Diana","Paul","Marina","Foxy","Jerry","Aatrox","Paul","Diana","Foxy","Aatrox","Paul","Diana","Scorpius","Diaz","Marina","Aatrox","Foxy","Diana","Paul","Aatrox","Derpy","Cole","Diana","Marina","Aatrox","Foxy","Paul","Diana","Jerry","Cole","Marina","Diana","Foxy","Marina","Paul","Aatrox","Scorpius","Marina","Diana","Aatrox","Foxy","Paul","Diana","Marina","Derpy","Paul","Diana","Marina","Cole","Aatrox","Marina","Foxy","Jerry","Paul","Diana","Paul","Barry","Aatrox","Barry","Marina","Scorpius","Aatrox","Foxy","Paul","Cole","Diana","Marina","Aatrox","Derpy","Aatrox","Diana","Cole","Aatrox","Diana","Paul","Marina","Jerry","Cole","Foxy","Aatrox","Paul","Diana","Cole","Aatrox","Scorpius","Paul","Marina","Aatrox","Diana","Barry","Marina","Barry","Derpy","Foxy","Aatrox","Diana","Cole","Diana","Marina","Aatrox","Jerry","Diana","Paul","Aatrox","Marina","Paul","Foxy","Diaz","Scorpius","Aatrox","Finnegan","Diana","Aatrox","Foxy","Diana","Aatrox","Derpy","Marina","Paul","Diana","Aatrox","Finnegan","Marina","Aatrox","Jerry","Paul","Foxy","Diaz","Diana","Cole","Diana","Paul","Scorpius","Marina","Aatrox","Diana","Paul","Aatrox","Cole","Finnegan","Derpy","Paul","Aatrox","Diana","Finnegan","Marina","Paul","Aatrox","Jerry","Cole","Aatrox","Marina","Diana","Paul","Diana","Finnegan","Scorpius","Marina","Cole","Diana","Aatrox","Paul","Diana","Cole","Derpy","Finnegan","Diana","Marina","Cole","Paul","Marina","Diana","Jerry","Aatrox","Cole","Marina","Paul","Foxy","Diana","Aatrox","Scorpius","Cole","Finnegan","Marina","Paul","Aatrox","Diana","Marina","Derpy","Foxy","Cole","Aatrox","Paul","Diana","Cole","Paul","Jerry","Aatrox","Marina","Cole","Diana","Paul","Aatrox","Finnegan","Scorpius","Marina","Foxy","Diana","Aatrox","Cole","Marina","Paul","Derpy","Aatrox","Cole","Paul","Diana","Aatrox","Finnegan","Diana","Jerry","Paul","Aatrox","Foxy","Paul","Marina","Diaz","Diana","Scorpius","Foxy","Paul","Diaz","Cole","Aatrox","Marina","Paul","Derpy","Finnegan","Diana","Paul","Cole","Diaz","Aatrox","Finnegan","Jerry","Diana","Paul","Diaz","Aatrox","Diana","Paul","Marina","Scorpius","Finnegan","Aatrox","Diaz","Cole","Aatrox","Paul","Marina","Derpy","Foxy","Aatrox","Cole","Finnegan","Marina","Diaz","Paul","Jerry","Finnegan","Diana","Diaz","Aatrox","Paul","Marina","Diana","Scorpius","Cole","Aatrox","Foxy","Paul","Diaz","Marina","Diana","Derpy","Finnegan","Aatrox","Diaz","Cole","Marina","Aatrox","Diaz","Jerry","Cole"]
ANCHOR_DATE = datetime.datetime(2025, 6, 16, 14, 15)
ANCHOR_MAYOR_INDEX = 336
TERM_DURATION_HOURS = 124

def get_mayor_and_index_for_date(target_date):
    if not isinstance(target_date, datetime.datetime):
        target_date = pd.to_datetime(target_date).to_pydatetime()
    target_date = target_date.replace(tzinfo=None)
    time_difference = ANCHOR_DATE - target_date
    diff_in_hours = time_difference.total_seconds() / 3600
    terms_to_go_back = math.ceil(diff_in_hours / TERM_DURATION_HOURS)
    target_mayor_index = ANCHOR_MAYOR_INDEX - terms_to_go_back
    if 0 <= target_mayor_index < len(MAYORS_LIST):
        return MAYORS_LIST[target_mayor_index], target_mayor_index
    else:
        return "Unknown", -1

def get_mayor_for_date(target_date):
    name, _ = get_mayor_and_index_for_date(target_date)
    return name

def analyze_trend_deviations(item_name):
    # --- Parameters for analysis ---
    one_year_ago = datetime.datetime.now() - datetime.timedelta(days=365)
    start_date_filter = one_year_ago.strftime('%Y-%m-%d')
    
    medium_term_window = '10D'
    smooth_window = '60D'

    api_url = f"https://sky.coflnet.com/api/bazaar/{item_name}/history/"
    
    try:
        response = requests.get(api_url)
        response.raise_for_status()
        data = response.json()
        points = [{'timestamp': p['timestamp'], 'avg_price': (p['buy'] + p['sell']) / 2}
                  for p in data if p.get('buy') and p.get('sell') and p.get('timestamp')]

        if not points:
            print("Error: Could not find any data points for the item.", file=sys.stderr)
            return

        df = pd.DataFrame(points)
        df['timestamp'] = pd.to_datetime(df['timestamp'], format='ISO8601')
        df = df.set_index('timestamp').sort_index()
        
        df = df.loc[start_date_filter:]
        if df.empty:
            print(f"Error: No data available since {start_date_filter}.", file=sys.stderr)
            return
        
        # --- Main Calculations ---
        df['medium_trend'] = df['avg_price'].rolling(window=medium_term_window, center=True, min_periods=1).mean()
        df['smooth_trend'] = df['avg_price'].rolling(window=smooth_window, center=True, min_periods=1).mean()
        df['mayor'] = df.index.to_series().apply(get_mayor_for_date)
        df['relative_deviation'] = (df['avg_price'] - df['smooth_trend']) / df['smooth_trend']

        # --- Identify Most Influential Mayor using Robust Averaging ---
        key_mayors = df['mayor'].unique()
        robust_influence_scores = {}
        for mayor in key_mayors:
            if mayor == "Unknown": continue
            
            # Find all terms for the current mayor
            term_starts = df[df['mayor'] != df['mayor'].shift()]
            mayor_terms_dates = []
            if df.iloc[0]['mayor'] == mayor:
                end_date = term_starts.index[0] if not term_starts.empty else df.index[-1]
                mayor_terms_dates.append({'start': df.index[0], 'end': end_date})
            for i, row in enumerate(term_starts.itertuples()):
                if row.mayor == mayor:
                    end_date = term_starts.index[i + 1] if i + 1 < len(term_starts) else df.index[-1]
                    mayor_terms_dates.append({'start': row.Index, 'end': end_date})
            
            # Calculate the average deviation for each term, then average those results
            term_deviations = []
            if not mayor_terms_dates: continue
            
            for term in mayor_terms_dates:
                term_df = df.loc[term['start']:term['end']]
                if not term_df.empty:
                    term_deviations.append(term_df['relative_deviation'].mean())
            
            if term_deviations:
                robust_influence_scores[mayor] = np.mean(term_deviations)

        if not robust_influence_scores:
            print("Error: Could not calculate robust influence scores.", file=sys.stderr)
            return
            
        most_influential_mayor = max(robust_influence_scores, key=lambda m: abs(robust_influence_scores[m]))
        avg_term_deviation = robust_influence_scores[most_influential_mayor]
        
        # --- Detailed Term-by-Term Analysis & Directional Consistency ---
        term_starts = df[df['mayor'] != df['mayor'].shift()]
        mayor_terms = []
        
        if df.iloc[0]['mayor'] == most_influential_mayor:
            end_date = term_starts.index[0] if not term_starts.empty else df.index[-1]
            mayor_terms.append({'start': df.index[0], 'end': end_date})
            
        for i, row in enumerate(term_starts.itertuples()):
            if row.mayor == most_influential_mayor:
                end_date = term_starts.index[i + 1] if i + 1 < len(term_starts) else df.index[-1]
                mayor_terms.append({'start': row.Index, 'end': end_date})
        
        term_details_log = []
        consistent_terms_count = 0
        expected_direction_is_positive = avg_term_deviation >= 0

        for i, term in enumerate(mayor_terms):
            term_df = df.loc[term['start']:term['end']]
            term_deviation = term_df['relative_deviation'].mean()
            
            # <<< NEW: Directional Consistency Check >>>
            is_consistent = (expected_direction_is_positive and term_deviation > 0) or \
                            (not expected_direction_is_positive and term_deviation < 0)
            
            if is_consistent:
                consistent_terms_count += 1
            
            sign = '+' if term_deviation >= 0 else ''
            detail = (
                f"  Term {i+1} ({term['start'].strftime('%Y-%m-%d')} to {term['end'].strftime('%Y-%m-%d')}): "
                f"[{'Consistent' if is_consistent else 'Inconsistent'}] "
                f"({sign}{term_deviation * 100:.2f}%)"
            )
            term_details_log.append(detail)
        
        total_terms = len(mayor_terms) if len(mayor_terms) > 0 else 1
        consistency_score = consistent_terms_count / total_terms
        
        # --- Print Final Results ---
        print(f"Item: {item_name}")
        print(f"Analysis Window: Last 1 Year")
        print("-" * 30)
        print(f"Most Influential Mayor: {most_influential_mayor}")
        print(f"Avg. Term Deviation: {'+' if avg_term_deviation >= 0 else ''}{avg_term_deviation * 100:.2f}%")
        # <<< CHANGE: Label the new consistency metric clearly >>>
        print(f"Directional Consistency: {consistency_score:.0%} ({consistent_terms_count} of {total_terms} terms)")
        print("Term Breakdown:")
        if term_details_log:
            for log_entry in term_details_log:
                print(log_entry)
        else:
            print("  No terms found for this mayor in the analysis window.")
            
    except requests.exceptions.RequestException as e:
        print(f"An error occurred while fetching data: {e}", file=sys.stderr)
    except Exception as e:
        print(f"An unexpected error occurred: {e}", file=sys.stderr)

if __name__ == "__main__":
    item_name = "ENCHANTMENT_LOOTING_5" 
    analyze_trend_deviations(item_name)