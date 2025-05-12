package main

import (
	"encoding/json"
	"log"
	"os"
	"time" // Added for timestamp
)

const (
	metricsFilename    = "latest_metrics.json"
	itemFilesDir       = "dependencies/items"
	outputJSONFilename = "optimizer_results.json"
)

// New struct to hold the summary and the results
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"` // OptimizedItemResult is defined in optimizer.go
}

// New struct for the summary information
type OptimizationSummary struct {
	RunTimestamp                string  `json:"run_timestamp"`
	APILastUpdatedTimestamp     string  `json:"api_last_updated_timestamp,omitempty"`
	TotalItemsConsidered        int     `json:"total_items_considered"`
	ItemsSuccessfullyCalculated int     `json:"items_successfully_calculated"` // CalculationPossible = true
	ItemsWithCalculationErrors  int     `json:"items_with_calculation_errors"` // CalculationPossible = false
	MaxAllowedCycleTimeSecs     float64 `json:"max_allowed_cycle_time_seconds"`
	MaxInitialSearchQuantity    float64 `json:"max_initial_search_quantity"`
}

func main() {
	runStartTime := time.Now()

	// 1. Load API Data
	log.Println("Loading Hypixel API data...")
	apiResp, err := getApiResponse() // This populates apiResponseCache
	if err != nil {
		log.Fatalf("CRITICAL: Initial API load failed: %v. Optimizer cannot run.", err)
	}
	if apiResp == nil || apiResp.Products == nil {
		log.Fatalf("CRITICAL: API data is nil or has no products after load attempt. Optimizer cannot run. Stored API fetch error: %v", apiFetchErr)
	}
	log.Println("Initial API data loaded successfully.")

	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 {
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).Format(time.RFC3339) // API gives ms
	}

	// 2. Load Metrics Data
	log.Println("Loading metrics data...")
	metricsMap, err := getMetricsMap(metricsFilename)
	if err != nil {
		log.Fatalf("CRITICAL: Cannot load metrics '%s': %v. Optimizer cannot run.", metricsFilename, err)
	}
	if metricsMap == nil {
		log.Fatalf("CRITICAL: Metrics map is nil after load, even without a direct load error. Optimizer cannot run.")
	}
	log.Printf("Metrics data loaded from '%s'.", metricsFilename)

	// 3. Get Item IDs for Optimization
	var itemIDs []string
	apiCacheMutex.RLock() // Accessing global apiResponseCache
	if apiResponseCache == nil || apiResponseCache.Products == nil {
		apiCacheMutex.RUnlock()
		log.Fatalf("CRITICAL: Global apiResponseCache is nil or has no products when trying to get item IDs.")
	}
	for id := range apiResponseCache.Products {
		itemIDs = append(itemIDs, id)
	}
	apiCacheMutex.RUnlock()

	if len(itemIDs) == 0 {
		log.Fatalf("CRITICAL: No items found in API data to optimize.")
	}
	log.Printf("Found %d items to potentially optimize.", len(itemIDs))

	// 4. Define Optimizer Parameters
	maxAllowedFillTime := 3600.0
	maxInitialSearchQty := 1000000.0

	log.Printf("Optimizer Parameters: Max Allowed Total Cycle Time=%.1fs, Max Initial Quantity for Search=%.1f", maxAllowedFillTime, maxInitialSearchQty)

	// 5. Run Optimization
	log.Println("Starting full optimization process...")
	optimizedResults := RunFullOptimization(itemIDs, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, maxInitialSearchQty)
	log.Printf("Optimization complete. Generated %d results.", len(optimizedResults))

	// 6. Prepare Summary
	successfullyCalculatedCount := 0
	calculationErrorCount := 0
	for _, res := range optimizedResults {
		if res.CalculationPossible {
			successfullyCalculatedCount++
		} else {
			calculationErrorCount++
		}
	}

	summary := OptimizationSummary{
		RunTimestamp:                runStartTime.Format(time.RFC3339),
		APILastUpdatedTimestamp:     apiLastUpdatedStr,
		TotalItemsConsidered:        len(itemIDs), // Could also be len(optimizedResults) if some items were skipped before optimization call
		ItemsSuccessfullyCalculated: successfullyCalculatedCount,
		ItemsWithCalculationErrors:  calculationErrorCount,
		MaxAllowedCycleTimeSecs:     maxAllowedFillTime,
		MaxInitialSearchQuantity:    maxInitialSearchQty,
	}

	// 7. Combine summary and results into the final output structure
	finalOutput := OptimizationRunOutput{
		Summary: summary,
		Results: optimizedResults,
	}

	// 8. Marshal the final output structure to JSON
	jsonBytes, err := json.MarshalIndent(finalOutput, "", "  ")
	if err != nil {
		log.Fatalf("CRITICAL: Failed to marshal final optimization output to JSON: %v", err)
	}

	// 9. Write JSON to File
	log.Printf("Writing optimizer results to '%s'...", outputJSONFilename)
	err = os.WriteFile(outputJSONFilename, jsonBytes, 0644)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to write JSON output to file '%s': %v", outputJSONFilename, err)
	}

	log.Printf("Optimizer results successfully written to '%s'.", outputJSONFilename)
}
