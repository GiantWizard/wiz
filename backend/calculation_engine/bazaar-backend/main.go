// main.go
package main

import (
	"fmt"
	"log"
	// "os" // No longer needed here unless more complex startup logic added
)

// --- Configuration (Application Wide) ---
const (
	metricsFilename = "latest_metrics.json"
	itemFilesDir    = "dependencies/items"
)

// --- Global Caches (Accessible within the 'main' package) ---
// These are declared here and populated by main, then read by server.go
var (
	apiRespGlobal    *HypixelAPIResponse
	metricsMapGlobal map[string]ProductMetrics
)

// --- Main Application Entry Point ---
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	fmt.Println("Starting Bazaar Calculation Application...")

	// --- Load Data ONCE at Startup ---
	var loadErr error
	fmt.Println("Initializing API data cache...")
	// Populate the global variable declared above
	apiRespGlobal, loadErr = getApiResponse()
	if loadErr != nil {
		// Log critical warning but allow server to potentially start
		// The handler will need to check if apiRespGlobal is nil
		log.Printf("WARNING: Initial API data fetch failed: %v. Calculations requiring fresh API data may fail or use stale data if available.", loadErr)
	} else {
		fmt.Println("API data cache initialized.")
	}

	fmt.Println("Initializing metrics data cache...")
	// Populate the global variable declared above
	metricsMapGlobal, loadErr = getMetricsMap(metricsFilename)
	if loadErr != nil {
		// Metrics are essential, so exit if loading fails
		log.Fatalf("CRITICAL: Initial metrics data load failed: %v. Server cannot start.", loadErr)
		// os.Exit(1) // Log.Fatalf already exits
	} else {
		fmt.Println("Metrics data cache initialized.")
	}
	fmt.Println("Data loading complete.")
	// --- End Data Loading ---

	// --- Start the Server ---
	// Call the function defined in server.go
	runServer()
}
