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
	initialMetricsDownloadDelay = 15 * time.Second // Shortened for testing, can revert
	initialOptimizationDelay    = 30 * time.Second // Shortened for testing, can revert
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
// downloadMetricsFromMega connects to MEGA, finds the newest metrics_<timestamp>.json
// under any recognized “remote_metrics” path, downloads it, and writes it to localTargetFilename.
func downloadMetricsFromMega(localTargetFilename string) error {
	// ── 1) Read environment variables ────────────────────────────────────────────
	megaEmail := os.Getenv("MEGA_EMAIL")
	megaPassword := os.Getenv("MEGA_PWD")
	if megaEmail == "" || megaPassword == "" {
		return fmt.Errorf("MEGA_EMAIL or MEGA_PWD not set")
	}

	// Ensure we have a HOME for MEGAcmd
	homeEnv := os.Getenv("HOME")
	if homeEnv == "" {
		homeEnv = "/home/appuser"
		log.Printf("DEBUG: HOME not set; defaulting to %s", homeEnv)
	}

	// Helper to run a command with HOME in the environment and return combined output.
	runCmd := func(name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Env = append(os.Environ(), "HOME="+homeEnv)
		outBytes, err := cmd.CombinedOutput()
		return string(outBytes), err
	}

	// ── 2) megacmd logout (ignore errors) ────────────────────────────────────────
	if out, err := runCmd("mega-cmd", "logout"); err != nil {
		log.Printf("WARNING: mega-cmd logout error (continuing anyway): %v\n%s", err, out)
	} else {
		log.Printf("DEBUG: mega-cmd logout output:\n%s", out)
	}

	// ── 3) mega-cmd ipc killserver (ignore errors) ──────────────────────────────
	if out, err := runCmd("mega-cmd", "ipc", "killserver"); err != nil {
		log.Printf("DEBUG: mega-cmd ipc killserver error (probably no server): %v\n%s", err, out)
	} else {
		log.Printf("DEBUG: mega-cmd ipc killserver output:\n%s", out)
	}
	time.Sleep(1 * time.Second) // give a moment for any server to actually shut down

	// ── 4) mega-login (treat exit code 54 + “already logged in” as OK) ─────────
	loginOut, loginErr := runCmd("mega-login", megaEmail, megaPassword)
	if loginErr != nil {
		if exitErr, ok := loginErr.(*exec.ExitError); ok && exitErr.ExitCode() == 54 &&
			strings.Contains(strings.ToLower(loginOut), "already logged in") {
			log.Printf("DEBUG: mega-login says “Already logged in.” Proceeding.")
		} else {
			return fmt.Errorf("mega-login failed: %v\nOutput:\n%s", loginErr, loginOut)
		}
	} else {
		log.Printf("DEBUG: mega-login succeeded (output:\n%s)", loginOut)
	}
	time.Sleep(2 * time.Second) // let the session fully initialize

	// ── 5) Ensure the remote folder exists via mega-mkdir -p "/remote_metrics" ───
	mkdirOut, mkdirErr := runCmd("mega-mkdir", "-p", "/remote_metrics")
	if mkdirErr != nil {
		if exitErr, ok := mkdirErr.(*exec.ExitError); ok && exitErr.ExitCode() == 54 &&
			strings.Contains(strings.ToLower(mkdirOut), "already exists") {
			log.Printf("DEBUG: remote folder already exists: /remote_metrics")
		} else {
			return fmt.Errorf("mega-mkdir -p /remote_metrics failed: %v\nOutput:\n%s", mkdirErr, mkdirOut)
		}
	} else {
		log.Printf("DEBUG: mega-mkdir succeeded (output:\n%s)", mkdirOut)
	}

	// ── 6) Try all candidate paths using megals until we find any “metrics_*.json” ─
	candidates := []string{
		"/remote_metrics",
		"/Root/remote_metrics",
		"remote_metrics",
		"Root/remote_metrics",
	}

	var lsOutput, folderInUse string
	for _, folder := range candidates {
		out, err := runCmd("megals", folder)
		if err == nil && strings.Contains(out, "metrics_") {
			lsOutput = out
			folderInUse = folder
			break
		}
	}

	if folderInUse == "" {
		return fmt.Errorf(
			"no metrics file found under any of %v; last megals output:\n%s",
			candidates, lsOutput,
		)
	}
	log.Printf("DEBUG: using MEGA folder path %q; `megals` returned:\n%s", folderInUse, lsOutput)

	// ── 7) Parse the listing for the newest “metrics_<YYYYMMDDhhmmss>.json” ─────
	var latestFilename string
	var latestTime time.Time
	const timeLayout = "20060102150405"

	scanner := bufio.NewScanner(strings.NewReader(lsOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Only consider lines that look exactly like “metrics_<timestamp>.json”
		if strings.HasPrefix(line, "metrics_") && strings.HasSuffix(line, ".json") {
			tsPart := strings.TrimSuffix(strings.TrimPrefix(line, "metrics_"), ".json")
			if t, err := time.Parse(timeLayout, tsPart); err == nil {
				if latestFilename == "" || t.After(latestTime) {
					latestTime = t
					latestFilename = line
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("WARNING: error scanning megals output: %v", err)
	}
	if latestFilename == "" {
		return fmt.Errorf(
			"found no file matching metrics_<timestamp>.json under %q; listing:\n%s",
			folderInUse, lsOutput,
		)
	}

	// ── 8) Download via “megaget” ───────────────────────────────────────────────
	remoteFile := filepath.Join(folderInUse, latestFilename)
	targetDir := filepath.Dir(localTargetFilename)
	tempPath := filepath.Join(targetDir, latestFilename)

	log.Printf("DEBUG: downloading %q → %q", remoteFile, tempPath)
	getOut, getErr := runCmd("megaget", remoteFile, "--path", targetDir)
	if getErr != nil {
		return fmt.Errorf("megaget %q failed: %v\nOutput:\n%s", remoteFile, getErr, getOut)
	}

	// ── 9) Rename the downloaded file into place ────────────────────────────────
	if tempPath != localTargetFilename {
		if err := os.Rename(tempPath, localTargetFilename); err != nil {
			return fmt.Errorf("failed to rename %q → %q: %v", tempPath, localTargetFilename, err)
		}
	}
	log.Printf("Successfully downloaded %q → %q", remoteFile, localTargetFilename)
	return nil
}

// Here is a typical wrapper showing how you might call it from your ticker/goroutine.
func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download...")
	tempMetricsFilename := filepath.Join(os.TempDir(), fmt.Sprintf("metrics_%d.tmp.json", time.Now().UnixNano()))
	defer os.Remove(tempMetricsFilename)

	err := downloadMetricsFromMega(tempMetricsFilename)
	if err != nil {
		status := fmt.Sprintf("Error during MEGA download at %s: %v",
			time.Now().Format(time.RFC3339), err)
		log.Println(status)
		// update your in-memory or global status variable here if needed
		return
	}

	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		status := fmt.Sprintf("Error reading downloaded file %q: %v", tempMetricsFilename, readErr)
		log.Println(status)
		return
	}

	// Check for empty or invalid JSON
	if len(data) == 0 || string(data) == "{}" {
		status := fmt.Sprintf("Downloaded metrics were empty or `{}` at %s", time.Now().Format(time.RFC3339))
		log.Println(status)
		return
	}

	// (Optional) Attempt to unmarshal into your metrics struct to validate
	var tmp []interface{} // replace with your actual slice type, e.g. []ProductMetrics
	if jsonErr := json.Unmarshal(data, &tmp); jsonErr != nil {
		sample := string(data)
		if len(sample) > 200 {
			sample = sample[:200] + "…"
		}
		status := fmt.Sprintf(
			"Downloaded file is not valid JSON array at %s: %v; sample: %q",
			time.Now().Format(time.RFC3339), jsonErr, sample,
		)
		log.Println(status)
		return
	}

	// Overwrite your permanent cache (e.g. metrics.json)
	permanentPath := "latest_metrics.json" // wherever you want to store it
	if writeErr := os.WriteFile(permanentPath, data, 0644); writeErr != nil {
		log.Printf("Warning: could not write to %q: %v (in-memory data is OK)", permanentPath, writeErr)
	} else {
		log.Printf("Successfully updated %q (%d bytes)", permanentPath, len(data))
	}
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
	if string(jsonData) == "{}" {
		log.Println("parseProductMetricsData: jsonData is '{}', which cannot be unmarshaled into []ProductMetrics.")
		// Depending on requirements, an empty JSON object might be valid if it means "no metrics"
		// but if it's always expected to be an array, this is an error.
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
		newStatus := fmt.Sprintf("Optimization skipped at %s: Parsed metrics map is empty (no valid ProductMetrics found).", time.Now().Format(time.RFC3339))
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

	optimizationStatusMutex.Lock()
	lastOptimizationTime = time.Now()
	if optErr != nil {
		lastOptimizationStatus = fmt.Sprintf("Optimization Error at %s: %v", lastOptimizationTime.Format(time.RFC3339), optErr)
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
		lastOptimizationStatus = fmt.Sprintf("Successfully optimized at %s. Results: %d bytes, Failed Items Report: %d bytes",
			lastOptimizationTime.Format(time.RFC3339), mainLen, failLen)
	}
	optimizationStatusMutex.Unlock()

	log.Printf("runSingleOptimizationAndUpdateResults: Cycle finished. Error: %v. Main JSON size: %d bytes, Failed JSON size: %d bytes. Status: %s",
		optErr, mainLen, failLen, lastOptimizationStatus) // Use the locally fetched lastOptimizationStatus
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
	// Consider making the ticker duration configurable
	ticker := time.NewTicker(20 * time.Minute) // Increased from 20s for less frequent runs
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
		// This case should ideally not be hit if initialized properly
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

	isOptimizingMutex.Lock() // Use Lock for read-modify-write simulation if needed, RLock for read-only
	currentIsOptimizing := isOptimizing
	isOptimizingMutex.Unlock()

	// These variables (apiCacheSt, apiLF, apiCLU) should be obtained from api.go's state
	// For now, assuming they are correctly populated by api.go (e.g., via exported functions or shared state)
	// This part needs access to api.go's apiCacheMutex, apiFetchErr, lastAPIFetchTime, apiResponseCache
	// This implies these variables need to be exported from api.go or accessed via functions.
	// For simplicity, I'll assume they are readable here.
	// In a real modular setup, you'd call functions from the api package.
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

	if optSt == "" { // Default status if not yet set
		optSt = "Service initializing; optimization pending."
	}
	if metSt == "" { // Default status
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
		return "N/A" // Or whatever placeholder you prefer
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Simple HTML response for the root
	fmt.Fprintln(w, "<html><head><title>Optimizer Microservice</title></head><body><h1>Optimizer Microservice</h1><p>Available endpoints:</p><ul><li><a href='/status'>/status</a></li><li><a href='/optimizer_results.json'>/optimizer_results.json</a></li><li><a href='/failed_items_report.json'>/failed_items_report.json</a></li><li><a href='/healthz'>/healthz</a></li><li><a href='/debug/pprof/'>/debug/pprof/</a> (if pprof is imported)</li><li><a href='/debug/memstats'>/debug/memstats</a></li><li><a href='/debug/forcegc'>/debug/forcegc</a></li></ul></body></html>")
}

// min function is not used in this file anymore, can be removed if not used elsewhere.
// func min(a, b int) int { ... }

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
		// Validate if it's a valid JSON array of ProductMetrics before assigning
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
	go optimizePeriodically() // Ensure api.go's init or a similar mechanism starts API data fetching

	// Setup HTTP handlers
	http.HandleFunc("/optimizer_results.json", optimizerResultsHandler)
	http.HandleFunc("/failed_items_report.json", failedItemsReportHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/", rootHandler) // Root handler for basic info
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
