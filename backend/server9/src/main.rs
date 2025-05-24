use axum::{routing::get, Router}; // Added for health check
use chrono::{Utc, Local};
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
use std::net::SocketAddr; // Added for health check
use std::process::Command;
use std::time::{Duration, Instant};
use tokio::time::sleep;

/// Represents an individual sell order.
#[derive(Debug, Clone, Deserialize, Serialize)]
struct SellOrder {
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
    buy_moving_week: i64,
    sell_volume: i64,
    /// List of sell orders from the snapshot.
    sell_orders: Vec<SellOrder>,
}

/// Holds the final analysis metrics for one product.
#[derive(Debug, Serialize)]
struct AnalysisResult {
    product_id: String,
    buy_price_average: f64,
    sell_price_average: f64,
    order_frequency_average: f64,
    order_size_average: f64,
    sell_frequency: f64,
    sell_size: f64,
}

/// Incremental state for each product; updated on each new snapshot.
#[derive(Debug)]
struct ProductMetricsState {
    sum_buy: f64,
    sum_sell: f64,
    count: usize,
    order_frequency_sum: f64,
    order_frequency_count: usize,
    total_new_orders: f64,
    total_new_order_amount: f64,
    sell_changes_count: usize,
    sell_size_total: f64,
    windows: usize,
    prev_snapshot: Option<BazaarInfo>,
}

impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
        Self {
            sum_buy: first.buy_price,
            sum_sell: first.sell_price,
            count: 1,
            order_frequency_sum: 0.0,
            order_frequency_count: 0,
            total_new_orders: 0.0,
            total_new_order_amount: 0.0,
            sell_changes_count: 0,
            sell_size_total: 0.0,
            windows: 0,
            prev_snapshot: Some(first.clone()),
        }
    }

    fn update(&mut self, current: &BazaarInfo) {
        self.count += 1;
        self.sum_buy += current.buy_price;
        self.sum_sell += current.sell_price;

        if let Some(prev) = &self.prev_snapshot {
            if prev.sell_orders.len() > 1 && !current.sell_orders.is_empty() {
                let anchor_order = &prev.sell_orders[1];
                let mut anchored_index = None;
                for (i, order) in current.sell_orders.iter().enumerate() {
                    if (order.price_per_unit - anchor_order.price_per_unit).abs() < 1e-6 {
                        anchored_index = Some(i);
                        break;
                    }
                }
                if let Some(idx) = anchored_index {
                    let new_orders = if idx > 1 { idx - 1 } else { 0 };
                    self.order_frequency_sum += new_orders as f64;
                    self.order_frequency_count += 1;
                    if new_orders > 0 {
                        let new_order_amount: i64 = current
                            .sell_orders
                            .iter()
                            .take(new_orders)
                            .map(|order| order.amount)
                            .sum();
                        self.total_new_order_amount += new_order_amount as f64;
                        self.total_new_orders += new_orders as f64;
                    }
                }
            }
            let diff = current.buy_moving_week - prev.buy_moving_week;
            self.windows += 1;
            if diff != 0 {
                self.sell_changes_count += 1;
                self.sell_size_total += diff.abs() as f64;
            }
        }
        self.prev_snapshot = Some(current.clone());
    }

    fn finalize(&self, product_id: String) -> AnalysisResult {
        let buy_price_average = self.sum_buy / self.count as f64;
        let sell_price_average = self.sum_sell / self.count as f64;
        let order_frequency_average = if self.order_frequency_count > 0 {
            self.order_frequency_sum / self.order_frequency_count as f64
        } else {
            0.0
        };
        let order_size_average = if self.total_new_orders > 0.0 {
            self.total_new_order_amount / self.total_new_orders
        } else {
            0.0
        };
        let sell_frequency = if self.windows > 0 {
            self.sell_changes_count as f64 / self.windows as f64
        } else {
            0.0
        };
        let sell_size = if self.sell_changes_count > 0 {
            self.sell_size_total / self.sell_changes_count as f64
        } else {
            0.0
        };

        AnalysisResult {
            product_id,
            buy_price_average,
            sell_price_average,
            order_frequency_average,
            order_size_average,
            sell_frequency,
            sell_size,
        }
    }
}

