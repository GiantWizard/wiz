package main

import (
	"fmt"
	"log"
	"math" // Needed for Abs, IsNaN, IsInf
	"os"
	"os/exec"
	"runtime"
	"strconv" // Needed for aggregateCells if moved here
	"strings"
	"sync" // Need sync for the normalization map initialization
)

// --- Debug helper ---
// Set environment variable DEBUG=1 to enable debug logging
var isDebug = os.Getenv("DEBUG") == "1"

// dlog conditionally logs debug messages.
func dlog(format string, args ...interface{}) {
	if isDebug {
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
		"LOG":             "OAK_LOG", // Base log
		"LOG-1":           "SPRUCE_LOG",
		"LOG-2":           "BIRCH_LOG",
		"LOG-3":           "JUNGLE_LOG",
		"LOG_2":           "ACACIA_LOG", // Note: LOG_2 might be Acacia or Dark Oak depending on context sometimes
		"LOG_2-0":         "ACACIA_LOG",
		"LOG_2-1":         "DARK_OAK_LOG",
		"WOOD":            "OAK_PLANKS", // Base planks
		"WOOD-1":          "SPRUCE_PLANKS",
		"WOOD-2":          "BIRCH_PLANKS",
		"WOOD-3":          "JUNGLE_PLANKS",
		"WOOD-4":          "ACACIA_PLANKS",
		"WOOD-5":          "DARK_OAK_PLANKS",
		"SAND":            "SAND", // Base sand
		"SAND-1":          "RED_SAND",
		"INK_SACK":        "INK_SAC",      // Base Ink Sac (Black Dye)
		"INK_SACK-1":      "RED_DYE",      // Rose Red
		"INK_SACK-2":      "GREEN_DYE",    // Cactus Green
		"INK_SACK-3":      "COCOA",        // Cocoa Beans (Brown Dye)
		"INK_SACK-4":      "LAPIS_LAZULI", // Lapis Lazuli (Blue Dye)
		"INK_SACK-15":     "BONE_MEAL",    // Bone Meal (White Dye)
		"WOOL":            "WHITE_WOOL",   // Base Wool
		"WOOL-1":          "ORANGE_WOOL",
		"WOOL-2":          "MAGENTA_WOOL",
		"WOOL-3":          "LIGHT_BLUE_WOOL",
		"WOOL-4":          "YELLOW_WOOL",
		"WOOL-5":          "LIME_WOOL",
		"WOOL-6":          "PINK_WOOL",
		"WOOL-7":          "GRAY_WOOL",
		"WOOL-8":          "LIGHT_GRAY_WOOL",
		"WOOL-9":          "CYAN_WOOL",
		"WOOL-10":         "PURPLE_WOOL",
		"WOOL-11":         "BLUE_WOOL",
		"WOOL-12":         "BROWN_WOOL",
		"WOOL-13":         "GREEN_WOOL",
		"WOOL-14":         "RED_WOOL",
		"WOOL-15":         "BLACK_WOOL",
		"RAW_FISH":        "RAW_FISH", // Base Raw Fish (Cod)
		"RAW_FISH-1":      "RAW_SALMON",
		"RAW_FISH-2":      "CLOWNFISH", // Tropical Fish? Often mapped this way.
		"RAW_FISH-3":      "PUFFERFISH",
		"QUARTZ_BLOCK":    "QUARTZ_BLOCK", // Base Quartz Block
		"QUARTZ_BLOCK-1":  "CHISELED_QUARTZ_BLOCK",
		"QUARTZ_BLOCK-2":  "PILLAR_QUARTZ_BLOCK",
		"HUGE_MUSHROOM_1": "BROWN_MUSHROOM_BLOCK",
		"HUGE_MUSHROOM_2": "RED_MUSHROOM_BLOCK",
		"POTION":          "WATER_BOTTLE", // Base potion if no effects specified

		// Hypixel Specific Renames / Common Items (Add more as identified)
		"ENCHANTED_CARROT_STICK": "ENCHANTED_CARROT_ON_A_STICK", // Example rename
		"SULPHUR":                "GUNPOWDER",                   // Common alternative name
		"SLIME_BALL":             "SLIMEBALL",                   // Often inconsistent
		// Add Pet IDs if they appear in recipes? e.g. "PET_WOLF_COMMON": "WOLF_PET_COMMON" ?
	}
	dlog("Normalization map initialized with %d entries.", len(itemIDNormalizationMap))
}

