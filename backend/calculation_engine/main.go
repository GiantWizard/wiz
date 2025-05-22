package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	// Your other necessary imports
)

const (
	metricsFilename      = "latest_metrics.json"
	itemFilesDir         = "dependencies/items"
	megaCmdCheckInterval = 5 * time.Minute
)

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
		log.Println("MEGA environment variables not fully set. Skipping MEGA download.")
		if _, err := os.Stat(localTargetFilename); os.IsNotExist(err) {
			return fmt.Errorf("MEGA download skipped (missing env config), and local metrics file '%s' not found to pre-populate", localTargetFilename)
		}
		log.Printf("Proceeding with existing local metrics file if available: %s", localTargetFilename)
		return nil
	}

	targetDir := filepath.Dir(localTargetFilename)
	if targetDir != "." && targetDir != "" {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory %s for metrics: %v", targetDir, err)
		}
	}

	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, "HOME=/app")

	logoutCmd := exec.Command("mega-logout")
	logoutCmd.Env = cmdEnv
	if out, err := logoutCmd.CombinedOutput(); err != nil {
		log.Printf("Warning executing 'mega-logout': %v. Output: %s", err, string(out))
	} else {
		log.Printf("'mega-logout' successful. Output: %s", string(out))
	}

	loginSpecificEnv := append([]string{}, cmdEnv...)
	loginSpecificEnv = append(loginSpecificEnv, "MEGA_EMAIL="+megaEmail, "MEGA_PWD="+megaPassword)
	loginCmd := exec.Command("mega-login", megaEmail, megaPassword)
	loginCmd.Env = loginSpecificEnv
	loginOutBytes, loginErr := loginCmd.CombinedOutput()
	loginOut := string(loginOutBytes)
	if loginErr != nil {
		log.Printf("ERROR executing 'mega-login': %v. Output: %s", loginErr, loginOut)
		if strings.Contains(loginOut, "Invalid credentials") || strings.Contains(loginOut, "Incorrect email or password") {
			return fmt.Errorf("failed to log in to MEGA: Invalid credentials. Output: %s", loginOut)
		}
		return fmt.Errorf("failed to log in to MEGA: %v. Output: %s", loginErr, loginOut)
	}
	log.Printf("'mega-login' successful. Output: %s", loginOut)

	log.Printf("Listing files in MEGA folder: %s", megaRemoteFolderPath)
	lsCmd := exec.Command("mega-ls", megaRemoteFolderPath)
	lsCmd.Env = cmdEnv
	lsOutBytes, lsErr := lsCmd.CombinedOutput()
	lsOut := string(lsOutBytes)
	if lsErr != nil {
		log.Printf("ERROR executing 'mega-ls %s': %v. Output: %s", megaRemoteFolderPath, lsErr, lsOut)
		if strings.Contains(lsOut, "Not logged in") {
			return fmt.Errorf("not logged in for 'mega-ls', session might have expired or login failed. Output: %s", lsOut)
		}
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
		match := metricsFileRegex.FindStringSubmatch(line)
		if len(match) == 2 {
			timestampStr := match[1]
			t, parseErr := time.Parse(timestampFormat, timestampStr)
			if parseErr != nil {
				log.Printf("Warning: could not parse timestamp in filename '%s': %v", line, parseErr)
				continue
			}
			if !foundFile || t.After(latestTimestamp) {
				foundFile = true
				latestTimestamp = t
				latestFilename = line
			}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		log.Printf("Warning: error scanning 'mega-ls' output: %v", scanErr)
	}

	if !foundFile {
		if lsErr != nil {
			return fmt.Errorf("no metrics file matching pattern found on MEGA in '%s' after 'mega-ls' encountered an error: %v. Output: %s", megaRemoteFolderPath, lsErr, lsOut)
		}
		log.Printf("No metrics file matching pattern found on MEGA in %s.", megaRemoteFolderPath)
		return fmt.Errorf("no new metrics file matching pattern found on MEGA in %s", megaRemoteFolderPath)
	}

	remoteFilePath := strings.TrimRight(megaRemoteFolderPath, "/") + "/" + latestFilename
	log.Printf("Downloading latest metrics file '%s' from MEGA to '%s'...", remoteFilePath, localTargetFilename)
	_ = os.Remove(localTargetFilename)
	getCmd := exec.Command("mega-get", remoteFilePath, localTargetFilename)
	getCmd.Env = cmdEnv
	getOutBytes, getErr := getCmd.CombinedOutput()
	getOut := string(getOutBytes)

	if getErr != nil && !strings.Contains(getOut, "ENABLING AUTOUPDATE BY DEFAULT") {
		log.Printf("ERROR executing 'mega-get': %v. Output: %s", getErr, getOut)
		return fmt.Errorf("failed to download '%s' to '%s' from MEGA: %v. Output: %s", remoteFilePath, localTargetFilename, getErr, getOut)
	}
	if _, statErr := os.Stat(localTargetFilename); os.IsNotExist(statErr) {
		return fmt.Errorf("mega-get command appeared to succeed but target file '%s' is missing. Output: %s", localTargetFilename, getOut)
	}

	log.Printf("Successfully downloaded '%s' from MEGA to '%s'.", latestFilename, localTargetFilename)
	return nil
}

