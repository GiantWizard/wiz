// utils.go
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath" // For filepath.Base in dlog
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var isDebug = os.Getenv("DEBUG") == "1" || strings.ToLower(os.Getenv("LOG_LEVEL")) == "debug"

// dlog logs debug messages if DEBUG=1 is set or LOG_LEVEL=debug.
func dlog(format string, args ...interface{}) {
	if isDebug {
		// Get caller info for better debug logs
		_, file, line, ok := runtime.Caller(1)
		if ok {
			shortFile := filepath.Base(file)
			log.Printf("DEBUG [%s:%d]: "+format, append([]interface{}{shortFile, line}, args...)...)
		} else {
			log.Printf("DEBUG: "+format, args...)
		}
	}
}

var itemIDNormalizationMap map[string]string
var normalizeMapOnce sync.Once

// initializeNormalizationMap populates the map for normalizing item IDs.
// This should be expanded with common Hypixel SkyBlock item ID variations.
func initializeNormalizationMap() {
	dlog("Initializing Item ID normalization map...")
	itemIDNormalizationMap = map[string]string{
		// Logs & Wood
		"LOG":     "OAK_LOG",
		"LOG:1":   "SPRUCE_LOG", // Common variant
		"LOG-1":   "SPRUCE_LOG",
		"LOG:2":   "BIRCH_LOG",
		"LOG-2":   "BIRCH_LOG",
		"LOG:3":   "JUNGLE_LOG",
		"LOG-3":   "JUNGLE_LOG",
		"LOG_2":   "ACACIA_LOG", // Actually has sub-id 0
		"LOG_2:0": "ACACIA_LOG",
		"LOG_2-0": "ACACIA_LOG",
		"LOG_2:1": "DARK_OAK_LOG",
		"LOG_2-1": "DARK_OAK_LOG",
		"WOOD":    "OAK_PLANKS",
		"WOOD:1":  "SPRUCE_PLANKS",
		"WOOD-1":  "SPRUCE_PLANKS",
		"WOOD:2":  "BIRCH_PLANKS",
		"WOOD-2":  "BIRCH_PLANKS",
		"WOOD:3":  "JUNGLE_PLANKS",
		"WOOD-3":  "JUNGLE_PLANKS",
		"WOOD:4":  "ACACIA_PLANKS",
		"WOOD-4":  "ACACIA_PLANKS",
		"WOOD:5":  "DARK_OAK_PLANKS",
		"WOOD-5":  "DARK_OAK_PLANKS",

		// Dyes
		"INK_SACK":    "INK_SAC", // Minecraft standard pre-1.13
		"INK_SACK:0":  "INK_SAC",
		"INK_SACK:1":  "ROSE_RED",     // or RED_DYE
		"INK_SACK:2":  "CACTUS_GREEN", // or GREEN_DYE
		"INK_SACK:3":  "COCOA_BEANS",  // Brown Dye
		"INK_SACK:4":  "LAPIS_LAZULI", // Blue Dye
		"INK_SACK-4":  "LAPIS_LAZULI", // Common variant
		"INK_SACK:5":  "PURPLE_DYE",
		"INK_SACK:6":  "CYAN_DYE",
		"INK_SACK:7":  "LIGHT_GRAY_DYE",
		"INK_SACK:8":  "GRAY_DYE",
		"INK_SACK:9":  "PINK_DYE",
		"INK_SACK:10": "LIME_DYE",
		"INK_SACK:11": "DANDELION_YELLOW", // or YELLOW_DYE
		"INK_SACK:12": "LIGHT_BLUE_DYE",
		"INK_SACK:13": "MAGENTA_DYE",
		"INK_SACK:14": "ORANGE_DYE",
		"INK_SACK:15": "BONE_MEAL", // White Dye

		// Misc
		"RAW_FISH":   "RAW_SALMON", // Default for raw fish if no sub-id often salmon in SB
		"RAW_FISH:0": "RAW_COD",    // Historically
		"RAW_FISH:1": "RAW_SALMON",
		"RAW_FISH:2": "PUFFERFISH", // Clownfish was 2, but puffer is more common
		"RAW_FISH:3": "PUFFERFISH", // Pufferfish

		"ENDER_STONE": "END_STONE", // Common typo/alternative

		// Enchanted Books (the ID is just ENCHANTED_BOOK, specific enchants are NBT)
		// For simplicity, we don't normalize specific enchants here unless they have unique IDs in Bazaar.

		// Potions (Base Potion ID is POTION, specifics are NBT)
		// Similar to enchanted books.

		// Add more mappings as identified from recipe files or API data.
		// Example: "ENCHANTED_LAPIS_LAZULI_BLOCK" -> "ENCHANTED_LAPIS_BLOCK" (if API uses shorter)
	}
	dlog("Normalization map initialized with %d entries.", len(itemIDNormalizationMap))
}

