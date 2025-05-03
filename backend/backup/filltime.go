package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
)

// --- Structs ---
type ProductMetrics struct {
	ProductID      string  `json:"product_id"`
	SellSize       float64 `json:"sell_size"`
	SellFrequency  float64 `json:"sell_frequency"`
	OrderSize      float64 `json:"order_size_average"`      // Corrected tag
	OrderFrequency float64 `json:"order_frequency_average"` // Corrected tag
	BuyMovingWeek  float64 `json:"buy_moving_week"`
}

type QuickStatus struct {
	BuyMovingWeek float64 `json:"buyMovingWeek"`
}

type HypixelProduct struct {
	QuickStatus QuickStatus `json:"quick_status"`
}

type HypixelAPIResponse struct {
	Success     bool                      `json:"success"`
	LastUpdated int64                     `json:"lastUpdated"`
	Products    map[string]HypixelProduct `json:"products"`
}

// --- Global variables for caching ---
var (
	metricsCache     map[string]ProductMetrics
	loadMetricsOnce  sync.Once
	metricsLoadErr   error
	apiResponseCache *HypixelAPIResponse
	fetchApiOnce     sync.Once
	apiFetchErr      error
	apiCacheMutex    sync.Mutex
)

// --- Debug helper (Optional) ---
var debug = os.Getenv("DEBUG") == "1"

func dlog(format string, args ...interface{}) {
	if debug {
		log.Printf("DEBUG: "+format, args...)
	}
}

// --- Metrics Loading Logic ---
func loadMetricsData(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		metricsLoadErr = fmt.Errorf("read metrics file '%s': %w", filename, err)
		log.Printf("ERROR: %v", metricsLoadErr)
		return
	}
	var metricsList []ProductMetrics
	if err := json.Unmarshal(data, &metricsList); err != nil {
		metricsLoadErr = fmt.Errorf("parse metrics JSON '%s': %w", filename, err)
		log.Printf("ERROR: %v", metricsLoadErr)
		return
	}
	metricsCache = make(map[string]ProductMetrics, len(metricsList))
	for _, pm := range metricsList {
		if pm.ProductID == "" {
			log.Printf("Warning: Skipping metric entry empty product_id")
			continue
		}
		metricsCache[pm.ProductID] = pm
	}
}
func getMetricsMap(filename string) (map[string]ProductMetrics, error) {
	loadMetricsOnce.Do(func() { loadMetricsData(filename) })
	if metricsLoadErr != nil {
		return nil, metricsLoadErr
	}
	if metricsCache == nil {
		metricsCache = make(map[string]ProductMetrics)
	}
	return metricsCache, nil
}

// --- API Fetching Logic ---
func fetchBazaarData() {
	apiCacheMutex.Lock()
	if apiResponseCache != nil || apiFetchErr != nil {
		apiCacheMutex.Unlock()
		return
	}
	apiCacheMutex.Unlock()
	url := "https://api.hypixel.net/v2/skyblock/bazaar"
	resp, err := http.Get(url)
	if err != nil {
		apiFetchErr = fmt.Errorf("fetch API (%s): %w", url, err)
		log.Printf("ERROR: %v", apiFetchErr)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		apiFetchErr = fmt.Errorf("API status %d: %s", resp.StatusCode, string(bodyBytes))
		log.Printf("ERROR: %v", apiFetchErr)
		return
	}
	var apiResp HypixelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		apiFetchErr = fmt.Errorf("parse API JSON: %w", err)
		log.Printf("ERROR: %v", apiFetchErr)
		return
	}
	if !apiResp.Success {
		apiFetchErr = fmt.Errorf("API success: false")
		log.Printf("ERROR: %v", apiFetchErr)
		return
	}
	apiCacheMutex.Lock()
	apiResponseCache = &apiResp
	apiCacheMutex.Unlock()
}
func getApiResponse() (*HypixelAPIResponse, error) {
	fetchApiOnce.Do(func() { fetchBazaarData() })
	if apiFetchErr != nil {
		return nil, apiFetchErr
	}
	apiCacheMutex.Lock()
	defer apiCacheMutex.Unlock()
	if apiResponseCache == nil {
		return nil, fmt.Errorf("internal: API response cache nil")
	}
	return apiResponseCache, nil
}

// --- calculateInstasellFillTime ---
func calculateInstasellFillTime(qty, buyMovingWeek float64) float64 {
	if qty <= 0 {
		return 0
	}
	if buyMovingWeek <= 0 {
		return math.Inf(1)
	}
	secondsInWeek := 604800.0
	buyRatePerSecond := buyMovingWeek / secondsInWeek
	if buyRatePerSecond <= 0 {
		dlog("WARN: buyRatePerSecond <= 0 (BMW=%.2f)", buyMovingWeek)
		return math.Inf(1)
	}
	timeToFill := qty / buyRatePerSecond
	if math.IsNaN(timeToFill) || math.IsInf(timeToFill, 0) {
		dlog("WARN: Instasell NaN/Inf (Qty=%.2f, Rate=%.5f)", qty, buyRatePerSecond)
		return math.Inf(1)
	}
	return timeToFill
}

