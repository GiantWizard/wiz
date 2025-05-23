// main.go
package main

import (
	"bufio"
	"context"
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
		// PROBLEM_NAN_FOUND: If this log appears, it means a NaN/Inf was about to be marshaled
		// where it might not be expected by downstream systems if they don't handle 'null'.
		// log.Printf("JSONFloat64 Marshal: NaN or Inf detected, marshaling as null. Value: %f", val)
		return []byte("null"), nil // Marshal as JSON 'null'
	}
	return json.Marshal(val) // Marshal as a standard float
}

// Helper to convert float64 to JSONFloat64, ensuring NaN/Inf are handled.
// This is the primary helper to use when assigning to JSONFloat64 fields.
func toJSONFloat64(v float64) JSONFloat64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return JSONFloat64(math.NaN()) // Store as NaN internally
	}
	return JSONFloat64(v)
}

// --- Constants ---
const (
	metricsFilename = "latest_metrics.json"
	itemFilesDir    = "dependencies/items"
	// megaCmdCheckInterval        = 5 * time.Minute // Not used in this file, but kept for context
	megaCmdTimeout              = 60 * time.Second // Increased timeout
	initialMetricsDownloadDelay = 10 * time.Second
	initialOptimizationDelay    = 20 * time.Second // Delay before first optimization
	timestampFormat             = "20060102150405" // For parsing MEGA filenames
)

// --- Struct Definitions ---
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"` // OptimizedItemResult is defined in optimizer.go
}

type OptimizationSummary struct {
	RunTimestamp                string      `json:"run_timestamp"`
	APILastUpdatedTimestamp     string      `json:"api_last_updated_timestamp,omitempty"` // From Hypixel API
	TotalItemsConsidered        int         `json:"total_items_considered"`
	ItemsSuccessfullyCalculated int         `json:"items_successfully_calculated"`
	ItemsWithCalculationErrors  int         `json:"items_with_calculation_errors"`
	MaxAllowedCycleTimeSecs     JSONFloat64 `json:"max_allowed_cycle_time_seconds"` // Uses JSONFloat64
	MaxInitialSearchQuantity    JSONFloat64 `json:"max_initial_search_quantity"`    // Uses JSONFloat64
}

