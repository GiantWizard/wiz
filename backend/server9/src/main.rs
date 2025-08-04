use chrono::{Utc, Local};
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
use std::process::Command;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use tokio::time::sleep;

/// Represents an individual order (can be a buy or sell order).
#[derive(Debug, Clone, Deserialize, Serialize)]
struct Order {
    amount: i64,
    price_per_unit: f64,
    orders: i64,
}

/// Represents the Bazaar snapshot for one product.
#[derive(Debug, Clone, Deserialize, Serialize)]
struct BazaarInfo {
    product_id: String,
    buy_price: f64,    // Top buy order price (what you can instasell for)
    sell_price: f64,   // Top sell order price (what you must pay to instabuy)
    buy_orders: Vec<Order>,
    sell_orders: Vec<Order>,
    buy_moving_week: i64,
    sell_moving_week: i64,
}

/// Represents a detected pattern period
#[derive(Debug, Clone)]
struct PatternPeriod {
    position: usize,
    moving_week_delta: i64,
    inferred_volume: i64,
    timestamp: u64,
}

/// Represents a modal pattern with its characteristics
#[derive(Debug, Clone)]
struct ModalPattern {
    size: i64,
    frequency_minutes: f64,
    occurrence_count: usize,
    confidence: f64,
}

/// Holds the final analysis metrics for one product.
#[derive(Debug, Serialize)]
struct AnalysisResult {
    product_id: String,
    instabuy_price_average: f64,
    instasell_price_average: f64,
    new_demand_offer_frequency_average: f64,
    new_demand_offer_size_average: f64,
    player_instabuy_transaction_frequency: f64,
    player_instabuy_transaction_size_average: f64,
    new_supply_offer_frequency_average: f64,
    new_supply_offer_size_average: f64,
    player_instasell_transaction_frequency: f64,
    player_instasell_transaction_size_average: f64,
    // New pattern-based fields
    instabuy_modal_size: i64,
    instabuy_pattern_frequency: f64,
    instabuy_scale_factor: f64,
    instabuy_estimated_true_volume: f64,
    instasell_modal_size: i64,
    instasell_pattern_frequency: f64,
    instasell_scale_factor: f64,
    instasell_estimated_true_volume: f64,
    pattern_detection_confidence: f64,
}

/// Incremental state for each product; updated on each new snapshot.
#[derive(Debug)]
struct ProductMetricsState {
    sum_instabuy_price: f64,
    sum_instasell_price: f64,
    snapshot_count: usize,
    windows_processed: usize,
    prev_snapshot: Option<BazaarInfo>,
    total_new_demand_offers: f64,
    total_new_demand_offer_amount: f64,
    total_new_supply_offers: f64,
    total_new_supply_offer_amount: f64,
    player_instabuy_event_count: usize,
    player_instabuy_volume_total: f64,
    player_instasell_event_count: usize,
    player_instasell_volume_total: f64,
    prev_buy_moving_week: i64,
    prev_sell_moving_week: i64,
    
    // Time series data for pattern detection (180 data points = 1 hour)
    buy_moving_week_history: Vec<i64>,
    sell_moving_week_history: Vec<i64>,
    inferred_buy_volume_history: Vec<i64>,
    inferred_sell_volume_history: Vec<i64>,
    timestamps: Vec<u64>,
    
    // Pattern analysis state
    instabuy_patterns: Vec<PatternPeriod>,
    instasell_patterns: Vec<PatternPeriod>,
    total_buy_moving_week_activity: i64,
    total_sell_moving_week_activity: i64,
}

impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
        let current_timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        
        let mut timestamps = Vec::with_capacity(180);
        timestamps.push(current_timestamp);
        
        let mut buy_history = Vec::with_capacity(180);
        buy_history.push(first.buy_moving_week);
        
        let mut sell_history = Vec::with_capacity(180);
        sell_history.push(first.sell_moving_week);
        
        Self {
            sum_instabuy_price: first.sell_price,
            sum_instasell_price: first.buy_price,
            snapshot_count: 1,
            windows_processed: 0,
            prev_snapshot: Some(first.clone()),
            total_new_demand_offers: 0.0,
            total_new_demand_offer_amount: 0.0,
            total_new_supply_offers: 0.0,
            total_new_supply_offer_amount: 0.0,
            player_instabuy_event_count: 0,
            player_instabuy_volume_total: 0.0,
            player_instasell_event_count: 0,
            player_instasell_volume_total: 0.0,
            prev_buy_moving_week: first.buy_moving_week,
            prev_sell_moving_week: first.sell_moving_week,
            buy_moving_week_history: buy_history,
            sell_moving_week_history: sell_history,
            inferred_buy_volume_history: Vec::with_capacity(180),
            inferred_sell_volume_history: Vec::with_capacity(180),
            timestamps,
            instabuy_patterns: Vec::new(),
            instasell_patterns: Vec::new(),
            total_buy_moving_week_activity: 0,
            total_sell_moving_week_activity: 0,
        }
    }

    fn price_to_key(price: f64) -> u64 { 
        (price * 1000.0).round() as u64 
    }

    fn maintain_rolling_window<T>(vec: &mut Vec<T>, capacity: usize) {
        while vec.len() > capacity {
            vec.remove(0);
        }
    }

    fn find_instabuy_patterns(&self) -> Vec<PatternPeriod> {
        let mut patterns = Vec::new();
        
        // Need at least 2 points to calculate deltas
        if self.buy_moving_week_history.len() < 2 || self.inferred_buy_volume_history.len() < 2 {
            return patterns;
        }
        
        for i in 1..self.buy_moving_week_history.len() {
            let moving_week_delta = self.buy_moving_week_history[i] - self.buy_moving_week_history[i-1];
            let inferred_volume = if i-1 < self.inferred_buy_volume_history.len() {
                self.inferred_buy_volume_history[i-1]
            } else {
                0
            };
            
            // Record pattern when both values are positive
            if moving_week_delta > 0 && inferred_volume > 0 {
                patterns.push(PatternPeriod {
                    position: i,
                    moving_week_delta,
                    inferred_volume,
                    timestamp: self.timestamps.get(i).copied().unwrap_or(0),
                });
            }
        }
        
        patterns
    }

    fn find_instasell_patterns(&self) -> Vec<PatternPeriod> {
        let mut patterns = Vec::new();
        
        // Need at least 2 points to calculate deltas
        if self.sell_moving_week_history.len() < 2 || self.inferred_sell_volume_history.len() < 2 {
            return patterns;
        }
        
        for i in 1..self.sell_moving_week_history.len() {
            let moving_week_delta = self.sell_moving_week_history[i] - self.sell_moving_week_history[i-1];
            let inferred_volume = if i-1 < self.inferred_sell_volume_history.len() {
                self.inferred_sell_volume_history[i-1]
            } else {
                0
            };
            
            // Record pattern when both values are positive
            if moving_week_delta > 0 && inferred_volume > 0 {
                patterns.push(PatternPeriod {
                    position: i,
                    moving_week_delta,
                    inferred_volume,
                    timestamp: self.timestamps.get(i).copied().unwrap_or(0),
                });
            }
        }
        
        patterns
    }

    fn detect_modal_pattern(pattern_periods: &[PatternPeriod]) -> Option<ModalPattern> {
        if pattern_periods.len() < 3 {
            return None;
        }
        
        // Extract MovingWeek delta values
        let mut delta_counts: HashMap<i64, usize> = HashMap::new();
        for period in pattern_periods {
            *delta_counts.entry(period.moving_week_delta).or_insert(0) += 1;
        }
        
        // Find most frequent delta (exact match phase)
        let (modal_size, occurrence_count) = delta_counts
            .iter()
            .filter(|(_, &count)| count >= 3)
            .max_by_key(|(_, &count)| count)
            .map(|(&size, &count)| (size, count))?;
        
        // Calculate frequency from timestamps of matching patterns
        let matching_timestamps: Vec<u64> = pattern_periods
            .iter()
            .filter(|p| p.moving_week_delta == modal_size)
            .map(|p| p.timestamp)
            .collect();
        
        if matching_timestamps.len() < 2 {
            return None;
        }
        
        // Calculate average interval between occurrences
        let mut intervals = Vec::new();
        for i in 1..matching_timestamps.len() {
            let interval_seconds = matching_timestamps[i].saturating_sub(matching_timestamps[i-1]);
            intervals.push(interval_seconds as f64 / 60.0); // Convert to minutes
        }
        
        let frequency_minutes = if !intervals.is_empty() {
            intervals.iter().sum::<f64>() / intervals.len() as f64
        } else {
            60.0 // Default to 1 hour if no intervals
        };
        
        let confidence = occurrence_count as f64 / pattern_periods.len() as f64;
        
        Some(ModalPattern {
            size: modal_size,
            frequency_minutes,
            occurrence_count,
            confidence,
        })
    }

    fn calculate_scaled_volume(&self, modal_pattern: &ModalPattern, total_observed: i64) -> (f64, f64) {
        // Calculate expected events in 1-hour window
        let window_minutes = 60.0;
        let expected_events = if modal_pattern.frequency_minutes > 0.0 {
            window_minutes / modal_pattern.frequency_minutes
        } else {
            1.0
        };
        
        let observed_events = modal_pattern.occurrence_count as f64;
        
        // Calculate scale factor (capped at 5.0)
        let scale_factor = if observed_events > 0.0 {
            (expected_events / observed_events).min(5.0).max(1.0)
        } else {
            1.0
        };
        
        let estimated_true_volume = total_observed as f64 * scale_factor;
        
        (scale_factor, estimated_true_volume)
    }

    fn update(&mut self, current: &BazaarInfo) {
        let current_timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        
        self.snapshot_count += 1;
        self.sum_instabuy_price += current.sell_price;
        self.sum_instasell_price += current.buy_price;

        // Update time series data
        self.buy_moving_week_history.push(current.buy_moving_week);
        self.sell_moving_week_history.push(current.sell_moving_week);
        self.timestamps.push(current_timestamp);
        
        // Maintain rolling window of 180 data points
        Self::maintain_rolling_window(&mut self.buy_moving_week_history, 180);
        Self::maintain_rolling_window(&mut self.sell_moving_week_history, 180);
        Self::maintain_rolling_window(&mut self.inferred_buy_volume_history, 180);
        Self::maintain_rolling_window(&mut self.inferred_sell_volume_history, 180);
        Self::maintain_rolling_window(&mut self.timestamps, 180);

        if let Some(prev) = &self.prev_snapshot {
            self.windows_processed += 1;

            // --- Player Instabuy Analysis (buy_orders vs buyMovingWeek) ---
            let prev_buy_offers: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let current_buy_offers: HashMap<u64, i64> = current.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let mut inferred_instabuy_volume = 0;
            let mut inferred_instabuy_events = 0;
            for (price_key, prev_amount) in &prev_buy_offers {
                let current_amount = current_buy_offers.get(price_key).unwrap_or(&0);
                if prev_amount > current_amount {
                    inferred_instabuy_volume += prev_amount - current_amount;
                    inferred_instabuy_events += 1;
                }
            }
            
            // Store inferred volume for pattern analysis
            self.inferred_buy_volume_history.push(inferred_instabuy_volume);
            
            let actual_instabuy_volume = (current.buy_moving_week - self.prev_buy_moving_week).max(0);
            self.total_buy_moving_week_activity += actual_instabuy_volume;
            
            if inferred_instabuy_events > 0 && actual_instabuy_volume > 0 {
                self.player_instabuy_event_count += inferred_instabuy_events;
                let volume_to_log = (inferred_instabuy_volume as i64).min(actual_instabuy_volume);
                self.player_instabuy_volume_total += volume_to_log as f64;
            }

            // --- Player Instasell Analysis (sell_orders vs sellMovingWeek) ---
            let prev_sell_offers: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let current_sell_offers: HashMap<u64, i64> = current.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let mut inferred_instasell_volume = 0;
            let mut inferred_instasell_events = 0;
            for (price_key, prev_amount) in &prev_sell_offers {
                let current_amount = current_sell_offers.get(price_key).unwrap_or(&0);
                if prev_amount > current_amount {
                    inferred_instasell_volume += prev_amount - current_amount;
                    inferred_instasell_events += 1;
                }
            }

            // Store inferred volume for pattern analysis
            self.inferred_sell_volume_history.push(inferred_instasell_volume);

            let actual_instasell_volume = (current.sell_moving_week - self.prev_sell_moving_week).max(0);
            self.total_sell_moving_week_activity += actual_instasell_volume;
            
            if inferred_instasell_events > 0 && actual_instasell_volume > 0 {
                self.player_instasell_event_count += inferred_instasell_events;
                let volume_to_log = (inferred_instasell_volume as i64).min(actual_instasell_volume);
                self.player_instasell_volume_total += volume_to_log as f64;
            }

            // --- New Demand Offer Analysis (new buy_orders) ---
            let prev_demand_orders: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.orders)).collect();
            let prev_demand_amount: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            for offer in &current.buy_orders {
                let key = Self::price_to_key(offer.price_per_unit);
                if let Some(prev_orders) = prev_demand_orders.get(&key) {
                    if offer.orders > *prev_orders {
                        self.total_new_demand_offers += (offer.orders - prev_orders) as f64;
                        let prev_amount = prev_demand_amount.get(&key).unwrap_or(&0);
                        if offer.amount > *prev_amount {
                            self.total_new_demand_offer_amount += (offer.amount - prev_amount) as f64;
                        }
                    }
                } else {
                    self.total_new_demand_offers += offer.orders as f64;
                    self.total_new_demand_offer_amount += offer.amount as f64;
                }
            }

            // --- New Supply Offer Analysis (new sell_orders) ---
            let prev_supply_orders: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.orders)).collect();
            let prev_supply_amount: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            for offer in &current.sell_orders {
                let key = Self::price_to_key(offer.price_per_unit);
                if let Some(prev_orders) = prev_supply_orders.get(&key) {
                    if offer.orders > *prev_orders {
                        self.total_new_supply_offers += (offer.orders - prev_orders) as f64;
                        let prev_amount = prev_supply_amount.get(&key).unwrap_or(&0);
                        if offer.amount > *prev_amount {
                            self.total_new_supply_offer_amount += (offer.amount - prev_amount) as f64;
                        }
                    }
                } else {
                    self.total_new_supply_offers += offer.orders as f64;
                    self.total_new_supply_offer_amount += offer.amount as f64;
                }
            }

            // Update pattern analysis every 10 data points to reduce overhead
            if self.windows_processed % 10 == 0 && self.buy_moving_week_history.len() >= 10 {
                self.instabuy_patterns = self.find_instabuy_patterns();
                self.instasell_patterns = self.find_instasell_patterns();
            }
        } else {
            // First update, initialize inferred volume histories
            self.inferred_buy_volume_history.push(0);
            self.inferred_sell_volume_history.push(0);
        }
        
        self.prev_snapshot = Some(current.clone());
        self.prev_buy_moving_week = current.buy_moving_week;
        self.prev_sell_moving_week = current.sell_moving_week;
    }

    fn finalize(&self, product_id: String) -> AnalysisResult {
        let windows = self.windows_processed as f64;
        let instabuy_price_average = if self.snapshot_count > 0 { self.sum_instabuy_price / self.snapshot_count as f64 } else { 0.0 };
        let instasell_price_average = if self.snapshot_count > 0 { self.sum_instasell_price / self.snapshot_count as f64 } else { 0.0 };
        let new_demand_offer_frequency_average = if windows > 0.0 { self.total_new_demand_offers / windows } else { 0.0 };
        let new_demand_offer_size_average = if self.total_new_demand_offers > 0.0 { self.total_new_demand_offer_amount / self.total_new_demand_offers } else { 0.0 };
        let new_supply_offer_frequency_average = if windows > 0.0 { self.total_new_supply_offers / windows } else { 0.0 };
        let new_supply_offer_size_average = if self.total_new_supply_offers > 0.0 { self.total_new_supply_offer_amount / self.total_new_supply_offers } else { 0.0 };
        let player_instabuy_transaction_frequency = if windows > 0.0 { self.player_instabuy_event_count as f64 / windows } else { 0.0 };
        let player_instabuy_transaction_size_average = if self.player_instabuy_event_count > 0 { self.player_instabuy_volume_total / self.player_instabuy_event_count as f64 } else { 0.0 };
        let player_instasell_transaction_frequency = if windows > 0.0 { self.player_instasell_event_count as f64 / windows } else { 0.0 };
        let player_instasell_transaction_size_average = if self.player_instasell_event_count > 0 { self.player_instasell_volume_total / self.player_instasell_event_count as f64 } else { 0.0 };
        
        // Pattern-based analysis
        let instabuy_modal_pattern = Self::detect_modal_pattern(&self.instabuy_patterns);
        let instasell_modal_pattern = Self::detect_modal_pattern(&self.instasell_patterns);
        
        let (instabuy_modal_size, instabuy_pattern_frequency, instabuy_scale_factor, instabuy_estimated_true_volume) = 
            if let Some(pattern) = &instabuy_modal_pattern {
                let (scale_factor, estimated_volume) = self.calculate_scaled_volume(pattern, self.total_buy_moving_week_activity);
                (pattern.size, pattern.frequency_minutes, scale_factor, estimated_volume)
            } else {
                (0, 0.0, 1.0, self.total_buy_moving_week_activity as f64)
            };
        
        let (instasell_modal_size, instasell_pattern_frequency, instasell_scale_factor, instasell_estimated_true_volume) = 
            if let Some(pattern) = &instasell_modal_pattern {
                let (scale_factor, estimated_volume) = self.calculate_scaled_volume(pattern, self.total_sell_moving_week_activity);
                (pattern.size, pattern.frequency_minutes, scale_factor, estimated_volume)
            } else {
                (0, 0.0, 1.0, self.total_sell_moving_week_activity as f64)
            };
        
        let pattern_detection_confidence = {
            let total_patterns = self.instabuy_patterns.len() + self.instasell_patterns.len();
            let total_possible = self.buy_moving_week_history.len() + self.sell_moving_week_history.len();
            if total_possible > 0 {
                (total_patterns as f64 / total_possible as f64) * 100.0
            } else {
                0.0
            }
        };
        
        AnalysisResult { 
            product_id, 
            instabuy_price_average, 
            instasell_price_average, 
            new_demand_offer_frequency_average, 
            new_demand_offer_size_average, 
            player_instabuy_transaction_frequency, 
            player_instabuy_transaction_size_average, 
            new_supply_offer_frequency_average, 
            new_supply_offer_size_average, 
            player_instasell_transaction_frequency, 
            player_instasell_transaction_size_average,
            instabuy_modal_size,
            instabuy_pattern_frequency,
            instabuy_scale_factor,
            instabuy_estimated_true_volume,
            instasell_modal_size,
            instasell_pattern_frequency,
            instasell_scale_factor,
            instasell_estimated_true_volume,
            pattern_detection_confidence,
        }
    }
}

