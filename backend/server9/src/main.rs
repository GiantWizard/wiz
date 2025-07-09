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
    // Note: Per your clarification:
    // sell_price here is from prod["sell_summary"][0]["pricePerUnit"], which is the Instasell Price (player receives).
    // buy_price here is from prod["buy_summary"][0]["pricePerUnit"], which is the Instabuy Price (player pays).
    product_id: String,
    sell_price: f64,
    buy_price: f64,
    buy_moving_week: i64,  // Tracks items BOUGHT FROM Bazaar (Player Instabuys)
    sell_moving_week: i64, // Tracks items SOLD TO Bazaar (Player Instasells)
    sell_volume: i64, // From quick_status, total volume of items for sale
    buy_volume: i64, // From quick_status, total volume of items being bought
    sell_orders: Vec<Order>, // From prod["sell_summary"], these are BUY ORDERS (player demand)
    buy_orders: Vec<Order>,  // From prod["buy_summary"], these are SELL ORDERS (player supply)
}

/// Holds the final analysis metrics for one product.
#[derive(Debug, Serialize)]
struct AnalysisResult {
    product_id: String,

    // Prices:
    // Average price a player PAYS to Instabuy (sourced from buy_summary[0] in API)
    instabuy_price_average: f64,
    // Average price a player RECEIVES when Instaselling (sourced from sell_summary[0] in API)
    instasell_price_average: f64,

    // Order Book Dynamics: Demand Side (New Buy Offers appearing)
    // How often new, competitive Buy Offers (demand) appear (monitors sell_summary list in API)
    new_demand_offer_frequency_average: f64,
    // Average quantity of new, competitive Buy Offers (demand) (monitors sell_summary list in API)
    new_demand_offer_size_average: f64,

    // Actual Player Transactions: Instabuys (items BOUGHT FROM Bazaar, tracked by buy_moving_week)
    // How often players perform Instabuys
    player_instabuy_transaction_frequency: f64,
    // Average quantity per player Instabuy transaction
    player_instabuy_transaction_size_average: f64,

    // Order Book Dynamics: Supply Side (New Sell Offers appearing)
    // How often new, competitive Sell Offers (supply) appear (monitors buy_summary list in API)
    new_supply_offer_frequency_average: f64,
    // Average quantity of new, competitive Sell Offers (supply) (monitors buy_summary list in API)
    new_supply_offer_size_average: f64,

    // Actual Player Transactions: Instasells (items SOLD TO Bazaar, tracked by sell_moving_week)
    // How often players perform Instasells
    player_instasell_transaction_frequency: f64,
    // Average quantity per player Instasell transaction
    player_instasell_transaction_size_average: f64,
}

/// Incremental state for each product; updated on each new snapshot.
#[derive(Debug)]
struct ProductMetricsState {
    // These sum the values from BazaarInfo
    sum_instabuy_price: f64,
    sum_instasell_price: f64,
    count: usize, // Number of snapshots processed

    windows: usize, // Number of snapshot pairs processed (for calculating frequencies/sizes of changes)

    // New Demand Offer (Buy Offer) Metrics (from sell_orders in BazaarInfo)
    new_demand_offer_frequency_sum: f64,
    new_demand_offer_frequency_count: usize,
    total_new_demand_offers: f64,
    total_new_demand_offer_amount: f64,

    // Player Instabuy Transaction Metrics (from buy_moving_week in BazaarInfo)
    player_instabuy_transaction_changes_count: usize,
    player_instabuy_transaction_size_total: f64,

    // New Supply Offer (Sell Offer) Metrics (from buy_orders in BazaarInfo)
    new_supply_offer_frequency_sum: f64,
    new_supply_offer_frequency_count: usize,
    total_new_supply_offers: f64,
    total_new_supply_offer_amount: f64,

    // Player Instasell Transaction Metrics (from sell_moving_week in BazaarInfo)
    player_instasell_transaction_changes_count: usize,
    player_instasell_transaction_size_total: f64,
    
    // Previous state for comparison
    prev_snapshot: Option<BazaarInfo>,
}

