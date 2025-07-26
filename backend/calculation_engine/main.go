package main

import (
	"encoding/json"
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

// Metric struct remains the same...
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
)

func main() {
	log.Println("[CALC-ENGINE] Application starting up...")
	if err := os.MkdirAll("/tmp/metrics", os.ModePerm); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Could not create temp directory: %v", err)
	}
	go startWebServer()
	time.Sleep(10 * time.Second)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	updateLatestMetrics()
	for range ticker.C {
		updateLatestMetrics()
	}
}

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
		http.Error(w, "Metrics not available yet.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(latestAveragedMetrics)
}

func updateLatestMetrics() {
	log.Println("[CALC-ENGINE] Starting metrics update cycle...")
	remoteDir := "/remote_metrics"
	localDir := "/tmp/metrics"

	_ = os.RemoveAll(localDir)
	_ = os.MkdirAll(localDir, os.ModePerm)

	// --- MODIFIED LOGIC ---
	// Revert to using 'mega-ls' as 'mega-find' flags are not supported.
	log.Println("[CALC-ENGINE] Listing all remote files...")
	cmd := exec.Command("mega-ls", remoteDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[CALC-ENGINE] ERROR: Failed to list remote files: %v\nOutput: %s", err, out)
		return
	}

	// The Go application will now handle sorting and limiting.
	filenames := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(filenames) == 0 || (len(filenames) == 1 && filenames[0] == "") {
		log.Println("[CALC-ENGINE] No metric files found in remote directory.")
		return
	}
	// Sort descending to get latest filenames first (e.g., "metrics_2025...").
	sort.Sort(sort.Reverse(sort.StringSlice(filenames)))

	numToDownload := 12
	if len(filenames) < numToDownload {
		numToDownload = len(filenames)
	}
	latestFiles := filenames[:numToDownload]
	log.Printf("[CALC-ENGINE] Identified latest %d files to process.", len(latestFiles))
	// --- END OF MODIFIED LOGIC ---

	var downloadedFiles []string
	for _, filename := range latestFiles {
		if filename == "" {
			continue
		}
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
	metricsMutex.Unlock()

	log.Println("[CALC-ENGINE] Metrics update cycle finished successfully.")
}

// calculateAverages function remains the same...
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
