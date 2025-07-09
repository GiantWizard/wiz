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
    sell_price: f64,
    buy_price: f64,
    buy_moving_week: i64,  // Used to calculate sell frequency/size
    sell_moving_week: i64, // Used to calculate buy frequency/size
    sell_volume: i64,
    buy_volume: i64,
    sell_orders: Vec<Order>,
    buy_orders: Vec<Order>,
}

/// Holds the final analysis metrics for one product.
#[derive(Debug, Serialize)]
struct AnalysisResult {
    product_id: String,
    buy_price_average: f64,
    sell_price_average: f64,
    // Sell-side metrics
    sell_order_frequency_average: f64,
    sell_order_size_average: f64,
    sell_frequency: f64,
    sell_size: f64,
    // Buy-side metrics
    buy_order_frequency_average: f64,
    buy_order_size_average: f64,
    buy_frequency: f64,
    buy_size: f64,
}

/// Incremental state for each product; updated on each new snapshot.
#[derive(Debug)]
struct ProductMetricsState {
    sum_buy: f64,
    sum_sell: f64,
    count: usize,
    windows: usize,
    // Sell-side state
    sell_order_frequency_sum: f64,
    sell_order_frequency_count: usize,
    total_new_sell_orders: f64,
    total_new_sell_order_amount: f64,
    sell_changes_count: usize,
    sell_size_total: f64,
    // Buy-side state
    buy_order_frequency_sum: f64,
    buy_order_frequency_count: usize,
    total_new_buy_orders: f64,
    total_new_buy_order_amount: f64,
    buy_changes_count: usize,
    buy_size_total: f64,
    // Previous state for comparison
    prev_snapshot: Option<BazaarInfo>,
}

impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
        Self {
            sum_buy: first.buy_price,
            sum_sell: first.sell_price,
            count: 1,
            windows: 0,
            sell_order_frequency_sum: 0.0,
            sell_order_frequency_count: 0,
            total_new_sell_orders: 0.0,
            total_new_sell_order_amount: 0.0,
            sell_changes_count: 0,
            sell_size_total: 0.0,
            buy_order_frequency_sum: 0.0,
            buy_order_frequency_count: 0,
            total_new_buy_orders: 0.0,
            total_new_buy_order_amount: 0.0,
            buy_changes_count: 0,
            buy_size_total: 0.0,
            prev_snapshot: Some(first.clone()),
        }
    }

    fn update(&mut self, current: &BazaarInfo) {
        self.count += 1;
        self.sum_buy += current.buy_price;
        self.sum_sell += current.sell_price;

        if let Some(prev) = &self.prev_snapshot {
            self.windows += 1;

            // --- Sell-Side Analysis ---
            // Sell Order frequency & size (how many new sell orders appear)
            if prev.sell_orders.len() > 1 && !current.sell_orders.is_empty() {
                let anchor = &prev.sell_orders[1]; // Use 2nd best offer as stable point
                if let Some(idx) = current.sell_orders.iter().position(|o| (o.price_per_unit - anchor.price_per_unit).abs() < 1e-6) {
                    let new_orders = if idx > 0 { idx } else { 0 }; // All orders before anchor
                    self.sell_order_frequency_sum += new_orders as f64;
                    self.sell_order_frequency_count += 1;
                    if new_orders > 0 {
                        let amount: i64 = current.sell_orders.iter().take(new_orders).map(|o| o.amount).sum();
                        self.total_new_sell_order_amount += amount as f64;
                        self.total_new_sell_orders += new_orders as f64;
                    }
                }
            }
            // Sell frequency & size (how often and how much is sold)
            let sell_diff = current.buy_moving_week - prev.buy_moving_week;
            if sell_diff != 0 {
                self.sell_changes_count += 1;
                self.sell_size_total += sell_diff.abs() as f64;
            }

            // --- Buy-Side Analysis ---
            // Buy Order frequency & size (how many new buy orders appear)
            if prev.buy_orders.len() > 1 && !current.buy_orders.is_empty() {
                let anchor = &prev.buy_orders[1]; // Use 2nd best offer as stable point
                if let Some(idx) = current.buy_orders.iter().position(|o| (o.price_per_unit - anchor.price_per_unit).abs() < 1e-6) {
                    let new_orders = if idx > 0 { idx } else { 0 };
                    self.buy_order_frequency_sum += new_orders as f64;
                    self.buy_order_frequency_count += 1;
                    if new_orders > 0 {
                        let amount: i64 = current.buy_orders.iter().take(new_orders).map(|o| o.amount).sum();
                        self.total_new_buy_order_amount += amount as f64;
                        self.total_new_buy_orders += new_orders as f64;
                    }
                }
            }
            // Buy frequency & size (how often and how much is bought)
            let buy_diff = current.sell_moving_week - prev.sell_moving_week;
            if buy_diff != 0 {
                self.buy_changes_count += 1;
                self.buy_size_total += buy_diff.abs() as f64;
            }
        }

        self.prev_snapshot = Some(current.clone());
    }

    fn finalize(&self, product_id: String) -> AnalysisResult {
        let buy_price_average = self.sum_buy / self.count as f64;
        let sell_price_average = self.sum_sell / self.count as f64;

        // Finalize sell-side metrics
        let sell_order_frequency_average = if self.sell_order_frequency_count > 0 { self.sell_order_frequency_sum / self.sell_order_frequency_count as f64 } else { 0.0 };
        let sell_order_size_average = if self.total_new_sell_orders > 0.0 { self.total_new_sell_order_amount / self.total_new_sell_orders } else { 0.0 };
        let sell_frequency = if self.windows > 0 { self.sell_changes_count as f64 / self.windows as f64 } else { 0.0 };
        let sell_size = if self.sell_changes_count > 0 { self.sell_size_total / self.sell_changes_count as f64 } else { 0.0 };

        // Finalize buy-side metrics
        let buy_order_frequency_average = if self.buy_order_frequency_count > 0 { self.buy_order_frequency_sum / self.buy_order_frequency_count as f64 } else { 0.0 };
        let buy_order_size_average = if self.total_new_buy_orders > 0.0 { self.total_new_buy_order_amount / self.total_new_buy_orders } else { 0.0 };
        let buy_frequency = if self.windows > 0 { self.buy_changes_count as f64 / self.windows as f64 } else { 0.0 };
        let buy_size = if self.buy_changes_count > 0 { self.buy_size_total / self.buy_changes_count as f64 } else { 0.0 };

        AnalysisResult {
            product_id,
            buy_price_average,
            sell_price_average,
            sell_order_frequency_average,
            sell_order_size_average,
            sell_frequency,
            sell_size,
            buy_order_frequency_average,
            buy_order_size_average,
            buy_frequency,
            buy_size,
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
            let sell_price = prod["sell_summary"][0]["pricePerUnit"].as_f64().unwrap_or_default();
            let buy_price = prod["buy_summary"][0]["pricePerUnit"].as_f64().unwrap_or_default();
            
            let quick_status = &prod["quick_status"];
            let buy_moving_week = quick_status["buyMovingWeek"].as_i64().unwrap_or_default();
            let sell_moving_week = quick_status["sellMovingWeek"].as_i64().unwrap_or_default();
            let sell_volume = quick_status["sellVolume"].as_i64().unwrap_or_default();
            let buy_volume = quick_status["buyVolume"].as_i64().unwrap_or_default();

            let mut sell_orders = Vec::new();
            if let Some(arr) = prod["sell_summary"].as_array() {
                for o in arr {
                    sell_orders.push(Order {
                        amount: o["amount"].as_i64().unwrap_or_default(),
                        price_per_unit: o["pricePerUnit"].as_f64().unwrap_or_default(),
                        orders: o["orders"].as_i64().unwrap_or_default(),
                    });
                }
            }

            let mut buy_orders = Vec::new();
            if let Some(arr) = prod["buy_summary"].as_array() {
                for o in arr {
                    buy_orders.push(Order {
                        amount: o["amount"].as_i64().unwrap_or_default(),
                        price_per_unit: o["pricePerUnit"].as_f64().unwrap_or_default(),
                        orders: o["orders"].as_i64().unwrap_or_default(),
                    });
                }
            }

            BazaarInfo {
                product_id: pid,
                sell_price,
                buy_price,
                buy_moving_week,
                sell_moving_week,
                sell_volume,
                buy_volume,
                sell_orders,
                buy_orders,
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
    let remote_dir = std::env::var("MEGA_METRICS_FOLDER_PATH").unwrap_or_else(|_| "/remote_metrics".to_string());
    
    let mut states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut last_mod: Option<String> = None;

    let export_interval_secs = std::env::var("EXPORT_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(3600); // Default 1 hour
    let api_poll_interval_secs = std::env::var("API_POLL_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(20); // Default 1 minute

    println!("Configuration: Exporting every {} seconds, polling API every {} seconds.", export_interval_secs, api_poll_interval_secs);

    let mut export_timer = Instant::now();

    loop {
        println!("💓 heartbeat at Local: {}  UTC: {}", Local::now(), Utc::now());

        match fetch_snapshot(&mut last_mod).await {
            Ok(Some(snap)) => {
                for info in snap {
                    states
                        .entry(info.product_id.clone())
                        .and_modify(|st| st.update(&info))
                        .or_insert_with(|| ProductMetricsState::new(&info));
                }
                println!("Updated {} product states with new snapshot.", states.len());
            }
            Ok(None) => println!("No new snapshot data from API (Last-Modified unchanged)."),
            Err(e) => eprintln!("Error fetching snapshot: {}", e),
        }

        if export_timer.elapsed() >= Duration::from_secs(export_interval_secs) {
            println!(">>> Exporting metrics after {} seconds…", export_timer.elapsed().as_secs());
            if !states.is_empty() {
                let results: Vec<_> = states
                    .iter()
                    .map(|(pid, st)| st.finalize(pid.clone()))
                    .collect();
                let ts = Utc::now().format("%Y%m%d%H%M%S").to_string();
                let path = format!("metrics/metrics_{}.json", ts);
                
                match fs::write(&path, serde_json::to_string_pretty(&results)?) {
                    Ok(_) => {
                        println!("Exported metrics for {} products to {}", results.len(), path);

                        let export_engine_path = std::env::var("EXPORT_ENGINE_PATH").unwrap_or_else(|_| "export_engine".to_string());
                        match Command::new(&export_engine_path).arg(&path).arg(&remote_dir).output() {
                            Ok(output) => {
                                println!("Export engine stdout:\n{}", String::from_utf8_lossy(&output.stdout));
                                if !output.stderr.is_empty() {
                                    eprintln!("Export engine stderr:\n{}", String::from_utf8_lossy(&output.stderr));
                                }
                            }
                            Err(e) => eprintln!("Failed to execute export_engine: {}", e),
                        }
                    }
                    Err(e) => eprintln!("Failed to write metrics file {}: {}", path, e),
                }
            } else {
                println!("No state to export this round.");
            }
            // Reset state for the next aggregation window
            states.clear();
            export_timer = Instant::now();
        }

        sleep(Duration::from_secs(api_poll_interval_secs)).await;
    }
}