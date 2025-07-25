package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metric struct and other variables remain unchanged...
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

type AveragedMetrics map[string]Metric

var (
	latestAveragedMetrics AveragedMetrics
	metricsMutex          sync.RWMutex
	isHealthy             bool
)

// The main function is correct and remains unchanged.
func main() {
	log.Println("[CALC-ENGINE] Application starting up...")
	if err := os.MkdirAll("/tmp/metrics", os.ModePerm); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Could not create temp directory: %v", err)
	}
	go startWebServer()
	go func() {
		updateLatestMetrics()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			updateLatestMetrics()
		}
	}()
	select {}
}

// --- MODIFIED Web Server with a better router ---
func startWebServer() {
	// 1. Create a new, non-global ServeMux (router). This is a best practice.
	mux := http.NewServeMux()

	// 2. Register your specific data endpoint first. No trailing slash for an exact match.
	mux.HandleFunc("/latest_metrics", metricsHandler)

	// 3. Register your catch-all health check handler for the root path.
	// This will now handle ANY request that doesn't match the more specific routes above.
	mux.HandleFunc("/", healthCheckHandler)

	log.Println("[CALC-ENGINE] Starting web server on :8080")
	// 4. Start the server with your custom router.
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Web server failed: %v", err)
	}
}

// --- MODIFIED Health Check Handler ---
// This is now a perfect, simple liveness probe. It will always pass.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// If the path isn't the data endpoint, we assume it's a health check or
	// a user browsing to the root. In either case, 200 OK is the correct response.
	// We no longer check if the path is exactly "/".
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK. Metrics are available at /latest_metrics")
}

// The metrics handler is correct and remains unchanged.
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	metricsMutex.RLock()
	defer metricsMutex.RUnlock()
	if !isHealthy {
		http.Error(w, "Metrics not available yet. Please try again in a moment.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(latestAveragedMetrics)
}

// updateLatestMetrics and calculateAverages are correct and remain unchanged.
func updateLatestMetrics() {
	log.Println("[CALC-ENGINE] Starting metrics update cycle...")
	remoteDir := "/remote_metrics"
	localDir := "/tmp/metrics"
	_ = os.RemoveAll(localDir)
	_ = os.MkdirAll(localDir, os.ModePerm)
	log.Println("[CALC-ENGINE] Listing all remote files...")
	cmd := exec.Command("mega-ls", remoteDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[CALC-ENGINE] ERROR: Failed to list remote files: %v\nOutput: %s", err, out)
		return
	}
	allLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var filenames []string
	for _, line := range allLines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "metrics_") && strings.HasSuffix(trimmedLine, ".json") {
			filenames = append(filenames, trimmedLine)
		}
	}
	if len(filenames) == 0 {
		log.Println("[CALC-ENGINE] No valid metric files found in remote directory.")
		return
	}
	sort.Sort(sort.Reverse(sort.StringSlice(filenames)))
	numToDownload := 12
	if len(filenames) < numToDownload {
		numToDownload = len(filenames)
	}
	latestFiles := filenames[:numToDownload]
	log.Printf("[CALC-ENGINE] Identified latest %d files to process.", len(latestFiles))
	var downloadedFiles []string
	for _, filename := range latestFiles {
		remotePath := filepath.Join(remoteDir, filename)
		log.Printf("[CALC-ENGINE] Downloading %s...", remotePath)
		getCmd := exec.Command("mega-get", remotePath, localDir)
		if out, err := getCmd.CombinedOutput(); err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Failed to download %s: %v\nOutput: %s", remotePath, err, out)
			continue
		}
		downloadedFiles = append(downloadedFiles, filepath.Join(localDir, filename))
	}
	if len(downloadedFiles) == 0 {
		log.Println("[CALC-ENGINE] No files were successfully downloaded. Aborting cycle.")
		return
	}
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
	log.Printf("[CALC-ENGINE] Averaging data from %d files.", len(allMetrics))
	newAverages := calculateAverages(allMetrics)
	metricsMutex.Lock()
	latestAveragedMetrics = newAverages
	isHealthy = true
	metricsMutex.Unlock()
	log.Println("[CALC-ENGINE] Metrics update cycle finished successfully.")
}
func calculateAverages(allMetrics [][]Metric) AveragedMetrics {
	type aggregator struct {
		Metric
		count int
	}
	aggregates := make(map[string]*aggregator)
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
