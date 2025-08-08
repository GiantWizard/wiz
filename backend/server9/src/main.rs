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

#[derive(Debug, Clone, Deserialize, Serialize)]
struct Order {
    amount: i64,
    price_per_unit: f64,
    orders: i64,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
struct BazaarInfo {
    product_id: String,
    buy_price: f64,
    sell_price: f64,
    buy_orders: Vec<Order>,
    sell_orders: Vec<Order>,
    buy_moving_week: i64,
    sell_moving_week: i64,
}

#[derive(Debug, Clone)]
struct PatternPeriod {
    position: usize,
    moving_week_delta: i64,
    inferred_volume: i64,
    timestamp: u64,
}

#[derive(Debug, Clone)]
struct ModalPattern {
    size: i64,
    ratio: f64,
    frequency_minutes: f64,
    occurrence_count: usize,
    confidence: f64,
}

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
    buy_moving_week_history: Vec<i64>,
    sell_moving_week_history: Vec<i64>,
    inferred_buy_volume_history: Vec<i64>,
    inferred_sell_volume_history: Vec<i64>,
    timestamps: Vec<u64>,
    total_buy_moving_week_activity: i64,
    total_sell_moving_week_activity: i64,
}

impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
        let current_timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        Self {
            sum_instabuy_price: first.buy_price,
            sum_instasell_price: first.sell_price,
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
            buy_moving_week_history: vec![first.buy_moving_week],
            sell_moving_week_history: vec![first.sell_moving_week],
            inferred_buy_volume_history: vec![],
            inferred_sell_volume_history: vec![],
            timestamps: vec![current_timestamp],
            total_buy_moving_week_activity: 0,
            total_sell_moving_week_activity: 0,
        }
    }

    fn price_to_key(price: f64) -> u64 { 
        (price * 1000.0).round() as u64 
    }

    fn update(&mut self, current: &BazaarInfo) {
        let current_timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        self.snapshot_count += 1;
        self.sum_instabuy_price += current.buy_price;
        self.sum_instasell_price += current.sell_price;

        self.buy_moving_week_history.push(current.buy_moving_week);
        self.sell_moving_week_history.push(current.sell_moving_week);
        self.timestamps.push(current_timestamp);

        if let Some(prev) = &self.prev_snapshot {
            self.windows_processed += 1;

            // INSTABUY: buy_orders (buysummary) and buy_moving_week
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
            self.inferred_buy_volume_history.push(inferred_instabuy_volume);
            let actual_instabuy_volume = (current.buy_moving_week - self.prev_buy_moving_week).max(0);
            self.total_buy_moving_week_activity += actual_instabuy_volume;
            
            // PHASE 1 FIX: Always trust detected volume when we detect events
            if inferred_instabuy_events > 0 {
                self.player_instabuy_event_count += inferred_instabuy_events;
                // Remove the .min() cap - always log detected volume
                self.player_instabuy_volume_total += inferred_instabuy_volume as f64;
            }

            // INSTASELL: sell_orders (sellsummmary) and sell_moving_week
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
            self.inferred_sell_volume_history.push(inferred_instasell_volume);
            let actual_instasell_volume = (current.sell_moving_week - self.prev_sell_moving_week).max(0);
            self.total_sell_moving_week_activity += actual_instasell_volume;
            
            // PHASE 1 FIX: Always trust detected volume when we detect events
            if inferred_instasell_events > 0 {
                self.player_instasell_event_count += inferred_instasell_events;
                // Remove the .min() cap - always log detected volume
                self.player_instasell_volume_total += inferred_instasell_volume as f64;
            }

            // New demand offers (buy_orders)
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

            // New supply offers (sell_orders)
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
        } else {
            self.inferred_buy_volume_history.push(0);
            self.inferred_sell_volume_history.push(0);
        }
        self.prev_snapshot = Some(current.clone());
        self.prev_buy_moving_week = current.buy_moving_week;
        self.prev_sell_moving_week = current.sell_moving_week;
    }

    fn find_patterns(
        moving_week_history: &[i64],
        inferred_volume_history: &[i64],
        timestamps: &[u64],
    ) -> Vec<PatternPeriod> {
        let mut patterns = Vec::new();
        for i in 1..moving_week_history.len().min(inferred_volume_history.len()).min(timestamps.len()) {
            let delta = moving_week_history[i] - moving_week_history[i - 1];
            let inferred = inferred_volume_history[i - 1];
            if delta > 0 && inferred > 0 {
                patterns.push(PatternPeriod {
                    position: i,
                    moving_week_delta: delta,
                    inferred_volume: inferred,
                    timestamp: timestamps[i],
                });
            }
        }
        patterns
    }

    fn detect_modal_pattern(pattern_periods: &[PatternPeriod]) -> Option<ModalPattern> {
        if pattern_periods.len() < 3 {
            return None;
        }
        // Group patterns by (delta, ratio)
        let mut cluster_map: HashMap<(i64, i64), Vec<PatternPeriod>> = HashMap::new();
        for p in pattern_periods {
            let ratio = if p.inferred_volume > 0 {
                (p.moving_week_delta as f64 / p.inferred_volume as f64 * 10000.0).round() as i64
            } else {
                0
            };
            cluster_map
                .entry((p.moving_week_delta, ratio))
                .or_default()
                .push(p.clone());
        }
        // Find cluster with most entries where it appears at least 3 times
        let mut modal: Option<(Vec<PatternPeriod>, i64, i64)> = None;
        for ((delta, ratio), cluster) in &cluster_map {
            if cluster.len() >= 3
                && (modal.is_none() || cluster.len() > modal.as_ref().unwrap().0.len())
            {
                modal = Some((cluster.clone(), *delta, *ratio));
            }
        }
        // If no exact modal, try cluster by ratio within 10% tolerance
        if modal.is_none() {
            let mut ratio_map: HashMap<i64, Vec<PatternPeriod>> = HashMap::new();
            for p in pattern_periods {
                let ratio = if p.inferred_volume > 0 {
                    (p.moving_week_delta as f64 / p.inferred_volume as f64 * 10000.0).round() as i64
                } else {
                    0
                };
                ratio_map.entry(ratio).or_default().push(p.clone());
            }
            for (ratio, cluster) in &ratio_map {
                if cluster.len() < 3 {
                    continue;
                }
                // Find average delta for this ratio cluster
                let avg_delta = cluster.iter().map(|p| p.moving_week_delta).sum::<i64>() / cluster.len() as i64;
                // Accept cluster if all deltas are within 10% of mean
                if cluster.iter().all(|p| (p.moving_week_delta - avg_delta).abs() <= (avg_delta as f64 * 0.1).max(1.0) as i64) {
                    if modal.is_none() || cluster.len() > modal.as_ref().unwrap().0.len() {
                        modal = Some((cluster.clone(), avg_delta, *ratio));
                    }
                }
            }
        }
        let (pattern_set, modal_size, modal_ratio) = match modal {
            Some(v) => v,
            None => return None,
        };
        let timestamps: Vec<u64> = pattern_set.iter().map(|p| p.timestamp).collect();
        if timestamps.len() < 2 {
            return None;
        }
        let mut intervals = Vec::new();
        for i in 1..timestamps.len() {
            let interval_seconds = timestamps[i].saturating_sub(timestamps[i - 1]);
            intervals.push(interval_seconds as f64 / 60.0);
        }
        let frequency_minutes = if !intervals.is_empty() {
            intervals.iter().sum::<f64>() / intervals.len() as f64
        } else {
            60.0
        };
        let confidence = pattern_set.len() as f64 / pattern_periods.len() as f64;
        Some(ModalPattern {
            size: modal_size,
            ratio: modal_ratio as f64 / 10000.0,
            frequency_minutes,
            occurrence_count: pattern_set.len(),
            confidence,
        })
    }

    // PHASE 4 FIX: Volume-aware scaling instead of event-based scaling
    fn calculate_scaled_volume(
        modal_pattern: &ModalPattern, 
        total_observed: i64,
        detected_volume_total: f64,
        executed_volume_total: i64
    ) -> (f64, f64) {
        // Calculate volume coverage ratio
        let volume_coverage = if executed_volume_total > 0 {
            detected_volume_total / executed_volume_total as f64
        } else {
            1.0
        };

        // Only scale if we're missing significant volume
        let scale_factor = if volume_coverage < 0.7 {
            // Conservative scaling based on volume coverage
            (1.0 / volume_coverage).min(2.0).max(1.0)
        } else {
            // High volume coverage: no scaling needed
            1.0
        };

        let estimated_true_volume = executed_volume_total as f64 * scale_factor;
        (scale_factor, estimated_true_volume)
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
        let instabuy_patterns = Self::find_patterns(&self.buy_moving_week_history, &self.inferred_buy_volume_history, &self.timestamps);
        let instasell_patterns = Self::find_patterns(&self.sell_moving_week_history, &self.inferred_sell_volume_history, &self.timestamps);
        let instabuy_modal_pattern = Self::detect_modal_pattern(&instabuy_patterns);
        let instasell_modal_pattern = Self::detect_modal_pattern(&instasell_patterns);

        // PHASE 4 FIX: Use volume-aware scaling for both buy and sell
        let (instabuy_modal_size, instabuy_pattern_frequency, instabuy_scale_factor, instabuy_estimated_true_volume) = 
            if let Some(pattern) = &instabuy_modal_pattern {
                let (scale_factor, estimated_volume) = Self::calculate_scaled_volume(
                    pattern, 
                    self.total_buy_moving_week_activity,
                    self.player_instabuy_volume_total,
                    self.total_buy_moving_week_activity
                );
                (pattern.size, pattern.frequency_minutes, scale_factor, estimated_volume)
            } else {
                // No pattern detected: use moving week as ground truth
                (0, 0.0, 1.0, self.total_buy_moving_week_activity as f64)
            };

        let (instasell_modal_size, instasell_pattern_frequency, instasell_scale_factor, instasell_estimated_true_volume) = 
            if let Some(pattern) = &instasell_modal_pattern {
                let (scale_factor, estimated_volume) = Self::calculate_scaled_volume(
                    pattern, 
                    self.total_sell_moving_week_activity,
                    self.player_instasell_volume_total,
                    self.total_sell_moving_week_activity
                );
                (pattern.size, pattern.frequency_minutes, scale_factor, estimated_volume)
            } else {
                // No pattern detected: use moving week as ground truth
                (0, 0.0, 1.0, self.total_sell_moving_week_activity as f64)
            };

        let pattern_detection_confidence = {
            let total_patterns = instabuy_patterns.len() + instasell_patterns.len();
            let total_possible = self.buy_moving_week_history.len().saturating_sub(1) + self.sell_moving_week_history.len().saturating_sub(1);
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
            let instabuy_price = prod["quick_status"]["buyPrice"].as_f64().unwrap_or_default();
            let instasell_price = prod["quick_status"]["sellPrice"].as_f64().unwrap_or_default();
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
                buy_price: instabuy_price,
                sell_price: instasell_price,
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
    println!("Pattern detection: 1-hour window, modal pattern detection, 1:1 or ratio clusters, 10% tolerance, minimum 3 occurrences.");
    let mut export_timer = Instant::now();

    loop {
        println!("💓 heartbeat at Local: {}  UTC: {}", Local::now(), Utc::now());
        match fetch_snapshot(&mut last_mod).await {
            Ok(Some(snap)) => {
                for info in snap {
                    states.entry(info.product_id.clone()).and_modify(|st| st.update(&info)).or_insert_with(|| ProductMetricsState::new(&info));
                }
                println!("Updated {} product states with new snapshot.", states.len());
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