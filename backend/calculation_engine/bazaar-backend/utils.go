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
// Set environment variable DEBUG=1 to enable debug logging
var isDebug = os.Getenv("DEBUG") == "1" // Renamed from 'debug' to avoid conflict with runtime/debug package

func dlog(format string, args ...interface{}) {
	if isDebug { // Use the renamed variable
		// Include a marker to distinguish debug logs easily
		log.Printf("DEBUG: "+format, args...)
	}
}

// --- ID Normalization ---
var itemIDNormalizationMap map[string]string
var normalizeMapOnce sync.Once

// initializeNormalizationMap sets up known variant ID mappings.
// Add more mappings here as needed. Keys should be uppercase.
func initializeNormalizationMap() {
	dlog("Initializing Item ID normalization map...")
	itemIDNormalizationMap = map[string]string{
		// Vanilla Minecraft Variants (Examples)
		"LOG-1":          "SPRUCE_LOG", // Assuming SPRUCE_LOG is canonical
		"LOG-2":          "BIRCH_LOG",
		"LOG-3":          "JUNGLE_LOG",
		"LOG_2-0":        "ACACIA_LOG",
		"LOG_2-1":        "DARK_OAK_LOG",
		"WOOD-1":         "SPRUCE_WOOD", // Planks
		"WOOD-2":         "BIRCH_WOOD",
		"WOOD-3":         "JUNGLE_WOOD",
		"WOOD-4":         "ACACIA_WOOD",
		"WOOD-5":         "DARK_OAK_WOOD",
		"SAND-1":         "RED_SAND",
		"INK_SACK-4":     "LAPIS_LAZULI", // Lapis Lazuli (Blue Dye)
		"INK_SACK-3":     "COCOA",        // Cocoa Beans (Brown Dye)
		"INK_SACK-1":     "RED_DYE",      // Rose Red
		"INK_SACK-2":     "GREEN_DYE",    // Cactus Green
		"INK_SACK-15":    "BONE_MEAL",    // Bone Meal (White Dye)
		"WOOL-1":         "ORANGE_WOOL",
		"WOOL-2":         "MAGENTA_WOOL",
		"WOOL-3":         "LIGHT_BLUE_WOOL",
		"WOOL-4":         "YELLOW_WOOL",
		"WOOL-5":         "LIME_WOOL",
		"WOOL-6":         "PINK_WOOL",
		"WOOL-7":         "GRAY_WOOL", // Note: often DARK_GRAY
		"WOOL-8":         "LIGHT_GRAY_WOOL",
		"WOOL-9":         "CYAN_WOOL",
		"WOOL-10":        "PURPLE_WOOL",
		"WOOL-11":        "BLUE_WOOL",
		"WOOL-12":        "BROWN_WOOL",
		"WOOL-13":        "GREEN_WOOL",
		"WOOL-14":        "RED_WOOL",
		"WOOL-15":        "BLACK_WOOL",
		"RAW_FISH-1":     "RAW_SALMON",
		"RAW_FISH-2":     "CLOWNFISH",
		"RAW_FISH-3":     "PUFFERFISH",
		"QUARTZ_BLOCK-1": "CHISELED_QUARTZ_BLOCK",
		"QUARTZ_BLOCK-2": "PILLAR_QUARTZ_BLOCK",

		// Hypixel Specific Variants / Renames (Examples - Adjust as needed!)
		"LOG":             "OAK_LOG",              // Map generic LOG to OAK_LOG? Or keep LOG? Depends on API. Let's assume OAK_LOG.
		"LOG_2":           "SPRUCE_LOG",           // Map LOG_2 to SPRUCE_LOG? Assumed above. Verify API keys.
		"HUGE_MUSHROOM_1": "BROWN_MUSHROOM_BLOCK", // Assuming this mapping
		"HUGE_MUSHROOM_2": "RED_MUSHROOM_BLOCK",   // Assuming this mapping
		"INK_SACK":        "INK_SAC",              // Rename if API uses Ink Sac
		"POTION":          "WATER_BOTTLE",         // Base potion item often maps to water bottle if type isn't specified
	}
	dlog("Normalization map initialized with %d entries.", len(itemIDNormalizationMap))
}

// NormalizeItemID converts known variant IDs (like SAND-1) to their base ID.
// It also handles general standardization (uppercase, trim space).
func NormalizeItemID(id string) string {
	// 1. Basic standardization
	standardID := strings.ToUpper(strings.TrimSpace(id))

	// 2. Ensure normalization map is initialized (thread-safe)
	normalizeMapOnce.Do(initializeNormalizationMap)

	// 3. Check for direct mapping
	if normalized, ok := itemIDNormalizationMap[standardID]; ok {
		// dlog("Normalizing Item ID '%s' -> '%s'", standardID, normalized) // Can be noisy
		return normalized // Return the mapped base ID
	}

	// 4. No specific mapping found, return the standardized ID
	return standardID
}

