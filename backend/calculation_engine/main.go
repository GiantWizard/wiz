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
	megaLsCmd                   = "/usr/local/bin/megals"
	megaGetCmd                  = "/usr/local/bin/megaget"
)

// --- Struct Definitions for main.go orchestration ---
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"` // Expects OptimizedItemResult from optimizer.go
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
// Global variables for API cache (apiResponseCache, etc.) are in api.go
var (
	latestOptimizerResultsJSON []byte
	optimizerResultsMutex      sync.RWMutex

	latestFailedItemsJSON []byte
	failedItemsMutex      sync.RWMutex

	lastOptimizationStatus  string
	lastOptimizationTime    time.Time
	optimizationStatusMutex sync.RWMutex

	latestMetricsData          []byte
	metricsDataMutex           sync.RWMutex
	lastMetricsDownloadStatus  string
	lastMetricsDownloadTime    time.Time
	metricsDownloadStatusMutex sync.RWMutex

	isOptimizing      bool
	isOptimizingMutex sync.Mutex

	metricsFileRegex = regexp.MustCompile(`^metrics_(\d{14})\.json$`)
)

func downloadMetricsFromMega(localTargetFilename string) error {
	megaEmail := os.Getenv("MEGA_EMAIL")
	megaPassword := os.Getenv("MEGA_PWD")
	megaRemoteFolderPath := os.Getenv("MEGA_METRICS_FOLDER_PATH")

	if megaEmail == "" || megaPassword == "" || megaRemoteFolderPath == "" {
		log.Println("downloadMetricsFromMega: MEGA environment variables (MEGA_EMAIL, MEGA_PWD, MEGA_METRICS_FOLDER_PATH) not fully set. Skipping MEGA download.")
		if _, err := os.Stat(metricsFilename); !os.IsNotExist(err) {
			log.Printf("downloadMetricsFromMega: Using existing local cache '%s' as fallback.", metricsFilename)
			data, readErr := os.ReadFile(metricsFilename)
			if readErr != nil {
				return fmt.Errorf("MEGA download skipped and failed to read local cache '%s': %v", metricsFilename, readErr)
			}
			if writeErr := os.WriteFile(localTargetFilename, data, 0644); writeErr != nil {
				return fmt.Errorf("MEGA download skipped, read local cache but failed to write to target '%s': %v", localTargetFilename, writeErr)
			}
			log.Printf("downloadMetricsFromMega: Successfully used local cache for '%s'.", localTargetFilename)
			return nil
		}
		return fmt.Errorf("MEGA download skipped (missing env config), and no local cache '%s' found", metricsFilename)
	}

	targetDir := filepath.Dir(localTargetFilename)
	if targetDir != "." && targetDir != "" {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory %s for metrics: %v", targetDir, err)
		}
	}

	// --- Debugging file existence and PATH ---
	log.Printf("Debug: Checking for command '%s'...", megaLsCmd)
	if _, err := os.Stat(megaLsCmd); os.IsNotExist(err) {
		log.Printf("Debug: Command '%s' does NOT exist at specified path.", megaLsCmd)
		// Check common alternative if specific copy failed
		altPath := "/usr/bin/megals"
		if _, err2 := os.Stat(altPath); os.IsNotExist(err2) {
			log.Printf("Debug: Alternative path '%s' also does NOT exist.", altPath)
		} else {
			log.Printf("Debug: Alternative path '%s' DOES exist. Consider updating megaLsCmd constant.", altPath)
		}
		// Log current PATH for the Go process
		currentPath := os.Getenv("PATH")
		log.Printf("Debug: Current PATH for Go process: %s", currentPath)
		// Try `which` equivalent
		foundPath, lookErr := exec.LookPath("megals")
		if lookErr != nil {
			log.Printf("Debug: 'megals' not found in PATH by exec.LookPath: %v", lookErr)
		} else {
			log.Printf("Debug: 'megals' found by exec.LookPath at: %s", foundPath)
		}
		return fmt.Errorf("megals command '%s' not found at specified path (os.Stat failed)", megaLsCmd)
	} else if err != nil {
		log.Printf("Debug: Error stating '%s': %v", megaLsCmd, err) // Other error like permission denied
		return fmt.Errorf("error stating megals command '%s': %v", megaLsCmd, err)
	} else {
		log.Printf("Debug: Command '%s' found. Proceeding with execution.", megaLsCmd)
	}
	// --- End Debugging ---

	log.Printf("Listing files in MEGA folder: %s (using '%s')", megaRemoteFolderPath, megaLsCmd)
	ctxLs, cancelLs := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelLs()

	lsCmd := exec.CommandContext(ctxLs, megaLsCmd, "--username", megaEmail, "--password", megaPassword, "--no-ask-password", megaRemoteFolderPath)
	lsOutBytes, lsErr := lsCmd.CombinedOutput()

	if ctxLs.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: '%s' command timed out after %v. Output: %s", megaLsCmd, megaCmdTimeout, string(lsOutBytes))
		return fmt.Errorf("'%s' command timed out. Output: %s", megaLsCmd, string(lsOutBytes))
	}
	lsOut := string(lsOutBytes)
	if len(lsOut) > 1000 {
		log.Printf("downloadMetricsFromMega: '%s' Output (first 1000 chars of %d): %.1000s...", megaLsCmd, len(lsOut), lsOut)
	} else {
		log.Printf("downloadMetricsFromMega: '%s' Output: %s", megaLsCmd, lsOut)
	}

	if lsErr != nil {
		if exitErr, ok := lsErr.(*exec.ExitError); ok {
			log.Printf("'%s' command exited with error: %v. Stderr: %s", megaLsCmd, lsErr, string(exitErr.Stderr))
			return fmt.Errorf("'%s' command failed: %v. Stderr: %s. Full Output: %s", megaLsCmd, lsErr, string(exitErr.Stderr), lsOut)
		}
		return fmt.Errorf("'%s' command failed: %v. Output: %s", megaLsCmd, lsErr, lsOut)
	}
	if strings.Contains(lsOut, "Unable to connect to service") || strings.Contains(lsOut, "Please ensure mega-cmd-server is running") {
		return fmt.Errorf("'%s' output indicates MEGAcmd server issue: %s", megaLsCmd, lsOut)
	}

	var latestFilename string
	var latestTimestamp time.Time
	foundFile := false

	scanner := bufio.NewScanner(strings.NewReader(lsOut))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToUpper(line), "WRN:") || strings.Contains(strings.ToUpper(line), "ERR:") || strings.Contains(line, "[Initiating MEGAcmd server") {
			log.Printf("MEGA LS Info/Warning/ServerMsg: %s", line)
			continue
		}
		filenameToParse := filepath.Base(line)
		match := metricsFileRegex.FindStringSubmatch(filenameToParse)
		if len(match) == 2 {
			timestampStr := match[1]
			t, parseErr := time.Parse(timestampFormat, timestampStr)
			if parseErr != nil {
				log.Printf("Warning: could not parse timestamp in filename '%s': %v", filenameToParse, parseErr)
				continue
			}
			if !foundFile || t.After(latestTimestamp) {
				foundFile = true
				latestTimestamp = t
				latestFilename = filenameToParse
			}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		log.Printf("Warning: error scanning '%s' output: %v", megaLsCmd, scanErr)
	}

	if !foundFile {
		return fmt.Errorf("no metrics file matching pattern '%s' found in MEGA folder '%s'. Review 'megals' output above.", metricsFileRegex.String(), megaRemoteFolderPath)
	}

	remoteFilePathFull := strings.TrimRight(megaRemoteFolderPath, "/") + "/" + latestFilename
	log.Printf("Identified latest metrics file: '%s'", remoteFilePathFull)

	tempDownloadPath := filepath.Join(targetDir, latestFilename)
	if localTargetFilename == tempDownloadPath {
		_ = os.Remove(localTargetFilename)
	} else {
		_ = os.Remove(tempDownloadPath)
	}

	log.Printf("Downloading '%s' from MEGA to temp path '%s' (will be moved to '%s')", remoteFilePathFull, tempDownloadPath, localTargetFilename)
	ctxGet, cancelGet := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelGet()

	getCmd := exec.CommandContext(ctxGet, megaGetCmd, "--username", megaEmail, "--password", megaPassword, "--no-ask-password", remoteFilePathFull, "--path", targetDir)
	getOutBytes, getErr := getCmd.CombinedOutput()

	if ctxGet.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: '%s' command timed out for %s after %v. Output: %s", megaGetCmd, latestFilename, megaCmdTimeout, string(getOutBytes))
		return fmt.Errorf("'%s' command timed out for %s. Output: %s", megaGetCmd, latestFilename, string(getOutBytes))
	}
	getOut := string(getOutBytes)
	log.Printf("downloadMetricsFromMega: '%s' Output (first 500 chars): %.500s", megaGetCmd, getOut)
	if getErr != nil {
		if exitErr, ok := getErr.(*exec.ExitError); ok {
			log.Printf("'%s' command exited with error: %v. Stderr: %s", megaGetCmd, getErr, string(exitErr.Stderr))
			return fmt.Errorf("failed to download '%s' with '%s': %v. Stderr: %s. Full Output: %s", remoteFilePathFull, megaGetCmd, getErr, string(exitErr.Stderr), getOut)
		}
		return fmt.Errorf("failed to download '%s' with '%s': %v. Output: %s", remoteFilePathFull, megaGetCmd, getErr, getOut)
	}
	if strings.Contains(getOut, "Unable to connect to service") || strings.Contains(getOut, "Please ensure mega-cmd-server is running") {
		return fmt.Errorf("'%s' output indicates MEGAcmd server issue during get: %s", megaGetCmd, getOut)
	}

	if _, statErr := os.Stat(tempDownloadPath); os.IsNotExist(statErr) {
		return fmt.Errorf("'%s' command appeared to succeed for '%s', but downloaded file '%s' is missing. Output: %s", megaGetCmd, latestFilename, tempDownloadPath, getOut)
	}

	if localTargetFilename != tempDownloadPath {
		log.Printf("Moving downloaded file from '%s' to '%s'", tempDownloadPath, localTargetFilename)
		if err := os.Rename(tempDownloadPath, localTargetFilename); err != nil {
			_ = os.Remove(tempDownloadPath)
			return fmt.Errorf("failed to move downloaded file from '%s' to '%s': %w", tempDownloadPath, localTargetFilename, err)
		}
	}
	log.Printf("Successfully downloaded and prepared metrics file at '%s'.", localTargetFilename)
	return nil
}