async fn fetch_snapshot(last_modified: &mut Option<String>) -> Result<Option<Vec<BazaarInfo>>, Box<dyn Error>> {
    let url = "https://api.hypixel.net/v2/skyblock/bazaar";
    let response = reqwest::get(url).await?.error_for_status()?;

    let new_last_modified = response
        .headers()
        .get("last-modified")
        .map(|h| h.to_str().unwrap_or("").to_string());

    if let Some(new_mod) = &new_last_modified {
        if let Some(prev_mod) = last_modified {
            if prev_mod == new_mod {
                println!("Last-Modified unchanged ({}). Disposing snapshot.", new_mod);
                return Ok(None);
            }
        }
    }

    *last_modified = new_last_modified;

    let json: Value = response.json().await?;
    let products = json["products"].as_object().ok_or("Products field missing or not an object")?;
    let mut tasks = Vec::new();
    for (product_id, product) in products {
        let product = product.clone();
        let product_id = product_id.clone();
        let task = tokio::spawn(async move {
            let sell_price = product["sell_summary"][0]["pricePerUnit"].as_f64().unwrap_or_default();
            let buy_price = product["buy_summary"][0]["pricePerUnit"].as_f64().unwrap_or_default();
            let buy_moving_week = product["quick_status"]["buyMovingWeek"].as_i64().unwrap_or_default();
            let sell_volume = product["quick_status"]["sellVolume"].as_i64().unwrap_or_default();

            let mut sell_orders_vec = Vec::new();
            if let Some(sell_orders_json) = product["sell_summary"].as_array() {
                for order in sell_orders_json {
                    let amount = order["amount"].as_i64().unwrap_or_default();
                    let price_per_unit = order["pricePerUnit"].as_f64().unwrap_or_default();
                    let orders = order["orders"].as_i64().unwrap_or_default();
                    sell_orders_vec.push(SellOrder {
                        amount,
                        price_per_unit,
                        orders,
                    });
                }
            }

            BazaarInfo {
                product_id,
                sell_price,
                buy_price,
                buy_moving_week,
                sell_volume,
                sell_orders: sell_orders_vec,
            }
        });
        tasks.push(task);
    }

    let mut snapshot = Vec::new();
    for task in tasks {
        if let Ok(info) = task.await {
            snapshot.push(info);
        }
    }
    println!("Fetched snapshot with {} products", snapshot.len());
    Ok(Some(snapshot))
}

// Simple health check handler
async fn health_check_handler() -> &'static str {
    "OK"
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    // Ensure the "metrics" directory exists.
    fs::create_dir_all("metrics")?;
    
    // --- Start Health Check Server ---
    // Listen on 0.0.0.0:9000 for health checks.
    // The port should match what's configured in Koyeb service settings.
    let health_check_addr = SocketAddr::from(([0, 0, 0, 0], 9000)); 
    println!("Health check server will listen on {}", health_check_addr);
    let health_router = Router::new().route("/healthz", get(health_check_handler));
    
    // Spawn the health check server as a background task
    // The `_` before server_handle indicates we don't need to await it directly in main logic
    let _server_handle = tokio::spawn(async move {
        match axum::serve(tokio::net::TcpListener::bind(health_check_addr).await.unwrap(), health_router).await {
            Ok(_) => println!("Health check server shut down gracefully."),
            Err(e) => eprintln!("Health check server error: {}", e),
        }
    });
    println!("Health check server task spawned.");
    // --- End Health Check Server ---

    let remote_dir = "/remote_metrics"; 
    let mut product_states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut last_modified: Option<String> = None;
    let mut export_timer = Instant::now();

    loop {
        let snapshot_opt = match fetch_snapshot(&mut last_modified).await {
            Ok(s) => s,
            Err(e) => {
                eprintln!("Error fetching snapshot: {}", e);
                sleep(Duration::from_secs(5)).await;
                continue;
            }
        };

        if let Some(snapshot) = snapshot_opt {
            for info in snapshot {
                let pid = info.product_id.clone();
                if let Some(state) = product_states.get_mut(&pid) {
                    state.update(&info);
                } else {
                    product_states.insert(pid, ProductMetricsState::new(&info));
                }
            }
            println!("Updated product states with new snapshot.");
        } else {
            println!("No new snapshot processed this round.");
        }
        
        if export_timer.elapsed() >= Duration::from_secs(3600) { // 1 hour
            if !product_states.is_empty() {
                let mut results = Vec::new();
                for (pid, state) in &product_states {
                    results.push(state.finalize(pid.clone()));
                }
                let timestamp = Utc::now().format("%Y%m%d%H%M%S").to_string();
                let metrics_path = format!("metrics/metrics_{}.json", timestamp);
                let output_json = serde_json::to_string_pretty(&results)?;
                fs::write(&metrics_path, output_json)?;
                println!("Exported metrics to {}", metrics_path);
                
                let export_result = Command::new("./export_engine")
                    .arg(&metrics_path)
                    .arg(remote_dir)
                    .output();
                
                match export_result {
                    Ok(output) => {
                        println!("Export engine output:\n{}", String::from_utf8_lossy(&output.stdout));
                        if !output.stderr.is_empty() {
                            eprintln!("Export engine errors:\n{}", String::from_utf8_lossy(&output.stderr));
                        }
                    }
                    Err(e) => {
                        eprintln!("Failed to run export engine: {}", e);
                    }
                }
            } else {
                println!("No snapshots processed in the last hour; nothing to export.");
            }
            product_states.clear();
            export_timer = Instant::now();
        }
        sleep(Duration::from_secs(5)).await;
    }
    // Note: The main loop is infinite, so the health check server will run as long as the main app.
    // If the main loop could exit, you might want to join/abort the server_handle.
}