// --- Function to Calculate Buy Order Fill Time & RR ---
// Uses DeltaRatio (SR/DR) > 1 as the condition switch, mirroring C10M logic.
func getEstimatedFillTime(itemID string, quantity float64, metricsFilename string) (float64, float64, error) {
	dlog("Calculating fill time for %.0f x %s using metrics from %s", quantity, itemID, metricsFilename)
	rrValue := math.NaN() // Default RR

	if quantity <= 0 {
		dlog("Quantity <= 0, returning 0 time, NaN RR, nil error")
		return 0, rrValue, nil
	}

	metricsMap, err := getMetricsMap(metricsFilename)
	if err != nil {
		return math.Inf(1), rrValue, fmt.Errorf("could not load metrics: %w", err)
	}
	if metricsMap == nil {
		return math.Inf(1), rrValue, fmt.Errorf("metrics map nil after loading")
	}

	pm, found := metricsMap[itemID]
	if !found {
		errMsg := fmt.Sprintf("metrics not found for '%s' in file '%s'", itemID, metricsFilename)
		dlog("Warning: %s", errMsg)
		return math.Inf(1), rrValue, fmt.Errorf(errMsg)
	}
	dlog("Found metrics for %s: SS=%.2f, SF=%.2f, OS=%.2f, OF=%.2f", itemID, pm.SellSize, pm.SellFrequency, pm.OrderSize, pm.OrderFrequency)

	// --- Calculations ---
	ss := math.Max(0, pm.SellSize)
	sf := math.Max(0, pm.SellFrequency)
	osz := math.Max(0, pm.OrderSize)
	of := math.Max(0, pm.OrderFrequency)
	dlog("  Clamped Metrics: ss=%.4f, sf=%.4f, osz=%.4f, of=%.4f", ss, sf, osz, of)

	supplyRate := ss * sf
	demandRate := osz * of
	dlog("  Rates: Supply=%.4f, Demand=%.4f", supplyRate, demandRate)

	var deltaRatio float64
	if demandRate <= 0 {
		if supplyRate <= 0 {
			deltaRatio = 1.0
		} else {
			deltaRatio = math.Inf(1)
		}
	} else {
		deltaRatio = supplyRate / demandRate
	}
	dlog("  DeltaRatio (SR/DR): %.4f", deltaRatio)

	var fillTime float64

	// --- Logic Switch based on DeltaRatio ---
	if deltaRatio > 1.0 {
		// Supply > Demand: Use simpler formula based on difference (delta)
		dlog("  DeltaRatio > 1.0: Using formula based on rate difference.")
		rrValue = 1.0 // Set RR to 1 for this case (as per previous request)
		deltaDifference := supplyRate - demandRate
		if deltaDifference <= 0 { // Should not happen if ratio > 1, but safety check
			dlog("  WARN: DeltaDifference <= 0 despite DeltaRatio > 1. Fill time Infinite.")
			fillTime = math.Inf(1)
		} else {
			fillTime = (20.0 * quantity) / deltaDifference
			dlog("  Fill Time = (20 * %.1f) / %.4f = %.4f", quantity, deltaDifference, fillTime)
		}

	} else { // DeltaRatio <= 1.0: Demand >= Supply - Use formula based on IF and RR
		dlog("  DeltaRatio <= 1.0: Using formula based on IF and RR.")
		if of <= 0 {
			dlog("  Order Frequency (of) <= 0. Fill time Infinite.")
			fillTime = math.Inf(1)
			// rrValue remains NaN as calculation is impossible
		} else {
			calculatedIF := (ss * sf) / of
			if calculatedIF < 0 {
				calculatedIF = 0
			}
			dlog("  Calculated IF = %.4f", calculatedIF)

			if calculatedIF <= 0 {
				rrValue = 1.0
				dlog("  Calculated IF <= 0 -> RR = 1.0")
			} else {
				rrValue = math.Ceil(quantity / calculatedIF)
				if rrValue < 1 {
					rrValue = 1
				}
				dlog("  Calculated IF > 0 -> RR = Ceil(%.1f / %.4f) = %.1f", quantity, calculatedIF, rrValue)
				if math.IsInf(rrValue, 0) || math.IsNaN(rrValue) {
					dlog("  WARN: RR calc resulted in Inf/NaN. Fill time Infinite.")
					fillTime = math.Inf(1)
					// Keep problematic rrValue for reporting
				}
			}

			// Calculate fill time only if it's not already infinite from RR calc
			if !math.IsInf(fillTime, 1) {
				if math.IsInf(rrValue, 1) || math.IsNaN(rrValue) { // Double check RR validity before using
					fillTime = math.Inf(1)
				} else {
					fillTime = (20.0 * rrValue * quantity) / of
					dlog("  Fill Time = (20 * %.1f * %.1f) / %.4f = %.4f", rrValue, quantity, of, fillTime)
				}
			}
		}
	}

	// Final validation for calculated fillTime
	if math.IsNaN(fillTime) || math.IsInf(fillTime, -1) || fillTime < 0 {
		dlog("  WARN: Final fill time validation failed (%.4f). Setting to Inf.", fillTime)
		fillTime = math.Inf(1)
	}
	// Final validation for RR (in case NaN persisted from of=0)
	if math.IsNaN(rrValue) {
		dlog("  WARN: Final RR is NaN (likely of=0).")
	}

	return fillTime, rrValue, nil
}

