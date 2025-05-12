package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

const (
	metricsFilename           = "latest_metrics.json"
	itemFilesDir              = "dependencies/items"
	outputJSONFilename        = "optimizer_results.json"
	failedItemsReportFilename = "failed_items_report.json" // New constant for the failed items report
)

// Struct to hold the summary and the results for the main output
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"`
}

// Struct for the summary information
type OptimizationSummary struct {
	RunTimestamp                string  `json:"run_timestamp"`
	APILastUpdatedTimestamp     string  `json:"api_last_updated_timestamp,omitempty"`
	TotalItemsConsidered        int     `json:"total_items_considered"`
	ItemsSuccessfullyCalculated int     `json:"items_successfully_calculated"`
	ItemsWithCalculationErrors  int     `json:"items_with_calculation_errors"`
	MaxAllowedCycleTimeSecs     float64 `json:"max_allowed_cycle_time_seconds"`
	MaxInitialSearchQuantity    float64 `json:"max_initial_search_quantity"`
}

// New struct for detailing failed items in the separate report
type FailedItemDetail struct {
	ItemName     string `json:"item_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func main() {
	runStartTime := time.Now()

	// 1. Load API Data
	log.Println("Loading Hypixel API data...")
	apiResp, err := getApiResponse()
	if err != nil {
		log.Fatalf("CRITICAL: Initial API load failed: %v. Optimizer cannot run.", err)
	}
	if apiResp == nil || apiResp.Products == nil {
		log.Fatalf("CRITICAL: API data is nil or has no products after load attempt. Optimizer cannot run. Stored API fetch error: %v", apiFetchErr)
	}
	log.Println("Initial API data loaded successfully.")

	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 {
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).Format(time.RFC3339)
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
	apiCacheMutex.RLock()
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

	// 6. Prepare Summary and collect failed items
	successfullyCalculatedCount := 0
	calculationErrorCount := 0
	var failedItemsDetails []FailedItemDetail // Slice to store details of failed items

	for _, res := range optimizedResults {
		if res.CalculationPossible {
			successfullyCalculatedCount++
		} else {
			calculationErrorCount++
			failedItemsDetails = append(failedItemsDetails, FailedItemDetail{
				ItemName:     res.ItemName,
				ErrorMessage: res.ErrorMessage,
			})
		}
	}

	summary := OptimizationSummary{
		RunTimestamp:                runStartTime.Format(time.RFC3339),
		APILastUpdatedTimestamp:     apiLastUpdatedStr,
		TotalItemsConsidered:        len(itemIDs),
		ItemsSuccessfullyCalculated: successfullyCalculatedCount,
		ItemsWithCalculationErrors:  calculationErrorCount,
		MaxAllowedCycleTimeSecs:     maxAllowedFillTime,
		MaxInitialSearchQuantity:    maxInitialSearchQty,
	}

	// 7. Combine summary and results for the main output file
	mainFinalOutput := OptimizationRunOutput{
		Summary: summary,
		Results: optimizedResults,
	}

	// 8. Marshal the main output structure to JSON
	mainJsonBytes, err := json.MarshalIndent(mainFinalOutput, "", "  ")
	if err != nil {
		log.Fatalf("CRITICAL: Failed to marshal main optimization output to JSON: %v", err)
	}

	// 9. Write main JSON to File
	log.Printf("Writing main optimizer results to '%s'...", outputJSONFilename)
	err = os.WriteFile(outputJSONFilename, mainJsonBytes, 0644)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to write main JSON output to file '%s': %v", outputJSONFilename, err)
	}
	log.Printf("Main optimizer results successfully written to '%s'.", outputJSONFilename)

	// 10. Marshal and Write Failed Items Report (if any)
	if len(failedItemsDetails) > 0 {
		failedItemsJsonBytes, err := json.MarshalIndent(failedItemsDetails, "", "  ")
		if err != nil {
			log.Fatalf("CRITICAL: Failed to marshal failed items report to JSON: %v", err)
		}

		log.Printf("Writing failed items report to '%s'...", failedItemsReportFilename)
		err = os.WriteFile(failedItemsReportFilename, failedItemsJsonBytes, 0644)
		if err != nil {
			log.Fatalf("CRITICAL: Failed to write failed items report to file '%s': %v", failedItemsReportFilename, err)
		}
		log.Printf("Failed items report successfully written to '%s'.", failedItemsReportFilename)
	} else {
		log.Println("No items failed calculation; failed items report not generated.")
	}
}