func downloadAndStoreMetrics() {
	log.Println("downloadAndStoreMetrics: Initiating metrics download from MEGA...")
	tempMetricsFilename := metricsFilename + ".downloading.tmp"
	defer os.Remove(tempMetricsFilename)

	err := downloadMetricsFromMega(tempMetricsFilename)

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadTime = time.Now()
	if err != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error during MEGA download at %s: %v", lastMetricsDownloadTime.Format(time.RFC3339), err)
		log.Printf("downloadAndStoreMetrics: %s", lastMetricsDownloadStatus)
		metricsDownloadStatusMutex.Unlock()
		return
	}

	data, readErr := os.ReadFile(tempMetricsFilename)
	if readErr != nil {
		lastMetricsDownloadStatus = fmt.Sprintf("Error reading metrics file %s at %s: %v", tempMetricsFilename, lastMetricsDownloadTime.Format(time.RFC3339), readErr)
		log.Printf("downloadAndStoreMetrics: %s", lastMetricsDownloadStatus)
		metricsDownloadStatusMutex.Unlock()
		return
	}

	metricsDataMutex.Lock()
	latestMetricsData = data
	metricsDataMutex.Unlock()

	if writeErr := os.WriteFile(metricsFilename, data, 0644); writeErr != nil {
		log.Printf("Warning: failed to write downloaded metrics to persistent cache file %s: %v", metricsFilename, writeErr)
	}

	lastMetricsDownloadStatus = fmt.Sprintf("Successfully updated metrics from MEGA/local file at %s. Size: %d bytes", lastMetricsDownloadTime.Format(time.RFC3339), len(data))
	log.Printf("downloadAndStoreMetrics: %s", lastMetricsDownloadStatus)
	metricsDownloadStatusMutex.Unlock()
}

func downloadMetricsPeriodically() {
	downloadAndStoreMetrics()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		downloadAndStoreMetrics()
	}
}