// NormalizeItemID converts a given item ID string to a standardized format.
// It converts to uppercase, trims whitespace, and applies mappings for known variations.
func NormalizeItemID(id string) string {
	if id == "" {
		return ""
	}
	// Ensure map is initialized (thread-safe)
	normalizeMapOnce.Do(initializeNormalizationMap)

	standardID := strings.ToUpper(strings.TrimSpace(id))

	// Apply mappings
	if normalized, ok := itemIDNormalizationMap[standardID]; ok {
		return normalized
	}
	return standardID // Return standardized ID if no specific mapping found
}

// BAZAAR_ID is an alias for NormalizeItemID, specifically for IDs used with Bazaar/API.
func BAZAAR_ID(id string) string {
	return NormalizeItemID(id)
}

// aggregateCells reads recipe cells ("ITEM_ID:AMOUNT" or "ITEM_ID")
// and returns a map of NORMALIZED ingredient IDs to their total amounts per single craft.
func aggregateCells(cells map[string]string) (map[string]float64, error) {
	// Standard crafting grid positions
	positions := []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"}
	ingredients := make(map[string]float64)
	var firstErrorEncountered error

	for _, pos := range positions {
		cellContent := strings.TrimSpace(cells[pos])
		if cellContent == "" { // Skip empty cells
			continue
		}

		parts := strings.SplitN(cellContent, ":", 2)
		ingIDRaw := strings.TrimSpace(parts[0])
		ingIDNormalized := BAZAAR_ID(ingIDRaw) // Normalize the ID

		if ingIDNormalized == "" {
			dlog("WARN (aggregateCells): Skipping empty or invalid ingredient ID in cell '%s': Content '%s'", pos, cellContent)
			continue
		}

		amt := 1.0 // Default amount if not specified
		if len(parts) == 2 {
			amtStr := strings.TrimSpace(parts[1])
			parsedAmt, err := strconv.ParseFloat(amtStr, 64)
			if err != nil || parsedAmt <= 0 || math.IsNaN(parsedAmt) || math.IsInf(parsedAmt, 0) {
				errMsg := fmt.Sprintf("invalid amount '%s' for ingredient '%s' (raw: '%s') in cell '%s'", amtStr, ingIDNormalized, ingIDRaw, pos)
				dlog("WARN (aggregateCells): %s. Using default amount 1.0. Error: %v", errMsg, err)
				// Don't set amt to 1.0 here if it failed, let the default 1.0 from initialization be used
				// or handle error more strictly if amounts are critical.
				// For now, log and use 1.0 as a fallback if parsing fails.
				if firstErrorEncountered == nil { // Capture the first error
					firstErrorEncountered = fmt.Errorf(errMsg + ": " + err.Error())
				}
				// Fallback to amount 1.0 for this problematic entry.
				// Depending on strictness, could error out completely.
				amt = 1.0
			} else {
				amt = parsedAmt
			}
		}
		ingredients[ingIDNormalized] += amt // Sum amounts for the same normalized ingredient ID
	}
	return ingredients, firstErrorEncountered
}

// isInPath checks if a NORMALIZED item name is already in the current expansion path.
// Path stores ItemSteps {name (normalized), quantity}
func isInPath(itemName string, path []ItemStep) bool {
	normalizedItemName := BAZAAR_ID(itemName) // Ensure comparison item is also normalized
	for _, step := range path {
		// Assuming step.name is already normalized when added to path
		if step.name == normalizedItemName {
			return true
		}
	}
	return false
}

// --- Formatting Helpers ---
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
	if sec < 0 { // Negative durations usually indicate an error or undefined state
		return fmt.Sprintf("N/A (<0s: %.2f)", sec)
	}
	if sec == 0 {
		return "0.0s"
	}

	// Simple formatting, can be expanded (e.g., to days, weeks)
	if sec < 1 { // Milliseconds or very short
		return fmt.Sprintf("%.2fs", sec) // e.g., 0.25s
	}
	if sec < 60 { // Seconds
		return fmt.Sprintf("%.1fs", sec) // e.g., 30.5s
	}
	mins := sec / 60
	if mins < 60 { // Minutes
		return fmt.Sprintf("%.1fm", mins) // e.g., 2.5m
	}
	hours := mins / 60
	if hours < 24 { // Hours
		return fmt.Sprintf("%.1fh", hours) // e.g., 3.2h
	}
	days := hours / 24
	return fmt.Sprintf("%.1fd", days) // e.g., 1.7d
}

