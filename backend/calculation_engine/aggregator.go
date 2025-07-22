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

// AggregatedMetrics holds the running sum of metrics for a single product across multiple files.
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

// runCommand executes a shell command and returns its output, logging any errors.
func runCommand(name string, args ...string) (string, error) {
	// Prepend "env HOME=/home/appuser" to ensure mega-cmd works correctly under the appuser context.
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error running command '%s %s': %v. Output: %s", name, strings.Join(args, " "), err, string(output))
		return string(output), err
	}
	log.Printf("Successfully ran command '%s %s'.", name, strings.Join(args, " "))
	return string(output), nil
}

// latestMetricsHandler is the HTTP handler for the /latest_metrics/ endpoint.
func latestMetricsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for /latest_metrics/")

	// 1. Check MEGA login status
	// mega-whoami returns a non-zero exit code if not logged in.
	if _, err := runCommand("mega-whoami"); err != nil {
		http.Error(w, "Failed to verify MEGA session. Is the session-keeper running and logged in?", http.StatusInternalServerError)
		return
	}

	// 2. List remote files
	remoteDir := "/remote_metrics"
	output, err := runCommand("megals", remoteDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list files in MEGA directory '%s'", remoteDir), http.StatusInternalServerError)
		return
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var filenames []string
	for _, line := range lines {
		// A simple filter to get only the expected metric files
		if strings.HasPrefix(line, "metrics_") && strings.HasSuffix(line, ".json") {
			filenames = append(filenames, line)
		}
	}

	if len(filenames) == 0 {
		http.Error(w, "No metric files found in MEGA directory.", http.StatusNotFound)
		return
	}

	// 3. Sort files to find the most recent ones
	// The timestamp format YYYYMMDDHHMMSS allows for simple string sorting.
	sort.Sort(sort.Reverse(sort.StringSlice(filenames)))

	// 4. Select the latest 12 files (or fewer if not available)
	numFilesToProcess := 12
	if len(filenames) < numFilesToProcess {
		numFilesToProcess = len(filenames)
	}
	filesToProcess := filenames[:numFilesToProcess]
	log.Printf("Found %d total metric files. Will process the latest %d.", len(filenames), numFilesToProcess)

	// 5. Create a temporary directory for downloaded files
	tmpDir, err := os.MkdirTemp("", "metrics-aggregator-*")
	if err != nil {
		http.Error(w, "Failed to create temporary directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)
	log.Printf("Created temporary directory: %s", tmpDir)

	// 6. Download, parse, and aggregate data
	aggregator := make(map[string]*AggregatedMetrics)

	for _, filename := range filesToProcess {
		remotePath := filepath.Join(remoteDir, filename)
		localPath := filepath.Join(tmpDir, filename)

		log.Printf("Downloading %s to %s", remotePath, localPath)
		if _, err := runCommand("megaget", remotePath, localPath); err != nil {
			// Log the error but continue to the next file if possible
			log.Printf("Warning: Failed to download %s: %v", remotePath, err)
			continue
		}

		fileData, err := os.ReadFile(localPath)
		if err != nil {
			log.Printf("Warning: Failed to read downloaded file %s: %v", localPath, err)
			continue
		}

		var results []AnalysisResult
		if err := json.Unmarshal(fileData, &results); err != nil {
			log.Printf("Warning: Failed to parse JSON from %s: %v", filename, err)
			continue
		}

		updateAggregator(aggregator, results)
	}

	if len(aggregator) == 0 {
		http.Error(w, "Could not process any metric files successfully.", http.StatusInternalServerError)
		return
	}

	// 7. Calculate the final averages
	finalAverages := calculateAverages(aggregator)

	// 8. Respond with the final JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(finalAverages); err != nil {
		log.Printf("Failed to encode final response: %v", err)
	}
	log.Println("Successfully served /latest_metrics/ request.")
}

// updateAggregator processes a list of results from one file and adds them to the aggregator map.
func updateAggregator(aggregator map[string]*AggregatedMetrics, results []AnalysisResult) {
	for _, r := range results {
		if _, ok := aggregator[r.ProductID]; !ok {
			aggregator[r.ProductID] = &AggregatedMetrics{ProductID: r.ProductID}
		}

		a := aggregator[r.ProductID]
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

// calculateAverages converts the aggregated sums into final averaged results.
func calculateAverages(aggregator map[string]*AggregatedMetrics) []AnalysisResult {
	finalAverages := make([]AnalysisResult, 0, len(aggregator))
	for _, a := range aggregator {
		count := float64(a.FileCount)
		if count == 0 {
			continue // Should not happen if updateAggregator is used correctly
		}
		avgResult := AnalysisResult{
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
		}
		finalAverages = append(finalAverages, avgResult)
	}
	return finalAverages
}
