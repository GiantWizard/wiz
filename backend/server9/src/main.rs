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
    buy_moving_week: Vec<i64>,     // 179 values
    sell_moving_week: Vec<i64>,    // 179 values
    buy_orders: Vec<i64>,          // 179 values
    sell_orders: Vec<i64>,         // 179 values
    buy_amount: Vec<i64>,          // 179 values
    sell_amount: Vec<i64>,         // 179 values
    timestamps: Vec<u64>,          // 180 values (snapshots)
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

            // Store ALL deltas - zeros are crucial for frequency analysis
            self.buy_orders_deltas.push(current_buy_orders_total - prev_buy_orders_total);
            self.sell_orders_deltas.push(current_sell_orders_total - prev_sell_orders_total);
            self.buy_amount_deltas.push(current_buy_amount_total - prev_buy_amount_total);
            self.sell_amount_deltas.push(current_sell_amount_total - prev_sell_amount_total);

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
            
            // Always trust detected volume when we detect events
            if inferred_instabuy_events > 0 {
                self.player_instabuy_event_count += inferred_instabuy_events;
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
            
            // Always trust detected volume when we detect events
            if inferred_instasell_events > 0 {
                self.player_instasell_event_count += inferred_instasell_events;
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

    // FUZZY PATTERN DETECTION IMPLEMENTATION

    // Method 1: Sequence Similarity Detection using Simple DTW
    fn detect_sequence_similarity_patterns(deltas: &[i64], timestamps: &[u64]) -> Vec<FuzzyPattern> {
        let mut patterns = Vec::new();
        let min_pattern_length = 3;
        let max_pattern_length = 15;
        let similarity_threshold = 0.6;

        for pattern_length in min_pattern_length..=max_pattern_length {
            for start in 0..=(deltas.len().saturating_sub(pattern_length)) {
                let pattern_candidate = &deltas[start..start + pattern_length];
                
                // Skip patterns that are all zeros
                if pattern_candidate.iter().all(|&x| x == 0) {
                    continue;
                }

                let mut similar_occurrences = Vec::new();
                
                // Find similar sequences elsewhere
                for search_start in (start + pattern_length)..=(deltas.len().saturating_sub(pattern_length)) {
                    let test_sequence = &deltas[search_start..search_start + pattern_length];
                    
                    if Self::sequence_similarity(pattern_candidate, test_sequence, similarity_threshold) {
                        similar_occurrences.push(search_start);
                    }
                }

                if similar_occurrences.len() >= 2 { // Including original, need at least 3 total
                    let avg_size = pattern_candidate.iter().filter(|&&x| x > 0).map(|&x| x as f64).sum::<f64>() 
                        / pattern_candidate.iter().filter(|&&x| x > 0).count().max(1) as f64;
                    
                    let frequency = if similar_occurrences.len() > 1 {
                        let first_timestamp = timestamps.get(start + 1).unwrap_or(&0);
                        let last_occurrence = similar_occurrences.last().unwrap_or(&start);
                        let last_timestamp = timestamps.get(last_occurrence + 1).unwrap_or(&0);
                        let total_time = last_timestamp.saturating_sub(*first_timestamp) as f64 / 60.0;
                        total_time / similar_occurrences.len() as f64
                    } else {
                        60.0
                    };

                    let confidence = (similar_occurrences.len() + 1) as f64 / (deltas.len() / pattern_length).max(1) as f64;

                    patterns.push(FuzzyPattern {
                        pattern_type: "sequence_similarity".to_string(),
                        size: avg_size,
                        frequency_minutes: frequency,
                        confidence: confidence.min(1.0),
                        occurrences: similar_occurrences.len() + 1,
                        method_confidence: confidence * 0.8, // Weight sequence detection
                    });
                }
            }
        }

        patterns.sort_by(|a, b| b.method_confidence.partial_cmp(&a.method_confidence).unwrap_or(std::cmp::Ordering::Equal));
        patterns.into_iter().take(3).collect() // Return top 3 patterns
    }

    fn sequence_similarity(seq1: &[i64], seq2: &[i64], tolerance: f64) -> bool {
        if seq1.len() != seq2.len() {
            return false;
        }

        if seq1.len() == 0 {
            return true;
        }

        let mut similarity_score = 0.0;
        let mut total_comparisons = 0;

        for (a, b) in seq1.iter().zip(seq2.iter()) {
            total_comparisons += 1;
            
            if *a == 0 && *b == 0 {
                similarity_score += 1.0;
            } else if *a == 0 || *b == 0 {
                similarity_score += 0.1; // Small penalty for zero vs non-zero
            } else {
                let max_val = (*a).abs().max((*b).abs()) as f64;
                let diff = (*a - *b).abs() as f64;
                let local_similarity = 1.0 - (diff / max_val.max(1.0)).min(1.0);
                similarity_score += local_similarity;
            }
        }

        (similarity_score / total_comparisons as f64) >= tolerance
    }

    // Method 2: Velocity Pattern Detection
    fn detect_velocity_patterns(deltas: &[i64], timestamps: &[u64]) -> Vec<FuzzyPattern> {
        let mut patterns = Vec::new();
        let velocity_tolerance = 0.4;

        // Extract non-zero periods and calculate velocities
        let mut velocities = Vec::new();
        for (i, &delta) in deltas.iter().enumerate() {
            if delta > 0 && i > 0 {
                let time_diff = timestamps.get(i + 1).unwrap_or(&0)
                    .saturating_sub(*timestamps.get(i).unwrap_or(&0)) as f64 / 60.0; // minutes
                let velocity = delta as f64 / time_diff.max(0.33); // min 20 seconds
                velocities.push((i, velocity, delta));
            }
        }

        if velocities.len() < 3 {
            return patterns;
        }

        // Cluster similar velocities
        let mut velocity_clusters: HashMap<u32, Vec<(usize, f64, i64)>> = HashMap::new();
        
        for &(pos, velocity, delta) in &velocities {
            let velocity_key = (velocity * 100.0).round() as u32;
            
            // Find existing cluster within tolerance
            let mut found_cluster = None;
            for &existing_key in velocity_clusters.keys() {
                let existing_velocity = existing_key as f64 / 100.0;
                if (velocity - existing_velocity).abs() / existing_velocity.max(0.01) <= velocity_tolerance {
                    found_cluster = Some(existing_key);
                    break;
                }
            }

            let cluster_key = found_cluster.unwrap_or(velocity_key);
            velocity_clusters.entry(cluster_key).or_default().push((pos, velocity, delta));
        }

        // Find clusters with temporal regularity
        for (velocity_key, cluster) in velocity_clusters {
            if cluster.len() >= 3 {
                let positions: Vec<usize> = cluster.iter().map(|&(pos, _, _)| pos).collect();
                let intervals: Vec<f64> = positions.windows(2)
                    .map(|w| {
                        let t1 = timestamps.get(w[0] + 1).unwrap_or(&0);
                        let t2 = timestamps.get(w[1] + 1).unwrap_or(&0);
                        t2.saturating_sub(*t1) as f64 / 60.0
                    })
                    .collect();

                if intervals.len() > 0 {
                    let avg_interval = intervals.iter().sum::<f64>() / intervals.len() as f64;
                    let variance = intervals.iter()
                        .map(|&x| (x - avg_interval).powi(2))
                        .sum::<f64>() / intervals.len() as f64;
                    let coefficient_of_variation = (variance.sqrt() / avg_interval.max(1.0)).min(1.0);

                    if coefficient_of_variation < 0.5 { // Reasonably regular
                        let avg_velocity = velocity_key as f64 / 100.0;
                        let avg_size = cluster.iter().map(|&(_, _, delta)| delta as f64).sum::<f64>() / cluster.len() as f64;
                        let confidence = cluster.len() as f64 / velocities.len() as f64;

                        patterns.push(FuzzyPattern {
                            pattern_type: "velocity_regular".to_string(),
                            size: avg_size,
                            frequency_minutes: avg_interval,
                            confidence: confidence.min(1.0),
                            occurrences: cluster.len(),
                            method_confidence: confidence * (1.0 - coefficient_of_variation) * 0.9,
                        });
                    }
                }
            }
        }

        patterns.sort_by(|a, b| b.method_confidence.partial_cmp(&a.method_confidence).unwrap_or(std::cmp::Ordering::Equal));
        patterns.into_iter().take(2).collect()
    }

    // Method 3: Rhythm Pattern Detection
    fn detect_rhythm_patterns(deltas: &[i64], timestamps: &[u64]) -> Vec<FuzzyPattern> {
        let mut patterns = Vec::new();

        // Find activity periods (non-zero deltas)
        let activity_periods: Vec<(usize, i64)> = deltas.iter().enumerate()
            .filter(|(_, &delta)| delta > 0)
            .map(|(i, &delta)| (i, delta))
            .collect();

        if activity_periods.len() < 3 {
            return patterns;
        }

        // Calculate intervals between activity in minutes
        let intervals: Vec<f64> = activity_periods.windows(2)
            .map(|w| {
                let t1 = timestamps.get(w[0].0 + 1).unwrap_or(&0);
                let t2 = timestamps.get(w[1].0 + 1).unwrap_or(&0);
                t2.saturating_sub(*t1) as f64 / 60.0
            })
            .collect();

        // Try different tolerance levels for rhythm detection
        for tolerance in [0.2, 0.4, 0.6] {
            let modal_intervals = Self::find_approximate_modes(&intervals, tolerance);
            
            for (modal_interval, occurrences) in modal_intervals {
                if occurrences >= 3 {
                    let avg_size = activity_periods.iter()
                        .map(|(_, delta)| *delta as f64)
                        .sum::<f64>() / activity_periods.len() as f64;
                    
                    let confidence = occurrences as f64 / intervals.len() as f64;
                    let regularity_bonus = 1.0 - tolerance; // Reward tighter tolerance

                    patterns.push(FuzzyPattern {
                        pattern_type: format!("rhythm_{}pct", (tolerance * 100.0) as u32),
                        size: avg_size,
                        frequency_minutes: modal_interval,
                        confidence: confidence.min(1.0),
                        occurrences,
                        method_confidence: confidence * regularity_bonus * 0.85,
                    });
                }
            }
        }

        patterns.sort_by(|a, b| b.method_confidence.partial_cmp(&a.method_confidence).unwrap_or(std::cmp::Ordering::Equal));
        patterns.into_iter().take(2).collect()
    }

    fn find_approximate_modes(intervals: &[f64], tolerance: f64) -> Vec<(f64, usize)> {
        if intervals.is_empty() {
            return Vec::new();
        }

        let mut modes = Vec::new();
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

            if cluster.len() >= 2 {
                let avg_interval = cluster.iter().sum::<f64>() / cluster.len() as f64;
                modes.push((avg_interval, cluster.len()));
            }
        }

        modes
    }

    // Pattern Fusion and Selection
    fn fuse_fuzzy_patterns(
        seq_patterns: Vec<FuzzyPattern>,
        vel_patterns: Vec<FuzzyPattern>,
        rhythm_patterns: Vec<FuzzyPattern>,
    ) -> Option<ModalPattern> {
        let mut all_patterns = Vec::new();
        all_patterns.extend(seq_patterns);
        all_patterns.extend(vel_patterns);
        all_patterns.extend(rhythm_patterns);

        if all_patterns.is_empty() {
            return None;
        }

        // Sort by method confidence and select best
        all_patterns.sort_by(|a, b| b.method_confidence.partial_cmp(&a.method_confidence).unwrap_or(std::cmp::Ordering::Equal));
        
        let best_pattern = &all_patterns[0];
        
        // Calculate overall confidence as weighted average
        let total_weight: f64 = all_patterns.iter().map(|p| p.method_confidence).sum();
        let weighted_confidence = if total_weight > 0.0 {
            all_patterns.iter()
                .map(|p| p.confidence * p.method_confidence)
                .sum::<f64>() / total_weight
        } else {
            0.0
        };

        Some(ModalPattern {
            size: best_pattern.size,
            ratio: 1.0, // Will be calculated later based on moving week vs inferred volume
            frequency_minutes: best_pattern.frequency_minutes,
            occurrence_count: best_pattern.occurrences,
            confidence: weighted_confidence,
            detection_method: best_pattern.pattern_type.clone(),
        })
    }

    // Main fuzzy pattern detection entry point
    fn detect_fuzzy_modal_pattern(
        moving_week_deltas: &[i64],
        inferred_volume_history: &[i64],
        timestamps: &[u64],
    ) -> (Option<ModalPattern>, PatternDetails) {
        // Try fuzzy methods first
        let seq_patterns = Self::detect_sequence_similarity_patterns(moving_week_deltas, timestamps);
        let vel_patterns = Self::detect_velocity_patterns(moving_week_deltas, timestamps);
        let rhythm_patterns = Self::detect_rhythm_patterns(moving_week_deltas, timestamps);

        let pattern_details = PatternDetails {
            detection_method: "fuzzy_combined".to_string(),
            fuzzy_confidence: 0.0, // Will be updated
            legacy_confidence: None,
            sequence_patterns_found: seq_patterns.len(),
            velocity_patterns_found: vel_patterns.len(),
            rhythm_patterns_found: rhythm_patterns.len(),
        };

        // Try to fuse fuzzy patterns
        if let Some(mut fuzzy_pattern) = Self::fuse_fuzzy_patterns(seq_patterns, vel_patterns, rhythm_patterns) {
            // Calculate ratio based on actual data
            let pattern_periods = Self::find_patterns_from_deltas(moving_week_deltas, inferred_volume_history, timestamps);
            if !pattern_periods.is_empty() {
                let total_mw_delta: i64 = pattern_periods.iter().map(|p| p.moving_week_delta).sum();
                let total_inferred: i64 = pattern_periods.iter().map(|p| p.inferred_volume).sum();
                fuzzy_pattern.ratio = if total_inferred > 0 {
                    total_mw_delta as f64 / total_inferred as f64
                } else {
                    1.0
                };
            }

            let mut updated_details = pattern_details;
            updated_details.fuzzy_confidence = fuzzy_pattern.confidence;
            return (Some(fuzzy_pattern), updated_details);
        }

        // Fallback to legacy clustering
        let pattern_periods = Self::find_patterns_from_deltas(moving_week_deltas, inferred_volume_history, timestamps);
        if let Some(legacy_pattern) = Self::detect_modal_pattern_legacy(&pattern_periods) {
            let mut legacy_details = pattern_details;
            legacy_details.detection_method = "legacy_clustering".to_string();
            legacy_details.legacy_confidence = Some(legacy_pattern.confidence);
            return (Some(legacy_pattern), legacy_details);
        }

        (None, pattern_details)
    }

    // Legacy pattern detection (preserved from original)
    fn find_patterns_from_deltas(
        moving_week_deltas: &[i64],
        inferred_volume_history: &[i64],
        timestamps: &[u64],
    ) -> Vec<PatternPeriod> {
        let mut patterns = Vec::new();
        for i in 0..moving_week_deltas.len().min(inferred_volume_history.len()) {
            let delta = moving_week_deltas[i];
            let inferred = inferred_volume_history[i];
            if delta > 0 && inferred > 0 {
                patterns.push(PatternPeriod {
                    position: i,
                    moving_week_delta: delta,
                    inferred_volume: inferred,
                    timestamp: timestamps[i + 1], // +1 because deltas are offset from snapshots
                });
            }
        }
        patterns
    }

    fn detect_modal_pattern_legacy(pattern_periods: &[PatternPeriod]) -> Option<ModalPattern> {
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

        // Use fuzzy pattern detection
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

        // Use moving week total as ground truth (no scaling)
        let (instabuy_modal_size, instabuy_pattern_frequency, instabuy_scale_factor, instabuy_estimated_true_volume) = 
            if let Some(pattern) = &instabuy_modal_pattern {
                // Calculate scale factor for diagnostics, but don't use it for final volume
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
                // Use moving week total as ground truth
                (pattern.size, pattern.frequency_minutes, scale_factor, self.total_buy_moving_week_activity as f64)
            } else {
                (0.0, 0.0, 1.0, self.total_buy_moving_week_activity as f64)
            };

        let (instasell_modal_size, instasell_pattern_frequency, instasell_scale_factor, instasell_estimated_true_volume) = 
            if let Some(pattern) = &instasell_modal_pattern {
                // Calculate scale factor for diagnostics, but don't use it for final volume
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
                // Use moving week total as ground truth
                (pattern.size, pattern.frequency_minutes, scale_factor, self.total_sell_moving_week_activity as f64)
            } else {
                (0.0, 0.0, 1.0, self.total_sell_moving_week_activity as f64)
            };

        // Calculate overall pattern detection confidence
        let buy_confidence = instabuy_modal_pattern.as_ref().map(|p| p.confidence).unwrap_or(0.0);
        let sell_confidence = instasell_modal_pattern.as_ref().map(|p| p.confidence).unwrap_or(0.0);
        let pattern_detection_confidence = ((buy_confidence + sell_confidence) / 2.0) * 100.0;

        // Combine pattern details
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
            sequence_patterns_found: instabuy_pattern_details.sequence_patterns_found + instasell_pattern_details.sequence_patterns_found,
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

    let api_poll_interval_secs = std::env::var("API_POLL_INTERVAL_SECONDS")
        .ok().and_then(|s| s.parse::<u64>().ok()).unwrap_or(20);

    const TARGET_WINDOWS: usize = 180;  // Exactly 1 hour at 20-second intervals

    println!("Configuration: Target windows = {} (1 hour), polling every {} seconds.", 
        TARGET_WINDOWS, api_poll_interval_secs);
    println!("Fuzzy pattern detection: Sequence similarity, velocity clustering, rhythm detection with legacy fallback.");
    println!("Note: All products exported including zero-activity periods for frequency analysis.");

    loop {
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
                println!("Updated {} product states. Progress: {}/{} windows", 
                    states.len(),
                    states.values().map(|s| s.windows_processed).max().unwrap_or(0),
                    TARGET_WINDOWS
                );
            }
            Ok(None) => println!("No new snapshot data (Last-Modified unchanged)."),
            Err(e) => eprintln!("Error fetching snapshot: {}", e),
        }

        // Check if we've completed a full hour cycle
        let max_windows = states.values().map(|s| s.windows_processed).max().unwrap_or(0);
        
        if max_windows >= TARGET_WINDOWS {
            println!(">>> Completing hourly cycle: {} windows collected (1 hour of data)", max_windows);
            
            // Process ALL products, including those with zero activity
            let results: Vec<_> = states.iter()
                .map(|(pid, state)| {
                    let result = state.finalize_with_sequences(pid.clone());
                    
                    // Log what we're including
                    let activity_summary = if result.instabuy_estimated_true_volume > 0.0 || result.instasell_estimated_true_volume > 0.0 {
                        format!("ACTIVE (buy: {:.1}, sell: {:.1}) [{}]", 
                            result.instabuy_estimated_true_volume, 
                            result.instasell_estimated_true_volume,
                            result.pattern_details.detection_method
                        )
                    } else {
                        "ZERO_ACTIVITY".to_string()
                    };
                    
                    println!("  Including {}: {} - {} windows", pid, activity_summary, state.windows_processed);
                    result
                })
                .collect();
                
            let ts = Utc::now().format("%Y%m%d%H%M%S").to_string();
            let local_path = format!("metrics/metrics_{}.json", ts);
            let remote_mega_path = format!("/remote_metrics/metrics_{}.json", ts);
            
            let zero_activity_count = results.iter()
                .filter(|r| r.instabuy_estimated_true_volume == 0.0 && r.instasell_estimated_true_volume == 0.0)
                .count();
            let active_count = results.len() - zero_activity_count;
            
            let fuzzy_detected = results.iter()
                .filter(|r| r.pattern_details.detection_method.contains("fuzzy"))
                .count();
            let legacy_detected = results.iter()
                .filter(|r| r.pattern_details.detection_method.contains("legacy"))
                .count();
            
            println!("Exporting hourly metrics: {} active, {} zero-activity, {} total", 
                active_count, zero_activity_count, results.len());
            println!("Pattern detection: {} fuzzy, {} legacy fallback", fuzzy_detected, legacy_detected);
            
            match fs::write(&local_path, serde_json::to_string_pretty(&results)?) {
                Ok(_) => {
                    println!("Successfully wrote 1-hour metrics for {} products (including zero-activity and delta sequences)", results.len());
                    
                    // Run export engine
                    let export_engine_path = std::env::var("EXPORT_ENGINE_PATH")
                        .unwrap_or_else(|_| "export_engine".to_string());
                    match Command::new(&export_engine_path)
                        .arg(&local_path)
                        .arg(&remote_mega_path)
                        .output() {
                        Ok(output) => {
                            println!("Export engine stdout:\n{}", String::from_utf8_lossy(&output.stdout));
                            if !output.stderr.is_empty() {
                                eprintln!("Export engine stderr:\n{}", String::from_utf8_lossy(&output.stderr));
                            }
                        }
                        Err(e) => eprintln!("Failed to execute export_engine: {}", e),
                    }
                }
                Err(e) => eprintln!("Failed to write metrics file {}: {}", local_path, e),
            }
            
            // Reset for next hour
            println!("Resetting states for next hourly cycle");
            states.clear();
        }

        sleep(Duration::from_secs(api_poll_interval_secs)).await;
    }
}