func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download...")
	tempMetricsFilename := filepath.Join(os.TempDir(), fmt.Sprintf("metrics_%d.downloading.tmp", time.Now().UnixNano()))
	defer os.Remove(tempMetricsFilename)

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadTime = time.Now()
	downloadErr := downloadMetricsFromMega(tempMetricsFilename)
	metricsDownloadStatusMutex.Unlock()

	if downloadErr != nil {
		newStatus := fmt.Sprintf("Error during MEGA download at %s: %v", lastMetricsDownloadTime.Format(time.RFC3339), downloadErr)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		newStatus := fmt.Sprintf("Error reading downloaded metrics file %s at %s: %v", tempMetricsFilename, lastMetricsDownloadTime.Format(time.RFC3339), readErr)
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	if len(data) == 0 {
		newStatus := fmt.Sprintf("Error: downloaded metrics data from %s was empty at %s.", tempMetricsFilename, lastMetricsDownloadTime.Format(time.RFC3339))
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	if string(data) == "{}" {
		newStatus := fmt.Sprintf("Error: downloaded metrics data was '{}' at %s.", lastMetricsDownloadTime.Format(time.RFC3339))
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	var tempMetricsSlice []ProductMetrics // Expects ProductMetrics from metrics.go
	if jsonErr := json.Unmarshal(data, &tempMetricsSlice); jsonErr != nil {
		newStatus := fmt.Sprintf("Error: downloaded metrics data was NOT A VALID JSON ARRAY of ProductMetrics at %s: %v. Body(first 200): %.200s", lastMetricsDownloadTime.Format(time.RFC3339), jsonErr, string(data))
		metricsDownloadStatusMutex.Lock()
		lastMetricsDownloadStatus = newStatus
		metricsDownloadStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}

	metricsDataMutex.Lock()
	latestMetricsData = data
	metricsDataMutex.Unlock()

	if writeErr := os.WriteFile(metricsFilename, data, 0644); writeErr != nil {
		log.Printf("Warning: failed to write metrics to permanent cache file %s: %v. In-memory cache IS updated.", metricsFilename, writeErr)
	} else {
		log.Printf("Successfully wrote new metrics data (len %d) to %s", len(data), metricsFilename)
	}

	newStatus := fmt.Sprintf("Successfully downloaded and updated metrics at %s. Size: %d bytes", lastMetricsDownloadTime.Format(time.RFC3339), len(data))
	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = newStatus
	metricsDownloadStatusMutex.Unlock()
	log.Println(newStatus)
}

func downloadMetricsPeriodically() {
	go func() {
		log.Printf("downloadMetricsPeriodically: Waiting %v before initial download...", initialMetricsDownloadDelay)
		time.Sleep(initialMetricsDownloadDelay)
		downloadAndStoreMetrics()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("downloadMetricsPeriodically: Triggering periodic metrics download.")
			downloadAndStoreMetrics()
		}
	}()
}

func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) { // Expects ProductMetrics from metrics.go
	if len(jsonData) == 0 {
		log.Println("parseProductMetricsData: jsonData is empty, returning empty map.")
		return make(map[string]ProductMetrics), nil
	}
	if string(jsonData) == "{}" {
		log.Println("parseProductMetricsData: jsonData is '{}', which cannot be unmarshaled into []ProductMetrics.")
		return nil, fmt.Errorf("cannot unmarshal JSON object into []ProductMetrics")
	}
	var productMetricsSlice []ProductMetrics
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
		if metric.ProductID == "" {
			skippedCount++
			continue
		}
		normalizedID := BAZAAR_ID(metric.ProductID) // Expects BAZAAR_ID from utils.go
		metric.ProductID = normalizedID
		productMetricsMap[normalizedID] = metric
	}
	if skippedCount > 0 {
		log.Printf("parseProductMetricsData: Skipped %d metric entries due to empty ProductID.", skippedCount)
	}
	log.Printf("parseProductMetricsData: Successfully parsed %d metrics objects into %d map entries.", len(productMetricsSlice), len(productMetricsMap))
	return productMetricsMap, nil
}