func formatCost(cost float64) string {
	if math.IsNaN(cost) {
		return "N/A (NaN)"
	}
	if math.IsInf(cost, 1) {
		return "Infinite"
	}
	if math.IsInf(cost, -1) {
		return "-Infinite" // Or "N/A (-Inf)"
	}
	if cost == 0 {
		return "0.00" // Consistent decimal places
	}
	// Format with commas for thousands, etc., if desired (more complex)
	return fmt.Sprintf("%.2f", cost) // Standard to 2 decimal places
}

// sanitizeFloat converts NaN or Inf float64 values to 0.0.
// Useful for fields where NaN/Inf are not semantically meaningful or cause issues downstream
// if not explicitly handled (e.g. some database types, or calculations expecting numbers).
// Consider if 0.0 is the correct default for your use case, or if another value (e.g. -1, or keeping NaN) is better.
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) { // Catches both +Inf and -Inf
		return 0.0
	}
	return f
}

// --- Safe Data Access Helpers (from API response and Metrics map) ---

// safeGetProductData retrieves product data from the API response using a normalized ID.
func safeGetProductData(apiResp *HypixelAPIResponse, productID string) (HypixelProduct, bool) {
	if apiResp == nil || apiResp.Products == nil {
		return HypixelProduct{}, false
	}
	lookupID := BAZAAR_ID(productID) // Normalize before lookup
	productData, ok := apiResp.Products[lookupID]
	return productData, ok
}

// safeGetMetricsData retrieves metrics data from the map using a normalized ID.
func safeGetMetricsData(metricsMap map[string]ProductMetrics, productID string) (ProductMetrics, bool) {
	if metricsMap == nil {
		return ProductMetrics{}, false
	}
	lookupID := BAZAAR_ID(productID) // Normalize before lookup
	metricsData, ok := metricsMap[lookupID]
	return metricsData, ok
}

// getSellPrice safely gets the top sell order price (price to insta-buy).
// Returns 0.0 if not available or invalid.
func getSellPrice(apiResp *HypixelAPIResponse, itemID string) float64 {
	itemIDNorm := BAZAAR_ID(itemID)
	prod, ok := safeGetProductData(apiResp, itemIDNorm)
	if !ok || len(prod.SellSummary) == 0 {
		return 0.0 // No data or no sell orders
	}
	price := prod.SellSummary[0].PricePerUnit
	// Validate price
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0.0 // Invalid price
	}
	return price
}

// getBuyPrice safely gets the top buy order price (price to insta-sell).
// Returns 0.0 if not available or invalid.
func getBuyPrice(apiResp *HypixelAPIResponse, itemID string) float64 {
	itemIDNorm := BAZAAR_ID(itemID)
	prod, ok := safeGetProductData(apiResp, itemIDNorm)
	if !ok || len(prod.BuySummary) == 0 {
		return 0.0 // No data or no buy orders
	}
	price := prod.BuySummary[0].PricePerUnit
	// Validate price
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0.0 // Invalid price
	}
	return price
}

// getMetrics safely retrieves ProductMetrics for a normalized item ID.
// Returns an empty ProductMetrics struct if not found.
func getMetrics(metricsMap map[string]ProductMetrics, itemID string) ProductMetrics {
	itemIDNorm := BAZAAR_ID(itemID)
	metrics, _ := safeGetMetricsData(metricsMap, itemIDNorm) // Ignores 'ok' for simplicity, returns empty struct on fail
	return metrics
}

// --- Console Clear (for interactive debugging if ever needed) ---
var clear map[string]func() // Map of OS to clear function

func init() { // init is called automatically by Go
	clear = make(map[string]func())
	clear["linux"] = func() {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		_ = cmd.Run() // Error ignored for simple clear
	}
	clear["darwin"] = clear["linux"] // MacOS uses 'clear'
	clear["windows"] = func() {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	}
}

// clearConsole attempts to clear the console screen.
func clearConsole() {
	value, ok := clear[runtime.GOOS] // Get clear function for current OS
	if ok {
		value() // Execute it
	} else {
		log.Println("Warning: Console clear not supported on OS:", runtime.GOOS)
	}
}

// --- Comparison Helper ---

// mapsAreEqual compares two maps of string to float64 for equality, within a tolerance.
func mapsAreEqual(map1, map2 map[string]float64) bool {
	if len(map1) != len(map2) {
		return false
	}
	tolerance := 1e-9 // Define a small tolerance for float comparison
	for key, val1 := range map1 {
		val2, ok := map2[key]
		if !ok || math.Abs(val1-val2) > tolerance { // Check if key exists and value is within tolerance
			return false
		}
	}
	return true
}
