// file: server9/src/main.rs

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
    // Best price a player PAYS to Instabuy
    buy_price: f64,
    // Best price a player RECEIVES when Instaselling
    sell_price: f64,
    // Full list of SELL ORDERS (player supply)
    buy_orders: Vec<Order>,
    // Full list of BUY ORDERS (player demand)
    sell_orders: Vec<Order>,
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
}

impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
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
        }
    }

    fn price_to_key(price: f64) -> u64 { (price * 1000.0).round() as u64 }

    fn update(&mut self, current: &BazaarInfo) {
        self.snapshot_count += 1;
        self.sum_instabuy_price += current.buy_price;
        self.sum_instasell_price += current.sell_price;

        if let Some(prev) = &self.prev_snapshot {
            self.windows_processed += 1;
            let prev_sell_offers: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let current_sell_offers: HashMap<u64, i64> = current.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let mut consumed_volume = 0;
            for (price_key, prev_amount) in &prev_sell_offers { if let Some(current_amount) = current_sell_offers.get(price_key) { if prev_amount > current_amount { consumed_volume += prev_amount - current_amount; } } else { consumed_volume += prev_amount; } }
            if consumed_volume > 0 { self.player_instabuy_event_count += 1; self.player_instabuy_volume_total += consumed_volume as f64; }
            let prev_buy_offers: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let current_buy_offers: HashMap<u64, i64> = current.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            let mut fulfilled_volume = 0;
            for (price_key, prev_amount) in &prev_buy_offers { if let Some(current_amount) = current_buy_offers.get(price_key) { if prev_amount > current_amount { fulfilled_volume += prev_amount - current_amount; } } else { fulfilled_volume += prev_amount; } }
            if fulfilled_volume > 0 { self.player_instasell_event_count += 1; self.player_instasell_volume_total += fulfilled_volume as f64; }
            let prev_demand_orders: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.orders)).collect();
            let prev_demand_amount: HashMap<u64, i64> = prev.sell_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            for offer in ¤t.sell_orders { let key = Self::price_to_key(offer.price_per_unit); if let Some(prev_orders) = prev_demand_orders.get(&key) { if offer.orders > *prev_orders { self.total_new_demand_offers += (offer.orders - prev_orders) as f64; let prev_amount = prev_demand_amount.get(&key).unwrap_or(&0); if offer.amount > *prev_amount { self.total_new_demand_offer_amount += (offer.amount - prev_amount) as f64; } } } else { self.total_new_demand_offers += offer.orders as f64; self.total_new_demand_offer_amount += offer.amount as f64; } }
            let prev_supply_orders: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.orders)).collect();
            let prev_supply_amount: HashMap<u64, i64> = prev.buy_orders.iter().map(|o| (Self::price_to_key(o.price_per_unit), o.amount)).collect();
            for offer in ¤t.buy_orders { let key = Self::price_to_key(offer.price_per_unit); if let Some(prev_orders) = prev_supply_orders.get(&key) { if offer.orders > *prev_orders { self.total_new_supply_offers += (offer.orders - prev_orders) as f64; let prev_amount = prev_supply_amount.get(&key).unwrap_or(&0); if offer.amount > *prev_amount { self.total_new_supply_offer_amount += (offer.amount - prev_amount) as f64; } } } else { self.total_new_supply_offers += offer.orders as f64; self.total_new_supply_offer_amount += offer.amount as f64; } }
        }
        self.prev_snapshot = Some(current.clone());
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
        AnalysisResult { product_id, instabuy_price_average, instasell_price_average, new_demand_offer_frequency_average, new_demand_offer_size_average, player_instabuy_transaction_frequency, player_instabuy_transaction_size_average, new_supply_offer_frequency_average, new_supply_offer_size_average, player_instasell_transaction_frequency, player_instasell_transaction_size_average }
    }
}

