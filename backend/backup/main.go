package main

import (
	"fmt"
	"log"
	"math" // Needed for price extraction defaults
	"os"
	// No longer need "io", "net/http", "os", "encoding/json" directly here
)

// --- Debug helper definition needs to be accessible ---
var debug = os.Getenv("DEBUG") == "1"

func dlog(format string, args ...interface{}) {
	if debug {
		log.Printf("DEBUG: "+format, args...)
	}
}

func main() {
	metricsFilename := "latest_metrics.json"

	// --- Pre-load/cache data ---
	fmt.Println("Loading metrics from", metricsFilename, "...")
	metricsMap, metricsErr := GetMetricsMap(metricsFilename) // Call uppercase function
	if metricsErr != nil {
		log.Printf("Warning: Initial metrics load failed: %v.", metricsErr)
		// Decide if fatal or continue
	} else if metricsMap != nil { // Check map isn't nil
		fmt.Println("Metrics loaded.")
	}

	fmt.Println("Fetching Bazaar data...")
	apiResp, apiErr := GetApiResponse() // Call uppercase function
	if apiErr != nil {
		log.Printf("Warning: Failed to fetch live API data: %v.", apiErr)
	} else if apiResp != nil {
		fmt.Println("Bazaar data fetched.")
	} else {
		log.Println("Warning: API response is nil despite no fetch error.")
	}
	// --- End Pre-load ---

	// --- User Inputs ---
	var prod string
	var qty float64

	fmt.Print("Product ID (e.g., ENCHANTED_LAPIS_BLOCK): ")
	if _, err := fmt.Scanln(&prod); err != nil {
		log.Fatalf("Invalid input for Product ID: %v", err)
	}
	fmt.Print("Quantity: ")
	if _, err := fmt.Scanln(&qty); err != nil || qty <= 0 {
		log.Fatalf("Invalid input for Quantity: %v", err)
	}

	// --- Extract Product Data from API Response ---
	var sellP, buyP float64 = math.NaN(), math.NaN() // Default to NaN
	var extractErr error

	if apiErr != nil {
		extractErr = fmt.Errorf("cannot extract prices due to API fetch error: %w", apiErr)
	} else if apiResp == nil {
		extractErr = fmt.Errorf("cannot extract prices; API response is nil")
	} else {
		prodData, ok := apiResp.Products[prod]
		if !ok {
			extractErr = fmt.Errorf("product '%s' not found in API response", prod)
		} else if len(prodData.SellSummary) == 0 || len(prodData.BuySummary) == 0 {
			extractErr = fmt.Errorf("sell_summary or buy_summary is empty for product '%s'", prod)
		} else {
			sellP = prodData.SellSummary[0].PricePerUnit
			buyP = prodData.BuySummary[0].PricePerUnit
			if sellP <= 0 || buyP <= 0 {
				extractErr = fmt.Errorf("invalid (non-positive) price found for product '%s'", prod)
			}
		}
	}

	// Handle extraction errors before calculation
	if extractErr != nil {
		log.Fatalf("Error extracting product data: %v", extractErr)
	}

	// --- Compute C10M ---
	fmt.Println("Calculating C10M...")
	var c10mResult C10MResult // Zero value will have Inf/NaN defaults

	if metricsErr != nil {
		log.Println("Skipping C10M calculation due to metrics load error.")
		// Initialize results with error state if needed
		c10mResult = C10MResult{
			Primary: math.Inf(1), Secondary: math.Inf(1), IF: math.NaN(), RR: math.NaN(),
			DeltaRatio: math.NaN(), Adjustment: math.NaN(), BestEstimate: math.Inf(1), BestMethod: "N/A (Metrics Error)",
		}
	} else if metricsMap == nil {
		log.Println("Skipping C10M calculation because metrics map is nil.")
		c10mResult = C10MResult{ /* Initialize with error state */ }
	} else {
		// Call function from c10m.go (now uppercase)
		c10mResult = CalculateC10M(prod, qty, sellP, buyP, metricsMap)
	}

	// --- Print Results ---
	// Call function from output.go (now uppercase)
	PrintResults(prod, qty, sellP, buyP, c10mResult)

}
