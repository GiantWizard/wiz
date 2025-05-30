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
	megaLsCmd                   = "megals"          // Should be found via PATH due to symlinks in /usr/local/bin or fallback
	megaGetCmd                  = "megaget"         // Should be found via PATH or fallback
	megaCmdFallbackDir          = "/usr/local/bin/" // Fallback directory for MEGA commands based on Dockerfile
)

// --- Struct Definitions for main.go orchestration ---
type OptimizationRunOutput struct {
	Summary OptimizationSummary   `json:"summary"`
	Results []OptimizedItemResult `json:"results"`
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

func downloadMetricsFromMega(localTargetFilename string) error {
	megaEmail := os.Getenv("MEGA_EMAIL")
	megaPassword := os.Getenv("MEGA_PWD") // Ensure this is the var name your Go app uses
	megaRemoteFolderPath := os.Getenv("MEGA_METRICS_FOLDER_PATH")

	// Your Go app reads MEGA_PWD, your Koyeb config shows MEGA_PASSWORD and MEGA_PWD.
	// Let's assume MEGA_PWD is the one being effectively used by the Go app.
	// If it's actually MEGA_PASSWORD, change os.Getenv("MEGA_PWD") to os.Getenv("MEGA_PASSWORD") below.

	if megaEmail == "" || megaPassword == "" || megaRemoteFolderPath == "" {
		log.Println("downloadMetricsFromMega: MEGA environment variables not fully set. Skipping MEGA download.")
		log.Printf("Debug: MEGA_EMAIL empty: %t, MEGA_PWD empty: %t, MEGA_METRICS_FOLDER_PATH empty: %t", megaEmail == "", megaPassword == "", megaRemoteFolderPath == "")
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
		return fmt.Errorf("MEGA download skipped (missing env config: EMAIL_SET:%t, PWD_SET:%t, PATH_SET:%t), and no local cache '%s' found",
			megaEmail != "", megaPassword != "", megaRemoteFolderPath != "", metricsFilename)
	}

	targetDir := filepath.Dir(localTargetFilename)
	if targetDir != "." && targetDir != "" {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory %s for metrics: %v", targetDir, err)
		}
	}

	var foundMegaLsPath string
	log.Printf("Debug: Attempting to find '%s' in PATH for Go process (using exec.LookPath)...", megaLsCmd)
	pathViaLookPathLs, lookErrLs := exec.LookPath(megaLsCmd)
	if lookErrLs == nil {
		log.Printf("Debug: Command '%s' found by LookPath at: '%s'. This path will be used.", megaLsCmd, pathViaLookPathLs)
		foundMegaLsPath = pathViaLookPathLs
	} else {
		log.Printf("Debug: Command '%s' not found in PATH using exec.LookPath: %v", megaLsCmd, lookErrLs)
		currentPath := os.Getenv("PATH")
		log.Printf("Debug: Current PATH for Go process: %s", currentPath)
		fallbackPath := filepath.Join(megaCmdFallbackDir, megaLsCmd)
		log.Printf("Debug: Attempting fallback for '%s' at known location: %s", megaLsCmd, fallbackPath)
		if _, statErr := os.Stat(fallbackPath); statErr == nil {
			log.Printf("Debug: Command '%s' successfully found at fallback location: '%s'. This path will be used.", megaLsCmd, fallbackPath)
			foundMegaLsPath = fallbackPath
		} else {
			log.Printf("Debug: Command '%s' also not found or not executable at fallback location '%s': %v", megaLsCmd, fallbackPath, statErr)
			return fmt.Errorf("command '%s' not found: LookPath error (%w), and fallback '%s' also unusable (stat error: %v)",
				megaLsCmd, lookErrLs, fallbackPath, statErr)
		}
	}

	log.Printf("Listing files in MEGA folder: %s (using '%s')", megaRemoteFolderPath, foundMegaLsPath)
	ctxLs, cancelLs := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelLs()

	lsCmdObj := exec.CommandContext(ctxLs, foundMegaLsPath, "--username", megaEmail, "--password", megaPassword, "--no-ask-password", megaRemoteFolderPath)
	homeEnv := os.Getenv("HOME")
	if homeEnv == "" {
		homeEnv = "/home/appuser"
		log.Printf("Debug: HOME env not set, defaulting to %s for mega command", homeEnv)
	}
	lsCmdObj.Env = append(os.Environ(), "HOME="+homeEnv)
	lsOutBytes, lsErr := lsCmdObj.CombinedOutput()

	if ctxLs.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: '%s' command timed out after %v. Output: %s", foundMegaLsPath, megaCmdTimeout, string(lsOutBytes))
		return fmt.Errorf("'%s' command timed out. Output: %s", foundMegaLsPath, string(lsOutBytes))
	}
	lsOut := string(lsOutBytes)
	if len(lsOut) > 1000 {
		log.Printf("downloadMetricsFromMega: '%s' Output (first 1000 chars of %d): %.1000s...", foundMegaLsPath, len(lsOut), lsOut)
	} else {
		log.Printf("downloadMetricsFromMega: '%s' Output: %s", foundMegaLsPath, lsOut)
	}

	if lsErr != nil {
		if exitErr, ok := lsErr.(*exec.ExitError); ok {
			log.Printf("'%s' command exited with error: %v. Stderr: %s", foundMegaLsPath, lsErr, string(exitErr.Stderr))
			return fmt.Errorf("'%s' command failed: %v. Stderr: %s. Full Output: %s", foundMegaLsPath, lsErr, string(exitErr.Stderr), lsOut)
		}
		return fmt.Errorf("'%s' command failed: %v. Output: %s", foundMegaLsPath, lsErr, lsOut)
	}
	if strings.Contains(lsOut, "Unable to connect to service") || strings.Contains(lsOut, "Please ensure mega-cmd-server is running") {
		return fmt.Errorf("'%s' output indicates MEGAcmd server issue: %s. Ensure mega-cmd-server is running and accessible by user '%s' with HOME='%s'", foundMegaLsPath, lsOut, os.Getenv("USER"), homeEnv)
	}

	var latestFilename string
	var latestTimestamp time.Time
	foundFile := false

	// --- MODIFIED PARSING LOGIC START ---
	scanner := bufio.NewScanner(strings.NewReader(lsOut))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Heuristic to skip banner, warnings, errors, and pure prompt lines
		// Pure prompt line: contains the user@domain:/$ pattern but does NOT contain a metrics file pattern
		isLikelyPromptLine := strings.Contains(line, megaEmail+":/$") // More specific prompt check
		if !isLikelyPromptLine && strings.Contains(line, ":/$") {     // Generic prompt for other users/cases
			isLikelyPromptLine = true
		}

		if strings.Contains(strings.ToUpper(line), "WRN:") ||
			strings.Contains(strings.ToUpper(line), "ERR:") ||
			strings.Contains(line, "[Initiating MEGAcmd server") ||
			strings.Contains(line, "MEGAcmd!") || // Banner keyword
			strings.HasPrefix(line, ".") || // Banner structure
			strings.HasPrefix(line, "|") || // Banner structure
			strings.HasPrefix(line, "`") || // Banner structure
			(isLikelyPromptLine && !metricsFileRegex.MatchString(line)) { // Skip prompt line ONLY if it doesn't also contain a metrics file pattern
			log.Printf("MEGA LS Info/Warning/Banner/Prompt (Skipping): %s", line)
			continue
		}

		filenameCandidate := line
		// If the line still contains the prompt but also a filename, try to extract the filename part.
		// Example: "bobofrogoo@gmail.com:/$ metrics_20230101120000.json"
		if isLikelyPromptLine { // Check if it was a prompt-like line that passed the filter above
			// Attempt to split by common prompt terminators like ':/$' or just '$'
			// This needs to be robust. Let's find the last occurrence of ':/$' or '$ '
			// and take the substring after it.
			promptEndMarkerPos := -1
			specificPromptMarker := megaEmail + ":/$"
			genericPromptMarker1 := ":/$" // Common for mega-cmd
			genericPromptMarker2 := "$ "  // Common for shell prompts if somehow mixed

			pos1 := strings.LastIndex(line, specificPromptMarker)
			if pos1 != -1 {
				promptEndMarkerPos = pos1 + len(specificPromptMarker)
			} else {
				pos2 := strings.LastIndex(line, genericPromptMarker1)
				if pos2 != -1 {
					promptEndMarkerPos = pos2 + len(genericPromptMarker1)
				} else {
					pos3 := strings.LastIndex(line, genericPromptMarker2)
					if pos3 != -1 {
						promptEndMarkerPos = pos3 + len(genericPromptMarker2)
					}
				}
			}

			if promptEndMarkerPos != -1 && promptEndMarkerPos < len(line) {
				extracted := strings.TrimSpace(line[promptEndMarkerPos:])
				// Further check if the extracted part contains spaces, typical filenames don't.
				// If it has spaces and isn't a quoted filename, it might be multiple files or garbage.
				if !strings.Contains(extracted, " ") || (strings.HasPrefix(extracted, "\"") && strings.HasSuffix(extracted, "\"")) {
					filenameCandidate = extracted
					log.Printf("Debug: Extracted filename candidate '%s' from prompt-like line '%s'", filenameCandidate, line)
				} else {
					log.Printf("Debug: Prompt-like line '%s' had complex content after prompt marker, using original line for filepath.Base.", line)
				}
			}
		}

		// filepath.Base is good for extracting the last component of a path.
		// If filenameCandidate is "metrics_....json", it returns "metrics_....json".
		// If filenameCandidate is "/some/path/metrics_....json", it returns "metrics_....json".
		filenameToParse := filepath.Base(filenameCandidate)

		log.Printf("Debug: Processing line: '%s' -> candidate: '%s' -> base for regex: '%s'", line, filenameCandidate, filenameToParse)

		match := metricsFileRegex.FindStringSubmatch(filenameToParse)
		if len(match) == 2 {
			timestampStr := match[1]
			t, parseErr := time.Parse(timestampFormat, timestampStr)
			if parseErr != nil {
				log.Printf("Warning: could not parse timestamp in filename '%s' (from line '%s', candidate '%s'): %v", filenameToParse, line, filenameCandidate, parseErr)
				continue
			}
			log.Printf("Debug: Matched metrics file: '%s' with timestamp %s", filenameToParse, t.Format(time.RFC3339))
			if !foundFile || t.After(latestTimestamp) {
				log.Printf("Debug: New latest file found: %s (old: %s, timestamp %s)", filenameToParse, latestFilename, t.Format(time.RFC3339))
				foundFile = true
				latestTimestamp = t
				latestFilename = filenameToParse // This should be just the filename
			}
		} else {
			// Only log if it wasn't an explicitly skipped line type
			if !(strings.Contains(strings.ToUpper(line), "WRN:") || strings.Contains(strings.ToUpper(line), "ERR:") /* etc. */) {
				log.Printf("Debug: Line did not match metrics regex after processing: '%s' (candidate '%s', base '%s')", line, filenameCandidate, filenameToParse)
			}
		}
	}
	// --- MODIFIED PARSING LOGIC END ---

	if scanErr := scanner.Err(); scanErr != nil {
		log.Printf("Warning: error scanning '%s' output: %v", foundMegaLsPath, scanErr)
	}

	if !foundFile {
		return fmt.Errorf("no metrics file matching pattern '%s' found in MEGA folder '%s'. Review 'megals' output and debug logs above.", metricsFileRegex.String(), megaRemoteFolderPath)
	}

	remoteFilePathFull := strings.TrimRight(megaRemoteFolderPath, "/") + "/" + latestFilename
	log.Printf("Identified latest metrics file for download: '%s' (will be fetched as '%s')", latestFilename, remoteFilePathFull)

	tempDownloadPath := filepath.Join(targetDir, latestFilename) // Use latestFilename directly as it's just the base name
	if localTargetFilename == tempDownloadPath {
		log.Printf("Debug: Target and temp download path are the same: %s. Removing if exists.", localTargetFilename)
		_ = os.Remove(localTargetFilename)
	} else {
		log.Printf("Debug: Temp download path: %s. Removing if exists.", tempDownloadPath)
		_ = os.Remove(tempDownloadPath)
	}

	log.Printf("Downloading '%s' from MEGA to temp path '%s' (will be moved to '%s')", remoteFilePathFull, tempDownloadPath, localTargetFilename)
	ctxGet, cancelGet := context.WithTimeout(context.Background(), megaCmdTimeout)
	defer cancelGet()

	var foundMegaGetPath string
	log.Printf("Debug: Attempting to find '%s' in PATH for Go process (for get operation using exec.LookPath)...", megaGetCmd)
	pathViaLookPathGet, lookErrGet := exec.LookPath(megaGetCmd)
	if lookErrGet == nil {
		log.Printf("Debug: Command '%s' found by LookPath at: '%s' for get operation. This path will be used.", megaGetCmd, pathViaLookPathGet)
		foundMegaGetPath = pathViaLookPathGet
	} else {
		log.Printf("Debug: Command '%s' not found in PATH using exec.LookPath for get operation: %v", megaGetCmd, lookErrGet)
		currentPath := os.Getenv("PATH")
		log.Printf("Debug: Current PATH for Go process (get op): %s", currentPath)
		fallbackPath := filepath.Join(megaCmdFallbackDir, megaGetCmd)
		log.Printf("Debug: Attempting fallback for '%s' (get op) at known location: %s", megaGetCmd, fallbackPath)
		if _, statErr := os.Stat(fallbackPath); statErr == nil {
			log.Printf("Debug: Command '%s' (get op) successfully found at fallback location: '%s'. This path will be used.", megaGetCmd, fallbackPath)
			foundMegaGetPath = fallbackPath
		} else {
			log.Printf("Debug: Command '%s' (get op) also not found or not executable at fallback location '%s': %v", megaGetCmd, fallbackPath, statErr)
			return fmt.Errorf("command '%s' (for get op) not found: LookPath error (%w), and fallback '%s' also unusable (stat error: %v)",
				megaGetCmd, lookErrGet, fallbackPath, statErr)
		}
	}
	log.Printf("Debug: Using command '%s' (resolved to '%s') for get operation.", megaGetCmd, foundMegaGetPath)

	// For megaget, the remote path should be the full path, and --path specifies the local directory.
	getCmdObj := exec.CommandContext(ctxGet, foundMegaGetPath, "--username", megaEmail, "--password", megaPassword, "--no-ask-password", remoteFilePathFull, "--path", targetDir)
	getCmdObj.Env = append(os.Environ(), "HOME="+homeEnv)
	getOutBytes, getErr := getCmdObj.CombinedOutput()

	if ctxGet.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: '%s' command timed out for %s after %v. Output: %s", foundMegaGetPath, remoteFilePathFull, megaCmdTimeout, string(getOutBytes))
		return fmt.Errorf("'%s' command timed out for %s. Output: %s", foundMegaGetPath, remoteFilePathFull, string(getOutBytes))
	}
	getOut := string(getOutBytes)
	log.Printf("downloadMetricsFromMega: '%s' Output (first 500 chars): %.500s", foundMegaGetPath, getOut) // Log output for get command
	if getErr != nil {
		if exitErr, ok := getErr.(*exec.ExitError); ok {
			log.Printf("'%s' command exited with error: %v. Stderr: %s", foundMegaGetPath, getErr, string(exitErr.Stderr))
			return fmt.Errorf("failed to download '%s' with '%s': %v. Stderr: %s. Full Output: %s", remoteFilePathFull, foundMegaGetPath, getErr, string(exitErr.Stderr), getOut)
		}
		return fmt.Errorf("failed to download '%s' with '%s': %v. Output: %s", remoteFilePathFull, foundMegaGetPath, getErr, getOut)
	}
	if strings.Contains(getOut, "Unable to connect to service") || strings.Contains(getOut, "Please ensure mega-cmd-server is running") {
		return fmt.Errorf("'%s' output indicates MEGAcmd server issue during get: %s. Ensure mega-cmd-server is running and accessible by user '%s' with HOME='%s'", foundMegaGetPath, getOut, os.Getenv("USER"), homeEnv)
	}

	// megaget places the file named `latestFilename` (which is the basename) into `targetDir`.
	// So, `tempDownloadPath` (which is `filepath.Join(targetDir, latestFilename)`) is the correct path to check.
	if _, statErr := os.Stat(tempDownloadPath); os.IsNotExist(statErr) {
		return fmt.Errorf("'%s' command appeared to succeed for '%s', but downloaded file '%s' is missing. Output: %s", foundMegaGetPath, latestFilename, tempDownloadPath, getOut)
	}
	log.Printf("Debug: Successfully found downloaded file at temp path: %s", tempDownloadPath)

	if localTargetFilename != tempDownloadPath {
		log.Printf("Moving downloaded file from '%s' to '%s'", tempDownloadPath, localTargetFilename)
		if err := os.Rename(tempDownloadPath, localTargetFilename); err != nil {
			// Try to remove temp file on failure to move
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
	var tempMetricsSlice []ProductMetrics // This type should come from metrics.go
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

func parseProductMetricsData(jsonData []byte) (map[string]ProductMetrics, error) {
	if len(jsonData) == 0 {
		log.Println("parseProductMetricsData: jsonData is empty, returning empty map.")
		return make(map[string]ProductMetrics), nil
	}
	if string(jsonData) == "{}" {
		log.Println("parseProductMetricsData: jsonData is '{}', which cannot be unmarshaled into []ProductMetrics.")
		return nil, fmt.Errorf("cannot unmarshal JSON object into []ProductMetrics")
	}
	var productMetricsSlice []ProductMetrics // This type should come from metrics.go
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
		normalizedID := BAZAAR_ID(metric.ProductID) // This function should come from utils.go
		metric.ProductID = normalizedID
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
		emptyOutput := OptimizationRunOutput{Summary: emptySummary, Results: []OptimizedItemResult{}} // OptimizedItemResult from optimizer.go
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
	var allOptimizedResults []OptimizedItemResult // OptimizedItemResult from optimizer.go
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
		// RunFullOptimization should come from optimizer.go
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
	apiResp, apiErr := getApiResponse() // This function should come from api.go
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
	apiCacheMutex.RLock() // apiCacheMutex from api.go
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
		var tempMetricsSlice []ProductMetrics // This type should come from metrics.go
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
	}
	addr := "0.0.0.0:" + port
	log.Printf("Main: Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server on %s: %v", addr, err)
	}
}
