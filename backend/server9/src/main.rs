use axum::{routing::get, Router};
use chrono::Utc;
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
use std::net::SocketAddr;
use std::process::Command;
use std::time::{Duration, Instant};
use tokio::time::sleep;

#[derive(Debug, Clone, Deserialize, Serialize)]
struct SellOrder {
    amount: i64,
    price_per_unit: f64,
    orders: i64,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
struct BazaarInfo {
    product_id: String,
    sell_price: f64,
    buy_price: f64,
    buy_moving_week: i64,
    sell_volume: i64,
    sell_orders: Vec<SellOrder>,
}

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
        let buy_price_average = if self.count > 0 { self.sum_buy / self.count as f64 } else { 0.0 };
        let sell_price_average = if self.count > 0 { self.sum_sell / self.count as f64 } else { 0.0 };
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
    let client = reqwest::Client::new(); // Create a client to reuse
    let response = client.get(url).send().await?.error_for_status()?;

    let new_last_modified = response
        .headers()
        .get("last-modified")
        .and_then(|h| h.to_str().ok())
        .map(String::from);

    if new_last_modified.is_some() && last_modified.is_some() && new_last_modified == *last_modified {
        if let Some(val) = &new_last_modified { // Only print if new_mod is Some
             println!("Last-Modified unchanged ({}). Disposing snapshot.", val);
        } else {
            println!("Last-Modified unchanged. Disposing snapshot.");
        }
        return Ok(None);
    }
    
    *last_modified = new_last_modified;

    let json: Value = response.json().await?;
    let products_obj = json["products"].as_object().ok_or("Products field missing or not an object")?;
    
    let mut snapshot_data = Vec::with_capacity(products_obj.len());

    for (product_id_str, product_val) in products_obj {
        let sell_price = product_val["sell_summary"].get(0)
            .and_then(|s| s["pricePerUnit"].as_f64())
            .unwrap_or_default();
        let buy_price = product_val["buy_summary"].get(0)
            .and_then(|b| b["pricePerUnit"].as_f64())
            .unwrap_or_default();
        let buy_moving_week = product_val["quick_status"]["buyMovingWeek"].as_i64().unwrap_or_default();
        let sell_volume = product_val["quick_status"]["sellVolume"].as_i64().unwrap_or_default();

        let mut sell_orders_vec = Vec::new();
        if let Some(orders_array) = product_val["sell_summary"].as_array() {
            for order_val in orders_array {
                sell_orders_vec.push(SellOrder {
                    amount: order_val["amount"].as_i64().unwrap_or_default(),
                    price_per_unit: order_val["pricePerUnit"].as_f64().unwrap_or_default(),
                    orders: order_val["orders"].as_i64().unwrap_or_default(),
                });
            }
        }
        snapshot_data.push(BazaarInfo {
            product_id: product_id_str.clone(),
            sell_price,
            buy_price,
            buy_moving_week,
            sell_volume,
            sell_orders: sell_orders_vec,
        });
    }
    println!("Fetched snapshot with {} products", snapshot_data.len());
    Ok(Some(snapshot_data))
}

async fn health_check_handler() -> &'static str {
    "OK"
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    fs::create_dir_all("metrics")?;
    
    let health_check_addr = SocketAddr::from(([0, 0, 0, 0], 9000)); 
    println!("Health check server will listen on {}", health_check_addr);
    let health_router = Router::new().route("/healthz", get(health_check_handler));
    let _server_handle = tokio::spawn(async move {
        match axum::serve(tokio::net::TcpListener::bind(health_check_addr).await.unwrap(), health_router).await {
            Ok(_) => println!("Health check server shut down gracefully."),
            Err(e) => eprintln!("Health check server error: {}", e),
        }
    });
    println!("Health check server task spawned.");

    let remote_dir = "/remote_metrics"; 
    let mut product_states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut last_modified: Option<String> = None;
    let mut export_timer = Instant::now();

    loop {
        let snapshot_opt = match fetch_snapshot(&mut last_modified).await {
            Ok(s) => s,
            Err(e) => {
                eprintln!("Error fetching snapshot: {}", e);
                sleep(Duration::from_secs(10)).await; // Increased sleep on error
                continue;
            }
        };

        if let Some(snapshot) = snapshot_opt {
            for info in snapshot {
                let pid = info.product_id.clone();
                product_states.entry(pid)
                    .and_modify(|state| state.update(&info))
                    .or_insert_with(|| ProductMetricsState::new(&info));
            }
            println!("Updated {} product states with new snapshot.", product_states.len());
        } else {
            // No need to print "No new snapshot" every 5s if last_modified was the reason
        }
        
        if export_timer.elapsed() >= Duration::from_secs(3600) { // 1 hour
            if !product_states.is_empty() {
                let results: Vec<AnalysisResult> = product_states.iter()
                    .map(|(pid, state)| state.finalize(pid.clone()))
                    .collect();
                
                let timestamp = Utc::now().format("%Y%m%d%H%M%S").to_string();
                let metrics_path = format!("metrics/metrics_{}.json", timestamp);
                
                match serde_json::to_string_pretty(&results) {
                    Ok(output_json) => {
                        if let Err(e) = fs::write(&metrics_path, output_json) {
                            eprintln!("Error writing metrics file {}: {}", metrics_path, e);
                        } else {
                            println!("Exported metrics to {}", metrics_path);
                            let export_result = Command::new("./export_engine")
                                .arg(&metrics_path)
                                .arg(remote_dir)
                                .output();
                            
                            match export_result {
                                Ok(output) => {
                                    if !output.stdout.is_empty() {
                                        println!("Export engine output:\n{}", String::from_utf8_lossy(&output.stdout));
                                    }
                                    if !output.stderr.is_empty() {
                                        eprintln!("Export engine errors:\n{}", String::from_utf8_lossy(&output.stderr));
                                    }
                                    // Optionally delete local file after successful upload attempt
                                    // if output.status.success() { fs::remove_file(&metrics_path).ok(); }
                                }
                                Err(e) => {
                                    eprintln!("Failed to run export engine: {}", e);
                                }
                            }
                        }
                    }
                    Err(e) => {
                        eprintln!("Error serializing metrics to JSON: {}", e);
                    }
                }
            } else {
                println!("No product data processed in the last hour; nothing to export.");
            }
            product_states.clear();
            export_timer = Instant::now();
        }
        sleep(Duration::from_secs(60)).await; // Fetch every 60 seconds
    }
}