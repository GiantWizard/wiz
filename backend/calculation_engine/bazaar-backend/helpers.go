package main

import (
	"encoding/json"
	"fmt"
	"log" // Import log package
	"math"
	"os"
	"path/filepath"
	// Ensure all necessary imports for structs used below are present
	// e.g., if Item struct is defined elsewhere
)

// --- Function Stubs (If these helpers call external functions) ---
// These are placeholders if needed, replace with actual function calls.
/*
func safeGetProductData(apiResp *HypixelAPIResponse, productID string) (HypixelProduct, bool) { return HypixelProduct{}, false }
func safeGetMetricsData(metricsMap map[string]ProductMetrics, productID string) (ProductMetrics, bool) { return ProductMetrics{}, false }
func getBestC10M(itemID string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) (float64, string, float64, float64, error) { return 0, "", 0, 0, nil }
func BAZAAR_ID(id string) string { return id }
func dlog(format string, args ...interface{}) {}
func formatCost(cost float64) string { return fmt.Sprintf("%.2f", cost)} // Basic stub for formatCost
*/

// --- Level Cost Calculation ---

// Calculate the total best C10M cost for a map of ingredients.
// Returns the total cost. Returns Inf if any ingredient cost is Inf or calculation fails.
func calculateTotalC10MForLevel(
	ingredients map[string]float64,
	apiResp *HypixelAPIResponse, // Assumes HypixelAPIResponse struct defined elsewhere
	metricsMap map[string]ProductMetrics, // Assumes ProductMetrics struct defined elsewhere
) float64 {
	totalCost := 0.0
	calculationPossible := true

	if len(ingredients) == 0 {
		return 0.0
	}

	dlog("Calculating total C10M for level with %d ingredient types", len(ingredients)) // Assumes dlog defined elsewhere

	for itemID, quantity := range ingredients {
		if quantity <= 0 {
			continue
		}
		// Get 5 values now, discard unused ones (assocCost, rrValue)
		// Assumes getBestC10M defined elsewhere
		cost, method, _, _, err := getBestC10M(itemID, quantity, apiResp, metricsMap)

		// *** TEMPORARY DEBUG LOG ***
		// Print details if the cost calculation for THIS item results in Inf/NaN/Error
		if err != nil || math.IsInf(cost, 1) || math.IsNaN(cost) {
			// Assumes formatCost defined elsewhere (in utils.go)
			log.Printf("!!! Level Cost Debug: Item '%s' (Qty: %.2f) has invalid cost (Cost: %s, Method: %s, Err: %v)",
				itemID, quantity, formatCost(cost), method, err)
		}
		// **************************

		if err != nil {
			// If getBestC10M itself returned an error, treat cost as Inf
			dlog("  WARN: Error getting C10M for %.2f x %s: %v. Cost assumed Infinite.", quantity, itemID, err) // Assumes dlog defined elsewhere
			cost = math.Inf(1)
		}

		if math.IsInf(cost, 1) || math.IsNaN(cost) {
			// If cost is Inf/NaN (either originally or set due to error)
			dlog("  Infinite/NaN cost detected for %s. Total level cost will be Infinite.", itemID) // Assumes dlog defined elsewhere
			calculationPossible = false
			totalCost = math.Inf(1) // Set total cost to Inf
			break                   // Stop processing this level
		}

		// Accumulate cost only if it's finite and valid
		totalCost += cost
	} // End for loop over ingredients

	if !calculationPossible {
		dlog("  Total C10M for level: Infinite") // Assumes dlog defined elsewhere
		return math.Inf(1)
	}
	dlog("  Total C10M for level: %.2f", totalCost) // Assumes dlog defined elsewhere
	return totalCost
}

// --- Level Expansion Helpers ---