// --- Helper to format seconds nicely ---
func formatSeconds(sec float64) string {
	if math.IsNaN(sec) {
		return "N/A (NaN)"
	}
	if math.IsInf(sec, 1) {
		return "Infinite"
	}
	if math.IsInf(sec, -1) {
		return "N/A (NegInf)"
	}
	if sec < 0 {
		return "N/A (<0)"
	}
	if sec == 0 {
		return "0s"
	}
	if sec < 60 {
		return fmt.Sprintf("%.1fs", sec)
	}
	mins := sec / 60
	if mins < 60 {
		return fmt.Sprintf("%.1fm", mins)
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%.1fh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.1fd", days)
}

// --- Main Function ---
func main() {
	metricsFilename := "latest_metrics.json"
	labelWidth := 25

	// --- Pre-load/cache data ---
	_, metricsErr := getMetricsMap(metricsFilename)
	if metricsErr != nil {
		log.Printf("Warning: Metrics load failed: %v.", metricsErr)
	}
	fmt.Println("Fetching Bazaar data...")
	apiResp, apiErr := getApiResponse()
	if apiErr != nil {
		log.Printf("Warning: API fetch failed: %v.", apiErr)
	} else if apiResp != nil {
		fmt.Println("Bazaar data fetched.")
	} else {
		log.Println("Warning: API response nil despite no error.")
	}
	// --- End Pre-load ---

	// --- Input Loop ---
	var productID string
	var quantityStr string

	fmt.Print("Product ID: ")
	if _, err := fmt.Scanln(&productID); err != nil {
		log.Fatalf("Input Product ID: %v", err)
	}
	fmt.Print("Quantity: ")
	if _, err := fmt.Scanln(&quantityStr); err != nil {
		log.Fatalf("Input Quantity: %v", err)
	}
	quantity, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil || quantity <= 0 {
		log.Fatalf("Quantity invalid: must be positive number.")
	}

	// --- Calculations ---
	buyOrderFillTime, rrValue, buyFillErr := getEstimatedFillTime(productID, quantity, metricsFilename)
	if buyFillErr != nil {
		log.Printf("Error BO fill time calc: %v", buyFillErr)
	}

	instaSellFillTime := math.Inf(1)
	buyMovingWeek := 0.0
	apiErrorReason := ""

	if apiErr == nil && apiResp != nil {
		if productData, ok := apiResp.Products[productID]; ok {
			buyMovingWeek = productData.QuickStatus.BuyMovingWeek
			dlog("LIVE BMW for %s: %.2f", productID, buyMovingWeek)
			if buyMovingWeek > 0 {
				instaSellFillTime = calculateInstasellFillTime(quantity, buyMovingWeek)
			} else {
				apiErrorReason = "LIVE BuyMovingWeek <= 0"
			}
		} else {
			apiErrorReason = "Item not found in API"
		}
	} else {
		if apiErr != nil {
			apiErrorReason = fmt.Sprintf("API fetch error: %v", apiErr)
		} else {
			apiErrorReason = "API response nil"
		}
	}

	// --- Print Results ---
	fmt.Println("\n--- Results ---")
	fmt.Printf("%-*s %s\n", labelWidth, "Product:", productID)
	fmt.Printf("%-*s %.0f\n", labelWidth, "Quantity:", quantity)

	// Format/Print RR
	rrStr := "N/A"
	if buyFillErr != nil {
		rrStr += " (Calc Error)"
	} else if math.IsNaN(rrValue) {
		rrStr += " (Order Freq is 0 or Metrics Error)"
	} else if math.IsInf(rrValue, 0) {
		rrStr = "Infinite"
	} else {
		rrStr = fmt.Sprintf("%.2f", rrValue)
	}
	fmt.Printf("%-*s %s\n", labelWidth, "Refill Rate (RR):", rrStr)

	// Format/Print Buy Order Fill Time
	buyFillStr := "N/A"
	if buyFillErr != nil {
		buyFillStr += fmt.Sprintf(" (Error: %v)", buyFillErr)
	} else {
		buyFillStr = formatSeconds(buyOrderFillTime)
	}
	fmt.Printf("%-*s %s (from file metrics)\n", labelWidth, "Buy Order Fill Time:", buyFillStr)

	// Format/Print InstaSell Fill Time
	isFillStr := formatSeconds(instaSellFillTime)
	isFillNote := ""
	if instaSellFillTime == math.Inf(1) {
		if apiErrorReason != "" {
			isFillNote = fmt.Sprintf("(Reason: %s)", apiErrorReason)
		} else {
			isFillNote = "(Reason: Unknown)"
		}
	}
	fmt.Printf("%-*s %s %s\n", labelWidth, "InstaSell Fill Time:", isFillStr, isFillNote)
}