// *** THIS IS THE CORRECTED FUNCTION ***
// parseProductMetricsData unmarshals a JSON array of product metrics objects
// and converts it into a map[string]ProductMetrics, using the ProductID field as the key.
func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) {
	if len(jsonData) == 0 {
		log.Println("parseProductMetricsData: received empty JSON data, returning empty map.")
		return make(map[string]ProductMetrics), nil
	}

	var productMetricsSlice []ProductMetrics // Step 1: Unmarshal JSON array into a slice of ProductMetrics structs
	if err := json.Unmarshal(jsonData, &productMetricsSlice); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics JSON array into []ProductMetrics: %w. JSON(start): %s", err, string(jsonData[:min(100, len(jsonData))]))
	}

	// Step 2: Convert the slice of ProductMetrics structs into a map, keyed by ProductID
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
	)
	log.Printf("performOptimizationCycleNow: Optimization Params: maxCycleTimePerItem=%.0fs, maxInitialSearchQty=%.0f", maxAllowedCycleTimePerItem, maxInitialSearchQty)

	optimizedResults := RunFullOptimization(itemIDs, maxAllowedCycleTimePerItem, apiResp, productMetrics, itemFilesDir, maxInitialSearchQty)
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
	currentMetricsBytes := latestMetricsData
	metricsDataMutex.RUnlock()

	if currentMetricsBytes == nil || len(currentMetricsBytes) == 0 {
		log.Println("runSingleOptimizationAndUpdateResults: Metrics data not yet available or empty. Skipping this optimization cycle.")
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: Metrics data not ready or empty.", time.Now().Format(time.RFC3339))
		optimizationStatusMutex.Unlock()
		return
	}
	metricsBytesCopy := make([]byte, len(currentMetricsBytes))
	copy(metricsBytesCopy, currentMetricsBytes)

	productMetrics, parseErr := parseProductMetricsData(metricsBytesCopy) // Uses the corrected function
	if parseErr != nil {
		log.Printf("runSingleOptimizationAndUpdateResults: Failed to parse current metrics data: %v. Skipping cycle.", parseErr)
		optimizationStatusMutex.Lock()
		lastOptimizationStatus = fmt.Sprintf("Optimization skipped at %s: Metrics parsing failed: %v", time.Now().Format(time.RFC3339), parseErr)
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

	mainJSON, failedJSON, optErr := performOptimizationCycleNow(productMetrics, apiResp)
	mainLen, failLen := len(mainJSON), len(failedJSON)

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
		isOptimizingMutex.Lock()
		if isOptimizing {
			isOptimizingMutex.Unlock()
			log.Println("optimizePeriodically (tick): Previous optimization still in progress. Skipping this cycle.")
			continue
		}
		isOptimizing = true
		isOptimizingMutex.Unlock()

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
	}
}

func optimizerResultsHandler(w http.ResponseWriter, r *http.Request) {
	optimizerResultsMutex.RLock()
	data := latestOptimizerResultsJSON
	optimizerResultsMutex.RUnlock()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if data == nil {
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
		"is_currently_optimizing":           isOptimizing,
	}
	b, err := json.Marshal(resp)
	if err != nil {
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
	lastOptimizationTime = time.Time{}
	optimizationStatusMutex.Unlock()

	metricsDownloadStatusMutex.Lock()
	lastMetricsDownloadStatus = "Service starting; initial metrics download pending."
	lastMetricsDownloadTime = time.Time{}
	metricsDownloadStatusMutex.Unlock()

	optimizerResultsMutex.Lock()
	latestOptimizerResultsJSON = []byte(`{"summary":{"run_timestamp":"N/A","total_items_considered":0,"items_successfully_calculated":0,"items_with_calculation_errors":0},"message":"Initializing optimizer results... First run pending."}`)
	optimizerResultsMutex.Unlock()

	failedItemsMutex.Lock()
	latestFailedItemsJSON = []byte("[]")
	failedItemsMutex.Unlock()

	metricsDataMutex.Lock()
	initialMetricsBytes, err := os.ReadFile(metricsFilename)
	if err == nil && len(initialMetricsBytes) > 0 {
		if _, parseErr := parseProductMetricsData(initialMetricsBytes); parseErr == nil { // Test parse
			latestMetricsData = initialMetricsBytes
			log.Printf("Main: Successfully loaded and validated initial metrics from cache file '%s' (%d bytes)", metricsFilename, len(initialMetricsBytes))
		} else {
			log.Printf("Warning: Initial metrics cache file '%s' exists but could not be parsed: %v. Starting with empty metrics.", metricsFilename, parseErr)
			latestMetricsData = []byte("{}")
		}
	} else {
		if os.IsNotExist(err) {
			log.Printf("Main: No initial metrics cache file '%s' found. Starting with empty metrics.", metricsFilename)
		} else {
			log.Printf("Warning: Error reading initial metrics cache file '%s': %v. Starting with empty metrics.", metricsFilename, err)
		}
		latestMetricsData = []byte("{}")
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

	addr := "0.0.0.0:8081"
	log.Printf("Main: Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server: %v", err)
	}
}
