package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Represents the structure of a single metric object in the JSON files.
type Metric struct {
	ProductID                             string  `json:"product_id"`
	InstabuyPriceAverage                  float64 `json:"instabuy_price_average"`
	InstasellPriceAverage                 float64 `json:"instasell_price_average"`
	NewDemandOfferFrequencyAverage        float64 `json:"new_demand_offer_frequency_average"`
	NewDemandOfferSizeAverage             float64 `json:"new_demand_offer_size_average"`
	PlayerInstabuyTransactionFrequency    float64 `json:"player_instabuy_transaction_frequency"`
	PlayerInstabuyTransactionSizeAverage  float64 `json:"player_instabuy_transaction_size_average"`
	NewSupplyOfferFrequencyAverage        float64 `json:"new_supply_offer_frequency_average"`
	NewSupplyOfferSizeAverage             float64 `json:"new_supply_offer_size_average"`
	PlayerInstasellTransactionFrequency   float64 `json:"player_instasell_transaction_frequency"`
	PlayerInstasellTransactionSizeAverage float64 `json:"player_instasell_transaction_size_average"`
}

// Holds the aggregated and averaged metrics that will be served.
type AveragedMetrics map[string]Metric

// Global variable to store the latest averaged metrics, with a mutex for safe concurrent access.
var (
	latestAveragedMetrics AveragedMetrics
	metricsMutex          sync.RWMutex
)

// --- Main Application Logic ---

func main() {
	log.Println("[CALC-ENGINE] Application starting up...")

	// Create a temporary directory for downloading metric files.
	if err := os.MkdirAll("/tmp/metrics", os.ModePerm); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Could not create temp directory: %v", err)
	}

	// Launch the web server in a separate goroutine.
	go startWebServer()

	// A small initial delay to ensure the mega-session-manager is fully initialized.
	time.Sleep(10 * time.Second)

	// Main loop to periodically update the metrics.
	// Updates every 5 minutes, you can adjust this duration.
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run once immediately, then on every tick.
	updateLatestMetrics()
	for range ticker.C {
		updateLatestMetrics()
	}
}

// --- Web Server ---

