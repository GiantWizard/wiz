package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	_ "net/http/pprof" // For profiling
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- Custom JSONFloat64 Type ---
type JSONFloat64 float64

func (f JSONFloat64) MarshalJSON() ([]byte, error) {
	val := float64(f)
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return []byte("null"), nil
	}
	return json.Marshal(val)
}

func toJSONFloat64(v float64) JSONFloat64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return JSONFloat64(math.NaN())
	}
	return JSONFloat64(v)
}

// --- Constants ---
const (
	metricsFilename             = "latest_metrics.json"
	itemFilesDir                = "dependencies/items"
	megaCmdTimeout              = 90 * time.Second
	initialMetricsDownloadDelay = 15 * time.Second
	initialOptimizationDelay    = 30 * time.Second
	timestampFormat             = "20060102150405"
	megaLsCmd                   = "mega-ls"
	megaGetCmd                  = "mega-get"
	megaCmdFallbackDir          = "/usr/local/bin/"
)

// --- Struct Definitions for main.go orchestration ---
// These types (OptimizationRunOutput, OptimizationSummary, FailedItemDetail)
// are specific to main.go's orchestration and reporting.
// Types like ProductMetrics, OptimizedItemResult, HypixelAPIResponse
// should be defined in their respective files (metrics.go, optimizer.go, api.go).
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"` // OptimizedItemResult from optimizer.go
}

type OptimizationSummary struct {
	RunTimestamp                string      `json:"run_timestamp"`
	APILastUpdatedTimestamp     string      `json:"api_last_updated_timestamp,omitempty"`
	TotalItemsConsidered        int         `json:"total_items_considered"`
	ItemsSuccessfullyCalculated int         `json:"items_successfully_calculated"`
	ItemsWithCalculationErrors  int         `json:"items_with_calculation_errors"`
	MaxAllowedCycleTimeSecs     JSONFloat64 `json:"max_allowed_cycle_time_seconds"`
	MaxInitialSearchQuantity    JSONFloat64 `json:"max_initial_search_quantity"`
}