// NormalizeItemID converts known variant IDs and standardizes format.
func NormalizeItemID(id string) string {
	// 1. Basic standardization: Uppercase, trim space
	standardID := strings.ToUpper(strings.TrimSpace(id))

	// 2. Ensure normalization map is initialized (thread-safe)
	normalizeMapOnce.Do(initializeNormalizationMap)

	// 3. Check for direct mapping in our custom map
	if normalized, ok := itemIDNormalizationMap[standardID]; ok {
		// dlog("Normalizing Item ID '%s' -> '%s'", standardID, normalized) // Uncomment for verbose logging
		return normalized // Return the mapped canonical ID
	}

	// 4. If no specific mapping found, return the standardized ID
	return standardID
}

// BAZAAR_ID is the primary function to use for getting a canonical item ID.
func BAZAAR_ID(id string) string {
	return NormalizeItemID(id)
}

// --- Recipe Parsing Helpers (Moved from recipe.go) ---

// aggregateCells reads recipe cells ("ITEM_ID:AMOUNT" or "ITEM_ID")
// and returns a map of NORMALIZED ingredient IDs to their total amounts.
func aggregateCells(cells map[string]string) (map[string]float64, error) {
	positions := []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"}
	ingredients := make(map[string]float64)
	var firstError error // Store first amount parsing error

	for _, pos := range positions {
		cellContent := strings.TrimSpace(cells[pos])
		if cellContent == "" {
			continue // Skip empty cell
		}

		parts := strings.SplitN(cellContent, ":", 2)
		// Normalize the ingredient ID right after extracting it
		ingID := BAZAAR_ID(strings.TrimSpace(parts[0])) // Normalize ID
		if ingID == "" {
			dlog("WARN: Skipping empty ingredient ID in cell '%s': '%s'", pos, cellContent)
			continue // Skip if ID becomes empty after trimming/normalizing
		}

		amt := 1.0 // Default amount is 1
		if len(parts) == 2 {
			amtStr := strings.TrimSpace(parts[1])
			parsedAmt, err := strconv.ParseFloat(amtStr, 64)
			if err != nil || parsedAmt <= 0 || math.IsNaN(parsedAmt) || math.IsInf(parsedAmt, 0) {
				// Log error but maybe continue with default amount? Or return error?
				// Let's log and continue with 1.0 for now, but store the first error.
				errMsg := fmt.Sprintf("invalid amount '%s' for ingredient '%s' in cell '%s'", amtStr, ingID, pos)
				dlog("WARN: %s. Using 1.0. Error: %v", errMsg, err) // Assumes dlog is defined elsewhere
				amt = 1.0
				if firstError == nil { // Store only the first parsing error encountered
					firstError = fmt.Errorf(errMsg)
				}
			} else {
				amt = parsedAmt
			}
		}
		ingredients[ingID] += amt // Add amount to the map for the normalized ID
	}
	// Return the map and the first error encountered during amount parsing (if any)
	return ingredients, firstError
}

// isInPath checks if a NORMALIZED item name is already in the current expansion path.
func isInPath(itemName string, path []ItemStep) bool {
	// Item name passed should ideally already be normalized by the caller,
	// but normalize again for safety/consistency.
	normalizedItemName := BAZAAR_ID(itemName)
	for _, step := range path {
		// Path should store normalized names (ensure ItemStep uses normalized name)
		if step.name == normalizedItemName {
			return true // Found a match
		}
	}
	return false // Not found in the path
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
		return fmt.Sprintf("N/A (<0: %.2f)", sec) // Should not happen for time
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
	// Go's standard fmt doesn't have easy built-in comma formatting.
	// Using standard float formatting for now.
	if cost == 0 {
		return "0.00" // Ensure two decimal places for zero
	}
	// Use %.2f for two decimal places. Add comma logic if needed later.
	return fmt.Sprintf("%.2f", cost)
}

// sanitizeFloat converts NaN or Inf float values to 0.0.
// Useful for preparing data for JSON responses where NaN/Inf are invalid.
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) { // Checks for positive and negative infinity
		return 0.0
	}
	return f
}

// --- Safe Data Access Helpers ---
// These help retrieve data from potentially nil maps/structs, using normalized IDs.

