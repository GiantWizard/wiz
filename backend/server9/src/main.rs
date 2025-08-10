use chrono::{Utc, Local};
use reqwest;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::error::Error;
use std::fs;
use std::process::Command;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
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
struct FuzzyPattern {
    pattern_type: String,
    size: f64,
    frequency_minutes: f64,
    confidence: f64,
    occurrences: usize,
    method_confidence: f64,
}

#[derive(Debug, Clone)]
struct ModalPattern {
    size: f64,
    ratio: f64,
    frequency_minutes: f64,
    occurrence_count: usize,
    confidence: f64,
    detection_method: String,
}

#[derive(Debug, Serialize)]
struct DeltaSequences {
    buy_moving_week: Vec<i64>,
    sell_moving_week: Vec<i64>,
    buy_orders: Vec<i64>,
    sell_orders: Vec<i64>,
    buy_amount: Vec<i64>,
    sell_amount: Vec<i64>,
    timestamps: Vec<u64>,
}

#[derive(Debug, Serialize)]
struct PatternDetails {
    detection_method: String,
    fuzzy_confidence: f64,
    legacy_confidence: Option<f64>,
    sequence_patterns_found: usize,
    velocity_patterns_found: usize,
    rhythm_patterns_found: usize,
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
    instabuy_modal_size: f64,
    instabuy_pattern_frequency: f64,
    instabuy_scale_factor: f64,
    instabuy_estimated_true_volume: f64,
    instasell_modal_size: f64,
    instasell_pattern_frequency: f64,
    instasell_scale_factor: f64,
    instasell_estimated_true_volume: f64,
    pattern_detection_confidence: f64,
    delta_sequences: DeltaSequences,
    pattern_details: PatternDetails,
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
    // Delta sequences for fuzzy pattern detection
    buy_moving_week_deltas: Vec<i64>,
    sell_moving_week_deltas: Vec<i64>,
    buy_orders_deltas: Vec<i64>,
    sell_orders_deltas: Vec<i64>,
    buy_amount_deltas: Vec<i64>,
    sell_amount_deltas: Vec<i64>,
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
            buy_moving_week_deltas: Vec::new(),
            sell_moving_week_deltas: Vec::new(),
            buy_orders_deltas: Vec::new(),
            sell_orders_deltas: Vec::new(),
            buy_amount_deltas: Vec::new(),
            sell_amount_deltas: Vec::new(),
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

            // Calculate and store deltas for fuzzy pattern detection
            let buy_mw_delta = current.buy_moving_week - self.prev_buy_moving_week;
            let sell_mw_delta = current.sell_moving_week - self.prev_sell_moving_week;
            
            self.buy_moving_week_deltas.push(buy_mw_delta);
            self.sell_moving_week_deltas.push(sell_mw_delta);

            // Calculate order book summary deltas
            let prev_buy_orders_total: i64 = prev.buy_orders.iter().map(|o| o.orders).sum();
            let current_buy_orders_total: i64 = current.buy_orders.iter().map(|o| o.orders).sum();
            let prev_sell_orders_total: i64 = prev.sell_orders.iter().map(|o| o.orders).sum();
            let current_sell_orders_total: i64 = current.sell_orders.iter().map(|o| o.orders).sum();
            
            let prev_buy_amount_total: i64 = prev.buy_orders.iter().map(|o| o.amount).sum();
            let current_buy_amount_total: i64 = current.buy_orders.iter().map(|o| o.amount).sum();
            let prev_sell_amount_total: i64 = prev.sell_orders.iter().map(|o| o.amount).sum();
            let current_sell_amount_total: i64 = current.sell_orders.iter().map(|o| o.amount).sum();

            self.buy_orders_deltas.push(current_buy_orders_total - prev_buy_orders_total);
            self.sell_orders_deltas.push(current_sell_orders_total - prev_sell_orders_total);
            self.buy_amount_deltas.push(current_buy_amount_total - prev_buy_amount_total);
            self.sell_amount_deltas.push(current_sell_amount_total - prev_sell_amount_total);

            // INSTABUY analysis
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
            
            if inferred_instabuy_events > 0 {
                self.player_instabuy_event_count += inferred_instabuy_events;
                self.player_instabuy_volume_total += inferred_instabuy_volume as f64;
            }

            // INSTASELL analysis
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
            
            if inferred_instasell_events > 0 {
                self.player_instasell_event_count += inferred_instasell_events;
                self.player_instasell_volume_total += inferred_instasell_volume as f64;
            }

            // New offer tracking
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

    // CORRECTED: More efficient fuzzy pattern detection