async fn fetch_snapshot(last_modified: &mut Option<String>) -> Result<Option<Vec<BazaarInfo>>, Box<dyn Error>> {
    let url = "https://api.hypixel.net/v2/skyblock/bazaar";
    let resp = reqwest::get(url).await?.error_for_status()?;
    let new_mod = resp.headers().get("last-modified").and_then(|h| h.to_str().ok()).map(String::from);
    if let (Some(prev), Some(curr)) = (last_modified.as_ref(), new_mod.as_ref()) {
        if prev == curr {
            println!("Last-Modified unchanged ({}). Disposing snapshot.", curr);
            return Ok(None);
        }
    }
    *last_modified = new_mod;
    let json: Value = resp.json().await?;
    let products = json["products"].as_object().ok_or("Invalid products")?;
    let mut tasks = Vec::new();
    for (pid, prod) in products {
        let pid = pid.clone();
        let prod = prod.clone();
        tasks.push(tokio::spawn(async move {
            let instasell_price = prod["quick_status"]["buyPrice"].as_f64().unwrap_or_default();
            let instabuy_price = prod["quick_status"]["sellPrice"].as_f64().unwrap_or_default();
            
            let buy_moving_week = prod["quick_status"]["buyMovingWeek"].as_i64().unwrap_or_default();
            let sell_moving_week = prod["quick_status"]["sellMovingWeek"].as_i64().unwrap_or_default();

            let mut sell_orders_vec = Vec::new();
            if let Some(arr) = prod["sell_summary"].as_array() {
                for o in arr {
                    sell_orders_vec.push(Order {
                        amount: o["amount"].as_i64().unwrap_or_default(),
                        price_per_unit: o["pricePerUnit"].as_f64().unwrap_or_default(),
                        orders: o["orders"].as_i64().unwrap_or_default(),
                    });
                }
            }
            let mut buy_orders_vec = Vec::new();
            if let Some(arr) = prod["buy_summary"].as_array() {
                for o in arr {
                    buy_orders_vec.push(Order {
                        amount: o["amount"].as_i64().unwrap_or_default(),
                        price_per_unit: o["pricePerUnit"].as_f64().unwrap_or_default(),
                        orders: o["orders"].as_i64().unwrap_or_default(),
                    });
                }
            }
            BazaarInfo {
                product_id: pid,
                sell_price: instabuy_price,
                buy_price: instasell_price,
                sell_orders: sell_orders_vec,
                buy_orders: buy_orders_vec,
                buy_moving_week,
                sell_moving_week,
            }
        }));
    }
    let mut snapshot = Vec::new();
    for t in tasks {
        if let Ok(info) = t.await {
            snapshot.push(info);
        }
    }
    println!("Fetched snapshot with {} products", snapshot.len());
    Ok(Some(snapshot))
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    fs::create_dir_all("metrics")?;
    let mut states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut last_mod: Option<String> = None;

    let export_interval_secs = std::env::var("EXPORT_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(3600);

    let api_poll_interval_secs = std::env::var("API_POLL_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(20);

    println!("Configuration: Exporting every {} seconds, polling API every {} seconds.", export_interval_secs, api_poll_interval_secs);
    println!("Pattern detection: 180-point rolling window (1 hour), detecting modal patterns with frequency analysis.");
    let mut export_timer = Instant::now();

    loop {
        println!("💓 heartbeat at Local: {}  UTC: {}", Local::now(), Utc::now());
        match fetch_snapshot(&mut last_mod).await {
            Ok(Some(snap)) => {
                for info in snap {
                    states.entry(info.product_id.clone()).and_modify(|st| st.update(&info)).or_insert_with(|| ProductMetricsState::new(&info));
                }
                println!("Updated {} product states with new snapshot.", states.len());
                
                // Log pattern detection stats periodically
                if states.len() > 0 && export_timer.elapsed().as_secs() % 300 == 0 { // Every 5 minutes
                    let pattern_stats: Vec<_> = states.iter()
                        .filter(|(_, state)| state.instabuy_patterns.len() > 0 || state.instasell_patterns.len() > 0)
                        .map(|(id, state)| (id.clone(), state.instabuy_patterns.len(), state.instasell_patterns.len()))
                        .collect();
                    
                    if !pattern_stats.is_empty() {
                        println!("Pattern detection active for {} products", pattern_stats.len());
                    }
                }
            }
            Ok(None) => println!("No new snapshot data from API (Last-Modified unchanged)."),
            Err(e) => eprintln!("Error fetching snapshot: {}", e),
        }

        if export_timer.elapsed() >= Duration::from_secs(export_interval_secs) {
            println!(">>> Exporting metrics after {} seconds…", export_timer.elapsed().as_secs());
            if !states.is_empty() {
                let results: Vec<_> = states.iter().map(|(pid, st)| st.finalize(pid.clone())).collect();
                let ts = Utc::now().format("%Y%m%d%H%M%S").to_string();
                
                let local_path = format!("metrics/metrics_{}.json", ts);
                let remote_mega_path = format!("/remote_metrics/metrics_{}.json", ts);
                
                println!("Attempting to export local file '{}' to remote path '{}'", local_path, remote_mega_path);

                // Log some pattern detection statistics
                let with_patterns = results.iter().filter(|r| r.instabuy_modal_size > 0 || r.instasell_modal_size > 0).count();
                let avg_confidence = results.iter().map(|r| r.pattern_detection_confidence).sum::<f64>() / results.len() as f64;
                println!("Pattern detection summary: {}/{} products with patterns, avg confidence: {:.1}%", with_patterns, results.len(), avg_confidence);

                match fs::write(&local_path, serde_json::to_string_pretty(&results)?) {
                    Ok(_) => {
                        println!("Successfully wrote metrics for {} products to {}", results.len(), local_path);
                        let export_engine_path = std::env::var("EXPORT_ENGINE_PATH").unwrap_or_else(|_| "export_engine".to_string());
                        
                        match Command::new(&export_engine_path)
                            .arg(&local_path)
                            .arg(&remote_mega_path)
                            .output() {
                            Ok(output) => {
                                println!("Export engine stdout:\n{}", String::from_utf8_lossy(&output.stdout));
                                if !output.stderr.is_empty() { eprintln!("Export engine stderr:\n{}", String::from_utf8_lossy(&output.stderr)); }
                            }
                            Err(e) => eprintln!("Failed to execute export_engine: {}", e),
                        }
                    }
                    Err(e) => eprintln!("Failed to write metrics file {}: {}", local_path, e),
                }
            } else {
                println!("No state to export this round.");
            }
            states.clear();
            export_timer = Instant::now();
        }
        sleep(Duration::from_secs(api_poll_interval_secs)).await;
    }
}