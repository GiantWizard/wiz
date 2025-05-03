package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
)

// --- Metrics Structs ---
type ProductMetrics struct {
	ProductID      string  `json:"product_id"`
	SellSize       float64 `json:"sell_size"`
	SellFrequency  float64 `json:"sell_frequency"`
	OrderSize      float64 `json:"order_size_average"`      // Corrected tag
	OrderFrequency float64 `json:"order_frequency_average"` // Corrected tag
	// Note: BuyMovingWeek from the original filltime example is omitted here,
	// as the live API data is preferred for instasell calculations.
	// If latest_metrics.json *does* contain a useful historical BMW, add it back.
	// BuyMovingWeek  float64 `json:"buy_moving_week"`
}

// --- Global variables for Metrics caching ---
var (
	metricsCache    map[string]ProductMetrics // Key is ProductID
	loadMetricsOnce sync.Once
	metricsLoadErr  error
	metricsMutex    sync.RWMutex // Use RWMutex for better concurrent reads
)

// --- Metrics Loading Logic ---
func loadMetricsData(filename string) {
	metricsMutex.Lock() // Lock for writing
	defer metricsMutex.Unlock()

	// Check if already loaded (double-check)
	if metricsCache != nil || metricsLoadErr != nil {
		return
	}

	dlog("Loading metrics data from '%s'...", filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		metricsLoadErr = fmt.Errorf("failed to read metrics file '%s': %w", filename, err)
		log.Printf("ERROR: %v", metricsLoadErr)
		return
	}

	var metricsList []ProductMetrics
	if err := json.Unmarshal(data, &metricsList); err != nil {
		metricsLoadErr = fmt.Errorf("failed to parse metrics JSON from '%s': %w", filename, err)
		log.Printf("ERROR: %v", metricsLoadErr)
		return
	}

	tempCache := make(map[string]ProductMetrics, len(metricsList))
	for _, pm := range metricsList {
		if pm.ProductID == "" {
			log.Printf("Warning: Skipping metric entry with empty product_id in '%s'", filename)
			continue
		}
		tempCache[pm.ProductID] = pm
	}
	metricsCache = tempCache // Assign once at the end
	dlog("Metrics data loaded and cached successfully.")
}

// getMetricsMap ensures metrics are loaded once and returns the cached map.
func getMetricsMap(filename string) (map[string]ProductMetrics, error) {
	loadMetricsOnce.Do(func() { loadMetricsData(filename) }) // Ensure loadMetricsData runs only once

	metricsMutex.RLock() // Lock for reading
	defer metricsMutex.RUnlock()

	if metricsLoadErr != nil {
		return nil, metricsLoadErr
	}
	// It's possible metricsCache is still nil if loading failed before error was set, or file was empty
	if metricsCache == nil {
		// Return an empty map instead of nil if no error occurred but cache is nil
		if metricsLoadErr == nil {
			dlog("Metrics cache is nil but no load error reported. Returning empty map.")
			return make(map[string]ProductMetrics), nil
		}
		return nil, fmt.Errorf("internal error: metrics cache is nil after load attempt")
	}
	return metricsCache, nil
}