    fn detect_velocity_patterns(deltas: &[i64], timestamps: &[u64]) -> Vec<FuzzyPattern> {
        let mut patterns = Vec::new();
        let mut activity_periods = Vec::new();

        // Extract activity periods with proper bounds checking
        for (i, &delta) in deltas.iter().enumerate() {
            if delta > 0 && i + 1 < timestamps.len() && i > 0 {
                let time_diff = (timestamps[i + 1] - timestamps[i]) as f64 / 60.0;
                if time_diff > 0.0 {
                    let velocity = delta as f64 / time_diff;
                    activity_periods.push((i, velocity, delta, time_diff));
                }
            }
        }

        if activity_periods.len() < 3 {
            return patterns;
        }

        // Simple velocity clustering
        activity_periods.sort_by(|a, b| a.1.partial_cmp(&b.1).unwrap_or(std::cmp::Ordering::Equal));
        
        let mut clusters = Vec::new();
        let mut current_cluster = vec![activity_periods[0]];
        
        for i in 1..activity_periods.len() {
            let prev_velocity = current_cluster.last().unwrap().1;
            let curr_velocity = activity_periods[i].1;
            
            if (curr_velocity - prev_velocity).abs() / prev_velocity.max(0.1) <= 0.4 {
                current_cluster.push(activity_periods[i]);
            } else {
                if current_cluster.len() >= 3 {
                    clusters.push(current_cluster);
                }
                current_cluster = vec![activity_periods[i]];
            }
        }
        if current_cluster.len() >= 3 {
            clusters.push(current_cluster);
        }

        // Analyze clusters for regularity
        for cluster in clusters {
            let intervals: Vec<f64> = cluster.windows(2)
                .map(|w| (timestamps[w[1].0 + 1] - timestamps[w[0].0 + 1]) as f64 / 60.0)
                .collect();
            
            if intervals.len() > 0 {
                let avg_interval = intervals.iter().sum::<f64>() / intervals.len() as f64;
                let variance = intervals.iter()
                    .map(|&x| (x - avg_interval).powi(2))
                    .sum::<f64>() / intervals.len() as f64;
                let cv = (variance.sqrt() / avg_interval.max(1.0)).min(1.0);

                if cv < 0.6 {
                    let avg_size = cluster.iter().map(|&(_, _, delta, _)| delta as f64).sum::<f64>() / cluster.len() as f64;
                    let confidence = cluster.len() as f64 / activity_periods.len() as f64;

                    patterns.push(FuzzyPattern {
                        pattern_type: "velocity_pattern".to_string(),
                        size: avg_size,
                        frequency_minutes: avg_interval,
                        confidence: confidence.min(1.0),
                        occurrences: cluster.len(),
                        method_confidence: confidence * (1.0 - cv),
                    });
                }
            }
        }

        patterns.sort_by(|a, b| b.method_confidence.partial_cmp(&a.method_confidence).unwrap_or(std::cmp::Ordering::Equal));
        patterns.into_iter().take(2).collect()
    }

    fn detect_rhythm_patterns(deltas: &[i64], timestamps: &[u64]) -> Vec<FuzzyPattern> {
        let mut patterns = Vec::new();
        
        let activity_indices: Vec<usize> = deltas.iter().enumerate()
            .filter(|(_, &delta)| delta > 0)
            .map(|(i, _)| i)
            .collect();

        if activity_indices.len() < 3 {
            return patterns;
        }

        let intervals: Vec<f64> = activity_indices.windows(2)
            .filter_map(|w| {
                if w[1] + 1 < timestamps.len() && w[0] + 1 < timestamps.len() {
                    Some((timestamps[w[1] + 1] - timestamps[w[0] + 1]) as f64 / 60.0)
                } else {
                    None
                }
            })
            .collect();

        if intervals.is_empty() {
            return patterns;
        }

        // Find modal intervals with tolerance
        for tolerance in [0.25, 0.5] {
            let mut used = vec![false; intervals.len()];
            
            for (i, &interval) in intervals.iter().enumerate() {
                if used[i] {
                    continue;
                }

                let mut cluster = vec![interval];
                used[i] = true;

                for (j, &other_interval) in intervals.iter().enumerate() {
                    if i != j && !used[j] {
                        let relative_diff = (interval - other_interval).abs() / interval.max(0.1);
                        if relative_diff <= tolerance {
                            cluster.push(other_interval);
                            used[j] = true;
                        }
                    }
                }

                if cluster.len() >= 3 {
                    let avg_interval = cluster.iter().sum::<f64>() / cluster.len() as f64;
                    let avg_size = activity_indices.iter()
                        .map(|&i| deltas[i] as f64)
                        .sum::<f64>() / activity_indices.len() as f64;
                    let confidence = cluster.len() as f64 / intervals.len() as f64;

                    patterns.push(FuzzyPattern {
                        pattern_type: format!("rhythm_{}pct", (tolerance * 100.0) as u32),
                        size: avg_size,
                        frequency_minutes: avg_interval,
                        confidence: confidence.min(1.0),
                        occurrences: cluster.len(),
                        method_confidence: confidence * (1.0 - tolerance * 0.5),
                    });
                }
            }
        }

        patterns.sort_by(|a, b| b.method_confidence.partial_cmp(&a.method_confidence).unwrap_or(std::cmp::Ordering::Equal));
        patterns.into_iter().take(1).collect()
    }

