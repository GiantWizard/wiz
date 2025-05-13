package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ... (dlog, itemIDNormalizationMap, initializeNormalizationMap, NormalizeItemID, BAZAAR_ID remain) ...
var isDebug = os.Getenv("DEBUG") == "1"

func dlog(format string, args ...interface{}) {
	if isDebug {
		log.Printf("DEBUG: "+format, args...)
	}
}

var itemIDNormalizationMap map[string]string
var normalizeMapOnce sync.Once

func initializeNormalizationMap() {
	dlog("Initializing Item ID normalization map...")
	itemIDNormalizationMap = map[string]string{
		"LOG":        "OAK_LOG",
		"LOG-1":      "SPRUCE_LOG",
		"LOG-2":      "BIRCH_LOG",
		"LOG-3":      "JUNGLE_LOG",
		"LOG_2":      "ACACIA_LOG",
		"LOG_2-0":    "ACACIA_LOG",
		"LOG_2-1":    "DARK_OAK_LOG",
		"WOOD":       "OAK_PLANKS",
		"WOOD-1":     "SPRUCE_PLANKS",
		"WOOD-2":     "BIRCH_PLANKS",
		"WOOD-3":     "JUNGLE_PLANKS",
		"WOOD-4":     "ACACIA_PLANKS",
		"WOOD-5":     "DARK_OAK_PLANKS",
		"INK_SACK":   "INK_SAC",
		"INK_SACK-4": "LAPIS_LAZULI",
		// Add many more as needed
	}
	dlog("Normalization map initialized with %d entries.", len(itemIDNormalizationMap))
}

func NormalizeItemID(id string) string {
	standardID := strings.ToUpper(strings.TrimSpace(id))
	normalizeMapOnce.Do(initializeNormalizationMap)
	if normalized, ok := itemIDNormalizationMap[standardID]; ok {
		return normalized
	}
	return standardID
}

func BAZAAR_ID(id string) string {
	return NormalizeItemID(id)
}

// aggregateCells reads recipe cells ("ITEM_ID:AMOUNT" or "ITEM_ID")
// and returns a map of NORMALIZED ingredient IDs to their total amounts per single craft.
func aggregateCells(cells map[string]string) (map[string]float64, error) {
	positions := []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"}
	ingredients := make(map[string]float64)
	var firstError error

	for _, pos := range positions {
		cellContent := strings.TrimSpace(cells[pos])
		if cellContent == "" {
			continue
		}

		parts := strings.SplitN(cellContent, ":", 2)
		ingID := BAZAAR_ID(strings.TrimSpace(parts[0]))
		if ingID == "" {
			dlog("WARN: Skipping empty ingredient ID in cell '%s': '%s'", pos, cellContent)
			continue
		}

		amt := 1.0
		if len(parts) == 2 {
			amtStr := strings.TrimSpace(parts[1])
			parsedAmt, err := strconv.ParseFloat(amtStr, 64)
			if err != nil || parsedAmt <= 0 || math.IsNaN(parsedAmt) || math.IsInf(parsedAmt, 0) {
				errMsg := fmt.Sprintf("invalid amount '%s' for ingredient '%s' in cell '%s'", amtStr, ingID, pos)
				dlog("WARN (aggregateCells): %s. Using 1.0. Error: %v", errMsg, err)
				amt = 1.0
				if firstError == nil {
					firstError = fmt.Errorf(errMsg)
				}
			} else {
				amt = parsedAmt
			}
		}
		ingredients[ingID] += amt
	}
	return ingredients, firstError
}

// isInPath checks if a NORMALIZED item name is already in the current expansion path.
// Path stores ItemSteps {name, quantity}
func isInPath(itemName string, path []ItemStep) bool {
	normalizedItemName := BAZAAR_ID(itemName) // Ensure comparison is with normalized ID
	for _, step := range path {
		// Assuming step.name is already normalized when added to path
		if step.name == normalizedItemName {
			return true
		}
	}
	return false
}

// --- Formatting Helpers (formatSeconds, formatCost, sanitizeFloat remain) ---
func formatSeconds(sec float64) string {
	if math.IsNaN(sec) {
		return "N/A (NaN)"
	}
	if math.IsInf(sec, 1) {
		return "Infinite"
	}
	if math.IsInf(sec, -1) {
		return "N/A (-Inf)"
	}
	if sec < 0 {
		return fmt.Sprintf("N/A (<0: %.2f)", sec)
	}
	if sec == 0 {
		return "0.0s"
	}
	if sec < 1 {
		return fmt.Sprintf("%.2fs", sec)
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

func formatCost(cost float64) string {
	if math.IsNaN(cost) {
		return "N/A (NaN)"
	}
	if math.IsInf(cost, 1) {
		return "Infinite"
	}
	if math.IsInf(cost, -1) {
		return "-Infinite"
	}
	if cost == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", cost)
}

func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0.0
	}
	return f
}

// --- Safe Data Access Helpers (safeGetProductData, safeGetMetricsData, getSellPrice, getBuyPrice, getMetrics remain) ---
func safeGetProductData(apiResp *HypixelAPIResponse, productID string) (HypixelProduct, bool) {
	if apiResp == nil || apiResp.Products == nil {
		return HypixelProduct{}, false
	}
	lookupID := BAZAAR_ID(productID)
	productData, ok := apiResp.Products[lookupID]
	return productData, ok
}

func safeGetMetricsData(metricsMap map[string]ProductMetrics, productID string) (ProductMetrics, bool) {
	if metricsMap == nil {
		return ProductMetrics{}, false
	}
	lookupID := BAZAAR_ID(productID)
	metricsData, ok := metricsMap[lookupID]
	return metricsData, ok
}

func getSellPrice(apiResp *HypixelAPIResponse, itemIDNorm string) float64 {
	prod, ok := safeGetProductData(apiResp, itemIDNorm)
	if !ok || len(prod.SellSummary) == 0 {
		return 0.0
	}
	price := prod.SellSummary[0].PricePerUnit
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0.0
	}
	return price
}

func getBuyPrice(apiResp *HypixelAPIResponse, itemIDNorm string) float64 {
	prod, ok := safeGetProductData(apiResp, itemIDNorm)
	if !ok || len(prod.BuySummary) == 0 {
		return 0.0
	}
	price := prod.BuySummary[0].PricePerUnit
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0.0
	}
	return price
}

func getMetrics(metricsMap map[string]ProductMetrics, itemIDNorm string) ProductMetrics {
	metrics, _ := safeGetMetricsData(metricsMap, itemIDNorm)
	return metrics
}

// --- Console Clear (clear, init, clearConsole remain) ---
var clear map[string]func()

func init() { // init is called automatically
	clear = make(map[string]func())
	clear["linux"] = func() { cmd := exec.Command("clear"); cmd.Stdout = os.Stdout; _ = cmd.Run() }
	clear["darwin"] = clear["linux"]
	clear["windows"] = func() { cmd := exec.Command("cmd", "/c", "cls"); cmd.Stdout = os.Stdout; _ = cmd.Run() }
}
func clearConsole() {
	value, ok := clear[runtime.GOOS]
	if ok {
		value()
	} else {
		log.Println("Warning: Console clear not supported on OS:", runtime.GOOS)
	}
}

// --- Comparison Helper (mapsAreEqual remains) ---
func mapsAreEqual(map1, map2 map[string]float64) bool {
	if len(map1) != len(map2) {
		return false
	}
	tolerance := 1e-9
	for key, val1 := range map1 {
		val2, ok := map2[key]
		if !ok || math.Abs(val1-val2) > tolerance {
			return false
		}
	}
	return true
}
