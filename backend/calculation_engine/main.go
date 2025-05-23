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
		return []byte("null"), nil // Marshal as JSON 'null'
	}
	return json.Marshal(val) // Marshal as a standard float
}

// Helper to convert float64 to JSONFloat64, ensuring NaN/Inf are handled.
// This is the primary helper to use when assigning to JSONFloat64 fields.
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
	megaCmdCheckInterval        = 5 * time.Minute // Not used in this file, but kept for context
	megaCmdTimeout              = 60 * time.Second
	initialMetricsDownloadDelay = 10 * time.Second
	initialOptimizationDelay    = 20 * time.Second
	timestampFormat             = "20060102150405"
)

// --- Struct Definitions ---
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"` // OptimizedItemResult is defined in optimizer.go
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

// --- Global Variables ---
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
		log.Println("downloadMetricsFromMega: MEGA environment variables not fully set. Skipping MEGA download.")
		if _, err := os.Stat(metricsFilename); !os.IsNotExist(err) {
			log.Printf("downloadMetricsFromMega: MEGA download skipped, using existing local cache '%s' if readable later.", metricsFilename)
		} else {
			return fmt.Errorf("MEGA download skipped (missing env config), and no local cache file '%s' found", metricsFilename)
		}
		return nil
	}

	targetDir := filepath.Dir(localTargetFilename)
	if targetDir != "." && targetDir != "" {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory %s for metrics: %v", targetDir, err)
		}
	}

	cmdEnv := os.Environ()
	log.Printf("Listing files in MEGA folder: %s (using 'megals' with direct auth flags)", megaRemoteFolderPath)
	ctxLs, cancelLs := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelLs()
	lsCmd := exec.CommandContext(ctxLs, "megals", "--username", megaEmail, "--password", megaPassword, "--no-ask-password", megaRemoteFolderPath)
	lsCmd.Env = cmdEnv
	lsOutBytes, lsErr := lsCmd.CombinedOutput()
	if ctxLs.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: 'megals' command timed out after %v", megaCmdTimeout)
		return fmt.Errorf("'megals' command timed out")
	}
	lsOut := string(lsOutBytes)
	log.Printf("downloadMetricsFromMega: 'megals' Output (first 500 chars): %.500s", lsOut)
	if lsErr != nil {
		return fmt.Errorf("megals command failed: %v. Output: %s", lsErr, lsOut)
	}

	var latestFilename string
	var latestTimestamp time.Time
	foundFile := false
	scanner := bufio.NewScanner(strings.NewReader(lsOut))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(strings.ToUpper(line), "WRN:") || strings.Contains(strings.ToUpper(line), "ERR:") {
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
		log.Printf("Warning: error scanning 'megals' output: %v", scanErr)
	}
	if !foundFile {
		return fmt.Errorf("no metrics file matching pattern found in '%s'", megaRemoteFolderPath)
	}

	remoteFilePathFull := strings.TrimRight(megaRemoteFolderPath, "/") + "/" + latestFilename
	log.Printf("Downloading latest metrics file '%s' from MEGA to '%s'", remoteFilePathFull, localTargetFilename)
	_ = os.Remove(localTargetFilename)
	ctxGet, cancelGet := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelGet()
	getCmd := exec.CommandContext(ctxGet, "megaget", "--username", megaEmail, "--password", megaPassword, "--no-ask-password", remoteFilePathFull, "--path", localTargetFilename)
	getCmd.Env = cmdEnv
	getOutBytes, getErr := getCmd.CombinedOutput()
	if ctxGet.Err() == context.DeadlineExceeded {
		return fmt.Errorf("'megaget' command timed out")
	}
	getOut := string(getOutBytes)
	log.Printf("downloadMetricsFromMega: 'megaget' Output (first 500 chars): %.500s", getOut)
	if getErr != nil {
		return fmt.Errorf("failed to download '%s': %v. Output: %s", remoteFilePathFull, getErr, getOut)
	}
	if _, statErr := os.Stat(localTargetFilename); os.IsNotExist(statErr) {
		return fmt.Errorf("megaget succeed but target '%s' missing. Output: %s", localTargetFilename, getOut)
	}
	log.Printf("Successfully downloaded '%s' to '%s'.", latestFilename, localTargetFilename)
	return nil
}

