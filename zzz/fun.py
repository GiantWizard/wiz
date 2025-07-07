import requests
import pandas as pd
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import matplotlib.dates as mdates
import datetime
import math
from collections import Counter

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

# --- Plotting Functions ---
def format_millions(x, pos):
    if x >= 1_000_000:
        return f'{int(x / 1_000_000)}M'
    return f'{int(x / 1_000)}K'

def merge_overlapping_intervals(intervals):
    if not intervals: return []
    intervals.sort(key=lambda x: x[0])
    merged = [intervals[0]]
    for current_start, current_end in intervals[1:]:
        last_start, last_end = merged[-1]
        if current_start <= last_end:
            merged[-1] = (last_start, max(last_end, current_end))
        else:
            merged.append((current_start, current_end))
    return merged

def plot_and_analyze_trend_deviations(item_name):
    # --- Parameters for analysis ---
    start_date_filter = '2022-05-15'
    medium_term_window = '10D'
    smooth_window = '60D'
    
    spike_relative_threshold = 0.07
    dip_relative_threshold = 0.07
    absolute_stdev_factor = 0.1

    api_url = f"https://sky.coflnet.com/api/bazaar/{item_name}/history/"
    print(f"Fetching data from: {api_url}")

    try:
        response = requests.get(api_url)
        response.raise_for_status()
        data = response.json()
        points = [{'timestamp': p['timestamp'], 'avg_price': (p['buy'] + p['sell']) / 2}
                  for p in data if p.get('buy') and p.get('sell') and p.get('timestamp')]

        if not points:
            print("Could not find any data points.")
            return

        df = pd.DataFrame(points)
        df['timestamp'] = pd.to_datetime(df['timestamp'], format='ISO8601')
        df = df.set_index('timestamp').sort_index()

        print(f"Filtering data from {start_date_filter} to now...")
        df = df.loc[start_date_filter:]
        if df.empty:
            print(f"No data available after {start_date_filter}.")
            return
        
        # --- Calculate Trends and Deviations ---
        print("Calculating trends and price deviations...")
        df['medium_trend'] = df['avg_price'].rolling(window=medium_term_window, center=True, min_periods=1).mean()
        df['smooth_trend'] = df['avg_price'].rolling(window=smooth_window, center=True, min_periods=1).mean()
        df['mayor'] = df.index.to_series().apply(get_mayor_for_date)
        df['relative_deviation'] = (df['avg_price'] - df['smooth_trend']) / df['smooth_trend']

        # --- Spike and Dip Detection ---
        absolute_min_deviation = df['avg_price'].std() * absolute_stdev_factor
        print(f"Deviation detection thresholds: >{spike_relative_threshold:.0%} AND >{absolute_min_deviation:,.0f} coins.")
        
        is_relative_spike = df['medium_trend'] > df['smooth_trend'] * (1 + spike_relative_threshold)
        is_absolute_spike = (df['medium_trend'] - df['smooth_trend']) > absolute_min_deviation
        df['is_spike'] = is_relative_spike & is_absolute_spike
        df['spike_group'] = (df['is_spike'] != df['is_spike'].shift()).cumsum()
        df.loc[~df['is_spike'], 'spike_group'] = None
        
        is_relative_dip = df['medium_trend'] < df['smooth_trend'] * (1 - dip_relative_threshold)
        is_absolute_dip = (df['smooth_trend'] - df['medium_trend']) > absolute_min_deviation
        df['is_dip'] = is_relative_dip & is_absolute_dip
        df['dip_group'] = (df['is_dip'] != df['is_dip'].shift()).cumsum()
        df.loc[~df['is_dip'], 'dip_group'] = None

        def find_extrema(df_series, group_col, mode='max'):
            # This function remains unchanged...
            potential_extrema = []
            groups_to_analyze = df_series.dropna(subset=[group_col])[group_col].unique()
            for group_id in groups_to_analyze:
                current_group_df = df_series[df_series[group_col] == group_id]
                if current_group_df.empty: continue
                extrema_timestamp = current_group_df['medium_trend'].idxmax() if mode == 'max' else current_group_df['medium_trend'].idxmin()
                extrema_data = df_series.loc[extrema_timestamp]
                extrema_mayor_name, extrema_mayor_index = get_mayor_and_index_for_date(extrema_timestamp)
                percentage_size = (extrema_data['medium_trend'] - extrema_data['smooth_trend']) / extrema_data['smooth_trend'] if mode == 'max' else (extrema_data['smooth_trend'] - extrema_data['medium_trend']) / extrema_data['smooth_trend']
                if extrema_mayor_index != -1:
                    mayor_before = MAYORS_LIST[extrema_mayor_index - 1] if extrema_mayor_index > 0 else "N/A"
                    mayor_after = MAYORS_LIST[extrema_mayor_index + 1] if extrema_mayor_index < len(MAYORS_LIST) - 1 else "N/A"
                    mayor_trio_string = f"{mayor_before} -> {extrema_mayor_name} -> {mayor_after}"
                    annotation_text = f"Trio:\n{mayor_before} ->\n{extrema_mayor_name} ->\n{mayor_after}"
                else:
                    mayor_trio_string = extrema_mayor_name; annotation_text = f"Event\n({extrema_mayor_name})"
                potential_extrema.append({'timestamp': extrema_timestamp, 'price': extrema_data['medium_trend'], 'trio': mayor_trio_string, 'annotation': annotation_text, 'mayor_index': extrema_mayor_index, 'percentage_size': percentage_size})
            if not potential_extrema: return []
            potential_extrema.sort(key=lambda p: p['timestamp'])
            final_extrema = [potential_extrema[0]]
            for current_item in potential_extrema[1:]:
                last_item = final_extrema[-1]; time_difference = current_item['timestamp'] - last_item['timestamp']
                is_same_cluster = current_item['trio'] == last_item['trio'] and time_difference.days < 5
                is_better_extrema = (mode == 'max' and current_item['price'] > last_item['price']) or (mode == 'min' and current_item['price'] < last_item['price'])
                if is_same_cluster and is_better_extrema: final_extrema[-1] = current_item
                elif not is_same_cluster: final_extrema.append(current_item)
            return final_extrema

        print("\n--- Analyzing Spikes & Dips ---")
        final_peaks = find_extrema(df, 'spike_group', mode='max')
        final_troughs = find_extrema(df, 'dip_group', mode='min')
        print(f"Found {len(final_peaks)} de-duplicated spike events and {len(final_troughs)} de-duplicated dip events.")

        # --- Visualization (code remains the same) ---
        print("\nPlotting analysis...")
        fig, ax = plt.subplots(figsize=(18, 10))
        ax.plot(df.index, df['avg_price'], label='Average Price', color='cornflowerblue', zorder=2, alpha=0.3, linewidth=1)
        ax.plot(df.index, df['medium_trend'], label=f'Medium Trend ({medium_term_window} Avg)', color='mediumseagreen', zorder=3, linewidth=2)
        ax.plot(df.index, df['smooth_trend'], label=f'Smooth Trend ({smooth_window} Avg)', color='crimson', zorder=4, linewidth=2.5)
        ax.fill_between(df.index, df['smooth_trend'], df['medium_trend'], where=df['is_spike'], color='orange', alpha=0.5, interpolate=True, label='Spike Period')
        ax.fill_between(df.index, df['smooth_trend'], df['medium_trend'], where=df['is_dip'], color='skyblue', alpha=0.6, interpolate=True, label='Dip Period')
        ax2 = ax.twiny(); ax2.set_xlim(ax.get_xlim()); mayor_changes = df[df['mayor'].shift() != df['mayor']]; ax2.set_xticks(mayor_changes.index); ax2.set_xticklabels(mayor_changes['mayor'], rotation=30, ha='left', fontsize=9); ax2.set_xlabel("Elected Mayor", fontsize=12, color='purple'); ax2.tick_params(axis='x', which='major', colors='purple')
        for peak in final_peaks:
            ax.annotate(peak['annotation'], xy=(peak['timestamp'], peak['price']), xytext=(0, 30), textcoords="offset points", arrowprops=dict(facecolor='black', shrink=0.05, width=1, headwidth=6), ha='center', va='bottom', fontsize=9, bbox=dict(boxstyle="round,pad=0.3", fc="yellow", ec="black", lw=1, alpha=0.9), zorder=5)
        for trough in final_troughs:
            ax.annotate(trough['annotation'], xy=(trough['timestamp'], trough['price']), xytext=(0, -40), textcoords="offset points", arrowprops=dict(facecolor='black', shrink=0.05, width=1, headwidth=6), ha='center', va='top', fontsize=9, bbox=dict(boxstyle="round,pad=0.3", fc="lightblue", ec="black", lw=1, alpha=0.9), zorder=5)
        context_spans = []; all_extrema = sorted(final_peaks + final_troughs, key=lambda x: x['timestamp'])
        for event in all_extrema:
            if event['mayor_index'] != -1:
                start_mayor_index = max(0, event['mayor_index'] - 2); end_mayor_index = min(len(MAYORS_LIST) - 1, event['mayor_index'] + 2); start_mayor_name = MAYORS_LIST[start_mayor_index]; start_date_candidates = mayor_changes.index[mayor_changes['mayor'] == start_mayor_name]; relevant_start_dates = start_date_candidates[start_date_candidates <= event['timestamp']]; span_start = relevant_start_dates[-1] if not relevant_start_dates.empty else df.index[0]; next_mayor_after_group_index = end_mayor_index + 1
                if next_mayor_after_group_index < len(MAYORS_LIST):
                    next_mayor_name = MAYORS_LIST[next_mayor_after_group_index]; end_date_candidates = mayor_changes.index[mayor_changes['mayor'] == next_mayor_name]; relevant_end_dates = end_date_candidates[end_date_candidates > event['timestamp']]; span_end = relevant_end_dates[0] if not relevant_end_dates.empty else df.index[-1]
                else: span_end = df.index[-1]
                context_spans.append((span_start, span_end))
        merged_spans = merge_overlapping_intervals(context_spans); label_added = False
        for start, end in merged_spans: ax.axvspan(start, end, color='purple', alpha=0.08, zorder=0, label="5-Mayor Event Context" if not label_added else ""); label_added = True
        annot = ax.annotate("", xy=(0,0), xytext=(15,15), textcoords="offset points", bbox=dict(boxstyle="round,pad=0.5", fc="yellow", alpha=0.8), arrowprops=dict(arrowstyle="->")); annot.set_visible(False); vert_line = ax.axvline(df.index[0], c='gray', lw=1, linestyle='--', visible=False)
        def update_ticker(event):
            if event.inaxes == ax: dt_cursor = mdates.num2date(event.xdata); idx = df.index.get_indexer([dt_cursor], method='nearest')[0]; dp = df.iloc[idx]; annot_text = (f"Date: {dp.name.strftime('%Y-%m-%d %H:%M')}\nPrice: {dp.avg_price:,.0f}\nDeviation: {dp.relative_deviation:.2%}\nMayor: {dp.mayor}"); annot.set_text(annot_text); annot.xy = (dp.name, dp.avg_price); annot.set_visible(True); vert_line.set_xdata([dp.name]); vert_line.set_visible(True); fig.canvas.draw_idle()
        def on_leave(event): annot.set_visible(False); vert_line.set_visible(False); fig.canvas.draw_idle()
        fig.canvas.mpl_connect('motion_notify_event', update_ticker); fig.canvas.mpl_connect('axes_leave_event', on_leave)

        # --- Summaries and Analysis ---
        def analyze_events(event_list, event_type):
            if not event_list:
                print(f"\nNo {event_type}s detected, cannot perform further analysis.")
                return [] 
            all_event_mayors = [name.strip() for event in event_list for name in event['trio'].split('->') if name != "N/A"]
            if not all_event_mayors:
                 print(f"No valid mayors found in {event_type} periods.")
                 return []
            mayor_counts = Counter(all_event_mayors)
            max_count = max(mayor_counts.values())
            mode_mayors = [mayor for mayor, count in mayor_counts.items() if count == max_count]
            print(f"\n--- Mode Mayor Analysis for {event_type.title()}s ---")
            print(f"The most frequent mayor(s) in {event_type} trios is/are: {', '.join(mode_mayors)}")
            print(f"Appeared {max_count} time(s) each in the detected {event_type} periods.")
            return mode_mayors

        # --- Run Spike Analysis ---
        print("\n" + "="*50 + "\n--- SPIKE ANALYSIS ---\n" + "="*50)
        spike_summary = [{'peak_time': p['timestamp'].strftime('%Y-%m-%d'), 'mayor_trio_at_peak': p['trio']} for p in final_peaks]
        if spike_summary: print(pd.DataFrame(spike_summary).to_string(index=False))
        spike_mode_mayors = analyze_events(final_peaks, "spike")

        # --- Run Dip Analysis ---
        print("\n" + "="*50 + "\n--- DIP ANALYSIS ---\n" + "="*50)
        dip_summary = [{'trough_time': t['timestamp'].strftime('%Y-%m-%d'), 'mayor_trio_at_trough': t['trio']} for t in final_troughs]
        if dip_summary: print(pd.DataFrame(dip_summary).to_string(index=False))
        dip_mode_mayors = analyze_events(final_troughs, "dip")

        # --- Influence Analysis of Key Event Mayors ---
        print("\n" + "="*50 + "\n--- OVERALL INFLUENCE ANALYSIS OF KEY MAYORS ---\n" + "="*50)
        key_mayors = sorted(list(set(spike_mode_mayors + dip_mode_mayors)))
        
        # Dictionary to store the calculated influence of each key mayor
        mayor_influence_scores = {}

        if not key_mayors:
            print("No mode mayors were identified from spikes or dips, so no overall analysis can be performed.")
        else:
            print("Analyzing the key mayors identified from spikes and dips...\n")
            for mayor in key_mayors:
                mayor_df = df[df['mayor'] == mayor]
                if mayor_df.empty:
                    print(f"No historical data found for Mayor {mayor} in the selected time frame.")
                    continue
                
                # Calculate and store the average deviation for this mayor
                avg_deviation = mayor_df['relative_deviation'].mean()
                mayor_influence_scores[mayor] = avg_deviation
                
                # Print the detailed breakdown for each mayor
                sign = '+' if avg_deviation >= 0 else ''
                print(f"Mayor {mayor:<12} | Overall Average Price Deviation from 60d Trend: {sign}{avg_deviation * 100:.2f}%")

        # --- NEW: Final Result Section ---
        # Find the mayor with the highest *absolute* deviation from the stored scores
        if mayor_influence_scores:
            # Find the mayor name corresponding to the max absolute deviation value
            most_influential_mayor = max(mayor_influence_scores, key=lambda m: abs(mayor_influence_scores[m]))
            # Get the actual deviation value (not absolute) for that mayor
            highest_deviation_value = mayor_influence_scores[most_influential_mayor]
            
            print("\n" + "="*50 + "\n--- FINAL RESULT: MOST INFLUENTIAL MAYOR ---\n" + "="*50)
            print(f"Based on the analysis of key event-driving mayors for {item_name.replace('_', ' ').title()},")
            print("the mayor with the highest average price deviation is:")
            
            sign = '+' if highest_deviation_value >= 0 else ''
            print(f"\n>>> Mayor: {most_influential_mayor}")
            print(f">>> Average Deviation: {sign}{highest_deviation_value * 100:.2f}%\n")
        else:
            print("\n" + "="*50 + "\n--- FINAL RESULT: MOST INFLUENTIAL MAYOR ---\n" + "="*50)
            print("Could not determine the most influential mayor as no key mayors were identified.")
            

        # --- Final Formatting and Plot Display ---
        handles, labels = ax.get_legend_handles_labels(); by_label = dict(zip(labels, handles))
        ax.legend(by_label.values(), by_label.keys(), loc='upper left')
        ax.yaxis.set_major_formatter(ticker.FuncFormatter(format_millions))
        ax.set_title(f'Spike & Dip Analysis for {item_name.replace("_", " ").title()}', fontsize=16, pad=40)
        ax.set_xlabel('Date', fontsize=12); ax.set_ylabel('Price (Coins)', fontsize=12)
        ax.grid(True, which='major', linestyle='--'); fig.tight_layout(rect=[0, 0, 1, 0.96])
        
        print("\nDisplaying plot... Move your mouse over the graph for details.")
        plt.show()

    except requests.exceptions.RequestException as e:
        print(f"An error occurred while fetching data: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")

if __name__ == "__main__":
    item_to_plot = "ENCHANTED_FERMENTED_SPIDER_EYE" # Using your example item
    plot_and_analyze_trend_deviations(item_to_plot)