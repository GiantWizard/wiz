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

// AnalysisResult matches the JSON structure from the Rust metrics generator.
type AnalysisResult struct {
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

// AggregatedMetrics holds the running sum of metrics for a single product.
type AggregatedMetrics struct {
	ProductID                                string
	SumInstabuyPriceAverage                  float64
	SumInstasellPriceAverage                 float64
	SumNewDemandOfferFrequencyAverage        float64
	SumNewDemandOfferSizeAverage             float64
	SumPlayerInstabuyTransactionFrequency    float64
	SumPlayerInstabuyTransactionSizeAverage  float64
	SumNewSupplyOfferFrequencyAverage        float64
	SumNewSupplyOfferSizeAverage             float64
	SumPlayerInstasellTransactionFrequency   float64
	SumPlayerInstasellTransactionSizeAverage float64
	FileCount                                int
}

// Globals for cache
var (
	cacheMu    sync.RWMutex
	cachedList []string
)

func main() {
	// Delay initial cache refresh to allow MEGA session-keeper to log in
	go func() {
		time.Sleep(10 * time.Second)
		updateFileCache()
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			updateFileCache()
		}
	}()

	// Register handlers
	http.HandleFunc("/latest_metrics/", latestMetricsHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	log.Printf("Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// runCommand executes a shell command and captures output.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error running %s %v: %v. Output: %s", name, args, err, string(output))
		return string(output), err
	}
	return string(output), nil
}

// updateFileCache refreshes the list of latest 12 metric files.
func updateFileCache() {
	const remoteDir = "/remote_metrics"
	out, err := runCommand("megals", "-q", remoteDir)
	if err != nil {
		log.Printf("Cache update: failed to list %s: %v", remoteDir, err)
		return
	}
	// Parse lines for filenames
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var files []string
	for _, line := range lines {
		// Split fields and find tokens matching our pattern
		for _, token := range strings.Fields(line) {
			if strings.HasPrefix(token, "metrics_") && strings.HasSuffix(token, ".json") {
				files = append(files, token)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	if len(files) > 12 {
		files = files[:12]
	}
	cacheMu.Lock()
	cachedList = files
	cacheMu.Unlock()
	log.Printf("Cache update: %d files cached: %v", len(files), files)
}

// latestMetricsHandler serves the aggregated metrics from the cached files.
func latestMetricsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for /latest_metrics/")

	// MEGA session check
	whoamiOutput, err := runCommand("mega-whoami")
	if err != nil {
		http.Error(w, "Failed to verify MEGA session.", http.StatusInternalServerError)
		return
	}
	log.Printf("MEGA session running as: %s", strings.TrimSpace(whoamiOutput))

	// Fetch filenames from cache
	cacheMu.RLock()
	filesToProcess := append([]string{}, cachedList...)
	cacheMu.RUnlock()
	if len(filesToProcess) == 0 {
		log.Println("No metric files in cache.")
		http.Error(w, "No metric files found.", http.StatusNotFound)
		return
	}
	log.Printf("Processing %d cached files: %v", len(filesToProcess), filesToProcess)

	// Download + aggregate
	aggregator := make(map[string]*AggregatedMetrics)
	tmpDir, err := os.MkdirTemp("", "metrics-aggregator-*")
	if err != nil {
		http.Error(w, "Failed to create temp dir.", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	for _, filename := range filesToProcess {
		remotePath := filepath.Join("/remote_metrics", filename)
		localPath := filepath.Join(tmpDir, filename)

		if _, err := runCommand("megaget", remotePath, localPath); err != nil {
			log.Printf("Warning: Failed to download %s: %v", remotePath, err)
			continue
		}

		data, err := os.ReadFile(localPath)
		if err != nil {
			log.Printf("Warning: Failed to read %s: %v", localPath, err)
			continue
		}

		var results []AnalysisResult
		if err := json.Unmarshal(data, &results); err != nil {
			log.Printf("Warning: Failed to parse JSON %s: %v", filename, err)
			continue
		}
		updateAggregator(aggregator, results)
	}

	if len(aggregator) == 0 {
		http.Error(w, "Could not process any metric files.", http.StatusInternalServerError)
		return
	}

	// Compute averages
	finalResults := calculateAverages(aggregator)

	// Respond
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(finalResults); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
	log.Println("Successfully served /latest_metrics/")
}

func updateAggregator(aggregator map[string]*AggregatedMetrics, results []AnalysisResult) {
	for _, r := range results {
		a, ok := aggregator[r.ProductID]
		if !ok {
			a = &AggregatedMetrics{ProductID: r.ProductID}
			aggregator[r.ProductID] = a
		}
		a.FileCount++
		a.SumInstabuyPriceAverage += r.InstabuyPriceAverage
		a.SumInstasellPriceAverage += r.InstasellPriceAverage
		a.SumNewDemandOfferFrequencyAverage += r.NewDemandOfferFrequencyAverage
		a.SumNewDemandOfferSizeAverage += r.NewDemandOfferSizeAverage
		a.SumPlayerInstabuyTransactionFrequency += r.PlayerInstabuyTransactionFrequency
		a.SumPlayerInstabuyTransactionSizeAverage += r.PlayerInstabuyTransactionSizeAverage
		a.SumNewSupplyOfferFrequencyAverage += r.NewSupplyOfferFrequencyAverage
		a.SumNewSupplyOfferSizeAverage += r.NewSupplyOfferSizeAverage
		a.SumPlayerInstasellTransactionFrequency += r.PlayerInstasellTransactionFrequency
		a.SumPlayerInstasellTransactionSizeAverage += r.PlayerInstasellTransactionSizeAverage
	}
}

func calculateAverages(aggregator map[string]*AggregatedMetrics) []AnalysisResult {
	final := make([]AnalysisResult, 0, len(aggregator))
	for _, a := range aggregator {
		count := float64(a.FileCount)
		if count == 0 {
			continue
		}
		final = append(final, AnalysisResult{
			ProductID:                             a.ProductID,
			InstabuyPriceAverage:                  a.SumInstabuyPriceAverage / count,
			InstasellPriceAverage:                 a.SumInstasellPriceAverage / count,
			NewDemandOfferFrequencyAverage:        a.SumNewDemandOfferFrequencyAverage / count,
			NewDemandOfferSizeAverage:             a.SumNewDemandOfferSizeAverage / count,
			PlayerInstabuyTransactionFrequency:    a.SumPlayerInstabuyTransactionFrequency / count,
			PlayerInstabuyTransactionSizeAverage:  a.SumPlayerInstabuyTransactionSizeAverage / count,
			NewSupplyOfferFrequencyAverage:        a.SumNewSupplyOfferFrequencyAverage / count,
			NewSupplyOfferSizeAverage:             a.SumNewSupplyOfferSizeAverage / count,
			PlayerInstasellTransactionFrequency:   a.SumPlayerInstasellTransactionFrequency / count,
			PlayerInstasellTransactionSizeAverage: a.SumPlayerInstasellTransactionSizeAverage / count,
		})
	}
	return final
}
