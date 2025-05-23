// metrics.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	// No other imports needed unless dlog uses more
)

// --- Metrics Structs ---
// Ensure this struct matches the fields in your latest_metrics.json file
type ProductMetrics struct {
	ProductID      string  `json:"product_id"`              // Expecting the canonical/normalized ID here
	SellSize       float64 `json:"sell_size"`               // Average size of insta-sell orders filled
	SellFrequency  float64 `json:"sell_frequency"`          // Rate at which insta-sell orders are filled (e.g., per hour?)
	OrderSize      float64 `json:"order_size_average"`      // Average size of buy orders placed/filled
	OrderFrequency float64 `json:"order_frequency_average"` // Rate at which buy orders are placed/filled
	// Add other metrics if they exist and are needed, e.g.:
	// BuyMovingWeek  float64 `json:"buy_moving_week"` // If present and useful, though live API is often preferred
}

// --- Global variables for Metrics caching (used by legacy direct file load) ---
// This caching mechanism is less used now that main.go handles metrics loading and updating.
// However, getMetricsMap might still be called by older code paths if they exist,
// or could be repurposed if direct file access is needed elsewhere.
var (
	metricsFileCache    map[string]ProductMetrics // Key is ProductID (should be normalized)
	loadMetricsFileOnce sync.Once                 // Ensures loading happens only once
	metricsFileLoadErr  error                     // Stores error encountered during loading
	metricsFileMutex    sync.RWMutex              // Read/Write mutex for thread-safe access
)

// loadMetricsDataFromFile reads a specific JSON file and populates the metricsFileCache.
// This is intended for a one-time load if direct file access is used, separate from main.go's mechanism.
func loadMetricsDataFromFile(filename string) {
	metricsFileMutex.Lock()
	defer metricsFileMutex.Unlock()

	// Prevent re-entry or redundant work if already attempted
	if metricsFileCache != nil || metricsFileLoadErr != nil {
		dlog("Metrics file loading already attempted (Cache:%v, Err:%v) for %s. Skipping.", metricsFileCache != nil, metricsFileLoadErr != nil, filename)
		return
	}

	dlog("Loading metrics data directly from file '%s'...", filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		metricsFileLoadErr = fmt.Errorf("failed to read metrics file '%s': %w", filename, err)
		log.Printf("ERROR (loadMetricsDataFromFile): %v", metricsFileLoadErr)
		return
	}

	var metricsList []ProductMetrics
	if err := json.Unmarshal(data, &metricsList); err != nil {
		metricsFileLoadErr = fmt.Errorf("failed to parse metrics JSON from '%s': %w", filename, err)
		log.Printf("ERROR (loadMetricsDataFromFile): %v", metricsFileLoadErr)
		return
	}

	tempCache := make(map[string]ProductMetrics, len(metricsList))
	skippedCount := 0
	for _, pm := range metricsList {
		if pm.ProductID == "" {
			log.Printf("Warning (loadMetricsDataFromFile): Skipping metric entry with empty product_id in '%s'", filename)
			skippedCount++
			continue
		}
		normalizedID := BAZAAR_ID(pm.ProductID)
		if existing, found := tempCache[normalizedID]; found {
			log.Printf("Warning (loadMetricsDataFromFile): Duplicate normalized ProductID '%s' found in metrics file '%s'. Overwriting previous entry (%+v) with (%+v).", normalizedID, filename, existing, pm)
		}
		pm.ProductID = normalizedID // Ensure ProductID within struct is also normalized
		tempCache[normalizedID] = pm
	}

	metricsFileCache = tempCache
	metricsFileLoadErr = nil // Clear error on success
	dlog("Metrics data from file '%s' loaded and cached successfully. Loaded: %d, Skipped: %d", filename, len(metricsFileCache), skippedCount)
}

// getMetricsMapFromFile ensures metrics are loaded once from a specific file and returns the cached map.
// This is distinct from main.go's metrics handling which uses `latestMetricsData`.
func getMetricsMapFromFile(filename string) (map[string]ProductMetrics, error) {
	loadMetricsFileOnce.Do(func() {
		loadMetricsDataFromFile(filename) // This will only run the loading logic once per application lifetime
	})

	metricsFileMutex.RLock() // Acquire read lock for accessing shared cache and error
	defer metricsFileMutex.RUnlock()

	if metricsFileLoadErr != nil {
		return nil, metricsFileLoadErr // Return error if loading failed
	}

	if metricsFileCache == nil {
		// This state implies loading was attempted (due to Once.Do) but cache remains nil,
		// and no error was set. This could mean an empty file or other non-error producing issue.
		log.Printf("Warning (getMetricsMapFromFile): Metrics cache for '%s' is nil after load attempt, but no error reported. Returning empty map.", filename)
		return make(map[string]ProductMetrics), nil // Return empty map rather than nil if no explicit error
	}

	return metricsFileCache, nil
}
