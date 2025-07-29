use chrono::{Utc, Local};
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
use std::process::Command;
use std::time::{Duration, Instant};
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
    // --- ADDED ---
    buy_moving_week: i64,  // Total items bought by players via instabuy
    sell_moving_week: i64, // Total items sold by players via instasell
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
    // --- ADDED ---
    prev_buy_moving_week: i64,
    prev_sell_moving_week: i64,
}

// Replace the existing ProductMetricsState implementation with this new one.
impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
        Self {
            // The price to instabuy is the top sell_price
            sum_instabuy_price: first.sell_price,
            // The price to instasell is the top buy_price
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
        }
    }

    fn price_to_key(price: f64) -> u64 { (price * 1000.0).round() as u64 }

    fn update(&mut self, current: &BazaarInfo) {
        self.snapshot_count += 1;
        self.sum_instabuy_price += current.sell_price;
        self.sum_instasell_price += current.buy_price;

        if let Some(prev) = &self.prev_snapshot {
            self.windows_processed += 1;

            // --- Player Instabuy Analysis (consuming sell_orders) ---
            let prev_sell_offers: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let current_sell_offers: HashMap<u64, i64> = current.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let mut inferred_instabuy_volume = 0;
            let mut inferred_instabuy_events = 0;
            for (price_key, prev_amount) in &prev_sell_offers {
                let current_amount = current_sell_offers.get(price_key).unwrap_or(&0);
                if prev_amount > current_amount {
                    inferred_instabuy_volume += prev_amount - current_amount;
                    inferred_instabuy_events += 1;
                }
            }
            
            let actual_instabuy_volume = (current.buy_moving_week - self.prev_buy_moving_week).max(0);
            if inferred_instabuy_volume > 0 && actual_instabuy_volume > 0 {
                self.player_instabuy_event_count += inferred_instabuy_events;
                self.player_instabuy_volume_total += actual_instabuy_volume as f64;
            }

            // --- Player Instasell Analysis (fulfilling buy_orders) ---
            let prev_buy_offers: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let current_buy_offers: HashMap<u64, i64> = current.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let mut inferred_instasell_volume = 0;
            let mut inferred_instasell_events = 0;
            for (price_key, prev_amount) in &prev_buy_offers {
                let current_amount = current_buy_offers.get(price_key).unwrap_or(&0);
                if prev_amount > current_amount {
                    inferred_instasell_volume += prev_amount - current_amount;
                    inferred_instasell_events += 1;
                }
            }

            let actual_instasell_volume = (current.sell_moving_week - self.prev_sell_moving_week).max(0);
            if inferred_instasell_volume > 0 && actual_instasell_volume > 0 {
                self.player_instasell_event_count += inferred_instasell_events;
                self.player_instasell_volume_total += actual_instasell_volume as f64;
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
        let player_instabuy_transaction_size_average = if self.player_instabuy_volume_total > 0.0 { self.player_instabuy_volume_total / self.player_instabuy_event_count as f64 } else { 0.0 };
        let player_instasell_transaction_frequency = if windows > 0.0 { self.player_instasell_event_count as f64 / windows } else { 0.0 };
        let player_instasell_transaction_size_average = if self.player_instasell_volume_total > 0.0 { self.player_instasell_volume_total / self.player_instasell_event_count as f64 } else { 0.0 };
        AnalysisResult { product_id, instabuy_price_average, instasell_price_average, new_demand_offer_frequency_average, new_demand_offer_size_average, player_instabuy_transaction_frequency, player_instabuy_transaction_size_average, new_supply_offer_frequency_average, new_supply_offer_size_average, player_instasell_transaction_frequency, player_instasell_transaction_size_average }
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
            // Correctly parse prices based on API definitions
            let instasell_price = prod["quick_status"]["buyPrice"].as_f64().unwrap_or_default();
            let instabuy_price = prod["quick_status"]["sellPrice"].as_f64().unwrap_or_default();
            
            // --- ADDED ---
            // Parse the moving week data from the quick_status field.
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
                // --- ADDED ---
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