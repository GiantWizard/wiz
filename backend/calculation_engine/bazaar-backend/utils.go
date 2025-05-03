package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync" // Need sync for the map initialization
)

// --- Debug helper ---
var debug = os.Getenv("DEBUG") == "1"

// --- ID Normalization ---
var itemIDNormalizationMap map[string]string
var normalizeMapOnce sync.Once

func initializeNormalizationMap() {
	itemIDNormalizationMap = map[string]string{
		"SAND-1":  "SAND",
		"LOG-1":   "LOG",   // Oak Log variant
		"LOG-2":   "LOG_2", // Spruce Log variant (assuming LOG_2 is the base ID for Spruce)
		"LOG_2-1": "LOG_2", // Birch Log variant
		"LOG_2-2": "LOG",   // Jungle Log variant (assuming LOG is the base ID for Jungle too - adjust if needed)
		// Add other known variants here, mapping variant ID -> base Bazaar/Minecraft ID
		// Example: "WOOL-14": "WOOL",
		// Example: "INK_SACK-4": "LAPIS_LAZULI", // Lapis Lazuli dye
	}
}

// NormalizeItemID converts known variant IDs (like SAND-1) to their base ID.
// It also handles general standardization (uppercase, trim space).
func NormalizeItemID(id string) string {
	standardID := strings.ToUpper(strings.TrimSpace(id))

	normalizeMapOnce.Do(initializeNormalizationMap) // Ensure map is initialized

	if normalized, ok := itemIDNormalizationMap[standardID]; ok {
		dlog("Normalizing Item ID '%s' -> '%s'", standardID, normalized)
		return normalized // Return the mapped base ID
	}
	return standardID // Return the standardized ID if no specific mapping found
}

// BAZAAR_ID now uses NormalizeItemID
// This ensures all ID lookups use the normalized version.
func BAZAAR_ID(id string) string {
	return NormalizeItemID(id)
}

// --- dlog ---
func dlog(format string, args ...interface{}) {
	if debug {
		log.Printf("DEBUG: "+format, args...)
	}
}

// --- Formatting Helpers ---
func formatSeconds(sec float64) string { /* ... implementation ... */
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
		return "N/A (<0)"
	}
	if sec == 0 {
		return "0.0s"
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
func formatCost(cost float64) string { /* ... implementation ... */
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

// --- Safe Data Access Helpers ---
// Assumes HypixelAPIResponse, HypixelProduct, ProductMetrics structs defined externally.
func safeGetProductData(apiResp *HypixelAPIResponse, productID string) (HypixelProduct, bool) {
	if apiResp == nil || apiResp.Products == nil {
		return HypixelProduct{}, false
	}
	lookupID := BAZAAR_ID(productID) // Uses NormalizeItemID via BAZAAR_ID
	productData, ok := apiResp.Products[lookupID]
	return productData, ok
}
func safeGetMetricsData(metricsMap map[string]ProductMetrics, productID string) (ProductMetrics, bool) {
	if metricsMap == nil {
		return ProductMetrics{}, false
	}
	lookupID := BAZAAR_ID(productID) // Uses NormalizeItemID via BAZAAR_ID
	metricsData, ok := metricsMap[lookupID]
	return metricsData, ok
}

// --- Console Clear ---
var clear map[string]func()

func init() { /* ... implementation ... */
	clear = make(map[string]func())
	clear["linux"] = func() { cmd := exec.Command("clear"); cmd.Stdout = os.Stdout; _ = cmd.Run() }
	clear["darwin"] = func() { cmd := exec.Command("clear"); cmd.Stdout = os.Stdout; _ = cmd.Run() }
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

// --- Comparison Helper ---
func mapsAreEqual(map1, map2 map[string]float64) bool { /* ... implementation ... */
	if len(map1) != len(map2) {
		return false
	}
	for key, val1 := range map1 {
		val2, ok := map2[key]
		if !ok || math.Abs(val1-val2) > 1e-9 {
			return false
		}
	}
	return true
}