func performOptimizationCycleNow(productMetrics map[string]ProductMetrics, apiResp *HypixelAPIResponse) ([]byte, []byte, error) { // Expects types from metrics.go and api.go
	runStartTime := time.Now()
	log.Println("performOptimizationCycleNow: Starting new optimization cycle...")
	if apiResp == nil || apiResp.Products == nil {
		return nil, nil, fmt.Errorf("CRITICAL: API data is nil or has no products in performOptimizationCycleNow")
	}
	if productMetrics == nil {
		return nil, nil, fmt.Errorf("CRITICAL: Product metrics map is nil in performOptimizationCycleNow")
	}
	if len(productMetrics) == 0 && len(apiResp.Products) > 0 {
		log.Println("performOptimizationCycleNow: Product metrics map is empty, but API has products. This may lead to limited optimization.")
	}
	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 {
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).UTC().Format(time.RFC3339Nano)
	}
	var itemIDs []string
	for id := range apiResp.Products {
		itemIDs = append(itemIDs, id)
	}
	if len(itemIDs) == 0 {
		log.Println("performOptimizationCycleNow: No item IDs from API response to optimize.")
		emptySummary := OptimizationSummary{RunTimestamp: runStartTime.Format(time.RFC3339Nano), APILastUpdatedTimestamp: apiLastUpdatedStr, TotalItemsConsidered: 0, ItemsSuccessfullyCalculated: 0, ItemsWithCalculationErrors: 0, MaxAllowedCycleTimeSecs: toJSONFloat64(0), MaxInitialSearchQuantity: toJSONFloat64(0)}
		emptyOutput := OptimizationRunOutput{Summary: emptySummary, Results: []OptimizedItemResult{}}
		mainJSON, _ := json.MarshalIndent(emptyOutput, "", "  ")
		return mainJSON, []byte("[]"), nil
	}
	itemsPerChunk := 50
	if ipcStr := os.Getenv("ITEMS_PER_CHUNK"); ipcStr != "" {
		if val, err := strconv.Atoi(ipcStr); err == nil && val > 0 {
			itemsPerChunk = val
		}
	}
	pauseBetweenChunks := 500 * time.Millisecond
	if pbcStr := os.Getenv("PAUSE_MS_BETWEEN_CHUNKS"); pbcStr != "" {
		if val, err := strconv.Atoi(pbcStr); err == nil && val >= 0 {
			pauseBetweenChunks = time.Duration(val) * time.Millisecond
		}
	}
	const (
		maxAllowedCycleTimePerItemRaw = 3600.0
		maxInitialSearchQtyRaw        = 1000000.0
	)
	log.Printf("performOptimizationCycleNow: Parameters - MaxCycleTime: %.0fs, MaxInitialSearchQty: %.0f, ChunkSize: %d, Pause: %v", maxAllowedCycleTimePerItemRaw, maxInitialSearchQtyRaw, itemsPerChunk, pauseBetweenChunks)
	var allOptimizedResults []OptimizedItemResult // Expects OptimizedItemResult from optimizer.go
	for i := 0; i < len(itemIDs); i += itemsPerChunk {
		end := i + itemsPerChunk
		if end > len(itemIDs) {
			end = len(itemIDs)
		}
		currentChunkItemIDs := itemIDs[i:end]
		if len(currentChunkItemIDs) == 0 {
			continue
		}
		log.Printf("performOptimizationCycleNow: Optimizing chunk %d/%d (items %d to %d of %d total)", (i/itemsPerChunk)+1, (len(itemIDs)+itemsPerChunk-1)/itemsPerChunk, i, end-1, len(itemIDs))
		chunkResults := RunFullOptimization(currentChunkItemIDs, maxAllowedCycleTimePerItemRaw, apiResp, productMetrics, itemFilesDir, maxInitialSearchQtyRaw) // Expects RunFullOptimization from optimizer.go
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
		if r.CalculationPossible {
			successCount++
		} else {
			failDetails = append(failDetails, FailedItemDetail{ItemName: r.ItemName, ErrorMessage: r.ErrorMessage})
		}
	}
	summary := OptimizationSummary{RunTimestamp: runStartTime.Format(time.RFC3339Nano), APILastUpdatedTimestamp: apiLastUpdatedStr, TotalItemsConsidered: len(itemIDs), ItemsSuccessfullyCalculated: successCount, ItemsWithCalculationErrors: len(failDetails), MaxAllowedCycleTimeSecs: toJSONFloat64(maxAllowedCycleTimePerItemRaw), MaxInitialSearchQuantity: toJSONFloat64(maxInitialSearchQtyRaw)}
	mainOutput := OptimizationRunOutput{Summary: summary, Results: optimizedResults}
	mainJSON, err := json.MarshalIndent(mainOutput, "", "  ")
	if err != nil {
		log.Printf("CRITICAL: Failed to marshal main optimization output: %v.", err)
		return nil, nil, fmt.Errorf("CRITICAL: Failed to marshal main optimization output: %w", err)
	}
	var failedJSON []byte
	if len(failDetails) > 0 {
		if b, errMarshal := json.MarshalIndent(failDetails, "", "  "); errMarshal != nil {
			log.Printf("Error: Failed to marshal failed items report: %v", errMarshal)
			failedJSON = []byte(`[{"error":"failed to marshal failed items report"}]`)
		} else {
			failedJSON = b
		}
	} else {
		failedJSON = []byte("[]")
	}
	return mainJSON, failedJSON, nil
}