async fn fetch_snapshot(last_modified: &mut Option<String>) -> Result<Option<Vec<BazaarInfo>>, Box<dyn Error>> {
    let url = "https://api.hypixel.net/v2/skyblock/bazaar";
    let resp = reqwest::get(url).await?.error_for_status()?;
    let new_mod = resp.headers().get("last-modified").and_then(|h| h.to_str().ok()).map(String::from);
    if let (Some(prev), Some(curr)) = (last_modified.as_ref(), new_mod.as_ref()) {
        if prev == curr { println!("Last-Modified unchanged ({}). Disposing snapshot.", curr); return Ok(None); }
    }
    *last_modified = new_mod;
    let json: Value = resp.json().await?;
    let products = json["products"].as_object().ok_or("Invalid products")?;
    let mut tasks = Vec::new();
    for (pid, prod) in products {
        let pid = pid.clone(); let prod = prod.clone();
        tasks.push(tokio::spawn(async move {
            let instasell_price = prod["sell_summary"].get(0).and_then(|o| o["pricePerUnit"].as_f64()).unwrap_or_default();
            let instabuy_price = prod["buy_summary"].get(0).and_then(|o| o["pricePerUnit"].as_f64()).unwrap_or_default();
            let mut sell_orders_vec = Vec::new(); if let Some(arr) = prod["sell_summary"].as_array() { for o in arr { sell_orders_vec.push(Order { amount: o["amount"].as_i64().unwrap_or_default(), price_per_unit: o["pricePerUnit"].as_f64().unwrap_or_default(), orders: o["orders"].as_i64().unwrap_or_default() }); } }
            let mut buy_orders_vec = Vec::new(); if let Some(arr) = prod["buy_summary"].as_array() { for o in arr { buy_orders_vec.push(Order { amount: o["amount"].as_i64().unwrap_or_default(), price_per_unit: o["pricePerUnit"].as_f64().unwrap_or_default(), orders: o["orders"].as_i64().unwrap_or_default() }); } }
            BazaarInfo { product_id: pid, sell_price: instasell_price, buy_price: instabuy_price, sell_orders: sell_orders_vec, buy_orders: buy_orders_vec }
        }));
    }
    let mut snapshot = Vec::new(); for t in tasks { if let Ok(info) = t.await { snapshot.push(info); } }
    println!("Fetched snapshot with {} products", snapshot.len());
    Ok(Some(snapshot))
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    fs::create_dir_all("metrics")?;
    let mut states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut last_mod: Option<String> = None;

    // Set default export interval to 300 seconds (5 minutes) for faster testing.
    let export_interval_secs = std::env::var("EXPORT_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(300);

    let api_poll_interval_secs = std::env::var("API_POLL_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(20);

    println!("Configuration: Exporting every {} seconds, polling API every {} seconds.", export_interval_secs, api_poll_interval_secs);
    let mut export_timer = Instant::now();
    loop {
        println!("💓 heartbeat at Local: {}  UTC: {}", Local::now(), Utc::now());
        match fetch_snapshot(&mut last_mod).await {
            Ok(Some(snap)) => { for info in snap { states.entry(info.product_id.clone()).and_modify(|st| st.update(&info)).or_insert_with(|| ProductMetricsState::new(&info)); } println!("Updated {} product states with new snapshot.", states.len()); }
            Ok(None) => println!("No new snapshot data from API (Last-Modified unchanged)."),
            Err(e) => eprintln!("Error fetching snapshot: {}", e),
        }

        if export_timer.elapsed() >= Duration::from_secs(export_interval_secs) {
            println!(">>> Exporting metrics after {} seconds…", export_timer.elapsed().as_secs());
            if !states.is_empty() {
                let results: Vec<_> = states.iter().map(|(pid, st)| st.finalize(pid.clone())).collect();
                let ts = Utc::now().format("%Y%m%d%H%M%S").to_string();
                
                // The local path still needs the .json extension for the content.
                let local_path = format!("metrics/metrics_{}.json", ts);
                // The remote path is the desired filename in the root of the MEGA drive.
                let remote_mega_path = format!("/metrics_{}", ts);
                
                println!("Attempting to export local file '{}' to remote path '{}'", local_path, remote_mega_path);

                match fs::write(&local_path, serde_json::to_string_pretty(&results)?) {
                    Ok(_) => {
                        println!("Successfully wrote metrics for {} products to {}", results.len(), local_path);
                        let export_engine_path = std::env::var("EXPORT_ENGINE_PATH").unwrap_or_else(|_| "export_engine".to_string());
                        
                        // Pass the correct local path and the full desired remote path.
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