// BAZAAR_ID is the primary function to use for getting a canonical item ID.
// It ensures all ID lookups and processing steps use the normalized version.
func BAZAAR_ID(id string) string {
	return NormalizeItemID(id)
}

// --- Formatting Helpers ---

// formatSeconds converts a duration in seconds to a human-readable string (s, m, h, d).
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
		// Should not happen for time, but handle defensively
		return fmt.Sprintf("N/A (<0: %.2f)", sec)
	}
	if sec == 0 {
		return "0.0s"
	}

	if sec < 1 {
		return fmt.Sprintf("%.2fs", sec) // More precision for sub-second
	}
	if sec < 60 {
		return fmt.Sprintf("%.1fs", sec) // Seconds
	}
	mins := sec / 60
	if mins < 60 {
		return fmt.Sprintf("%.1fm", mins) // Minutes
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%.1fh", hours) // Hours
	}
	days := hours / 24
	// Add weeks/months/years if needed
	return fmt.Sprintf("%.1fd", days) // Days
}

// formatCost formats a float64 cost value into a string, handling NaN/Inf.
func formatCost(cost float64) string {
	if math.IsNaN(cost) {
		return "N/A (NaN)"
	}
	if math.IsInf(cost, 1) {
		return "Infinite" // Positive infinity
	}
	if math.IsInf(cost, -1) {
		return "-Infinite" // Negative infinity
	}
	// Format with commas and 2 decimal places for typical currency
	// Go's standard fmt doesn't have built-in comma formatting easily.
	// For simplicity here, just use standard float formatting.
	// For production, consider using a library or custom function for commas.
	if cost == 0 {
		return "0.00" // Ensure two decimal places for zero
	}
	return fmt.Sprintf("%.2f", cost) // Standard 2 decimal places
}

// --- Safe Data Access Helpers ---
// These help retrieve data from potentially nil maps/structs, using normalized IDs.

// safeGetProductData safely retrieves a HypixelProduct from the API cache using a normalized ID.
func safeGetProductData(apiResp *HypixelAPIResponse, productID string) (HypixelProduct, bool) {
	if apiResp == nil || apiResp.Products == nil {
		// dlog("safeGetProductData: Called with nil apiResp or Products map for ID '%s'", productID)
		return HypixelProduct{}, false // Return zero value and false if cache is bad
	}
	lookupID := BAZAAR_ID(productID) // Ensure lookup uses normalized ID
	productData, ok := apiResp.Products[lookupID]
	// if !ok {
	//	 dlog("safeGetProductData: Product '%s' (Normalized: '%s') not found in API cache.", productID, lookupID)
	// }
	return productData, ok
}

// safeGetMetricsData safely retrieves ProductMetrics from the metrics cache using a normalized ID.
func safeGetMetricsData(metricsMap map[string]ProductMetrics, productID string) (ProductMetrics, bool) {
	if metricsMap == nil {
		// dlog("safeGetMetricsData: Called with nil metricsMap for ID '%s'", productID)
		return ProductMetrics{}, false // Return zero value and false if map is nil
	}
	lookupID := BAZAAR_ID(productID) // Ensure lookup uses normalized ID
	metricsData, ok := metricsMap[lookupID]
	// if !ok {
	//	 dlog("safeGetMetricsData: Metrics for '%s' (Normalized: '%s') not found in metrics cache.", productID, lookupID)
	// }
	return metricsData, ok
}

// --- Console Clear ---
var clear map[string]func() // Map OS name to clear function

// init initializes the clear map based on the runtime OS.
func init() {
	clear = make(map[string]func())
	clear["linux"] = func() {
		cmd := exec.Command("clear") // Linux, macOS
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	}
	clear["darwin"] = clear["linux"] // macOS uses the same command
	clear["windows"] = func() {
		cmd := exec.Command("cmd", "/c", "cls") // Windows
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	}
}

// clearConsole attempts to clear the console screen based on the OS.
func clearConsole() {
	value, ok := clear[runtime.GOOS] // Get function for current OS
	if ok {
		value() // Call the clear function
	} else {
		// Fallback or warning if OS not supported
		log.Println("Warning: Console clear not supported on OS:", runtime.GOOS)
		// Optionally print newlines as a basic fallback
		// fmt.Println("\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n")
	}
}

// --- Comparison Helper ---

// mapsAreEqual checks if two maps[string]float64 are identical.
// Uses a small tolerance for float comparison.
func mapsAreEqual(map1, map2 map[string]float64) bool {
	if len(map1) != len(map2) {
		return false // Different number of keys
	}
	for key, val1 := range map1 {
		val2, ok := map2[key]
		// Check if key exists in map2 AND values are approximately equal
		if !ok || math.Abs(val1-val2) > 1e-9 { // Use tolerance for float comparison
			return false
		}
	}
	return true // All keys exist and values match
}