type FailedItemDetail struct {
	ItemName     string `json:"item_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// --- Global Variables ---
var (
	latestOptimizerResultsJSON []byte // Holds the JSON ready for HTTP response
	optimizerResultsMutex      sync.RWMutex

	latestFailedItemsJSON []byte // Holds JSON for failed items report
	failedItemsMutex      sync.RWMutex

	lastOptimizationStatus  string    // User-friendly status message
	lastOptimizationTime    time.Time // Timestamp of the last attempt
	optimizationStatusMutex sync.RWMutex

	latestMetricsData          []byte // Raw bytes of the latest metrics JSON
	metricsDataMutex           sync.RWMutex
	lastMetricsDownloadStatus  string    // User-friendly status for metrics download
	lastMetricsDownloadTime    time.Time // Timestamp of last metrics download attempt
	metricsDownloadStatusMutex sync.RWMutex

	isOptimizing      bool // Flag to prevent concurrent full optimization runs
	isOptimizingMutex sync.Mutex

	// Regex to find metrics files in MEGA listing, e.g., "metrics_20231027153000.json"
	metricsFileRegex = regexp.MustCompile(`^metrics_(\d{14})\.json$`)
)

// downloadMetricsFromMega attempts to download the latest metrics file from MEGA.
// It identifies the latest file based on a timestamp in the filename.
func downloadMetricsFromMega(localTargetFilename string) error {
	megaEmail := os.Getenv("MEGA_EMAIL")
	megaPassword := os.Getenv("MEGA_PWD")
	megaRemoteFolderPath := os.Getenv("MEGA_METRICS_FOLDER_PATH") // e.g., "/Root/MyMetricsFolder"

	if megaEmail == "" || megaPassword == "" || megaRemoteFolderPath == "" {
		log.Println("downloadMetricsFromMega: MEGA environment variables not fully set. Skipping MEGA download.")
		// Check if a local cache exists and use it if MEGA download is skipped.
		if _, err := os.Stat(metricsFilename); !os.IsNotExist(err) {
			log.Printf("downloadMetricsFromMega: MEGA download skipped, using existing local cache '%s' if readable later.", metricsFilename)
		} else {
			return fmt.Errorf("MEGA download skipped (missing env config), and no local cache file '%s' found", metricsFilename)
		}
		return nil // Not an error if local cache will be used or is intended fallback
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(localTargetFilename)
	if targetDir != "." && targetDir != "" { // Avoid trying to create "."
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory %s for metrics: %v", targetDir, err)
		}
	}

	cmdEnv := os.Environ() // Pass current environment

	// List files in the MEGA folder
	log.Printf("Listing files in MEGA folder: %s (using 'megals' with direct auth flags)", megaRemoteFolderPath)
	ctxLs, cancelLs := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelLs()
	lsCmd := exec.CommandContext(ctxLs, "megals", "--username", megaEmail, "--password", megaPassword, "--no-ask-password", megaRemoteFolderPath)
	lsCmd.Env = cmdEnv
	lsOutBytes, lsErr := lsCmd.CombinedOutput() // Capture both stdout and stderr

	if ctxLs.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: 'megals' command timed out after %v", megaCmdTimeout)
		return fmt.Errorf("'megals' command timed out")
	}
	lsOut := string(lsOutBytes)
	log.Printf("downloadMetricsFromMega: 'megals' Output (first 500 chars): %.500s", lsOut) // Log some output
	if lsErr != nil {
		// Check if the error is due to context deadline exceeded, which is already handled
		if ctxLs.Err() != context.DeadlineExceeded {
			return fmt.Errorf("megals command failed: %v. Output: %s", lsErr, lsOut)
		}
		// If it was deadline exceeded, the earlier return fmt.Errorf("'megals' command timed out") covers it.
	}

	// Find the latest metrics file from the listing
	var latestFilename string
	var latestTimestamp time.Time
	foundFile := false

	scanner := bufio.NewScanner(strings.NewReader(lsOut))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// megals might output warnings or progress; filter them if necessary.
		// For now, assume valid lines are file paths.
		// Example line: "/Root/MyMetricsFolder/metrics_20231027153000.json"
		// We only need the filename part.
		if strings.Contains(strings.ToUpper(line), "WRN:") || strings.Contains(strings.ToUpper(line), "ERR:") {
			log.Printf("MEGA LS Info/Warning: %s", line)
			continue
		}

		filenameToParse := filepath.Base(line) // Get "metrics_20231027153000.json"
		match := metricsFileRegex.FindStringSubmatch(filenameToParse)

		if len(match) == 2 { // match[0] is full string, match[1] is the timestamp part
			timestampStr := match[1]
			t, parseErr := time.Parse(timestampFormat, timestampStr)
			if parseErr != nil {
				log.Printf("Warning: could not parse timestamp in filename '%s': %v", filenameToParse, parseErr)
				continue
			}
			if !foundFile || t.After(latestTimestamp) {
				foundFile = true
				latestTimestamp = t
				latestFilename = filenameToParse // Store just the filename, not the full path from megals
			}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		log.Printf("Warning: error scanning 'megals' output: %v", scanErr)
		// This might not be fatal if a file was already found.
	}

	if !foundFile {
		return fmt.Errorf("no metrics file matching pattern '%s' found in MEGA folder '%s'", metricsFileRegex.String(), megaRemoteFolderPath)
	}

	// Construct the full remote path for megaget
	remoteFilePathFull := strings.TrimRight(megaRemoteFolderPath, "/") + "/" + latestFilename

	// Download the latest file
	log.Printf("Downloading latest metrics file '%s' from MEGA to '%s'", remoteFilePathFull, localTargetFilename)
	_ = os.Remove(localTargetFilename) // Attempt to remove old temp file if exists
	ctxGet, cancelGet := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelGet()
	// Using --path with megaget specifies the local directory to save to, and it uses the remote filename.
	// To save to a specific *localFilename*, we need to ensure the directory exists and megaget saves it there.
	// Simpler: download to current dir with remote name, then move. Or use --path with local dir.
	// If localTargetFilename includes the desired filename:
	targetDownloadDir := filepath.Dir(localTargetFilename)
	targetBaseFilename := filepath.Base(localTargetFilename)

	// megaget will save as `latestFilename` in `targetDownloadDir` if targetDownloadDir is just a dir.
	// If localTargetFilename is `path/to/file.json`, megaget needs `--path path/to/` and it saves `latestFilename`.
	// To force a specific local filename, it's often easier to download to temp location and rename.
	// Let's use localTargetFilename directly with --path, assuming it's the full desired path.
	getCmd := exec.CommandContext(ctxGet, "megaget", "--username", megaEmail, "--password", megaPassword, "--no-ask-password", remoteFilePathFull, "--path", localTargetFilename)
	getCmd.Env = cmdEnv
	getOutBytes, getErr := getCmd.CombinedOutput()

	if ctxGet.Err() == context.DeadlineExceeded {
		return fmt.Errorf("'megaget' command timed out for %s", latestFilename)
	}
	getOut := string(getOutBytes)
	log.Printf("downloadMetricsFromMega: 'megaget' Output (first 500 chars): %.500s", getOut)
	if getErr != nil {
		return fmt.Errorf("failed to download '%s': %v. Output: %s", remoteFilePathFull, getErr, getOut)
	}

	// Verify download
	if _, statErr := os.Stat(localTargetFilename); os.IsNotExist(statErr) {
		// Check if megaget saved it with the original remote name in the target directory instead
		altPath := filepath.Join(targetDownloadDir, latestFilename)
		if _, altStatErr := os.Stat(altPath); !os.IsNotExist(altStatErr) && altPath != localTargetFilename {
			log.Printf("megaget saved as %s in dir, renaming to %s", latestFilename, targetBaseFilename)
			if renameErr := os.Rename(altPath, localTargetFilename); renameErr != nil {
				return fmt.Errorf("megaget downloaded but failed to rename from %s to %s: %v", altPath, localTargetFilename, renameErr)
			}
		} else {
			return fmt.Errorf("megaget command appeared to succeed for '%s', but target file '%s' is missing. Output: %s", latestFilename, localTargetFilename, getOut)
		}
	}

	log.Printf("Successfully downloaded '%s' from MEGA to '%s'.", latestFilename, localTargetFilename)
	return nil
}

// downloadAndStoreMetrics orchestrates the download and update of metrics data.
func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download...")
	tempMetricsFilename := metricsFilename + ".downloading.tmp" // Temporary file for download
	defer os.Remove(tempMetricsFilename)                        // Clean up temp file

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadTime = time.Now() // Record attempt time
	err := downloadMetricsFromMega(tempMetricsFilename)
	metricsDownloadStatusMutex.Unlock() // Unlock early if error allows other status updates

	if err != nil {
		newStatus := fmt.Sprintf("Error during MEGA download at %s: %v", lastMetricsDownloadTime.Format(time.RFC3339), err)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	// Read the downloaded data from the temporary file
	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		newStatus := fmt.Sprintf("Error reading downloaded metrics file %s at %s: %v", tempMetricsFilename, lastMetricsDownloadTime.Format(time.RFC3339), readErr)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	// Basic validation: not empty, not just "{}"
	if len(data) == 0 {
		newStatus := fmt.Sprintf("Error: downloaded metrics data was empty at %s.", lastMetricsDownloadTime.Format(time.RFC3339))
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if string(data) == "{}" { // Check for placeholder empty JSON object
		newStatus := fmt.Sprintf("Error: downloaded metrics data was '{}' at %s.", lastMetricsDownloadTime.Format(time.RFC3339))
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	// Validate JSON structure (expecting an array of ProductMetrics)
	var tempMetricsSlice []ProductMetrics
	if jsonErr := json.Unmarshal(data, &tempMetricsSlice); jsonErr != nil {
		newStatus := fmt.Sprintf("Error: downloaded metrics data was NOT A VALID JSON ARRAY of ProductMetrics at %s: %v. Body(first 200): %.200s", lastMetricsDownloadTime.Format(time.RFC3339), jsonErr, string(data))
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	// Update global cache
	metricsDataMutex.Lock()
	latestMetricsData = data
	metricsDataMutex.Unlock()

	// Persist to the main metrics file (overwrite)
	if writeErr := os.WriteFile(metricsFilename, data, 0644); writeErr != nil {
		// This is a warning because the in-memory cache is updated.
		log.Printf("Warning: failed to write metrics to permanent cache file %s: %v", metricsFilename, writeErr)
	} else {
		log.Printf("Successfully wrote new metrics data (len %d) to %s", len(data), metricsFilename)
	}

	newStatus := fmt.Sprintf("Successfully downloaded and updated metrics at %s. Size: %d bytes", lastMetricsDownloadTime.Format(time.RFC3339), len(data))
	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = newStatus
	metricsDownloadStatusMutex.Unlock()
	log.Println(newStatus)
}

// downloadMetricsPeriodically sets up a ticker to download metrics.
func downloadMetricsPeriodically() {
	go func() {
		log.Printf("downloadMetricsPeriodically: Waiting %v before initial download...", initialMetricsDownloadDelay)
		time.Sleep(initialMetricsDownloadDelay)

		downloadAndStoreMetrics() // Initial download

		// Subsequent downloads on a ticker
		// ticker := time.NewTicker(megaCmdCheckInterval) // Use configured interval
		// For testing, a shorter interval:
		ticker := time.NewTicker(1 * time.Hour) // Example: 1 hour
		defer ticker.Stop()

		for range ticker.C {
			log.Println("downloadMetricsPeriodically: Triggering periodic metrics download.")
			downloadAndStoreMetrics()
		}
	}()
}

// parseProductMetricsData parses the raw JSON bytes into a map.
func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) {
	if len(jsonData) == 0 {
		log.Println("parseProductMetricsData: jsonData is empty, returning empty map.")
		return make(map[string]ProductMetrics), nil // Return empty map, not error
	}
	// Check for "{}", which is invalid for a slice unmarshal
	if string(jsonData) == "{}" {
		log.Println("parseProductMetricsData: jsonData is '{}', which cannot be unmarshaled into []ProductMetrics.")
		return nil, fmt.Errorf("cannot unmarshal JSON object into []ProductMetrics")
	}

	var productMetricsSlice []ProductMetrics
	if err := json.Unmarshal(jsonData, &productMetricsSlice); err != nil {
		// Log more details on unmarshal failure
		sample := string(jsonData)
		if len(sample) > 200 {
			sample = sample[:200] + "... (truncated)"
		}
		log.Printf("parseProductMetricsData: Failed to unmarshal metrics JSON. Error: %v. Sample: %s", err, sample)
		return nil, fmt.Errorf("failed to unmarshal metrics JSON: %w. Sample: %s", err, sample)
	}

	productMetricsMap := make(map[string]ProductMetrics, len(productMetricsSlice))
	skippedCount := 0
	for _, metric := range productMetricsSlice {
		if metric.ProductID == "" {
			skippedCount++
			continue // Skip entries with no ProductID
		}
		// IDs in metrics file should already be normalized by the source if possible,
		// but normalizing here ensures consistency if they aren't.
		normalizedID := BAZAAR_ID(metric.ProductID)
		if _, exists := productMetricsMap[normalizedID]; exists {
			// log.Printf("parseProductMetricsData: Warning - Duplicate normalized ProductID '%s' in metrics. Overwriting.", normalizedID)
		}
		metric.ProductID = normalizedID // Store with normalized ID in the map value as well
		productMetricsMap[normalizedID] = metric
	}
	if skippedCount > 0 {
		log.Printf("parseProductMetricsData: Skipped %d metric entries due to empty ProductID.", skippedCount)
	}
	log.Printf("parseProductMetricsData: Successfully parsed %d metrics objects into %d map entries.", len(productMetricsSlice), len(productMetricsMap))
	return productMetricsMap, nil
}

// performOptimizationCycleNow runs the full optimization logic.
func performOptimizationCycleNow(productMetrics map[string]ProductMetrics, apiResp *HypixelAPIResponse) ([]byte, []byte, error) {
	runStartTime := time.Now()
	log.Println("performOptimizationCycleNow: Starting new optimization cycle...")

	if apiResp == nil || apiResp.Products == nil {
		return nil, nil, fmt.Errorf("CRITICAL: API data is nil or has no products in performOptimizationCycleNow")
	}
	if productMetrics == nil { // Check if map itself is nil
		return nil, nil, fmt.Errorf("CRITICAL: Product metrics map is nil in performOptimizationCycleNow")
	}
	if len(productMetrics) == 0 { // Check if map is empty
		log.Println("performOptimizationCycleNow: Product metrics map is empty. No items to optimize based on metrics.")
		// Depending on desired behavior, could return empty results or an error.
		// For now, let's proceed, RunFullOptimization will handle empty item list.
	}

	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 {
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).UTC().Format(time.RFC3339Nano)
	}

	var itemIDs []string
	for id := range apiResp.Products { // Iterate over products from API response
		itemIDs = append(itemIDs, id)
	}
	if len(itemIDs) == 0 {
		log.Println("performOptimizationCycleNow: No item IDs from API response to optimize.")
		// Construct empty valid JSON for results
		emptySummary := OptimizationSummary{
			RunTimestamp: runStartTime.Format(time.RFC3339Nano), APILastUpdatedTimestamp: apiLastUpdatedStr, TotalItemsConsidered: 0,
			ItemsSuccessfullyCalculated: 0, ItemsWithCalculationErrors: 0,
			MaxAllowedCycleTimeSecs: toJSONFloat64(0), MaxInitialSearchQuantity: toJSONFloat64(0),
		}
		emptyOutput := OptimizationRunOutput{Summary: emptySummary, Results: []OptimizedItemResult{}}
		mainJSON, _ := json.MarshalIndent(emptyOutput, "", "  ")
		return mainJSON, []byte("[]"), nil // No error, just no items
	}

	// Configuration for optimization run (chunking, etc.)
	itemsPerChunk := 50 // Default
	if ipcStr := os.Getenv("ITEMS_PER_CHUNK"); ipcStr != "" {
		if val, err := strconv.Atoi(ipcStr); err == nil && val > 0 {
			itemsPerChunk = val
		}
	}
	pauseBetweenChunks := 500 * time.Millisecond // Default
	if pbcStr := os.Getenv("PAUSE_MS_BETWEEN_CHUNKS"); pbcStr != "" {
		if val, err := strconv.Atoi(pbcStr); err == nil && val >= 0 { // Allow 0ms pause
			pauseBetweenChunks = time.Duration(val) * time.Millisecond
		}
	}
	// These are the parameters passed to the optimizer
	const (
		maxAllowedCycleTimePerItemRaw = 3600.0    // e.g., 1 hour
		maxInitialSearchQtyRaw        = 1000000.0 // Max quantity for binary search start
	)
	log.Printf("performOptimizationCycleNow: Parameters - MaxCycleTime: %.0fs, MaxInitialSearchQty: %.0f, ChunkSize: %d, Pause: %v",
		maxAllowedCycleTimePerItemRaw, maxInitialSearchQtyRaw, itemsPerChunk, pauseBetweenChunks)

	var allOptimizedResults []OptimizedItemResult
	// Chunking logic
	for i := 0; i < len(itemIDs); i += itemsPerChunk {
		end := i + itemsPerChunk
		if end > len(itemIDs) {
			end = len(itemIDs)
		}
		currentChunkItemIDs := itemIDs[i:end]
		if len(currentChunkItemIDs) == 0 {
			continue
		}

		log.Printf("performOptimizationCycleNow: Optimizing chunk %d/%d (items %d to %d of %d total)",
			(i/itemsPerChunk)+1, (len(itemIDs)+itemsPerChunk-1)/itemsPerChunk, i, end-1, len(itemIDs))

		// Run optimization for the current chunk
		// RunFullOptimization now returns []OptimizedItemResult which are smaller (no full trees)
		chunkResults := RunFullOptimization(currentChunkItemIDs, maxAllowedCycleTimePerItemRaw, apiResp, productMetrics, itemFilesDir, maxInitialSearchQtyRaw)
		allOptimizedResults = append(allOptimizedResults, chunkResults...)

		if end < len(itemIDs) && pauseBetweenChunks > 0 {
			log.Printf("Pausing for %v before next chunk...", pauseBetweenChunks)
			time.Sleep(pauseBetweenChunks)
		}
	}
	// allOptimizedResults now contains results from all chunks
	optimizedResults := allOptimizedResults
	log.Printf("performOptimizationCycleNow: Optimization complete for all chunks. Generated %d total results.", len(optimizedResults))

	var successCount int
	var failDetails []FailedItemDetail
	for _, r := range optimizedResults {
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
		MaxAllowedCycleTimeSecs:     toJSONFloat64(maxAllowedCycleTimePerItemRaw), // Store config
		MaxInitialSearchQuantity:    toJSONFloat64(maxInitialSearchQtyRaw),        // Store config
	}
	mainOutput := OptimizationRunOutput{Summary: summary, Results: optimizedResults}

	// Marshal the main output to JSON
	mainJSON, err := json.MarshalIndent(mainOutput, "", "  ")
	if err != nil {
		// This is a critical failure, potentially due to NaN/Inf not handled by JSONFloat64 (though it should be)
		// or some other struct issue.
		log.Printf("CRITICAL: Failed to marshal main optimization output: %v. Check for 'PROBLEM_NAN_FOUND' in logs if NaNs are suspected.", err)
		// To aid debugging, try to log a sample of what failed to marshal if possible, but mainOutput can be huge.
		return nil, nil, fmt.Errorf("CRITICAL: Failed to marshal main optimization output: %w", err)
	}

	// Marshal failed items report to JSON
	var failedJSON []byte
	if len(failDetails) > 0 {
		if b, errMarshal := json.MarshalIndent(failDetails, "", "  "); errMarshal != nil {
			log.Printf("Error: Failed to marshal failed items report: %v", errMarshal)
			failedJSON = []byte(`[{"error":"failed to marshal failed items report"}]`) // Fallback
		} else {
			failedJSON = b
		}
	} else {
		failedJSON = []byte("[]") // Empty JSON array if no failures
	}

	return mainJSON, failedJSON, nil
}

// runSingleOptimizationAndUpdateResults is the top-level function for one optimization pass.
func runSingleOptimizationAndUpdateResults() {
	log.Println("runSingleOptimizationAndUpdateResults: Initiating new optimization process...")

	// Get a consistent snapshot of metrics data
	metricsDataMutex.RLock()
	currentMetricsBytes := make([]byte, len(latestMetricsData)) // Create a copy
	copy(currentMetricsBytes, latestMetricsData)
	metricsDataMutex.RUnlock()

	if len(currentMetricsBytes) == 0 || string(currentMetricsBytes) == "[]" {
		newStatus := fmt.Sprintf("Optimization skipped at %s: Metrics data is not ready or is empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if string(currentMetricsBytes) == "{}" {
		newStatus := fmt.Sprintf("Optimization skipped at %s: Metrics data was '{}', which is invalid for parsing.", time.Now().Format(time.RFC3339))
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
	if len(productMetrics) == 0 { // Check after parsing
		newStatus := fmt.Sprintf("Optimization skipped at %s: Parsed metrics map is empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	// Get latest API data
	apiResp, apiErr := getApiResponse() // This function now always fetches
	if apiErr != nil {
		newStatus := fmt.Sprintf("Optimization skipped at %s: API data load failed: %v", time.Now().Format(time.RFC3339), apiErr)
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if apiResp == nil || len(apiResp.Products) == 0 {
		newStatus := fmt.Sprintf("Optimization skipped at %s: API response is nil or contains no products.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	// Perform the optimization
	mainJSON, failedJSON, optErr := performOptimizationCycleNow(productMetrics, apiResp)
	mainLen, failLen := 0, 0
	if mainJSON != nil {
		mainLen = len(mainJSON)
	}
	if failedJSON != nil {
		failLen = len(failedJSON)
	}

	// Update global results and status
	optimizationStatusMutex.Lock()
	lastOptimizationTime = time.Now()
	if optErr != nil {
		lastOptimizationStatus = fmt.Sprintf("Optimization Error at %s: %v", lastOptimizationTime.Format(time.RFC3339), optErr)
		// Still update JSON if partially available (e.g., summary might exist even if results errored)
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
		lastOptimizationStatus = fmt.Sprintf("Successfully optimized at %s. Results: %d bytes, Failed Items Report: %d bytes", lastOptimizationTime.Format(time.RFC3339), mainLen, failLen)
	}
	optimizationStatusMutex.Unlock()
	log.Printf("runSingleOptimizationAndUpdateResults: Cycle finished. Error: %v. Main JSON size: %d bytes, Failed JSON size: %d bytes. Status: %s", optErr, mainLen, failLen, lastOptimizationStatus)
}

// optimizePeriodically manages the periodic execution of optimization.
func optimizePeriodically() {
	go func() { // Initial delayed run
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

	// Subsequent runs on a ticker
	// ticker := time.NewTicker(1 * time.Minute) // Example: 1 minute for frequent updates
	ticker := time.NewTicker(20 * time.Second) // Per original
	defer ticker.Stop()

	for t := range ticker.C {
		canStart := false
		isOptimizingMutex.Lock()
		if !isOptimizing {
			isOptimizing = true // Set flag anead of goroutine
			canStart = true
		}
		isOptimizingMutex.Unlock()

		if canStart {
			log.Printf("optimizePeriodically (tick %s): Scheduling optimization work.", t.Format(time.RFC3339))
			go func(tickTime time.Time) { // Launch actual work in a new goroutine
				defer func() {
					isOptimizingMutex.Lock()
					isOptimizing = false // Reset flag when done
					isOptimizingMutex.Unlock()
					log.Printf("optimizePeriodically (goroutine for tick %s): Optimization work finished.", tickTime.Format(time.RFC3339))
				}()
				log.Printf("optimizePeriodically (goroutine for tick %s): Starting optimization work.", tickTime.Format(time.RFC3339))
				runSingleOptimizationAndUpdateResults()
			}(t)
		} else {
			log.Println("optimizePeriodically (tick): Previous optimization still in progress, skipping this tick.")
		}
	}
}

// --- HTTP Handlers ---
func optimizerResultsHandler(w http.ResponseWriter, r *http.Request) {
	optimizerResultsMutex.RLock()
	data := latestOptimizerResultsJSON // Serve the pre-marshaled JSON
	optimizerResultsMutex.RUnlock()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if data == nil { // Should have initial JSON from main()
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
	if data == nil { // Should have initial "[]" from main()
		w.Write([]byte("[]")) // Empty array if nil for some reason
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

	isOptimizingMutex.Lock() // Use lock for single bool read for safety
	currentIsOptimizing := isOptimizing
	isOptimizingMutex.Unlock()

	// API Cache Status
	apiCacheMutex.RLock()
	apiCacheStatus := "OK"
	if apiFetchErr != nil {
		apiCacheStatus = fmt.Sprintf("Error: %v", apiFetchErr)
	}
	apiLastFetch := lastAPIFetchTime
	apiCacheLastUpdated := int64(0)
	if apiResponseCache != nil {
		apiCacheLastUpdated = apiResponseCache.LastUpdated
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
		"hypixel_api_cache_status":               apiCacheStatus,
		"hypixel_api_last_successful_fetch_utc":  formatTimeIfNotZero(apiLastFetch),
		"hypixel_api_data_last_updated_epoch_ms": apiCacheLastUpdated,
	}

	b, err := json.MarshalIndent(resp, "", "  ") // Indent for readability
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
	// Basic HTML response for root
	fmt.Fprintln(w, "<html><head><title>Optimizer Microservice</title></head><body>")
	fmt.Fprintln(w, "<h1>Optimizer Microservice</h1>")
	fmt.Fprintln(w, "<p>Available endpoints:</p>")
	fmt.Fprintln(w, "<ul>")
	fmt.Fprintln(w, "<li><a href='/status'>/status</a> - Current service status</li>")
	fmt.Fprintln(w, "<li><a href='/optimizer_results.json'>/optimizer_results.json</a> - Latest optimization results</li>")
	fmt.Fprintln(w, "<li><a href='/failed_items_report.json'>/failed_items_report.json</a> - Report of items that failed optimization</li>")
	fmt.Fprintln(w, "<li><a href='/healthz'>/healthz</a> - Health check (returns 200 OK)</li>")
	fmt.Fprintln(w, "<li><a href='/debug/pprof/'>/debug/pprof/</a> - Go profiling tools</li>")
	fmt.Fprintln(w, "<li><a href='/debug/memstats'>/debug/memstats</a> - Go runtime memory statistics</li>")
	fmt.Fprintln(w, "<li><a href='/debug/forcegc'>/debug/forcegc</a> - Force garbage collection</li>")
	fmt.Fprintln(w, "</ul>")
	fmt.Fprintln(w, "</body></html>")
}

// min helper (not strictly needed by current logic but was in original)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("Main: Optimizer service starting...")

	// Initialize statuses
	optimizationStatusMutex.Lock()
	lastOptimizationStatus = "Service starting; initial optimization pending."
	optimizationStatusMutex.Unlock()
	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = "Service starting; initial metrics download pending."
	metricsDownloadStatusMutex.Unlock()

	// Initialize JSON caches with valid empty/default JSON
	optimizerResultsMutex.Lock()
	latestOptimizerResultsJSON = []byte(`{"summary":{"run_timestamp":"N/A","total_items_considered":0,"items_successfully_calculated":0,"items_with_calculation_errors":0,"max_allowed_cycle_time_seconds":null,"max_initial_search_quantity":null,"message":"Initializing..."},"results":[]}`)
	optimizerResultsMutex.Unlock()
	failedItemsMutex.Lock()
	latestFailedItemsJSON = []byte("[]")
	failedItemsMutex.Unlock()

	// Load initial metrics from local file if available
	metricsDataMutex.Lock()
	initialMetricsBytes, err := os.ReadFile(metricsFilename)
	if err == nil && len(initialMetricsBytes) > 0 {
		// Validate that it's a JSON array before assigning
		var tempMetricsSlice []ProductMetrics
		if parseErr := json.Unmarshal(initialMetricsBytes, &tempMetricsSlice); parseErr == nil {
			latestMetricsData = initialMetricsBytes
			log.Printf("Main: Successfully loaded initial metrics from %s (%d bytes)", metricsFilename, len(initialMetricsBytes))
		} else {
			log.Printf("Main: Found %s, but it's not a valid JSON array (%v). Initializing metrics as empty.", metricsFilename, parseErr)
			latestMetricsData = []byte("[]") // Fallback to empty array
		}
	} else {
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Main: Error reading initial metrics file %s: %v. Initializing as empty.", metricsFilename, err)
		} else {
			log.Printf("Main: Initial metrics file %s not found or empty. Initializing as empty.", metricsFilename)
		}
		latestMetricsData = []byte("[]") // Default to empty JSON array if no valid file
	}
	metricsDataMutex.Unlock()

	// Start background processes
	go downloadMetricsPeriodically() // For MEGA downloads
	go optimizePeriodically()        // For main optimization cycles

	// Setup HTTP server
	http.HandleFunc("/optimizer_results.json", optimizerResultsHandler)
	http.HandleFunc("/failed_items_report.json", failedItemsReportHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write([]byte("OK\n")) })

	// Debug endpoints
	http.HandleFunc("/debug/memstats", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m) // Use encoder for proper JSON
	})
	http.HandleFunc("/debug/forcegc", func(w http.ResponseWriter, r *http.Request) {
		log.Println("HTTP: Forcing GC via /debug/forcegc...")
		runtime.GC()
		log.Println("HTTP: GC forced.")
		w.Write([]byte("GC forced\n"))
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081" // Default port
	}
	addr := "0.0.0.0:" + port // Listen on all interfaces
	log.Printf("Main: Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server on %s: %v", addr, err)
	}
}