func startWebServer() {
	http.HandleFunc("/latest_metrics/", metricsHandler)
	log.Println("[CALC-ENGINE] Starting web server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Web server failed: %v", err)
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	metricsMutex.RLock()
	defer metricsMutex.RUnlock()

	if latestAveragedMetrics == nil {
		http.Error(w, "Metrics not available yet, please try again shortly.", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(latestAveragedMetrics); err != nil {
		log.Printf("[CALC-ENGINE] ERROR: Failed to encode metrics to JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// --- Metrics Processing ---

func updateLatestMetrics() {
	log.Println("[CALC-ENGINE] Starting metrics update cycle...")
	remoteDir := "/remote_metrics"
	localDir := "/tmp/metrics"

	// 1. Clean up old files from the previous cycle.
	_ = os.RemoveAll(localDir)
	_ = os.MkdirAll(localDir, os.ModePerm)

	// 2. Find the 12 most recent files directly using mega-find.
	// This command asks the server to sort by modification time descending and return the top 12.
	log.Println("[CALC-ENGINE] Finding 12 most recent remote files...")
	cmd := exec.Command("mega-find", remoteDir, "--sort-by", "mtime-desc", "--limit", "12")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[CALC-ENGINE] ERROR: Failed to find remote files: %v\nOutput: %s", err, out)
		return
	}

	// 3. Get the list of full file paths from the output.
	filePaths := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(filePaths) == 0 || (len(filePaths) == 1 && filePaths[0] == "") {
		log.Println("[CALC-ENGINE] No metric files found in remote directory.")
		return
	}
	log.Printf("[CALC-ENGINE] Identified %d files to process.", len(filePaths))

	// 4. Download the identified files.
	var downloadedFiles []string
	for _, remotePath := range filePaths {
		if remotePath == "" {
			continue
		}
		log.Printf("[CALC-ENGINE] Downloading %s...", remotePath)
		getCmd := exec.Command("mega-get", remotePath, localDir)
		if out, err := getCmd.CombinedOutput(); err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Failed to download %s: %v\nOutput: %s", remotePath, err, out)
			continue // Skip this file and try the next one.
		}
		// Construct the full local path for the downloaded file.
		localFilePath := filepath.Join(localDir, filepath.Base(remotePath))
		downloadedFiles = append(downloadedFiles, localFilePath)
	}

	if len(downloadedFiles) == 0 {
		log.Println("[CALC-ENGINE] No files were successfully downloaded. Aborting cycle.")
		return
	}

	// 5. Read and parse the JSON from each downloaded file.
	var allMetrics [][]Metric
	for _, localPath := range downloadedFiles {
		file, err := os.Open(localPath)
		if err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Failed to open downloaded file %s: %v", localPath, err)
			continue
		}

		var metrics []Metric
		if err := json.NewDecoder(file).Decode(&metrics); err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Failed to parse JSON from %s: %v", localPath, err)
			file.Close()
			continue
		}
		file.Close()
		allMetrics = append(allMetrics, metrics)
	}

	if len(allMetrics) == 0 {
		log.Println("[CALC-ENGINE] No metric files could be successfully parsed. Aborting cycle.")
		return
	}

	// 6. Aggregate and average the data.
	log.Printf("[CALC-ENGINE] Averaging data from %d files.", len(allMetrics))
	newAverages := calculateAverages(allMetrics)

	// 7. Safely update the global metrics variable.
	metricsMutex.Lock()
	latestAveragedMetrics = newAverages
	metricsMutex.Unlock()

	log.Println("[CALC-ENGINE] Metrics update cycle finished successfully.")
}

func calculateAverages(allMetrics [][]Metric) AveragedMetrics {
	// Intermediate struct to hold sums and counts before averaging.
	type aggregator struct {
		Metric
		count int
	}
	aggregates := make(map[string]*aggregator)

	// Sum up all values for each product_id.
	for _, metricFile := range allMetrics {
		for _, metric := range metricFile {
			if _, ok := aggregates[metric.ProductID]; !ok {
				aggregates[metric.ProductID] = &aggregator{}
			}
			agg := aggregates[metric.ProductID]
			agg.ProductID = metric.ProductID
			agg.count++
			agg.InstabuyPriceAverage += metric.InstabuyPriceAverage
			agg.InstasellPriceAverage += metric.InstasellPriceAverage
			agg.NewDemandOfferFrequencyAverage += metric.NewDemandOfferFrequencyAverage
			agg.NewDemandOfferSizeAverage += metric.NewDemandOfferSizeAverage
			agg.PlayerInstabuyTransactionFrequency += metric.PlayerInstabuyTransactionFrequency
			agg.PlayerInstabuyTransactionSizeAverage += metric.PlayerInstabuyTransactionSizeAverage
			agg.NewSupplyOfferFrequencyAverage += metric.NewSupplyOfferFrequencyAverage
			agg.NewSupplyOfferSizeAverage += metric.NewSupplyOfferSizeAverage
			agg.PlayerInstasellTransactionFrequency += metric.PlayerInstasellTransactionFrequency
			agg.PlayerInstasellTransactionSizeAverage += metric.PlayerInstasellTransactionSizeAverage
		}
	}

	// Divide sums by counts to get the final averages.
	finalAverages := make(AveragedMetrics)
	for id, agg := range aggregates {
		c := float64(agg.count)
		if c > 0 {
			finalAverages[id] = Metric{
				ProductID:                             id,
				InstabuyPriceAverage:                  agg.InstabuyPriceAverage / c,
				InstasellPriceAverage:                 agg.InstasellPriceAverage / c,
				NewDemandOfferFrequencyAverage:        agg.NewDemandOfferFrequencyAverage / c,
				NewDemandOfferSizeAverage:             agg.NewDemandOfferSizeAverage / c,
				PlayerInstabuyTransactionFrequency:    agg.PlayerInstabuyTransactionFrequency / c,
				PlayerInstabuyTransactionSizeAverage:  agg.PlayerInstabuyTransactionSizeAverage / c,
				NewSupplyOfferFrequencyAverage:        agg.NewSupplyOfferFrequencyAverage / c,
				NewSupplyOfferSizeAverage:             agg.NewSupplyOfferSizeAverage / c,
				PlayerInstasellTransactionFrequency:   agg.PlayerInstasellTransactionFrequency / c,
				PlayerInstasellTransactionSizeAverage: agg.PlayerInstasellTransactionSizeAverage / c,
			}
		}
	}
	return finalAverages
}
