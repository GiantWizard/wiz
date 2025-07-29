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

// Metric represents the schema for each metric entry.
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
	isHealthy             bool // flag for readiness
)

func main() {
	log.Println("[CALC-ENGINE] Application starting up...")

	// Ensure temp directory exists
	if err := os.MkdirAll("/tmp/metrics", os.ModePerm); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Could not create temp directory: %v", err)
	}

	// Start the HTTP server (liveness & readiness probes)
	go startWebServer()

	// Kick off metrics update immediately, then every 5 minutes
	go func() {
		updateLatestMetrics()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			updateLatestMetrics()
		}
	}()

	// Block forever
	select {}
}

func startWebServer() {
	// Liveness probe: always returns 200 on /healthz
	http.HandleFunc("/healthz", healthCheckHandler)

	// Readiness/data probe: returns metrics once ready
	http.HandleFunc("/latest_metrics/", metricsHandler)

	// Bind to port from environment (e.g., Koyeb sets PORT), default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("[CALC-ENGINE] Starting web server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[CALC-ENGINE] FATAL: Web server failed: %v", err)
	}
}

// healthCheckHandler is the liveness endpoint.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

// metricsHandler is the readiness endpoint, serving JSON once data is loaded.
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

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var filenames []string
	for _, line := range lines {
		if strings.HasPrefix(line, "metrics_") && strings.HasSuffix(line, ".json") {
			filenames = append(filenames, strings.TrimSpace(line))
		}
	}
	if len(filenames) == 0 {
		log.Println("[CALC-ENGINE] No valid metric files found.")
		return
	}

	// Sort newest first and pick up to 12
	sort.Sort(sort.Reverse(sort.StringSlice(filenames)))
	n := 24
	if len(filenames) < n {
		n = len(filenames)
	}
	latest := filenames[:n]
	log.Printf("[CALC-ENGINE] Identified latest %d files to process.", len(latest))

	var allMetrics [][]Metric
	for _, fn := range latest {
		path := filepath.Join(remoteDir, fn)
		log.Printf("[CALC-ENGINE] Downloading %s...", path)
		get := exec.Command("mega-get", path, localDir)
		if out, err := get.CombinedOutput(); err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Failed to download %s: %v\nOutput: %s", path, err, out)
			continue
		}
		localPath := filepath.Join(localDir, fn)
		f, err := os.Open(localPath)
		if err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Could not open %s: %v", localPath, err)
			continue
		}
		var metrics []Metric
		if err := json.NewDecoder(f).Decode(&metrics); err != nil {
			log.Printf("[CALC-ENGINE] ERROR: Failed to parse JSON %s: %v", localPath, err)
			f.Close()
			continue
		}
		f.Close()
		allMetrics = append(allMetrics, metrics)
	}

	if len(allMetrics) == 0 {
		log.Println("[CALC-ENGINE] No metrics parsed. Aborting cycle.")
		return
	}

	log.Printf("[CALC-ENGINE] Averaging data from %d files.", len(allMetrics))
	newAvg := calculateAverages(allMetrics)

	metricsMutex.Lock()
	latestAveragedMetrics = newAvg
	isHealthy = true
	metricsMutex.Unlock()

	log.Println("[CALC-ENGINE] Metrics update cycle finished successfully.")
}

// calculateAverages computes the average of each metric over all files.
func calculateAverages(allMetrics [][]Metric) AveragedMetrics {
	type agg struct {
		Metric
		count int
	}
	m := make(map[string]*agg)
	for _, fileMetrics := range allMetrics {
		for _, met := range fileMetrics {
			if _, ok := m[met.ProductID]; !ok {
				m[met.ProductID] = &agg{}
			}
			a := m[met.ProductID]
			a.ProductID = met.ProductID
			a.count++
			a.InstabuyPriceAverage += met.InstabuyPriceAverage
			a.InstasellPriceAverage += met.InstasellPriceAverage
			a.NewDemandOfferFrequencyAverage += met.NewDemandOfferFrequencyAverage
			a.NewDemandOfferSizeAverage += met.NewDemandOfferSizeAverage
			a.PlayerInstabuyTransactionFrequency += met.PlayerInstabuyTransactionFrequency
			a.PlayerInstabuyTransactionSizeAverage += met.PlayerInstabuyTransactionSizeAverage
			a.NewSupplyOfferFrequencyAverage += met.NewSupplyOfferFrequencyAverage
			a.NewSupplyOfferSizeAverage += met.NewSupplyOfferSizeAverage
			a.PlayerInstasellTransactionFrequency += met.PlayerInstasellTransactionFrequency
			a.PlayerInstasellTransactionSizeAverage += met.PlayerInstasellTransactionSizeAverage
		}
	}
	final := make(AveragedMetrics)
	for id, a := range m {
		c := float64(a.count)
		if c > 0 {
			final[id] = Metric{
				ProductID:                             id,
				InstabuyPriceAverage:                  a.InstabuyPriceAverage / c,
				InstasellPriceAverage:                 a.InstasellPriceAverage / c,
				NewDemandOfferFrequencyAverage:        a.NewDemandOfferFrequencyAverage / c,
				NewDemandOfferSizeAverage:             a.NewDemandOfferSizeAverage / c,
				PlayerInstabuyTransactionFrequency:    a.PlayerInstabuyTransactionFrequency / c,
				PlayerInstabuyTransactionSizeAverage:  a.PlayerInstabuyTransactionSizeAverage / c,
				NewSupplyOfferFrequencyAverage:        a.NewSupplyOfferFrequencyAverage / c,
				NewSupplyOfferSizeAverage:             a.NewSupplyOfferSizeAverage / c,
				PlayerInstasellTransactionFrequency:   a.PlayerInstasellTransactionFrequency / c,
				PlayerInstasellTransactionSizeAverage: a.PlayerInstasellTransactionSizeAverage / c,
			}
		}
	}
	return final
}
