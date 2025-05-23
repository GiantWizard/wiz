package main

import (
	"bufio"
	"context" // Added for command timeout
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // For profiling
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	metricsFilename      = "latest_metrics.json"
	itemFilesDir         = "dependencies/items"
	megaCmdCheckInterval = 5 * time.Minute
	megaCmdTimeout       = 60 * time.Second // Timeout for MEGA CLI commands
)

// Assume external definitions for ProductMetrics, OptimizedItemResult, HypixelAPIResponse, etc.

type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"`
}

type OptimizationSummary struct {
	RunTimestamp                string  `json:"run_timestamp"`
	APILastUpdatedTimestamp     string  `json:"api_last_updated_timestamp,omitempty"`
	TotalItemsConsidered        int     `json:"total_items_considered"`
	ItemsSuccessfullyCalculated int     `json:"items_successfully_calculated"`
	ItemsWithCalculationErrors  int     `json:"items_with_calculation_errors"`
	MaxAllowedCycleTimeSecs     float64 `json:"max_allowed_cycle_time_seconds"`
	MaxInitialSearchQuantity    float64 `json:"max_initial_search_quantity"`
}

type FailedItemDetail struct {
	ItemName     string `json:"item_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

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

const timestampFormat = "20060102150405"

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
	// cmdEnv = append(cmdEnv, "HOME=/app") // .megarc is now a fallback, direct flags preferred.

	// .megarc creation (now a fallback, can be removed if direct flags are always used and work)
	/*
		homeDir := "/app"
		megarcPath := filepath.Join(homeDir, ".megarc")
		if _, err := os.Stat(megarcPath); os.IsNotExist(err) {
			log.Printf(".megarc not found at %s, attempting to create (as fallback).", megarcPath)
			megarcContent := fmt.Sprintf("[Login]\nUsername = %s\nPassword = %s\n", megaEmail, megaPassword)
			if errWrite := os.WriteFile(megarcPath, []byte(megarcContent), 0600); errWrite != nil {
				log.Printf("Warning: failed to write .megarc file to %s: %v. Relying on direct flags.", megarcPath, errWrite)
			} else {
				log.Printf("Successfully wrote .megarc file to %s.", megarcPath)
			}
		}
	*/

	log.Printf("Listing files in MEGA folder: %s (using 'megals' with direct auth flags)", megaRemoteFolderPath)

	ctxLs, cancelLs := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelLs()

	lsCmd := exec.CommandContext(ctxLs, "megals",
		"--username", megaEmail,
		"--password", megaPassword,
		"--no-ask-password",
		megaRemoteFolderPath)
	lsCmd.Env = cmdEnv

	log.Printf("downloadMetricsFromMega: PREPARING TO EXECUTE: megals --username **** --password **** --no-ask-password %s (with timeout %v)", megaRemoteFolderPath, megaCmdTimeout)
	lsOutBytes, lsErr := lsCmd.CombinedOutput()
	log.Printf("downloadMetricsFromMega: 'megals' EXECUTION FINISHED. Error: %v", lsErr)

	if ctxLs.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: 'megals' command timed out after %v", megaCmdTimeout)
		return fmt.Errorf("'megals' command timed out")
	}

	lsOut := string(lsOutBytes)
	log.Printf("downloadMetricsFromMega: 'megals' Output (first 500 chars): %.500s", lsOut)

	if lsErr != nil {
		log.Printf("ERROR executing 'megals %s': %v. Full Output: %s", megaRemoteFolderPath, lsErr, lsOut)
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
				log.Printf("Warning: could not parse timestamp in filename '%s' (from '%s'): %v", filenameToParse, line, parseErr)
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
		log.Printf("No metrics file matching pattern found via megals in '%s'. 'megals' output was: %.500s", megaRemoteFolderPath, lsOut)
		return fmt.Errorf("no metrics file matching pattern found in '%s' via megals (ls command succeeded but found no matching files or output was empty/irrelevant)", megaRemoteFolderPath)
	}

	remoteFilePathFull := strings.TrimRight(megaRemoteFolderPath, "/") + "/" + latestFilename
	log.Printf("Downloading latest metrics file '%s' from MEGA to '%s' (using 'megaget' with direct auth flags)...", remoteFilePathFull, localTargetFilename)
	_ = os.Remove(localTargetFilename)

	ctxGet, cancelGet := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelGet()

	// IMPORTANT: VERIFY 'megaget' flags with 'megaget --help'
	// Assuming it uses similar flags: --username, --password
	getCmd := exec.CommandContext(ctxGet, "megaget",
		"--username", megaEmail,
		"--password", megaPassword,
		"--no-ask-password",
		remoteFilePathFull,
		"--path", localTargetFilename)
	getCmd.Env = cmdEnv

	log.Printf("downloadMetricsFromMega: PREPARING TO EXECUTE: megaget --username **** --password **** --no-ask-password %s --path %s (with timeout %v)", remoteFilePathFull, localTargetFilename, megaCmdTimeout)
	getOutBytes, getErr := getCmd.CombinedOutput()
	log.Printf("downloadMetricsFromMega: 'megaget' EXECUTION FINISHED. Error: %v", getErr)

	if ctxGet.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: 'megaget' command timed out after %v", megaCmdTimeout)
		return fmt.Errorf("'megaget' command timed out")
	}

	getOut := string(getOutBytes)
	log.Printf("downloadMetricsFromMega: 'megaget' Output (first 500 chars): %.500s", getOut)

	if getErr != nil {
		log.Printf("ERROR executing 'megaget': %v. Full Output: %s", getErr, getOut)
		return fmt.Errorf("failed to download '%s' to '%s' with megaget: %v. Output: %s", remoteFilePathFull, localTargetFilename, getErr, getOut)
	}
	if _, statErr := os.Stat(localTargetFilename); os.IsNotExist(statErr) {
		return fmt.Errorf("megaget command appeared to succeed but target file '%s' is missing. 'megaget' output: %s", localTargetFilename, getOut)
	}

	log.Printf("Successfully downloaded '%s' from MEGA to '%s'.", latestFilename, localTargetFilename)
	return nil
}

func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download from MEGA...")
	tempMetricsFilename := metricsFilename + ".downloading.tmp"
	defer os.Remove(tempMetricsFilename)

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadTime = time.Now()
	err := downloadMetricsFromMega(tempMetricsFilename)

	if err != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error during MEGA download at %s: %v", lastMetricsDownloadTime.Format(time.RFC3339), err)
		log.Printf("downloadAndStoreMetrics: %s. latestMetricsData REMAINS UNCHANGED.", lastMetricsDownloadStatus)
		metricsDownloadStatusMutex.Unlock()
		return
	}

	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error reading downloaded metrics file %s at %s: %v", tempMetricsFilename, lastMetricsDownloadTime.Format(time.RFC3339), readErr)
		log.Printf("downloadAndStoreMetrics: %s. latestMetricsData REMAINS UNCHANGED.", lastMetricsDownloadStatus)
		metricsDownloadStatusMutex.Unlock()
		return
	}

	log.Printf("downloadAndStoreMetrics: Downloaded data from MEGA/temp file (len %d) for validation: '%.100s'", len(data), string(data))

	if string(data) == "{}" {
		lastMetricsDownloadStatus = fmt.Sprintf("Error: downloaded metrics data from MEGA was literally '{}' at %s. This is invalid for an array.", lastMetricsDownloadTime.Format(time.RFC3339))
		log.Printf("downloadAndStoreMetrics: %s. latestMetricsData REMAINS UNCHANGED.", lastMetricsDownloadStatus)
		metricsDownloadStatusMutex.Unlock()
		return
	}

	var tempMetricsSlice []ProductMetrics
	if jsonErr := json.Unmarshal(data, &tempMetricsSlice); jsonErr != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error: downloaded metrics data from MEGA is NOT A VALID JSON ARRAY at %s: %v. Data(start): %.100s",
			lastMetricsDownloadTime.Format(time.RFC3339), jsonErr, string(data))
		log.Printf("downloadAndStoreMetrics: %s. latestMetricsData REMAINS UNCHANGED.", lastMetricsDownloadStatus)
		metricsDownloadStatusMutex.Unlock()
		return
	}

	log.Printf("downloadAndStoreMetrics: Downloaded data IS a valid JSON array. Proceeding to update latestMetricsData.")
	metricsDataMutex.Lock()
	latestMetricsData = data
	log.Printf("downloadAndStoreMetrics: SET latestMetricsData to (len %d): %.100s", len(latestMetricsData), string(latestMetricsData))
	metricsDataMutex.Unlock()

	if writeErr := os.WriteFile(metricsFilename, data, 0644); writeErr != nil {
		log.Printf("Warning: failed to write downloaded metrics to persistent cache file %s: %v", metricsFilename, writeErr)
	} else {
		log.Printf("Successfully wrote downloaded data (len %d) to %s", len(data), metricsFilename)
	}

	lastMetricsDownloadStatus = fmt.Sprintf("Successfully updated metrics from MEGA/local file at %s. Size: %d bytes", lastMetricsDownloadTime.Format(time.RFC3339), len(data))
	log.Printf("downloadAndStoreMetrics: %s", lastMetricsDownloadStatus)
	metricsDownloadStatusMutex.Unlock()
}

func downloadMetricsPeriodically() {
	go func() {
		log.Println("downloadMetricsPeriodically: Performing initial metrics download...")
		downloadAndStoreMetrics()
		log.Println("downloadMetricsPeriodically: Initial metrics download finished.")

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("downloadMetricsPeriodically (tick): Initiating scheduled metrics download...")
			downloadAndStoreMetrics()
			log.Println("downloadMetricsPeriodically (tick): Scheduled metrics download finished.")
		}
	}()
}

func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) {
	if len(jsonData) == 0 {
		log.Println("parseProductMetricsData: received empty JSON data (e.g. after '[]' init), returning empty map.")
		return make(map[string]ProductMetrics), nil
	}
	if string(jsonData) == "{}" {
		log.Printf("parseProductMetricsData: CRITICAL - received literal '{}' which is not an array. Returning error.")
		return nil, fmt.Errorf("cannot unmarshal JSON object %s into Go value of type []ProductMetrics", string(jsonData[:min(10, len(jsonData))]))
	}

	var productMetricsSlice []ProductMetrics
	if err := json.Unmarshal(jsonData, &productMetricsSlice); err != nil {
		previewLen := 200
		if len(jsonData) < previewLen {
			previewLen = len(jsonData)
		}
		return nil, fmt.Errorf("failed to unmarshal metrics JSON array into []ProductMetrics: %w. JSON(start %d bytes): %s", err, previewLen, string(jsonData[:previewLen]))
	}

	productMetricsMap := make(map[string]ProductMetrics, len(productMetricsSlice))
	for _, metric := range productMetricsSlice {
		if metric.ProductID == "" {
			log.Printf("Warning: ProductMetrics entry found with empty/missing ProductID: %+v. Skipping this entry.", metric)
			continue
		}
		if _, exists := productMetricsMap[metric.ProductID]; exists {
			log.Printf("Warning: Duplicate ProductID ('%s') found in metrics data. Overwriting with the later entry: %+v", metric.ProductID, metric)
		}
		productMetricsMap[metric.ProductID] = metric
	}

	log.Printf("parseProductMetricsData: Successfully parsed %d metrics objects into a map with %d entries.", len(productMetricsSlice), len(productMetricsMap))
	return productMetricsMap, nil
}

func performOptimizationCycleNow(productMetrics map[string]ProductMetrics, apiResp *HypixelAPIResponse) ([]byte, []byte, error) {
	runStartTime := time.Now()
	log.Println("performOptimizationCycleNow: Starting new optimization cycle...")

	if apiResp == nil || apiResp.Products == nil {
		return nil, nil, fmt.Errorf("CRITICAL: API data or products map is nil in performOptimizationCycleNow")
	}
	if productMetrics == nil {
		return nil, nil, fmt.Errorf("CRITICAL: Product metrics map is nil in performOptimizationCycleNow")
	}
	log.Printf("performOptimizationCycleNow: API data (LastUpdated: %d, %d products) and Product Metrics data (%d entries) received.", apiResp.LastUpdated, len(apiResp.Products), len(productMetrics))

	var apiLastUpdatedStr string
	if apiResp.LastUpdated > 0 {
		apiLastUpdatedStr = time.Unix(apiResp.LastUpdated/1000, 0).Format(time.RFC3339Nano)
	}

	var itemIDs []string
	for id := range apiResp.Products {
		itemIDs = append(itemIDs, id)
	}
	log.Printf("performOptimizationCycleNow: Processing %d items from API.", len(itemIDs))

	const (
		maxAllowedCycleTimePerItem = 3600.0
		maxInitialSearchQty        = 1000000.0
		itemsPerChunk              = 50
		pauseBetweenChunks         = 500 * time.Millisecond
	)
	log.Printf("performOptimizationCycleNow: Optimization Params: maxCycleTimePerItem=%.0fs, maxInitialSearchQty=%.0f, itemsPerChunk=%d, pauseBetweenChunks=%v",
		maxAllowedCycleTimePerItem, maxInitialSearchQty, itemsPerChunk, pauseBetweenChunks)

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

		log.Printf("performOptimizationCycleNow: Optimizing chunk %d/%d (items %d to %d of %d)",
			(i/itemsPerChunk)+1, (len(itemIDs)+itemsPerChunk-1)/itemsPerChunk, i, end-1, len(itemIDs))

		chunkResults := RunFullOptimization(currentChunkItemIDs, maxAllowedCycleTimePerItem, apiResp, productMetrics, itemFilesDir, maxInitialSearchQty)
		allOptimizedResults = append(allOptimizedResults, chunkResults...)

		if end < len(itemIDs) {
			log.Printf("performOptimizationCycleNow: Pausing for %v after processing chunk.", pauseBetweenChunks)
			time.Sleep(pauseBetweenChunks)
		}
	}
	optimizedResults := allOptimizedResults
	log.Printf("performOptimizationCycleNow: Optimization processing complete. Generated %d results.", len(optimizedResults))

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
		MaxAllowedCycleTimeSecs:     maxAllowedCycleTimePerItem,
		MaxInitialSearchQuantity:    maxInitialSearchQty,
	}

	mainOutput := OptimizationRunOutput{Summary: summary, Results: optimizedResults}
	mainJSON, err := json.MarshalIndent(mainOutput, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("CRITICAL: Failed to marshal main optimization output: %w", err)
	}

	var failedJSON []byte
	if len(failDetails) > 0 {
		if b, errMarshal := json.MarshalIndent(failDetails, "", "  "); errMarshal != nil {
			log.Printf("Error marshalling failed items report: %v", errMarshal)
			failedJSON = []byte(`[{"error":"failed to marshal detailed failed items report"}]`)
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
	log.Printf("runSingleOptimizationAndUpdateResults: Copied latestMetricsData (len %d) for processing: '%.200s'", len(currentMetricsBytes), string(currentMetricsBytes))
	metricsDataMutex.RUnlock()

	if len(currentMetricsBytes) == 0 || string(currentMetricsBytes) == "[]" {
		log.Println("runSingleOptimizationAndUpdateResults: Metrics data not yet available or is an empty array. Skipping this optimization cycle.")
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: Metrics data not ready or empty array.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}
	if string(currentMetricsBytes) == "{}" {
		log.Printf("runSingleOptimizationAndUpdateResults: CRITICAL - currentMetricsBytes is '{}' before parsing. This should have been caught earlier. Skipping cycle.")
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: Metrics data was an invalid object '{}'.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}

	productMetrics, parseErr := parseProductMetricsData(currentMetricsBytes)
	if parseErr != nil {
		log.Printf("runSingleOptimizationAndUpdateResults: Failed to parse current metrics data: %v. Skipping cycle.", parseErr)
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: Metrics parsing failed: %v", time.Now().Format(time.RFC3339), parseErr)
		optimizationStatusMutex.Unlock()
		return
	}
	if len(productMetrics) == 0 {
		log.Println("runSingleOptimizationAndUpdateResults: Parsed metrics data resulted in an empty product map. Skipping cycle.")
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: Parsed metrics map is empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}

	log.Println("runSingleOptimizationAndUpdateResults: Loading fresh Hypixel API data...")
	apiResp, apiErr := getApiResponse()
	if apiErr != nil {
		log.Printf("runSingleOptimizationAndUpdateResults: Error fetching Hypixel API data: %v. Skipping cycle.", apiErr)
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: API data load failed: %v", time.Now().Format(time.RFC3339), apiErr)
		optimizationStatusMutex.Unlock()
		return
	}
	if apiResp == nil || len(apiResp.Products) == 0 {
		log.Println("runSingleOptimizationAndUpdateResults: API response is nil or contains no products. Skipping cycle.")
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: API response nil or no products.", time.Now().Format(time.RFC3339))
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

	log.Printf("runSingleOptimizationAndUpdateResults: Cycle finished. Error: %v. Main JSON: %d bytes, Failed JSON: %d bytes. Status: %s",
		optErr, mainLen, failLen, lastOptimizationStatus)
}

func optimizePeriodically() {
	go func() {
		isOptimizingMutex.Lock()
		if isOptimizing {
			isOptimizingMutex.Unlock()
			log.Println("optimizePeriodically (initial): Optimization already in progress. Skipping initial run.")
			return
		}
		isOptimizing = true
		isOptimizingMutex.Unlock()

		log.Println("optimizePeriodically (initial): Starting initial optimization run...")
		runSingleOptimizationAndUpdateResults()

		isOptimizingMutex.Lock()
		isOptimizing = false
		isOptimizingMutex.Unlock()
		log.Println("optimizePeriodically (initial): Initial optimization run finished.")
	}()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for t := range ticker.C {
		log.Printf("optimizePeriodically (tick at %s): Checking if optimization can start.", t.Format(time.RFC3339))
		canStart := false
		isOptimizingMutex.Lock()
		if !isOptimizing {
			isOptimizing = true
			canStart = true
		}
		isOptimizingMutex.Unlock()

		if canStart {
			go func(tickTime time.Time) {
				log.Printf("optimizePeriodically (goroutine for tick %s): Starting optimization work.", tickTime.Format(time.RFC3339))
				defer func() {
					isOptimizingMutex.Lock()
					isOptimizing = false
					isOptimizingMutex.Unlock()
					log.Printf("optimizePeriodically (goroutine for tick %s): Optimization work finished.", tickTime.Format(time.RFC3339))
				}()
				runSingleOptimizationAndUpdateResults()
			}(t)
		} else {
			log.Println("optimizePeriodically (tick): Previous optimization still in progress. Skipping this cycle.")
		}
	}
}

func optimizerResultsHandler(w http.ResponseWriter, r *http.Request) {
	optimizerResultsMutex.RLock()
	data := latestOptimizerResultsJSON
	optimizerResultsMutex.RUnlock()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if data == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"Optimizer results are not yet available or an error occurred during generation."}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func failedItemsReportHandler(w http.ResponseWriter, r *http.Request) {
	failedItemsMutex.RLock()
	data := latestFailedItemsJSON
	failedItemsMutex.RUnlock()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if data == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
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
		optSt = "Service initializing; first optimization run pending."
	}
	if metSt == "" {
		metSt = "Service initializing; first metrics download pending."
	}

	resp := map[string]interface{}{
		"service_status":                    "active",
		"current_utc_time":                  time.Now().UTC().Format(time.RFC3339Nano),
		"optimization_process_status":       optSt,
		"last_optimization_attempt_utc":     formatTimeIfNotZero(optT),
		"metrics_download_process_status":   metSt,
		"last_metrics_download_attempt_utc": formatTimeIfNotZero(metT),
		"is_currently_optimizing":           currentIsOptimizing,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"Failed to marshal status response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func formatTimeIfNotZero(t time.Time) string {
	if t.IsZero() {
		return "N/A (pending first run)"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprintln(w, "Optimizer Microservice - Status Endpoint: /status")
	fmt.Fprintln(w, "Data Endpoints:")
	fmt.Fprintln(w, "  /optimizer_results.json - Latest full optimization results")
	fmt.Fprintln(w, "  /failed_items_report.json - Report of items that failed calculation in the last run")
	fmt.Fprintln(w, "Debug Endpoints (if pprof is imported):")
	fmt.Fprintln(w, "  /debug/pprof/ - Profiling information")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	log.Printf("DEBUG ENV: MEGA_EMAIL from container's env: [%s]", os.Getenv("MEGA_EMAIL"))
	pwd := os.Getenv("MEGA_PWD")
	if len(pwd) > 0 {
		log.Printf("DEBUG ENV: MEGA_PWD from container's env (logging first char for presence): [%c]", pwd[0])
	} else {
		log.Printf("DEBUG ENV: MEGA_PWD from container's env (logging first char for presence): []")
	}
	log.Printf("DEBUG ENV: MEGA_METRICS_FOLDER_PATH from container's env: [%s]", os.Getenv("MEGA_METRICS_FOLDER_PATH"))

	log.Println("Main: Optimizer service starting...")

	optimizationStatusMutex.Lock()
	lastOptimizationStatus = "Service starting; initial optimization pending."
	lastOptimizationTime = time.Time{}
	optimizationStatusMutex.Unlock()

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = "Service starting; initial metrics download pending."
	lastMetricsDownloadTime = time.Time{}
	metricsDownloadStatusMutex.Unlock()

	optimizerResultsMutex.Lock()
	latestOptimizerResultsJSON = []byte(`{"summary":{"run_timestamp":"N/A","api_last_updated_timestamp":"","total_items_considered":0,"items_successfully_calculated":0,"items_with_calculation_errors":0,"max_allowed_cycle_time_seconds":0,"max_initial_search_quantity":0,"message":"Initializing optimizer results... First run pending."},"results":[]}`)
	optimizerResultsMutex.Unlock()

	failedItemsMutex.Lock()
	latestFailedItemsJSON = []byte("[]")
	failedItemsMutex.Unlock()

	metricsDataMutex.Lock()
	initialMetricsBytes, err := os.ReadFile(metricsFilename)
	log.Printf("Main Init: Attempting to read %s. Error: %v. Bytes read: %d", metricsFilename, err, len(initialMetricsBytes))
	if err == nil && len(initialMetricsBytes) > 0 {
		log.Printf("Main Init: %s content (start): %.100s", metricsFilename, string(initialMetricsBytes))
		var tempMetricsSlice []ProductMetrics
		if parseErr := json.Unmarshal(initialMetricsBytes, &tempMetricsSlice); parseErr == nil {
			latestMetricsData = initialMetricsBytes
			log.Printf("Main Init: SET latestMetricsData from valid cache file '%s' (len %d): %.100s",
				metricsFilename, len(latestMetricsData), string(latestMetricsData))
		} else {
			log.Printf("Main Init: Warning: Cache file '%s' (len %d) is NOT a valid JSON array: %v. Content(start): %.100s.",
				metricsFilename, len(initialMetricsBytes), parseErr, string(initialMetricsBytes))
			latestMetricsData = []byte("[]")
			log.Printf("Main Init: SET latestMetricsData to default '[]' (len %d) due to unparseable cache.", len(latestMetricsData))
		}
	} else {
		if os.IsNotExist(err) {
			log.Printf("Main Init: No cache file '%s'.", metricsFilename)
		} else if len(initialMetricsBytes) == 0 && err == nil {
			log.Printf("Main Init: Cache file '%s' is empty.", metricsFilename)
		} else {
			log.Printf("Main Init: Error reading cache file '%s': %v.", metricsFilename, err)
		}
		latestMetricsData = []byte("[]")
		log.Printf("Main Init: SET latestMetricsData to default '[]' (len %d) due to missing/empty/read-error cache.", len(latestMetricsData))
	}
	metricsDataMutex.Unlock()

	log.Println("Main: Starting periodic metrics download task...")
	go downloadMetricsPeriodically()

	log.Println("Main: Starting periodic optimization task...")
	go optimizePeriodically()

	http.HandleFunc("/optimizer_results.json", optimizerResultsHandler)
	http.HandleFunc("/failed_items_report.json", failedItemsReportHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

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

// Assume these are defined in your project:
// type ProductMetrics struct { ProductID string `json:"productId"` /* ... */ }
// type OptimizedItemResult struct { ItemName string; CalculationPossible bool; ErrorMessage string; /* ... */ }
// type HypixelAPIResponse struct { LastUpdated int64; Products map[string]ProductDetails; /* ... */ }
// type ProductDetails struct { ProductID string; /* ... */ }
// func RunFullOptimization(...) []OptimizedItemResult { /* ... */ }
// func getApiResponse() (*HypixelAPIResponse, error) { /* ... */ }