impl ProductMetricsState {
    fn new(first: &BazaarInfo) -> Self {
        Self {
            sum_instabuy_price: first.buy_price,   // BazaarInfo.buy_price is Instabuy Price
            sum_instasell_price: first.sell_price, // BazaarInfo.sell_price is Instasell Price
            count: 1,
            windows: 0,

            new_demand_offer_frequency_sum: 0.0,
            new_demand_offer_frequency_count: 0,
            total_new_demand_offers: 0.0,
            total_new_demand_offer_amount: 0.0,
            player_instabuy_transaction_changes_count: 0,
            player_instabuy_transaction_size_total: 0.0,

            new_supply_offer_frequency_sum: 0.0,
            new_supply_offer_frequency_count: 0,
            total_new_supply_offers: 0.0,
            total_new_supply_offer_amount: 0.0,
            player_instasell_transaction_changes_count: 0,
            player_instasell_transaction_size_total: 0.0,
            
            prev_snapshot: Some(first.clone()),
        }
    }

    fn update(&mut self, current: &BazaarInfo) {
        self.count += 1;
        self.sum_instabuy_price += current.buy_price;   // Accumulate Instabuy Price
        self.sum_instasell_price += current.sell_price; // Accumulate Instasell Price

        if let Some(prev) = &self.prev_snapshot {
            self.windows += 1;

            // --- Demand Side Order Book Dynamics (New Buy Offers / from current.sell_orders) ---
            // Measures how many new competitive Buy Offers (demand) appear
            if prev.sell_orders.len() > 1 && !current.sell_orders.is_empty() {
                // prev.sell_orders contains BUY ORDERS (demand)
                // current.sell_orders contains BUY ORDERS (demand)
                let anchor = &prev.sell_orders[1]; // Use 2nd best Buy Offer as stable point
                if let Some(idx) = current.sell_orders.iter().position(|o| (o.price_per_unit - anchor.price_per_unit).abs() < 1e-6) {
                    let new_offers = if idx > 0 { idx } else { 0 }; // All orders before anchor (better price)
                    self.new_demand_offer_frequency_sum += new_offers as f64;
                    self.new_demand_offer_frequency_count += 1;
                    if new_offers > 0 {
                        let amount: i64 = current.sell_orders.iter().take(new_offers).map(|o| o.amount).sum();
                        self.total_new_demand_offer_amount += amount as f64;
                        self.total_new_demand_offers += new_offers as f64; // Corrected: was new_orders
                    }
                }
            }

            // --- Player Instabuy Transaction Metrics (from buy_moving_week) ---
            // Measures how often and how much items are BOUGHT FROM the Bazaar (Player Instabuys)
            let instabuy_diff = current.buy_moving_week - prev.buy_moving_week;
            if instabuy_diff != 0 {
                self.player_instabuy_transaction_changes_count += 1;
                self.player_instabuy_transaction_size_total += instabuy_diff.abs() as f64;
            }

            // --- Supply Side Order Book Dynamics (New Sell Offers / from current.buy_orders) ---
            // Measures how many new competitive Sell Offers (supply) appear
            if prev.buy_orders.len() > 1 && !current.buy_orders.is_empty() {
                // prev.buy_orders contains SELL ORDERS (supply)
                // current.buy_orders contains SELL ORDERS (supply)
                let anchor = &prev.buy_orders[1]; // Use 2nd best Sell Offer as stable point
                if let Some(idx) = current.buy_orders.iter().position(|o| (o.price_per_unit - anchor.price_per_unit).abs() < 1e-6) {
                    let new_offers = if idx > 0 { idx } else { 0 };
                    self.new_supply_offer_frequency_sum += new_offers as f64;
                    self.new_supply_offer_frequency_count += 1;
                    if new_offers > 0 {
                        let amount: i64 = current.buy_orders.iter().take(new_offers).map(|o| o.amount).sum();
                        self.total_new_supply_offer_amount += amount as f64;
                        self.total_new_supply_offers += new_offers as f64; // Corrected: was new_orders
                    }
                }
            }

            // --- Player Instasell Transaction Metrics (from sell_moving_week) ---
            // Measures how often and how much items are SOLD TO the Bazaar (Player Instasells)
            let instasell_diff = current.sell_moving_week - prev.sell_moving_week;
            if instasell_diff != 0 {
                self.player_instasell_transaction_changes_count += 1;
                self.player_instasell_transaction_size_total += instasell_diff.abs() as f64;
            }
        }

        self.prev_snapshot = Some(current.clone());
    }