func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download...")
	tempMetricsFilename := metricsFilename + ".downloading.tmp"
	defer os.Remove(tempMetricsFilename)
	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadTime = time.Now()
	err := downloadMetricsFromMega(tempMetricsFilename)
	if err != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error MEGA download at %s: %v", lastMetricsDownloadTime.Format(time.RFC3339), err)
		metricsDownloadStatusMutex.Unlock()
		return
	}
	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error reading downloaded metrics %s at %s: %v", tempMetricsFilename, lastMetricsDownloadTime.Format(time.RFC3339), readErr)
		metricsDownloadStatusMutex.Unlock()
		return
	}
	if string(data) == "{}" {
		lastMetricsDownloadStatus = fmt.Sprintf("Error: downloaded metrics data was '{}' at %s.", lastMetricsDownloadTime.Format(time.RFC3339))
		metricsDownloadStatusMutex.Unlock()
		return
	}
	var tempMetricsSlice []ProductMetrics
	if jsonErr := json.Unmarshal(data, &tempMetricsSlice); jsonErr != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error: downloaded metrics data NOT VALID JSON ARRAY at %s: %v.", lastMetricsDownloadTime.Format(time.RFC3339), jsonErr)
		metricsDownloadStatusMutex.Unlock()
		return
	}
	metricsDataMutex.Lock()
	latestMetricsData = data
	metricsDataMutex.Unlock()
	if writeErr := os.WriteFile(metricsFilename, data, 0644); writeErr != nil {
		log.Printf("Warning: failed to write metrics to cache %s: %v", metricsFilename, writeErr)
	} else {
		log.Printf("Successfully wrote data (len %d) to %s", len(data), metricsFilename)
	}
	lastMetricsDownloadStatus = fmt.Sprintf("Successfully updated metrics at %s. Size: %d bytes", lastMetricsDownloadTime.Format(time.RFC3339), len(data))
	metricsDownloadStatusMutex.Unlock()
}

func downloadMetricsPeriodically() {
	go func() {
		log.Printf("downloadMetricsPeriodically: Waiting %v before initial download...", initialMetricsDownloadDelay)
		time.Sleep(initialMetricsDownloadDelay)
		downloadAndStoreMetrics()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			downloadAndStoreMetrics()
		}
	}()
}

func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) {
	if len(jsonData) == 0 {
		return make(map[string]ProductMetrics), nil
	}
	if string(jsonData) == "{}" {
		return nil, fmt.Errorf("cannot unmarshal JSON object into []ProductMetrics")
	}
	var productMetricsSlice []ProductMetrics
	if err := json.Unmarshal(jsonData, &productMetricsSlice); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics JSON: %w", err)
	}
	productMetricsMap := make(map[string]ProductMetrics, len(productMetricsSlice))
	for _, metric := range productMetricsSlice {
		if metric.ProductID == "" {
			continue
		}
		productMetricsMap[metric.ProductID] = metric
	}
	log.Printf("parseProductMetricsData: Successfully parsed %d metrics objects into %d map entries.", len(productMetricsSlice), len(productMetricsMap))
	return productMetricsMap, nil
}

func performOptimizationCycleNow(productMetrics map[string]ProductMetrics, apiResp *HypixelAPIResponse) ([]byte, []byte, error) {
	runStartTime := time.Now()
	log.Println("performOptimizationCycleNow: Starting new optimization cycle...")
	if apiResp == nil || apiResp.Products == nil {
		return nil, nil, fmt.Errorf("CRITICAL: API data nil in performOptimizationCycleNow")
	}
	if productMetrics == nil {
		return nil, nil, fmt.Errorf("CRITICAL: Product metrics nil in performOptimizationCycleNow")
	}
	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 {
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).Format(time.RFC3339Nano)
	}
	var itemIDs []string
	for id := range apiResp.Products {
		itemIDs = append(itemIDs, id)
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
	log.Printf("performOptimizationCycleNow: Params: cycleTime=%.0fs, searchQty=%.0f, chunkSize=%d, pause=%v", maxAllowedCycleTimePerItemRaw, maxInitialSearchQtyRaw, itemsPerChunk, pauseBetweenChunks)

	var allOptimizedResults []OptimizedItemResult
	for i := 0; i < len(itemIDs); i += itemsPerChunk {
		end := i + itemsPerChunk
		if end > len(itemIDs) {
			end = len(itemIDs)
		}
		currentChunkItemIDs := itemIDs[i:end]
		if len(currentChunkItemIDs) == 0 {
			continue
		}
		log.Printf("performOptimizationCycleNow: Optimizing chunk %d/%d (items %d to %d of %d)", (i/itemsPerChunk)+1, (len(itemIDs)+itemsPerChunk-1)/itemsPerChunk, i, end-1, len(itemIDs))
		chunkResults := RunFullOptimization(currentChunkItemIDs, maxAllowedCycleTimePerItemRaw, apiResp, productMetrics, itemFilesDir, maxInitialSearchQtyRaw)
		allOptimizedResults = append(allOptimizedResults, chunkResults...)
		if end < len(itemIDs) && pauseBetweenChunks > 0 {
			time.Sleep(pauseBetweenChunks)
		}
	}
	optimizedResults := allOptimizedResults
	log.Printf("performOptimizationCycleNow: Optimization complete. Generated %d results.", len(optimizedResults))

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
		RunTimestamp: runStartTime.Format(time.RFC3339Nano), APILastUpdatedTimestamp: apiLastUpdatedStr, TotalItemsConsidered: len(itemIDs),
		ItemsSuccessfullyCalculated: successCount, ItemsWithCalculationErrors: len(failDetails),
		MaxAllowedCycleTimeSecs: toJSONFloat64(maxAllowedCycleTimePerItemRaw), MaxInitialSearchQuantity: toJSONFloat64(maxInitialSearchQtyRaw),
	}
	mainOutput := OptimizationRunOutput{Summary: summary, Results: optimizedResults}

	mainJSON, err := json.MarshalIndent(mainOutput, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("CRITICAL: Failed to marshal main optimization output: %w. Check for 'PROBLEM_NAN_FOUND' in logs.", err)
	}
	var failedJSON []byte
	if len(failDetails) > 0 {
		if b, errMarshal := json.MarshalIndent(failDetails, "", "  "); errMarshal != nil {
			failedJSON = []byte(`[{"error":"failed to marshal failed items"}]`)
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
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Opt skipped at %s: Metrics data not ready/empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}
	if string(currentMetricsBytes) == "{}" {
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Opt skipped at %s: Metrics data was '{}'.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}
	productMetrics, parseErr := parseProductMetricsData(currentMetricsBytes)
	if parseErr != nil {
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Opt skipped at %s: Metrics parsing failed: %v", time.Now().Format(time.RFC3339), parseErr)
		optimizationStatusMutex.Unlock()
		return
	}
	if len(productMetrics) == 0 {
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Opt skipped at %s: Parsed metrics map empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}
	apiResp, apiErr := getApiResponse()
	if apiErr != nil {
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Opt skipped at %s: API data load failed: %v", time.Now().Format(time.RFC3339), apiErr)
		optimizationStatusMutex.Unlock()
		return
	}
	if apiResp == nil || len(apiResp.Products) == 0 {
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Opt skipped at %s: API response nil/no products.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
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
		lastOptimizationStatus = fmt.Sprintf("Successfully optimized at %s. Results: %d bytes, Failed: %d bytes", lastOptimizationTime.Format(time.RFC3339), mainLen, failLen)
	}
	optimizationStatusMutex.Unlock()
	log.Printf("runSingleOptimizationAndUpdateResults: Cycle finished. Error: %v. Main JSON: %d bytes, Failed JSON: %d bytes. Status: %s", optErr, mainLen, failLen, lastOptimizationStatus)
}