// expandSingleItemOneLevel expands a single item one level down, returning its direct ingredients.
// Does NOT perform cost checks or recursion. Checks for presence of recipe data.
// Returns: map[ingredientID -> quantityPerCraft], amountCrafted, bool wasExpanded, error criticalError
func expandSingleItemOneLevel(
	itemName string,
	itemFilesDir string,
) (map[string]float64, float64, bool, error) {

	filePath := filepath.Join(itemFilesDir, itemName+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		dlog("expandSingleItemOneLevel: No recipe file for %s", itemName) // Assumes dlog defined elsewhere
		return nil, 1.0, false, nil                                       // No file = cannot expand
	} else if err != nil {
		return nil, 1.0, false, fmt.Errorf("checking recipe file '%s': %w", filePath, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 1.0, false, fmt.Errorf("reading recipe file '%s': %w", filePath, err)
	}

	// Assumes Item struct (with Recipe and Recipes fields) is defined elsewhere
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		log.Printf("WARN: Failed to parse JSON for '%s' in expandSingleItemOneLevel. Error: %v", itemName, err)
		return nil, 1.0, false, fmt.Errorf("parsing JSON for '%s': %w", itemName, err)
	}

	var chosenRecipeCells map[string]string
	var craftedAmount float64 = 1.0
	recipeContentFound := false

	if len(item.Recipes) > 0 {
		firstRecipe := item.Recipes[0]
		tempCells := map[string]string{"A1": firstRecipe.A1, "A2": firstRecipe.A2, "A3": firstRecipe.A3, "B1": firstRecipe.B1, "B2": firstRecipe.B2, "B3": firstRecipe.B3, "C1": firstRecipe.C1, "C2": firstRecipe.C2, "C3": firstRecipe.C3}
		hasContent := false
		for _, v := range tempCells {
			if v != "" {
				hasContent = true
				break
			}
		}
		if hasContent {
			chosenRecipeCells = tempCells
			if firstRecipe.Count > 0 {
				craftedAmount = float64(firstRecipe.Count)
			}
			recipeContentFound = true
			dlog("  Using 'recipes' array[0] for '%s'.", itemName)
		} // Assumes dlog defined elsewhere
	}

	if !recipeContentFound && (item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" || item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" || item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "") {
		chosenRecipeCells = map[string]string{"A1": item.Recipe.A1, "A2": item.Recipe.A2, "A3": item.Recipe.A3, "B1": item.Recipe.B1, "B2": item.Recipe.B2, "B3": item.Recipe.B3, "C1": item.Recipe.C1, "C2": item.Recipe.C2, "C3": item.Recipe.C3}
		if item.Recipe.Count > 0 {
			craftedAmount = float64(item.Recipe.Count)
		}
		recipeContentFound = true
		dlog("  Using 'recipe' object for '%s'.", itemName) // Assumes dlog defined elsewhere
	}

	if !recipeContentFound {
		dlog("expandSingleItemOneLevel: No usable recipe content for '%s'.", itemName) // Assumes dlog defined elsewhere
		return nil, 1.0, false, nil
	}

	// Assumes aggregateCells is defined elsewhere
	ingredientsInOneCraft, err := aggregateCells(chosenRecipeCells)
	if err != nil {
		return nil, craftedAmount, false, fmt.Errorf("parsing recipe cells for '%s': %w", itemName, err)
	}
	if len(ingredientsInOneCraft) == 0 {
		dlog("expandSingleItemOneLevel: Recipe for '%s' yields zero ingredients.", itemName) // Assumes dlog defined elsewhere
		return nil, craftedAmount, false, nil
	}

	return ingredientsInOneCraft, craftedAmount, true, nil // Success, was expanded
}

// expandIngredientsOneLevel takes a map of ingredients for the current level
// and returns the map for the next level. It skips expanding items that
// don't have a valid, non-empty recipe defined.
func expandIngredientsOneLevel(
	currentIngredients map[string]float64,
	itemFilesDir string,
) (map[string]float64, bool, error) { // Returns next ingredients, if any expansion occurred, first critical error

	nextLevelIngredients := make(map[string]float64)
	anyItemWasExpanded := false
	var firstCriticalError error

	dlog("Expanding ingredients one level deeper...") // Assumes dlog defined elsewhere
	for itemID, quantity := range currentIngredients {
		if quantity <= 0 {
			continue
		}

		dlog("  Checking item: %.2f x %s", quantity, itemID) // Assumes dlog defined elsewhere
		ingredientsPerCraft, craftedAmount, wasExpanded, err := expandSingleItemOneLevel(itemID, itemFilesDir)

		if err != nil {
			log.Printf("WARN: Critical error trying to expand '%s': %v. Treating as base.", itemID, err)
			if firstCriticalError == nil {
				firstCriticalError = err
			}
			nextLevelIngredients[itemID] += quantity // Add item back as base
			continue
		}

		if !wasExpanded {
			dlog("  Item '%s' was not expanded (no recipe/empty). Treating as base.", itemID) // Assumes dlog defined elsewhere
			nextLevelIngredients[itemID] += quantity
		} else {
			dlog("  Expanded '%s' (%.2f per craft). Found %d ingredients.", itemID, craftedAmount, len(ingredientsPerCraft)) // Assumes dlog defined elsewhere
			anyItemWasExpanded = true
			numCraftsNeeded := math.Ceil(quantity / craftedAmount)
			dlog("    Need %.0f crafts.", numCraftsNeeded) // Assumes dlog defined elsewhere

			for subIngID, subIngQtyPerCraft := range ingredientsPerCraft {
				totalSubIngNeeded := subIngQtyPerCraft * numCraftsNeeded
				nextLevelIngredients[subIngID] += totalSubIngNeeded
				dlog("    Added %.2f x %s to next level.", totalSubIngNeeded, subIngID) // Assumes dlog defined elsewhere
			}
		}
	}
	dlog("One level expansion complete. Any item expanded this level: %v", anyItemWasExpanded) // Assumes dlog defined elsewhere

	// Assumes mapsAreEqual defined elsewhere
	if mapsAreEqual(currentIngredients, nextLevelIngredients) {
		dlog("Next level ingredients identical. Forcing stop.") // Assumes dlog defined elsewhere
		anyItemWasExpanded = false
	}

	return nextLevelIngredients, anyItemWasExpanded, firstCriticalError
}

// mapsAreEqual ... (keep implementation as before or ensure defined in utils.go) ...
/* // Example:
func mapsAreEqual(map1, map2 map[string]float64) bool {
    if len(map1) != len(map2) { return false }
    for key, val1 := range map1 {
        val2, ok := map2[key]
        if !ok || math.Abs(val1-val2) > 1e-9 { return false } // Tolerance
    }
    return true
}
*/

// NOTE: This file assumes necessary structs (Item, HypixelAPIResponse, ProductMetrics)
// and functions (dlog, BAZAAR_ID, aggregateCells, getBestC10M, mapsAreEqual, formatCost)
// are defined correctly in other files (recipe.go, api.go, metrics.go, utils.go, c10m.go).
