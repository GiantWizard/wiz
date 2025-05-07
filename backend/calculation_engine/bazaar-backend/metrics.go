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

// --- Global variables for Metrics caching ---
var (
	metricsCache    map[string]ProductMetrics // Key is ProductID (should be normalized)
	loadMetricsOnce sync.Once                 // Ensures loading happens only once
	metricsLoadErr  error                     // Stores error encountered during loading
	metricsMutex    sync.RWMutex              // Read/Write mutex for thread-safe access
)

// --- Metrics Loading Logic ---

// loadMetricsData reads the JSON file and populates the metricsCache.
// It should only be called once via loadMetricsOnce.Do().
// It handles setting metricsLoadErr internally.
func loadMetricsData(filename string) {
	// Lock for writing, as we are potentially modifying metricsCache and metricsLoadErr
	metricsMutex.Lock()
	defer metricsMutex.Unlock()

	// Double-check locking pattern: Check if already loaded/failed *inside* the lock
	// This prevents redundant work if multiple goroutines call getMetricsMap concurrently
	// before loadMetricsOnce.Do() completes.
	if metricsCache != nil || metricsLoadErr != nil {
		dlog("Metrics loading already attempted (Cache:%v, Err:%v). Skipping.", metricsCache != nil, metricsLoadErr != nil)
		return // Already loaded or failed
	}

	dlog("Loading metrics data from '%s'...", filename) // Assumes dlog defined in utils.go
	data, err := os.ReadFile(filename)
	if err != nil {
		metricsLoadErr = fmt.Errorf("failed to read metrics file '%s': %w", filename, err)
		log.Printf("ERROR: %v", metricsLoadErr)
		// metricsCache remains nil
		return // Exit after setting the error
	}

	// Expecting a JSON array of ProductMetrics objects
	var metricsList []ProductMetrics
	if err := json.Unmarshal(data, &metricsList); err != nil {
		metricsLoadErr = fmt.Errorf("failed to parse metrics JSON from '%s': %w", filename, err)
		log.Printf("ERROR: %v", metricsLoadErr)
		// metricsCache remains nil
		return // Exit after setting the error
	}

	// Create the map and populate it using normalized IDs
	tempCache := make(map[string]ProductMetrics, len(metricsList))
	skippedCount := 0
	for _, pm := range metricsList {
		if pm.ProductID == "" {
			log.Printf("Warning: Skipping metric entry with empty product_id in '%s'", filename)
			skippedCount++
			continue
		}
		// Normalize the ID *before* using it as a key
		normalizedID := BAZAAR_ID(pm.ProductID) // Assumes BAZAAR_ID defined in utils.go
		if existing, found := tempCache[normalizedID]; found {
			log.Printf("Warning: Duplicate normalized ProductID '%s' found in metrics file '%s'. Overwriting previous entry (%+v) with (%+v).", normalizedID, filename, existing, pm)
		}
		// Ensure the stored ProductID within the struct is also normalized for consistency? Optional.
		// pm.ProductID = normalizedID
		tempCache[normalizedID] = pm
	}

	// Assign to the global variable once the map is fully populated
	metricsCache = tempCache

	// Reset error state as loading was successful
	metricsLoadErr = nil

	dlog("Metrics data loaded and cached successfully. Loaded: %d, Skipped: %d", len(metricsCache), skippedCount)
}

// getMetricsMap ensures metrics are loaded once and returns the cached map.
// It provides thread-safe read access to the cache.
func getMetricsMap(filename string) (map[string]ProductMetrics, error) {
	// Ensure loadMetricsData runs only once across all goroutines calling this function.
	loadMetricsOnce.Do(func() {
		loadMetricsData(filename)
	})

	// Acquire read lock to safely access the shared cache and error variables.
	metricsMutex.RLock()
	defer metricsMutex.RUnlock()

	// Check if loading failed
	if metricsLoadErr != nil {
		return nil, metricsLoadErr
	}

	// Check if cache is nil (could happen if loading failed silently before error was set, or file was empty)
	if metricsCache == nil {
		// If no error was recorded, but cache is nil, it implies an empty or problematic file,
		// but not necessarily a read/parse error caught by loadMetricsData.
		// Return an empty map and log a warning, rather than an error, unless an error *was* previously set.
		log.Printf("Warning: Metrics cache is nil after load attempt, but no error reported. Returning empty map. Check file '%s'.", filename)
		return make(map[string]ProductMetrics), nil // Return empty map, not nil, if no error
	}

	// Return the populated cache and nil error
	return metricsCache, nil
}