    fn detect_fuzzy_modal_pattern(
        moving_week_deltas: &[i64],
        inferred_volume_history: &[i64],
        timestamps: &[u64],
    ) -> (Option<ModalPattern>, PatternDetails) {
        
        let vel_patterns = Self::detect_velocity_patterns(moving_week_deltas, timestamps);
        let rhythm_patterns = Self::detect_rhythm_patterns(moving_week_deltas, timestamps);

        let pattern_details = PatternDetails {
            detection_method: "fuzzy_combined".to_string(),
            fuzzy_confidence: 0.0,
            legacy_confidence: None,
            sequence_patterns_found: 0, // Simplified - removed expensive sequence similarity
            velocity_patterns_found: vel_patterns.len(),
            rhythm_patterns_found: rhythm_patterns.len(),
        };

        // Combine fuzzy patterns
        let mut all_patterns = vel_patterns;
        all_patterns.extend(rhythm_patterns);

        if let Some(best_pattern) = all_patterns.first() {
            // Calculate ratio from actual pattern periods
            let pattern_periods = Self::find_patterns_from_deltas(moving_week_deltas, inferred_volume_history, timestamps);
            let ratio = if !pattern_periods.is_empty() {
                let total_mw: i64 = pattern_periods.iter().map(|p| p.moving_week_delta).sum();
                let total_inf: i64 = pattern_periods.iter().map(|p| p.inferred_volume).sum();
                if total_inf > 0 { total_mw as f64 / total_inf as f64 } else { 1.0 }
            } else {
                1.0
            };

            let fuzzy_pattern = ModalPattern {
                size: best_pattern.size,
                ratio,
                frequency_minutes: best_pattern.frequency_minutes,
                occurrence_count: best_pattern.occurrences,
                confidence: best_pattern.confidence,
                detection_method: best_pattern.pattern_type.clone(),
            };

            let mut updated_details = pattern_details;
            updated_details.fuzzy_confidence = best_pattern.confidence;
            return (Some(fuzzy_pattern), updated_details);
        }

        // Fallback to legacy
        let pattern_periods = Self::find_patterns_from_deltas(moving_week_deltas, inferred_volume_history, timestamps);
        if let Some(legacy_pattern) = Self::detect_modal_pattern_legacy(&pattern_periods) {
            let mut legacy_details = pattern_details;
            legacy_details.detection_method = "legacy_clustering".to_string();
            legacy_details.legacy_confidence = Some(legacy_pattern.confidence);
            return (Some(legacy_pattern), legacy_details);
        }

        (None, pattern_details)
    }

    fn find_patterns_from_deltas(
        moving_week_deltas: &[i64],
        inferred_volume_history: &[i64],
        timestamps: &[u64],
    ) -> Vec<PatternPeriod> {
        let mut patterns = Vec::new();
        let max_len = moving_week_deltas.len().min(inferred_volume_history.len()).min(timestamps.len().saturating_sub(1));
        
        for i in 0..max_len {
            let delta = moving_week_deltas[i];
            let inferred = inferred_volume_history[i];
            if delta > 0 && inferred > 0 {
                patterns.push(PatternPeriod {
                    position: i,
                    moving_week_delta: delta,
                    inferred_volume: inferred,
                    timestamp: timestamps[i + 1],
                });
            }
        }
        patterns
    }

