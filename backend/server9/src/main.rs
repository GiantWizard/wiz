use chrono::{Utc};
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
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
            // Order frequency & size
            if prev.sell_orders.len() > 1 && !current.sell_orders.is_empty() {
                let anchor = &prev.sell_orders[1];
                if let Some(idx) = current
                    .sell_orders
                    .iter()
                    .position(|o| (o.price_per_unit - anchor.price_per_unit).abs() < 1e-6)
                {
                    let new_orders = if idx > 1 { idx - 1 } else { 0 };
                    self.order_frequency_sum += new_orders as f64;
                    self.order_frequency_count += 1;
                    if new_orders > 0 {
                        let amount: i64 = current.sell_orders.iter()
                            .take(new_orders)
                            .map(|o| o.amount)
                            .sum();
                        self.total_new_order_amount += amount as f64;
                        self.total_new_orders += new_orders as f64;
                    }
                }
            }
            // Sell frequency & size
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
    let resp = reqwest::get(url).await?.error_for_status()?;

    let new_mod = resp
        .headers()
        .get("last-modified")
        .and_then(|h| h.to_str().ok())
        .map(String::from);

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
            let buy_moving_week = prod["quick_status"]["buyMovingWeek"].as_i64().unwrap_or_default();
            let sell_volume = prod["quick_status"]["sellVolume"].as_i64().unwrap_or_default();

            let mut sell_orders = Vec::new();
            if let Some(arr) = prod["sell_summary"].as_array() {
                for o in arr {
                    sell_orders.push(SellOrder {
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
                sell_volume,
                sell_orders,
            }
        }));
    }

    let mut out = Vec::new();
    for t in tasks {
        if let Ok(info) = t.await {
            out.push(info);
        }
    }
    println!("Fetched snapshot with {} products", out.len());
    Ok(Some(out))
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    fs::create_dir_all("metrics")?;
    let remote_dir = "/remote_metrics";
    let mut states = HashMap::new();
    let mut last_mod = None;
    let mut timer = Instant::now();

    loop {
        match fetch_snapshot(&mut last_mod).await {
            Ok(Some(snap)) => {
                for info in snap {
                    states
                        .entry(info.product_id.clone())
                        .and_modify(|st: &mut ProductMetricsState| st.update(&info))
                        .or_insert_with(|| ProductMetricsState::new(&info));
                }
                println!("Updated product states with new snapshot.");
            }
            Ok(None) => println!("No new snapshot processed this round."),
            Err(e) => {
                eprintln!("Error fetching snapshot: {}", e);
                sleep(Duration::from_secs(5)).await;
                continue;
            }
        }

        // every 5 minutes
        if timer.elapsed() >= Duration::from_secs(300) {
            println!(">>> Exporting metrics after {} seconds…", timer.elapsed().as_secs());
            if !states.is_empty() {
                let results: Vec<_> = states
                    .iter()
                    .map(|(pid, st)| st.finalize(pid.clone()))
                    .collect();

                let ts = Utc::now().format("%Y%m%d%H%M%S").to_string();
                let path = format!("metrics/metrics_{}.json", ts);
                fs::write(&path, serde_json::to_string_pretty(&results)?)?;
                println!("Exported metrics to {}", path);

                // run export_engine from PATH
                match Command::new("export_engine")
                    .arg(&path)
                    .arg(remote_dir)
                    .output()
                {
                    Ok(o) => {
                        println!("Export engine output:\n{}", String::from_utf8_lossy(&o.stdout));
                        if !o.stderr.is_empty() {
                            eprintln!("Export engine errors:\n{}", String::from_utf8_lossy(&o.stderr));
                        }
                    }
                    Err(e) => eprintln!("Failed to run export engine: {}", e),
                }
            } else {
                println!("No snapshots in the last interval; skipping export.");
            }
            states.clear();
            timer = Instant::now();
        }

        sleep(Duration::from_secs(5)).await;
    }
}