    fn finalize(&self, product_id: String) -> AnalysisResult {
        // Calculate average prices
        let instabuy_price_average = self.sum_instabuy_price / self.count as f64;
        let instasell_price_average = self.sum_instasell_price / self.count as f64;

        // Finalize New Demand Offer metrics
        let new_demand_offer_frequency_average = if self.new_demand_offer_frequency_count > 0 { 
            self.new_demand_offer_frequency_sum / self.new_demand_offer_frequency_count as f64 
        } else { 0.0 };
        let new_demand_offer_size_average = if self.total_new_demand_offers > 0.0 { 
            self.total_new_demand_offer_amount / self.total_new_demand_offers 
        } else { 0.0 };

        // Finalize Player Instabuy Transaction metrics
        let player_instabuy_transaction_frequency = if self.windows > 0 { 
            self.player_instabuy_transaction_changes_count as f64 / self.windows as f64 
        } else { 0.0 };
        let player_instabuy_transaction_size_average = if self.player_instabuy_transaction_changes_count > 0 { 
            self.player_instabuy_transaction_size_total / self.player_instabuy_transaction_changes_count as f64 
        } else { 0.0 };

        // Finalize New Supply Offer metrics
        let new_supply_offer_frequency_average = if self.new_supply_offer_frequency_count > 0 { 
            self.new_supply_offer_frequency_sum / self.new_supply_offer_frequency_count as f64 
        } else { 0.0 };
        let new_supply_offer_size_average = if self.total_new_supply_offers > 0.0 { 
            self.total_new_supply_offer_amount / self.total_new_supply_offers 
        } else { 0.0 };

        // Finalize Player Instasell Transaction metrics
        let player_instasell_transaction_frequency = if self.windows > 0 { 
            self.player_instasell_transaction_changes_count as f64 / self.windows as f64 
        } else { 0.0 };
        let player_instasell_transaction_size_average = if self.player_instasell_transaction_changes_count > 0 { 
            self.player_instasell_transaction_size_total / self.player_instasell_transaction_changes_count as f64 
        } else { 0.0 };

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
            // Per your clarification:
            // prod["sell_summary"][0]["pricePerUnit"] is the price a player RECEIVES (Instasell Price)
            // prod["buy_summary"][0]["pricePerUnit"] is the price a player PAYS (Instabuy Price)
            let instasell_price = prod["sell_summary"][0]["pricePerUnit"].as_f64().unwrap_or_default();
            let instabuy_price = prod["buy_summary"][0]["pricePerUnit"].as_f64().unwrap_or_default();
            
            let quick_status = &prod["quick_status"];
            // buyMovingWeek: items BOUGHT FROM Bazaar (Player Instabuys)
            let buy_moving_week = quick_status["buyMovingWeek"].as_i64().unwrap_or_default();
            // sellMovingWeek: items SOLD TO Bazaar (Player Instasells)
            let sell_moving_week = quick_status["sellMovingWeek"].as_i64().unwrap_or_default();
            let sell_volume = quick_status["sellVolume"].as_i64().unwrap_or_default();
            let buy_volume = quick_status["buyVolume"].as_i64().unwrap_or_default();

            // sell_summary: This list contains BUY ORDERS (player demand)
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

            // buy_summary: This list contains SELL ORDERS (player supply)
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
                sell_price: instasell_price, // This is what the API calls 'sell_price', but is Instasell Price
                buy_price: instabuy_price,   // This is what the API calls 'buy_price', but is Instabuy Price
                buy_moving_week,
                sell_moving_week,
                sell_volume,
                buy_volume,
                sell_orders: sell_orders_vec, // These are BUY OFFERS (DEMAND)
                buy_orders: buy_orders_vec,   // These are SELL OFFERS (SUPPLY)
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
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(20); // Default 20 seconds

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