// safeGetProductData safely retrieves a HypixelProduct from the API cache using a normalized ID.
func safeGetProductData(apiResp *HypixelAPIResponse, productID string) (HypixelProduct, bool) {
	if apiResp == nil || apiResp.Products == nil {
		// dlog("safeGetProductData: Called with nil apiResp or Products map for ID '%s'", productID) // Uncomment for verbose logging
		return HypixelProduct{}, false // Return zero value and false if cache is bad
	}
	lookupID := BAZAAR_ID(productID) // Ensure lookup uses normalized ID
	productData, ok := apiResp.Products[lookupID]
	// if !ok {
	//	 dlog("safeGetProductData: Product '%s' (Normalized: '%s') not found in API cache.", productID, lookupID) // Uncomment for verbose logging
	// }
	return productData, ok
}

// safeGetMetricsData safely retrieves ProductMetrics from the metrics cache using a normalized ID.
func safeGetMetricsData(metricsMap map[string]ProductMetrics, productID string) (ProductMetrics, bool) {
	if metricsMap == nil {
		// dlog("safeGetMetricsData: Called with nil metricsMap for ID '%s'", productID) // Uncomment for verbose logging
		return ProductMetrics{}, false // Return zero value and false if map is nil
	}
	lookupID := BAZAAR_ID(productID) // Ensure lookup uses normalized ID
	metricsData, ok := metricsMap[lookupID]
	// if !ok {
	//	 dlog("safeGetMetricsData: Metrics for '%s' (Normalized: '%s') not found in metrics cache.", productID, lookupID) // Uncomment for verbose logging
	// }
	return metricsData, ok
}

// getSellPrice safely retrieves the top sell price (user buy order price).
// Returns 0.0 if not available or invalid.
func getSellPrice(apiResp *HypixelAPIResponse, itemIDNorm string) float64 {
	prod, ok := safeGetProductData(apiResp, itemIDNorm)
	if !ok || len(prod.SellSummary) == 0 {
		return 0.0
	}
	price := prod.SellSummary[0].PricePerUnit
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0.0 // Return 0 if price is invalid
	}
	return price
}

// getBuyPrice safely retrieves the top buy price (user instabuy price).
// Returns 0.0 if not available or invalid.
func getBuyPrice(apiResp *HypixelAPIResponse, itemIDNorm string) float64 {
	prod, ok := safeGetProductData(apiResp, itemIDNorm)
	if !ok || len(prod.BuySummary) == 0 {
		return 0.0
	}
	price := prod.BuySummary[0].PricePerUnit
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0.0 // Return 0 if price is invalid
	}
	return price
}

// getMetrics safely retrieves the metrics data for an item.
// Returns an empty ProductMetrics struct if not found.
func getMetrics(metricsMap map[string]ProductMetrics, itemIDNorm string) ProductMetrics {
	// safeGetMetricsData handles nil map and returns zero struct if not found
	metrics, _ := safeGetMetricsData(metricsMap, itemIDNorm)
	return metrics
}

// --- Console Clear ---
// Keep this if used by background tasks or CLI modes, otherwise optional.
var clear map[string]func() // Map OS name to clear function

// init initializes the clear map based on the runtime OS.
func init() {
	clear = make(map[string]func())
	clear["linux"] = func() {
		cmd := exec.Command("clear") // Linux, macOS
		cmd.Stdout = os.Stdout
		_ = cmd.Run() // Ignore error
	}
	clear["darwin"] = clear["linux"] // macOS uses the same command
	clear["windows"] = func() {
		cmd := exec.Command("cmd", "/c", "cls") // Windows
		cmd.Stdout = os.Stdout
		_ = cmd.Run() // Ignore error
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
// Keep this if used by background loops comparing results, otherwise optional.

// mapsAreEqual checks if two maps[string]float64 are identical.
// Uses a small tolerance for float comparison.
func mapsAreEqual(map1, map2 map[string]float64) bool {
	if len(map1) != len(map2) {
		return false // Different number of keys
	}
	tolerance := 1e-9 // Define tolerance for float comparison
	for key, val1 := range map1 {
		val2, ok := map2[key]
		// Check if key exists in map2 AND values are approximately equal
		if !ok || math.Abs(val1-val2) > tolerance {
			return false
		}
	}
	return true // All keys exist and values match within tolerance
}
