use axum::{
    routing::get,
    Router,
    response::IntoResponse,
    http::StatusCode,
};
use chrono::Utc;
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
use std::net::SocketAddr;
use std::env;
use tokio::time::{sleep, Duration, Instant};
use tokio::process::Command;

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
    // For order frequency and order size (from sell orders changes)
    order_frequency_sum: f64,
    order_frequency_count: usize,
    total_new_orders: f64,
    total_new_order_amount: f64,
    // For sell frequency and sell size (from buy_moving_week changes)
    sell_changes_count: usize,
    sell_size_total: f64,
    windows: usize,
    // Store previous snapshot for pairwise comparisons.
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

    /// Update state with a new snapshot.
    fn update(&mut self, current: &BazaarInfo) {
        self.count += 1;
        self.sum_buy += current.buy_price;
        self.sum_sell += current.sell_price;

        if let Some(prev) = &self.prev_snapshot {
            // --- Order frequency and order size calculation ---
            // Use the second sell order of the previous snapshot as anchor.
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
            // --- Sell frequency and sell size calculation ---
            let diff = current.buy_moving_week - prev.buy_moving_week;
            self.windows += 1;
            if diff != 0 {
                self.sell_changes_count += 1;
                self.sell_size_total += diff.abs() as f64;
            }
        }

        self.prev_snapshot = Some(current.clone());
    }

    /// Finalize and compute the analysis metrics.
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

/// Fetch a snapshot from the Hypixel Bazaar API and return a vector of BazaarInfo for all products.
/// This function also checks the "last-modified" header. If it is unchanged, it disposes the snapshot.
async fn fetch_snapshot(last_modified: &mut Option<String>) -> Result<Option<Vec<BazaarInfo>>, Box<dyn Error>> {
    let url = "https://api.hypixel.net/v2/skyblock/bazaar";
    println!("Fetching snapshot from {}", url);
    let response = reqwest::get(url).await?;
    println!("Received response with status: {}", response.status());
    let response = response.error_for_status()?;

    // Extract the "last-modified" header.
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
    println!("Parsing {} products...", products.len());
    let mut tasks = Vec::new();
    for (product_id, product) in products {
        let product = product.clone();
        let product_id = product_id.clone();
        let task = tokio::spawn(async move {
            let sell_price = product["sell_summary"][0]["pricePerUnit"]
                .as_f64()
                .unwrap_or_default();
            let buy_price = product["buy_summary"][0]["pricePerUnit"]
                .as_f64()
                .unwrap_or_default();
            let buy_moving_week = product["quick_status"]["buyMovingWeek"]
                .as_i64()
                .unwrap_or_default();
            let sell_volume = product["quick_status"]["sellVolume"]
                .as_i64()
                .unwrap_or_default();

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

/// Export the processed metrics JSON file to Mega.nz using the Mega CLI.
/// It assumes that the Mega session is active and the remote folder is configured.
async fn export_to_mega(file_path: &str) -> Result<(), Box<dyn Error>> {
    let mega_remote_folder = env::var("MEGA_REMOTE_FOLDER")?;
    println!("Exporting {} to Mega.nz folder {}", file_path, mega_remote_folder);
    let put_status = Command::new("mega-put")
        .arg(file_path)
        .arg(&mega_remote_folder)
        .status()
        .await?;
    if !put_status.success() {
        return Err("mega-put command failed".into());
    }
    println!("Successfully exported {} to Mega.nz.", file_path);
    Ok(())
}

/// HTTP healthcheck endpoint.
async fn health_handler() -> impl IntoResponse {
    println!("Healthcheck endpoint called");
    StatusCode::OK
}

/// Main loop: every 5 seconds, fetch a snapshot and update state.
/// After 12 snapshots or 60 seconds, compute metrics, export them to a JSON file,
/// then call export_to_mega to upload that file to Mega.nz, and finally clear state.
#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    fs::create_dir_all("metrics")?;

    // Start a basic HTTP server for healthchecks.
    let port: u16 = env::var("PORT").unwrap_or_else(|_| "3000".to_string()).parse()?;
    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    println!("Starting HTTP server on {}", addr);
    let app = axum::Router::new().route("/health", get(health_handler));
    let server_task = tokio::spawn(async move {
        axum::Server::bind(&addr)
            .serve(app.into_make_service())
            .await
            .unwrap();
    });

    let mut product_states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut snapshot_count = 0;
    let mut last_modified: Option<String> = None;
    let mut export_timer = Instant::now();

    loop {
        match fetch_snapshot(&mut last_modified).await {
            Ok(Some(snapshot)) => {
                for info in snapshot {
                    let pid = info.product_id.clone();
                    if let Some(state) = product_states.get_mut(&pid) {
                        state.update(&info);
                    } else {
                        product_states.insert(pid, ProductMetricsState::new(&info));
                    }
                }
                snapshot_count += 1;
                println!("Processed {} snapshots (total state entries: {})", snapshot_count, product_states.len());
            },
            Ok(None) => {
                println!("No new snapshot processed this round.");
            },
            Err(e) => {
                eprintln!("Error fetching snapshot: {}", e);
                sleep(Duration::from_secs(5)).await;
                continue;
            }
        }

        sleep(Duration::from_secs(5)).await;

        // Export metrics either after 12 snapshots OR after 60 seconds, whichever comes first.
        if snapshot_count >= 12 || export_timer.elapsed() >= Duration::from_secs(60) {
            let mut results = Vec::new();
            for (pid, state) in &product_states {
                results.push(state.finalize(pid.clone()));
            }
            let timestamp = Utc::now().format("%Y%m%d%H%M%S").to_string();
            let metrics_path = format!("metrics/metrics_{}.json", timestamp);
            let output_json = serde_json::to_string_pretty(&results)?;
            fs::write(&metrics_path, output_json)?;
            println!("Exported metrics to {}", metrics_path);

            // Now export the file to Mega.nz.
            if let Err(e) = export_to_mega(&metrics_path).await {
                eprintln!("Error exporting metrics to Mega.nz: {}", e);
            }

            // Reset state and timer for the next period.
            product_states.clear();
            snapshot_count = 0;
            export_timer = Instant::now();
        }
    }

    // The server_task runs indefinitely.
    // Ok(()) unreachable here.
}