func optimizePeriodically() {
	go func() {
		log.Printf("optimizePeriodically: Waiting %v before initial optimization...", initialOptimizationDelay)
		time.Sleep(initialOptimizationDelay)
		isOptimizingMutex.Lock()
		if isOptimizing {
			isOptimizingMutex.Unlock()
			return
		}
		isOptimizing = true
		isOptimizingMutex.Unlock()
		runSingleOptimizationAndUpdateResults()
		isOptimizingMutex.Lock()
		isOptimizing = false
		isOptimizingMutex.Unlock()
		log.Println("optimizePeriodically: Initial optimization run finished.")
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
			log.Println("optimizePeriodically (tick): Previous optimization still in progress.")
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
		http.Error(w, `{"summary":{"message":"Optimizer results not yet available"},"results":[]}`, http.StatusServiceUnavailable)
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
	optSt, optT := lastOptimizationStatus, lastOptimizationTime
	optimizationStatusMutex.RUnlock()
	metricsDownloadStatusMutex.RLock()
	metSt, metT := lastMetricsDownloadStatus, lastMetricsDownloadTime
	metricsDownloadStatusMutex.RUnlock()
	isOptimizingMutex.Lock()
	currentIsOptimizing := isOptimizing
	isOptimizingMutex.Unlock()
	if optSt == "" {
		optSt = "Service initializing; optimization pending."
	}
	if metSt == "" {
		metSt = "Service initializing; metrics download pending."
	}
	resp := map[string]interface{}{
		"service_status": "active", "current_utc_time": time.Now().UTC().Format(time.RFC3339Nano),
		"optimization_process_status": optSt, "last_optimization_attempt_utc": formatTimeIfNotZero(optT),
		"metrics_download_process_status": metSt, "last_metrics_download_attempt_utc": formatTimeIfNotZero(metT),
		"is_currently_optimizing": currentIsOptimizing,
	}
	b, err := json.Marshal(resp)
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
	fmt.Fprintln(w, "Optimizer Microservice - /status, /optimizer_results.json, /failed_items_report.json")
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
		} else {
			latestMetricsData = []byte("[]")
		}
	} else {
		latestMetricsData = []byte("[]")
	}
	metricsDataMutex.Unlock()

	go downloadMetricsPeriodically()
	go optimizePeriodically()

	http.HandleFunc("/optimizer_results.json", optimizerResultsHandler)
	http.HandleFunc("/failed_items_report.json", failedItemsReportHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write([]byte("OK")) })
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := "0.0.0.0:" + port
	log.Printf("Main: Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server on %s: %v", addr, err)
	}
}