type FailedItemDetail struct {
	ItemName     string `json:"item_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// --- Global Variables for main.go orchestration ---
var (
	latestOptimizerResultsJSON []byte
	optimizerResultsMutex      sync.RWMutex
	latestFailedItemsJSON      []byte
	failedItemsMutex           sync.RWMutex
	lastOptimizationStatus     string
	lastOptimizationTime       time.Time
	optimizationStatusMutex    sync.RWMutex
	latestMetricsData          []byte
	metricsDataMutex           sync.RWMutex
	lastMetricsDownloadStatus  string
	lastMetricsDownloadTime    time.Time
	metricsDownloadStatusMutex sync.RWMutex
	isOptimizing               bool
	isOptimizingMutex          sync.Mutex
	metricsFileRegex           = regexp.MustCompile(`^metrics_(\d{14})\.json$`)
)

// downloadMetricsFromMega connects to MEGA, finds the newest metrics_<timestamp>.json
// under any recognized “remote_metrics” path, downloads it, and writes it to localTargetFilename.
// A much simpler and safer version of the download function.
// It assumes a session is already managed externally (by the session-keeper).
func downloadMetricsFromMega(localTargetFilename string) error {
	homeEnv := os.Getenv("HOME")
	if homeEnv == "" {
		homeEnv = "/home/appuser"
	}

	runCmd := func(name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Env = append(os.Environ(), "HOME="+homeEnv)
		outBytes, err := cmd.CombinedOutput()
		return string(outBytes), err
	}

	// 1. List files in the remote directory.
	const remoteDir = "/remote_metrics"
	lsOutput, lsErr := runCmd("mega-ls", remoteDir)
	if lsErr != nil {
		return fmt.Errorf("mega-ls %q failed: %v\nOutput:\n%s", remoteDir, lsErr, lsOutput)
	}

	// 2. Parse the listing to find the newest metrics file.
	var latestFilename string
	scanner := bufio.NewScanner(strings.NewReader(lsOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "metrics_") && strings.HasSuffix(line, ".json") {
			if line > latestFilename { // Simple string comparison works for this format
				latestFilename = line
			}
		}
	}
	if latestFilename == "" {
		return fmt.Errorf("no metrics file found in %q; listing:\n%s", remoteDir, lsOutput)
	}

	// 3. Download the latest file.
	remoteFile := filepath.Join(remoteDir, latestFilename)
	targetDir := filepath.Dir(localTargetFilename)

	log.Printf("DEBUG: downloading %q to target dir: %s", remoteFile, targetDir)
	getOut, getErr := runCmd("megaget", remoteFile, "--path", targetDir)
	if getErr != nil {
		return fmt.Errorf("megaget %q failed: %v\nOutput:\n%s", remoteFile, getErr, getOut)
	}

	// 4. Rename the downloaded file to its final temporary name.
	downloadedPath := filepath.Join(targetDir, latestFilename)
	if err := os.Rename(downloadedPath, localTargetFilename); err != nil {
		os.Remove(downloadedPath) // Cleanup
		return fmt.Errorf("failed to rename %q to %q: %v", downloadedPath, localTargetFilename, err)
	}

	log.Printf("Successfully downloaded %q and prepared as %q", remoteFile, localTargetFilename)
	return nil
}

// Here is a typical wrapper showing how you might call it from your ticker/goroutine.
func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download...")
	// Use a predictable temporary name for the downloaded file before validation/processing
	tempMetricsFilename := filepath.Join(os.TempDir(), fmt.Sprintf("metrics_download_%d.tmp.json", time.Now().UnixNano()))
	defer os.Remove(tempMetricsFilename) // Clean up the temp file

	var statusMessage string // To hold the final status message for this attempt
	currentTime := time.Now()

	err := downloadMetricsFromMega(tempMetricsFilename)
	if err != nil {
		statusMessage = fmt.Sprintf("Error during MEGA download at %s: %v",
			currentTime.Format(time.RFC3339), err)
		log.Println(statusMessage)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = statusMessage
		lastMetricsDownloadTime = currentTime
		metricsDownloadStatusMutex.Unlock()
		return
	}

	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		statusMessage = fmt.Sprintf("Error reading downloaded file %q at %s: %v",
			tempMetricsFilename, currentTime.Format(time.RFC3339), readErr)
		log.Println(statusMessage)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = statusMessage
		lastMetricsDownloadTime = currentTime
		metricsDownloadStatusMutex.Unlock()
		return
	}

	// Check for empty or invalid JSON (e.g. just "{}")
	if len(data) == 0 {
		statusMessage = fmt.Sprintf("Downloaded metrics file %s was empty at %s",
			tempMetricsFilename, currentTime.Format(time.RFC3339))
		log.Println(statusMessage)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = statusMessage
		lastMetricsDownloadTime = currentTime
		metricsDownloadStatusMutex.Unlock()
		return
	}
	if string(data) == "{}" { // An empty object is not a valid array of metrics
		statusMessage = fmt.Sprintf("Downloaded metrics file %s was '{}', which is not a valid metrics array, at %s",
			tempMetricsFilename, currentTime.Format(time.RFC3339))
		log.Println(statusMessage)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = statusMessage
		lastMetricsDownloadTime = currentTime
		metricsDownloadStatusMutex.Unlock()
		return
	}

	// Validate that the data is a valid JSON array of ProductMetrics
	var tempMetricsSlice []ProductMetrics // ProductMetrics from metrics.go
	if jsonErr := json.Unmarshal(data, &tempMetricsSlice); jsonErr != nil {
		sample := string(data)
		if len(sample) > 200 {
			sample = sample[:200] + "…" // Truncate for logging
		}
		statusMessage = fmt.Sprintf(
			"Downloaded file %s is not a valid JSON array of ProductMetrics at %s: %v; sample: %q",
			tempMetricsFilename, currentTime.Format(time.RFC3339), jsonErr, sample,
		)
		log.Println(statusMessage)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = statusMessage
		lastMetricsDownloadTime = currentTime
		metricsDownloadStatusMutex.Unlock()
		return
	}
	// At this point, 'data' contains valid JSON that unmarshaled into []ProductMetrics
	// (even if tempMetricsSlice is empty, if 'data' was "[]")

	// Update the global in-memory metrics data
	metricsDataMutex.Lock()
	latestMetricsData = make([]byte, len(data)) // Create a new slice
	copy(latestMetricsData, data)               // Copy data to the new slice
	metricsDataMutex.Unlock()
	log.Printf("Successfully updated in-memory latestMetricsData (%d bytes)", len(latestMetricsData))

	// Overwrite your permanent cache file (e.g., latest_metrics.json)
	permanentPath := metricsFilename // Use the constant
	if writeErr := os.WriteFile(permanentPath, data, 0644); writeErr != nil {
		log.Printf("Warning: could not write updated metrics to %q: %v (in-memory data is OK)", permanentPath, writeErr)
		statusMessage = fmt.Sprintf("Successfully updated in-memory metrics (%d bytes) at %s, but failed to write to %s: %v",
			len(data), currentTime.Format(time.RFC3339), permanentPath, writeErr)
	} else {
		log.Printf("Successfully updated metrics file %q (%d bytes)", permanentPath, len(data))
		statusMessage = fmt.Sprintf("Successfully downloaded and stored metrics (in-memory and to %s; %d bytes) at %s",
			permanentPath, len(data), currentTime.Format(time.RFC3339))
	}

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = statusMessage
	lastMetricsDownloadTime = currentTime
	metricsDownloadStatusMutex.Unlock()
	log.Println(statusMessage) // Log the final status of this operation
}

func downloadMetricsPeriodically() {
	go func() {
		log.Printf("downloadMetricsPeriodically: Waiting %v before initial download...", initialMetricsDownloadDelay)
		time.Sleep(initialMetricsDownloadDelay)

		downloadAndStoreMetrics() // Initial download

		ticker := time.NewTicker(1 * time.Hour) // Period for subsequent downloads
		defer ticker.Stop()

		for range ticker.C {
			log.Println("downloadMetricsPeriodically: Triggering periodic metrics download.")
			downloadAndStoreMetrics()
		}
	}()
}

func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) {
	if len(jsonData) == 0 {
		log.Println("parseProductMetricsData: jsonData is empty, returning empty map.")
		return make(map[string]ProductMetrics), nil
	}
	// Check for "{}", which is an invalid format for an array of ProductMetrics
	if string(jsonData) == "{}" {
		log.Println("parseProductMetricsData: jsonData is '{}', which cannot be unmarshaled into []ProductMetrics.")
		return nil, fmt.Errorf("cannot unmarshal JSON object '{}' into []ProductMetrics")
	}

	var productMetricsSlice []ProductMetrics // ProductMetrics from metrics.go
	if err := json.Unmarshal(jsonData, &productMetricsSlice); err != nil {
		sample := string(jsonData)
		if len(sample) > 200 {
			sample = sample[:200] + "... (truncated)"
		}
		log.Printf("parseProductMetricsData: Failed to unmarshal metrics JSON. Error: %v. Sample: %s", err, sample)
		return nil, fmt.Errorf("failed to unmarshal metrics JSON: %w. Sample: %s", err, sample)
	}

	// If jsonData was "[]", productMetricsSlice will be non-nil but empty (len 0). This is valid.
	if len(productMetricsSlice) == 0 && string(jsonData) != "[]" {
		// This case indicates that valid JSON was parsed but resulted in an empty slice,
		// which might be unexpected if the input wasn't explicitly an empty array "[]".
		// However, it's not an unmarshaling error itself.
		log.Printf("parseProductMetricsData: jsonData successfully unmarshaled into an empty slice of ProductMetrics. Original data length: %d. Input sample: %s", len(jsonData), string(jsonData[:min(len(jsonData), 50)]))
	}

	productMetricsMap := make(map[string]ProductMetrics, len(productMetricsSlice))
	skippedCount := 0
	for _, metric := range productMetricsSlice {
		if metric.ProductID == "" { // Assuming ProductID is a field in your ProductMetrics struct
			skippedCount++
			continue
		}
		normalizedID := BAZAAR_ID(metric.ProductID) // BAZAAR_ID from utils.go
		metric.ProductID = normalizedID             // Store normalized ID back if needed, or use map key
		productMetricsMap[normalizedID] = metric
	}
	if skippedCount > 0 {
		log.Printf("parseProductMetricsData: Skipped %d metric entries due to empty ProductID.", skippedCount)
	}
	log.Printf("parseProductMetricsData: Successfully parsed %d metrics objects into %d map entries.", len(productMetricsSlice), len(productMetricsMap))
	return productMetricsMap, nil
}

func performOptimizationCycleNow(productMetrics map[string]ProductMetrics, apiResp *HypixelAPIResponse) ([]byte, []byte, error) { // HypixelAPIResponse from api.go
	runStartTime := time.Now()
	log.Println("performOptimizationCycleNow: Starting new optimization cycle...")

	if apiResp == nil || apiResp.Products == nil { // Products field from HypixelAPIResponse struct
		return nil, nil, fmt.Errorf("CRITICAL: API data is nil or has no products in performOptimizationCycleNow")
	}
	if productMetrics == nil {
		return nil, nil, fmt.Errorf("CRITICAL: Product metrics map is nil in performOptimizationCycleNow")
	}
	if len(productMetrics) == 0 && len(apiResp.Products) > 0 {
		log.Println("performOptimizationCycleNow: Product metrics map is empty, but API has products. This may lead to limited optimization.")
	}

	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 { // LastUpdated field from HypixelAPIResponse struct
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).UTC().Format(time.RFC3339Nano)
	}

	var itemIDs []string
	for id := range apiResp.Products {
		itemIDs = append(itemIDs, id)
	}

	if len(itemIDs) == 0 {
		log.Println("performOptimizationCycleNow: No item IDs from API response to optimize.")
		emptySummary := OptimizationSummary{
			RunTimestamp:                runStartTime.Format(time.RFC3339Nano),
			APILastUpdatedTimestamp:     apiLastUpdatedStr,
			TotalItemsConsidered:        0,
			ItemsSuccessfullyCalculated: 0,
			ItemsWithCalculationErrors:  0,
			MaxAllowedCycleTimeSecs:     toJSONFloat64(0), // Or some default from config
			MaxInitialSearchQuantity:    toJSONFloat64(0), // Or some default from config
		}
		emptyOutput := OptimizationRunOutput{Summary: emptySummary, Results: []OptimizedItemResult{}} // OptimizedItemResult from optimizer.go
		mainJSON, _ := json.MarshalIndent(emptyOutput, "", "  ")
		return mainJSON, []byte("[]"), nil // Empty JSON array for failed items
	}

	itemsPerChunk := 50 // Default
	if ipcStr := os.Getenv("ITEMS_PER_CHUNK"); ipcStr != "" {
		if val, err := strconv.Atoi(ipcStr); err == nil && val > 0 {
			itemsPerChunk = val
		}
	}
	pauseBetweenChunks := 500 * time.Millisecond // Default
	if pbcStr := os.Getenv("PAUSE_MS_BETWEEN_CHUNKS"); pbcStr != "" {
		if val, err := strconv.Atoi(pbcStr); err == nil && val >= 0 { // Allow 0 pause
			pauseBetweenChunks = time.Duration(val) * time.Millisecond
		}
	}

	// These could also be configurable via env vars
	const (
		maxAllowedCycleTimePerItemRaw = 3600.0
		maxInitialSearchQtyRaw        = 1000000.0
	)

	log.Printf("performOptimizationCycleNow: Parameters - MaxCycleTime: %.0fs, MaxInitialSearchQty: %.0f, ChunkSize: %d, Pause: %v",
		maxAllowedCycleTimePerItemRaw, maxInitialSearchQtyRaw, itemsPerChunk, pauseBetweenChunks)

	var allOptimizedResults []OptimizedItemResult // OptimizedItemResult from optimizer.go
	for i := 0; i < len(itemIDs); i += itemsPerChunk {
		end := i + itemsPerChunk
		if end > len(itemIDs) {
			end = len(itemIDs)
		}
		currentChunkItemIDs := itemIDs[i:end]
		if len(currentChunkItemIDs) == 0 {
			continue // Should not happen if loop condition is i < len(itemIDs)
		}

		log.Printf("performOptimizationCycleNow: Optimizing chunk %d/%d (items %d to %d of %d total)",
			(i/itemsPerChunk)+1, (len(itemIDs)+itemsPerChunk-1)/itemsPerChunk, i, end-1, len(itemIDs))

		// RunFullOptimization from optimizer.go
		chunkResults := RunFullOptimization(currentChunkItemIDs, maxAllowedCycleTimePerItemRaw, apiResp, productMetrics, itemFilesDir, maxInitialSearchQtyRaw)
		allOptimizedResults = append(allOptimizedResults, chunkResults...)

		if end < len(itemIDs) && pauseBetweenChunks > 0 {
			log.Printf("Pausing for %v before next chunk...", pauseBetweenChunks)
			time.Sleep(pauseBetweenChunks)
		}
	}

	optimizedResults := allOptimizedResults
	log.Printf("performOptimizationCycleNow: Optimization complete for all chunks. Generated %d total results.", len(optimizedResults))

	var successCount int
	var failDetails []FailedItemDetail
	for _, r := range optimizedResults {
		// Assuming OptimizedItemResult has these fields
		if r.CalculationPossible {
			successCount++
		} else {
			failDetails = append(failDetails, FailedItemDetail{ItemName: r.ItemName, ErrorMessage: r.ErrorMessage})
		}
	}

	summary := OptimizationSummary{
		RunTimestamp:                runStartTime.Format(time.RFC3339Nano),
		APILastUpdatedTimestamp:     apiLastUpdatedStr,
		TotalItemsConsidered:        len(itemIDs),
		ItemsSuccessfullyCalculated: successCount,
		ItemsWithCalculationErrors:  len(failDetails),
		MaxAllowedCycleTimeSecs:     toJSONFloat64(maxAllowedCycleTimePerItemRaw),
		MaxInitialSearchQuantity:    toJSONFloat64(maxInitialSearchQtyRaw),
	}
	mainOutput := OptimizationRunOutput{Summary: summary, Results: optimizedResults}

	mainJSON, err := json.MarshalIndent(mainOutput, "", "  ")
	if err != nil {
		log.Printf("CRITICAL: Failed to marshal main optimization output: %v.", err)
		// Consider returning a placeholder error JSON if mainJSON is nil
		return nil, nil, fmt.Errorf("CRITICAL: Failed to marshal main optimization output: %w", err)
	}

	var failedJSON []byte
	if len(failDetails) > 0 {
		if b, errMarshal := json.MarshalIndent(failDetails, "", "  "); errMarshal != nil {
			log.Printf("Error: Failed to marshal failed items report: %v", errMarshal)
			failedJSON = []byte(`[{"error":"failed to marshal failed items report"}]`) // Placeholder
		} else {
			failedJSON = b
		}
	} else {
		failedJSON = []byte("[]") // Empty JSON array for no failures
	}

	return mainJSON, failedJSON, nil
}

func runSingleOptimizationAndUpdateResults() {
	log.Println("runSingleOptimizationAndUpdateResults: Initiating new optimization process...")

	metricsDataMutex.RLock()
	currentMetricsBytes := make([]byte, len(latestMetricsData))
	copy(currentMetricsBytes, latestMetricsData) // Make a copy to use outside the lock
	metricsDataMutex.RUnlock()

	if len(currentMetricsBytes) == 0 || string(currentMetricsBytes) == "[]" { // Check for empty array too
		newStatus := fmt.Sprintf("Optimization skipped at %s: Metrics data is not ready or is empty/blank_array.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		// lastOptimizationTime is not updated here as no attempt was made with data
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if string(currentMetricsBytes) == "{}" {
		newStatus := fmt.Sprintf("Optimization skipped at %s: Metrics data was '{}', which is invalid for parsing as an array.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	productMetrics, parseErr := parseProductMetricsData(currentMetricsBytes)
	if parseErr != nil {
		newStatus := fmt.Sprintf("Optimization skipped at %s: Metrics parsing failed: %v", time.Now().Format(time.RFC3339), parseErr)
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if len(productMetrics) == 0 {
		// This can happen if currentMetricsBytes was "[]" or all items had empty ProductID
		newStatus := fmt.Sprintf("Optimization skipped at %s: Parsed metrics map is empty (no valid ProductMetrics found for optimization).", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	apiResp, apiErr := getApiResponse() // getApiResponse from api.go
	if apiErr != nil {
		newStatus := fmt.Sprintf("Optimization skipped at %s: API data load failed: %v", time.Now().Format(time.RFC3339), apiErr)
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if apiResp == nil || len(apiResp.Products) == 0 { // Check Products map directly
		newStatus := fmt.Sprintf("Optimization skipped at %s: API response is nil or contains no products.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	mainJSON, failedJSON, optErr := performOptimizationCycleNow(productMetrics, apiResp)
	mainLen, failLen := 0, 0
	if mainJSON != nil {
		mainLen = len(mainJSON)
	}
	if failedJSON != nil {
		failLen = len(failedJSON)
	}

	currentOptTime := time.Now()
	var currentOptStatus string

	if optErr != nil {
		currentOptStatus = fmt.Sprintf("Optimization Error at %s: %v", currentOptTime.Format(time.RFC3339), optErr)
		// Still update results if they were partially generated or contain error info
		if mainJSON != nil {
			optimizerResultsMutex.Lock()
			latestOptimizerResultsJSON = mainJSON
			optimizerResultsMutex.Unlock()
		}
		if failedJSON != nil {
			failedItemsMutex.Lock()
			latestFailedItemsJSON = failedJSON
			failedItemsMutex.Unlock()
		}
	} else {
		optimizerResultsMutex.Lock()
		latestOptimizerResultsJSON = mainJSON
		optimizerResultsMutex.Unlock()

		failedItemsMutex.Lock()
		latestFailedItemsJSON = failedJSON
		failedItemsMutex.Unlock()
		currentOptStatus = fmt.Sprintf("Successfully optimized at %s. Results: %d bytes, Failed Items Report: %d bytes",
			currentOptTime.Format(time.RFC3339), mainLen, failLen)
	}

	optimizationStatusMutex.Lock()
	lastOptimizationTime = currentOptTime
	lastOptimizationStatus = currentOptStatus
	optimizationStatusMutex.Unlock()

	log.Printf("runSingleOptimizationAndUpdateResults: Cycle finished. Error: %v. Main JSON size: %d bytes, Failed JSON size: %d bytes. Status: %s",
		optErr, mainLen, failLen, currentOptStatus)
}

func optimizePeriodically() {
	// Initial optimization run after a delay
	go func() {
		log.Printf("optimizePeriodically: Waiting %v before initial optimization...", initialOptimizationDelay)
		time.Sleep(initialOptimizationDelay)

		isOptimizingMutex.Lock()
		if isOptimizing {
			isOptimizingMutex.Unlock()
			log.Println("optimizePeriodically (initial): Optimization already in progress, skipping.")
			return
		}
		isOptimizing = true
		isOptimizingMutex.Unlock()

		log.Println("optimizePeriodically (initial): Starting initial optimization run.")
		runSingleOptimizationAndUpdateResults()

		isOptimizingMutex.Lock()
		isOptimizing = false
		isOptimizingMutex.Unlock()
		log.Println("optimizePeriodically (initial): Initial optimization run finished.")
	}()

	// Periodic optimization runs
	ticker := time.NewTicker(20 * time.Minute)
	defer ticker.Stop()

	for t := range ticker.C {
		canStart := false
		isOptimizingMutex.Lock()
		if !isOptimizing {
			isOptimizing = true
			canStart = true
		}
		isOptimizingMutex.Unlock()

		if canStart {
			log.Printf("optimizePeriodically (tick %s): Scheduling optimization work.", t.Format(time.RFC3339))
			go func(tickTime time.Time) { // Pass tickTime to goroutine for accurate logging
				defer func() {
					isOptimizingMutex.Lock()
					isOptimizing = false
					isOptimizingMutex.Unlock()
					log.Printf("optimizePeriodically (goroutine for tick %s): Optimization work finished.", tickTime.Format(time.RFC3339))
				}()
				log.Printf("optimizePeriodically (goroutine for tick %s): Starting optimization work.", tickTime.Format(time.RFC3339))
				runSingleOptimizationAndUpdateResults()
			}(t) // Pass current tick time 't'
		} else {
			log.Println("optimizePeriodically (tick): Previous optimization still in progress, skipping this tick.")
		}
	}
}

func optimizerResultsHandler(w http.ResponseWriter, r *http.Request) {
	optimizerResultsMutex.RLock()
	data := latestOptimizerResultsJSON
	optimizerResultsMutex.RUnlock()

	w.Header().Set("Access-Control-Allow-Origin", "*") // Consider more restrictive CORS if needed
	w.Header().Set("Content-Type", "application/json")
	if data == nil { // Should be initialized in main, but good check
		http.Error(w, `{"summary":{"message":"Optimizer results not yet available or error in generation"},"results":[]}`, http.StatusServiceUnavailable)
		return
	}
	w.Write(data)
}

func failedItemsReportHandler(w http.ResponseWriter, r *http.Request) {
	failedItemsMutex.RLock()
	data := latestFailedItemsJSON
	failedItemsMutex.RUnlock()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if data == nil { // Should be initialized in main
		w.Write([]byte("[]")) // Return empty array if nil
		return
	}
	w.Write(data)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	optimizationStatusMutex.RLock()
	optSt := lastOptimizationStatus
	optT := lastOptimizationTime
	optimizationStatusMutex.RUnlock()

	metricsDownloadStatusMutex.RLock()
	metSt := lastMetricsDownloadStatus
	metT := lastMetricsDownloadTime
	metricsDownloadStatusMutex.RUnlock()

	isOptimizingMutex.Lock()
	currentIsOptimizing := isOptimizing
	isOptimizingMutex.Unlock()

	// These variables (apiCacheSt, apiLF, apiCLU) should be obtained from api.go's state
	// This part needs access to api.go's apiCacheMutex, apiFetchErr, lastAPIFetchTime, apiResponseCache
	// For simplicity, I'll assume they are readable here as per problem context.
	apiCacheMutex.RLock() // apiCacheMutex should be from api.go
	apiCacheSt := "OK"
	if apiFetchErr != nil { // apiFetchErr from api.go
		apiCacheSt = fmt.Sprintf("Error: %v", apiFetchErr)
	}
	apiLF := lastAPIFetchTime // lastAPIFetchTime from api.go
	apiCLU := int64(0)
	if apiResponseCache != nil { // apiResponseCache from api.go
		apiCLU = apiResponseCache.LastUpdated
	}
	apiCacheMutex.RUnlock()

	if optSt == "" {
		optSt = "Service initializing; optimization pending."
	}
	if metSt == "" {
		metSt = "Service initializing; metrics download pending."
	}

	resp := map[string]interface{}{
		"service_status":                         "active",
		"current_utc_time":                       time.Now().UTC().Format(time.RFC3339Nano),
		"optimization_process_status":            optSt,
		"last_optimization_attempt_utc":          formatTimeIfNotZero(optT),
		"metrics_download_process_status":        metSt,
		"last_metrics_download_attempt_utc":      formatTimeIfNotZero(metT),
		"is_currently_optimizing":                currentIsOptimizing,
		"hypixel_api_cache_status":               apiCacheSt,
		"hypixel_api_last_successful_fetch_utc":  formatTimeIfNotZero(apiLF),
		"hypixel_api_data_last_updated_epoch_ms": apiCLU,
	}

	b, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"Failed to marshal status"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func formatTimeIfNotZero(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// --- ADDED ---: Link to the new endpoint on the root page
	fmt.Fprintln(w, "<html><head><title>Optimizer Microservice</title></head><body><h1>Optimizer Microservice</h1><p>Available endpoints:</p><ul><li><a href='/status'>/status</a></li><li><a href='/optimizer_results.json'>/optimizer_results.json</a></li><li><a href='/failed_items_report.json'>/failed_items_report.json</a></li><li><a href='/latest-metric'>/latest-metric</a></li><li><a href='/healthz'>/healthz</a></li><li><a href='/debug/pprof/'>/debug/pprof/</a> (if pprof is imported)</li><li><a href='/debug/memstats'>/debug/memstats</a></li><li><a href='/debug/forcegc'>/debug/forcegc</a></li></ul></body></html>")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- ADDED ---: New handler to serve the latest raw metrics file
// This handler finds the most recently generated metrics file from the Rust service
// and returns its JSON content directly.
func latestMetricFileHandler(w http.ResponseWriter, r *http.Request) {
	// The Rust app saves metrics to "metrics/", and our working directory is "/app".
	const metricsDir = "metrics"

	entries, err := os.ReadDir(metricsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("INFO: Metrics directory '%s' not found.", metricsDir)
			http.Error(w, `{"error": "Metrics directory not found. No metrics have been generated yet."}`, http.StatusNotFound)
			return
		}
		log.Printf("ERROR: Could not read metrics directory %s: %v", metricsDir, err)
		http.Error(w, `{"error": "Internal Server Error: could not read metrics directory."}`, http.StatusInternalServerError)
		return
	}

	var latestFile string
	// The filename format `metrics_YYYYMMDDHHMMSS.json` is lexicographically sortable.
	// This means we can find the "latest" file with a simple string comparison, which is very efficient.
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "metrics_") && strings.HasSuffix(entry.Name(), ".json") {
			if entry.Name() > latestFile {
				latestFile = entry.Name()
			}
		}
	}

	if latestFile == "" {
		log.Printf("INFO: No metrics files found in %s", metricsDir)
		http.Error(w, `{"error": "Not Found: No metrics have been generated yet."}`, http.StatusNotFound)
		return
	}

	fullPath := filepath.Join(metricsDir, latestFile)
	log.Printf("INFO: Serving latest metrics from: %s", fullPath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		log.Printf("ERROR: Could not read latest metric file %s: %v", fullPath, err)
		http.Error(w, `{"error": "Internal Server Error: could not read metric file."}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("Main: Optimizer service starting...")

	// Initialize global status variables
	optimizationStatusMutex.Lock()
	lastOptimizationStatus = "Service starting; initial optimization pending."
	optimizationStatusMutex.Unlock()

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = "Service starting; initial metrics download pending."
	metricsDownloadStatusMutex.Unlock()

	// Initialize JSON cache variables with placeholder/default data
	optimizerResultsMutex.Lock()
	latestOptimizerResultsJSON = []byte(`{"summary":{"run_timestamp":"N/A","total_items_considered":0,"items_successfully_calculated":0,"items_with_calculation_errors":0,"max_allowed_cycle_time_seconds":null,"max_initial_search_quantity":null,"message":"Initializing..."},"results":[]}`)
	optimizerResultsMutex.Unlock()

	failedItemsMutex.Lock()
	latestFailedItemsJSON = []byte("[]") // Empty array for failed items
	failedItemsMutex.Unlock()

	// Load initial metrics from local cache if available
	metricsDataMutex.Lock()
	initialMetricsBytes, err := os.ReadFile(metricsFilename)
	if err == nil && len(initialMetricsBytes) > 0 {
		var tempMetricsSlice []ProductMetrics // ProductMetrics from metrics.go
		if parseErr := json.Unmarshal(initialMetricsBytes, &tempMetricsSlice); parseErr == nil {
			latestMetricsData = initialMetricsBytes
			log.Printf("Main: Successfully loaded initial metrics from %s (%d bytes)", metricsFilename, len(initialMetricsBytes))
		} else {
			log.Printf("Main: Found %s, but it's not a valid JSON array of ProductMetrics (%v). Initializing metrics as empty.", metricsFilename, parseErr)
			latestMetricsData = []byte("[]") // Default to empty array if parse fails
		}
	} else {
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Main: Error reading initial metrics file %s: %v. Initializing as empty.", metricsFilename, err)
		} else {
			log.Printf("Main: Initial metrics file %s not found or empty. Initializing as empty.", metricsFilename)
		}
		latestMetricsData = []byte("[]") // Default to empty JSON array
	}
	metricsDataMutex.Unlock()

	// Start periodic tasks
	go downloadMetricsPeriodically()
	go optimizePeriodically()

	// Setup HTTP handlers
	http.HandleFunc("/optimizer_results.json", optimizerResultsHandler)
	http.HandleFunc("/failed_items_report.json", failedItemsReportHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/", rootHandler) // Root handler for basic info

	// --- ADDED ---: Register the new endpoint handler
	http.HandleFunc("/latest-metric", latestMetricFileHandler)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n"))
	})
	// Debug handlers
	http.HandleFunc("/debug/memstats", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	})
	http.HandleFunc("/debug/forcegc", func(w http.ResponseWriter, r *http.Request) {
		log.Println("HTTP: Forcing GC via /debug/forcegc...")
		runtime.GC()
		log.Println("HTTP: GC forced.")
		w.Write([]byte("GC forced\n"))
	})
	// Note: pprof handlers are registered by importing _ "net/http/pprof"

	// Start HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000" // Default port
	}
	addr := "0.0.0.0:" + port
	log.Printf("Main: Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server on %s: %v", addr, err)
	}
}