func runSingleOptimizationAndUpdateResults() {
	log.Println("runSingleOptimizationAndUpdateResults: Initiating new optimization process...")
	metricsDataMutex.RLock()
	currentMetricsBytes := make([]byte, len(latestMetricsData))
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
	if len(productMetrics) == 0 {
		newStatus := fmt.Sprintf("Optimization skipped at %s: Parsed metrics map is empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = newStatus
		optimizationStatusMutex.Unlock()
		log.Println(newStatus)
		return
	}
	apiResp, apiErr := getApiResponse() // Expects getApiResponse from api.go
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

func optimizePeriodically() {
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
	ticker := time.NewTicker(20 * time.Second)
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
			go func(tickTime time.Time) {
				defer func() {
					isOptimizingMutex.Lock()
					isOptimizing = false
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

func optimizerResultsHandler(w http.ResponseWriter, r *http.Request) {
	optimizerResultsMutex.RLock()
	data := latestOptimizerResultsJSON
	optimizerResultsMutex.RUnlock()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if data == nil {
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
	if data == nil {
		w.Write([]byte("[]"))
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
	apiCacheMutex.RLock()
	apiCacheSt := "OK"
	if apiFetchErr != nil {
		apiCacheSt = fmt.Sprintf("Error: %v", apiFetchErr)
	}
	apiLF := lastAPIFetchTime
	apiCLU := int64(0)
	if apiResponseCache != nil {
		apiCLU = apiResponseCache.LastUpdated
	}
	apiCacheMutex.RUnlock()
	if optSt == "" {
		optSt = "Service initializing; optimization pending."
	}
	if metSt == "" {
		metSt = "Service initializing; metrics download pending."
	}
	resp := map[string]interface{}{"service_status": "active", "current_utc_time": time.Now().UTC().Format(time.RFC3339Nano), "optimization_process_status": optSt, "last_optimization_attempt_utc": formatTimeIfNotZero(optT), "metrics_download_process_status": metSt, "last_metrics_download_attempt_utc": formatTimeIfNotZero(metT), "is_currently_optimizing": currentIsOptimizing, "hypixel_api_cache_status": apiCacheSt, "hypixel_api_last_successful_fetch_utc": formatTimeIfNotZero(apiLF), "hypixel_api_data_last_updated_epoch_ms": apiCLU}
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
	fmt.Fprintln(w, "<html><head><title>Optimizer Microservice</title></head><body><h1>Optimizer Microservice</h1><p>Available endpoints:</p><ul><li><a href='/status'>/status</a></li><li><a href='/optimizer_results.json'>/optimizer_results.json</a></li><li><a href='/failed_items_report.json'>/failed_items_report.json</a></li><li><a href='/healthz'>/healthz</a></li><li><a href='/debug/pprof/'>/debug/pprof/</a></li><li><a href='/debug/memstats'>/debug/memstats</a></li><li><a href='/debug/forcegc'>/debug/forcegc</a></li></ul></body></html>")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
} // Not essential for current core logic

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("Main: Optimizer service starting...")
	optimizationStatusMutex.Lock()
	lastOptimizationStatus = "Service starting; initial optimization pending."
	optimizationStatusMutex.Unlock()
	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = "Service starting; initial metrics download pending."
	metricsDownloadStatusMutex.Unlock()
	optimizerResultsMutex.Lock()
	latestOptimizerResultsJSON = []byte(`{"summary":{"run_timestamp":"N/A","total_items_considered":0,"items_successfully_calculated":0,"items_with_calculation_errors":0,"max_allowed_cycle_time_seconds":null,"max_initial_search_quantity":null,"message":"Initializing..."},"results":[]}`)
	optimizerResultsMutex.Unlock()
	failedItemsMutex.Lock()
	latestFailedItemsJSON = []byte("[]")
	failedItemsMutex.Unlock()
	metricsDataMutex.Lock()
	initialMetricsBytes, err := os.ReadFile(metricsFilename)
	if err == nil && len(initialMetricsBytes) > 0 {
		var tempMetricsSlice []ProductMetrics
		if parseErr := json.Unmarshal(initialMetricsBytes, &tempMetricsSlice); parseErr == nil {
			latestMetricsData = initialMetricsBytes
			log.Printf("Main: Successfully loaded initial metrics from %s (%d bytes)", metricsFilename, len(initialMetricsBytes))
		} else {
			log.Printf("Main: Found %s, but it's not a valid JSON array (%v). Initializing metrics as empty.", metricsFilename, parseErr)
			latestMetricsData = []byte("[]")
		}
	} else {
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Main: Error reading initial metrics file %s: %v. Initializing as empty.", metricsFilename, err)
		} else {
			log.Printf("Main: Initial metrics file %s not found or empty. Initializing as empty.", metricsFilename)
		}
		latestMetricsData = []byte("[]")
	}
	metricsDataMutex.Unlock()

	go downloadMetricsPeriodically()
	go optimizePeriodically()

	http.HandleFunc("/optimizer_results.json", optimizerResultsHandler)
	http.HandleFunc("/failed_items_report.json", failedItemsReportHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write([]byte("OK\n")) })
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	} // Default to 9000
	addr := "0.0.0.0:" + port
	log.Printf("Main: Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server on %s: %v", addr, err)
	}
}

// Ensure these types are defined in their respective files (api.go, metrics.go, optimizer.go, utils.go)
// and not duplicated in main.go
//
// In api.go:
// type OrderSummary struct { ... }
// type QuickStatus struct { ... }
// type HypixelProduct struct { ... }
// type HypixelAPIResponse struct { ... }
// var apiResponseCache *HypixelAPIResponse
// var apiFetchErr error
// var apiCacheMutex sync.RWMutex
// var lastAPIFetchTime time.Time
// func fetchBazaarData() error { ... } // Make sure this is correctly defined in api.go
// func getApiResponse() (*HypixelAPIResponse, error) { ... } // Make sure this is correctly defined in api.go
//
// In metrics.go:
// type ProductMetrics struct { ... }
//
// In optimizer.go:
// type OptimizedItemResult struct { ... } // Make sure fields match usage in main.go
// func RunFullOptimization(...) []OptimizedItemResult { ... }
//
// In utils.go:
// func BAZAAR_ID(id string) string { ... }
// func dlog(format string, args ...interface{}) { ... }