    fn detect_modal_pattern_legacy(pattern_periods: &[PatternPeriod]) -> Option<ModalPattern> {
        if pattern_periods.len() < 3 {
            return None;
        }
        
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
        
        let mut modal: Option<(Vec<PatternPeriod>, i64, i64)> = None;
        for ((delta, ratio), cluster) in &cluster_map {
            if cluster.len() >= 3
                && (modal.is_none() || cluster.len() > modal.as_ref().unwrap().0.len())
            {
                modal = Some((cluster.clone(), *delta, *ratio));
            }
        }
        
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
            for (_ratio, cluster) in &ratio_map {
                if cluster.len() < 3 {
                    continue;
                }
                let avg_delta = cluster.iter().map(|p| p.moving_week_delta).sum::<i64>() / cluster.len() as i64;
                if cluster.iter().all(|p| (p.moving_week_delta - avg_delta).abs() <= (avg_delta as f64 * 0.1).max(1.0) as i64) {
                    if modal.is_none() || cluster.len() > modal.as_ref().unwrap().0.len() {
                        modal = Some((cluster.clone(), avg_delta, *_ratio));
                    }
                }
            }
        }
        
        let (pattern_set, modal_size, modal_ratio) = modal?;
        
        let timestamps: Vec<u64> = pattern_set.iter().map(|p| p.timestamp).collect();
        if timestamps.len() < 2 {
            return None;
        }
        
        let intervals: Vec<f64> = timestamps.windows(2)
            .map(|w| w[1].saturating_sub(w[0]) as f64 / 60.0)
            .collect();
        
        let frequency_minutes = if !intervals.is_empty() {
            intervals.iter().sum::<f64>() / intervals.len() as f64
        } else {
            60.0
        };
        
        let confidence = pattern_set.len() as f64 / pattern_periods.len() as f64;
        
        Some(ModalPattern {
            size: modal_size as f64,
            ratio: modal_ratio as f64 / 10000.0,
            frequency_minutes,
            occurrence_count: pattern_set.len(),
            confidence,
            detection_method: "legacy_exact_clustering".to_string(),
        })
    }

    fn finalize_with_sequences(&self, product_id: String) -> AnalysisResult {
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

        let (instabuy_modal_pattern, instabuy_pattern_details) = Self::detect_fuzzy_modal_pattern(
            &self.buy_moving_week_deltas, 
            &self.inferred_buy_volume_history, 
            &self.timestamps
        );
        let (instasell_modal_pattern, instasell_pattern_details) = Self::detect_fuzzy_modal_pattern(
            &self.sell_moving_week_deltas, 
            &self.inferred_sell_volume_history, 
            &self.timestamps
        );

        let (instabuy_modal_size, instabuy_pattern_frequency, instabuy_scale_factor, instabuy_estimated_true_volume) = 
            if let Some(pattern) = &instabuy_modal_pattern {
                let volume_coverage = if self.total_buy_moving_week_activity > 0 {
                    self.player_instabuy_volume_total / self.total_buy_moving_week_activity as f64
                } else {
                    1.0
                };
                let scale_factor = if volume_coverage < 0.7 {
                    (1.0 / volume_coverage).min(2.0).max(1.0)
                } else {
                    1.0
                };
                (pattern.size, pattern.frequency_minutes, scale_factor, self.total_buy_moving_week_activity as f64)
            } else {
                (0.0, 0.0, 1.0, self.total_buy_moving_week_activity as f64)
            };

        let (instasell_modal_size, instasell_pattern_frequency, instasell_scale_factor, instasell_estimated_true_volume) = 
            if let Some(pattern) = &instasell_modal_pattern {
                let volume_coverage = if self.total_sell_moving_week_activity > 0 {
                    self.player_instasell_volume_total / self.total_sell_moving_week_activity as f64
                } else {
                    1.0
                };
                let scale_factor = if volume_coverage < 0.7 {
                    (1.0 / volume_coverage).min(2.0).max(1.0)
                } else {
                    1.0
                };
                (pattern.size, pattern.frequency_minutes, scale_factor, self.total_sell_moving_week_activity as f64)
            } else {
                (0.0, 0.0, 1.0, self.total_sell_moving_week_activity as f64)
            };

        let buy_confidence = instabuy_modal_pattern.as_ref().map(|p| p.confidence).unwrap_or(0.0);
        let sell_confidence = instasell_modal_pattern.as_ref().map(|p| p.confidence).unwrap_or(0.0);
        let pattern_detection_confidence = ((buy_confidence + sell_confidence) / 2.0) * 100.0;

        let combined_pattern_details = PatternDetails {
            detection_method: format!("buy:{}, sell:{}", 
                instabuy_pattern_details.detection_method,
                instasell_pattern_details.detection_method
            ),
            fuzzy_confidence: (instabuy_pattern_details.fuzzy_confidence + instasell_pattern_details.fuzzy_confidence) / 2.0,
            legacy_confidence: match (instabuy_pattern_details.legacy_confidence, instasell_pattern_details.legacy_confidence) {
                (Some(a), Some(b)) => Some((a + b) / 2.0),
                (Some(a), None) => Some(a),
                (None, Some(b)) => Some(b),
                (None, None) => None,
            },
            sequence_patterns_found: 0, // Removed expensive sequence detection
            velocity_patterns_found: instabuy_pattern_details.velocity_patterns_found + instasell_pattern_details.velocity_patterns_found,
            rhythm_patterns_found: instabuy_pattern_details.rhythm_patterns_found + instasell_pattern_details.rhythm_patterns_found,
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
            delta_sequences: DeltaSequences {
                buy_moving_week: self.buy_moving_week_deltas.clone(),
                sell_moving_week: self.sell_moving_week_deltas.clone(),
                buy_orders: self.buy_orders_deltas.clone(),
                sell_orders: self.sell_orders_deltas.clone(),
                buy_amount: self.buy_amount_deltas.clone(),
                sell_amount: self.sell_amount_deltas.clone(),
                timestamps: self.timestamps.clone(),
            },
            pattern_details: combined_pattern_details,
        }
    }
}

async fn fetch_snapshot(last_modified: &mut Option<String>) -> Result<Option<Vec<BazaarInfo>>, Box<dyn Error>> {
    let url = "https://api.hypixel.net/v2/skyblock/bazaar";
    let resp = reqwest::get(url).await?.error_for_status()?;
    let new_mod = resp.headers().get("last-modified").and_then(|h| h.to_str().ok()).map(String::from);
    if let (Some(prev), Some(curr)) = (last_modified.as_ref(), new_mod.as_ref()) {
        if prev == curr {
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
    Ok(Some(snapshot))
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    fs::create_dir_all("metrics")?;
    let mut states: HashMap<String, ProductMetricsState> = HashMap::new();
    let mut last_mod: Option<String> = None;

    let api_poll_interval_secs = std::env::var("API_POLL_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(20);

    const TARGET_WINDOWS: usize = 180;

    println!("Configuration: Target windows = {} (1 hour), polling every {} seconds.", 
        TARGET_WINDOWS, api_poll_interval_secs);
    println!("Fuzzy pattern detection: Velocity clustering, rhythm detection with legacy fallback.");

    loop {
        // FIXED: Proper datetime formatting
        println!("💓 heartbeat at Local: {}  UTC: {}", 
            Local::now().format("%H:%M:%S"), 
            Utc::now().format("%Y-%m-%d %H:%M:%S")
        );
        
        match fetch_snapshot(&mut last_mod).await {
            Ok(Some(snap)) => {
                for info in snap {
                    states.entry(info.product_id.clone())
                        .and_modify(|st| st.update(&info))
                        .or_insert_with(|| ProductMetricsState::new(&info));
                }
                let max_windows = states.values().map(|s| s.windows_processed).max().unwrap_or(0);
                println!("Updated {} products. Progress: {}/{} windows", states.len(), max_windows, TARGET_WINDOWS);
            }
            Ok(None) => {} // No new data
            Err(e) => eprintln!("Fetch error: {}", e),
        }

        let max_windows = states.values().map(|s| s.windows_processed).max().unwrap_or(0);
        
        if max_windows >= TARGET_WINDOWS {
            println!(">>> Hourly cycle complete: {} windows", max_windows);
            
            let results: Vec<_> = states.iter()
                .map(|(pid, state)| state.finalize_with_sequences(pid.clone()))
                .collect();
                
            let ts = Utc::now().format("%Y%m%d%H%M%S").to_string();
            let local_path = format!("metrics/metrics_{}.json", ts);
            let remote_mega_path = format!("/remote_metrics/metrics_{}.json", ts);
            
            let fuzzy_count = results.iter().filter(|r| r.pattern_details.detection_method.contains("velocity") || r.pattern_details.detection_method.contains("rhythm")).count();
            let legacy_count = results.iter().filter(|r| r.pattern_details.detection_method.contains("legacy")).count();
            
            println!("Exporting {} products: {} fuzzy, {} legacy patterns", 
                results.len(), fuzzy_count, legacy_count);
            
            match fs::write(&local_path, serde_json::to_string_pretty(&results)?) {
                Ok(_) => {
                    println!("Exported to {}", local_path);
                    
                    let export_engine_path = std::env::var("EXPORT_ENGINE_PATH")
                        .unwrap_or_else(|_| "export_engine".to_string());
                    let _ = Command::new(&export_engine_path)
                        .arg(&local_path)
                        .arg(&remote_mega_path)
                        .output();
                }
                Err(e) => eprintln!("Export error: {}", e),
            }
            
            states.clear();
        }

        sleep(Duration::from_secs(api_poll_interval_secs)).await;
    }
}