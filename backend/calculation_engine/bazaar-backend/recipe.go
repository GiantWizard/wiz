package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// --- Recipe Structs ---
// Ensure these match the structure in your item JSON files
type Recipe struct {
	Type  string `json:"type"` // Optional type identifier
	A1    string `json:"A1"`
	A2    string `json:"A2"`
	A3    string `json:"A3"`
	B1    string `json:"B1"`
	B2    string `json:"B2"`
	B3    string `json:"B3"`
	C1    string `json:"C1"`
	C2    string `json:"C2"`
	C3    string `json:"C3"`
	Count int    `json:"count"` // How many items this recipe yields
}

type SingleRecipe struct {
	A1    string `json:"A1"`
	A2    string `json:"A2"`
	A3    string `json:"A3"`
	B1    string `json:"B1"`
	B2    string `json:"B2"`
	B3    string `json:"B3"`
	C1    string `json:"C1"`
	C2    string `json:"C2"`
	C3    string `json:"C3"`
	Count int    `json:"count"` // How many items this recipe yields
}

// Item represents the structure of your item JSON files
type Item struct {
	ItemID  string       `json:"itemid"`  // Or another field representing the canonical ID
	Recipe  SingleRecipe `json:"recipe"`  // Primary recipe object
	Recipes []Recipe     `json:"recipes"` // Array for alternative recipes
	// Add other fields present in your JSONs if needed (DisplayName, Lore, etc.)
}

// ItemStep is used for cycle detection during recursive expansion
type ItemStep struct {
	name     string // Should store the NORMALIZED item ID
	quantity float64
}

// aggregateCells reads recipe cells ("ITEM_ID:AMOUNT" or "ITEM_ID")
// and returns a map of NORMALIZED ingredient IDs to their total amounts.
func aggregateCells(cells map[string]string) (map[string]float64, error) {
	positions := []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"}
	ingredients := make(map[string]float64)
	var firstError error

	for _, pos := range positions {
		cell := strings.TrimSpace(cells[pos])
		if cell == "" {
			continue
		}

		parts := strings.SplitN(cell, ":", 2)
		// Normalize the ingredient ID right after extracting it
		ing := BAZAAR_ID(strings.TrimSpace(parts[0])) // Assumes BAZAAR_ID (which uses NormalizeItemID) is defined elsewhere
		if ing == "" {
			continue // Skip empty ingredient names
		}

		amt := 1.0 // Default amount is 1
		if len(parts) == 2 {
			amtStr := strings.TrimSpace(parts[1])
			parsedAmt, err := strconv.ParseFloat(amtStr, 64)
			if err != nil || parsedAmt <= 0 {
				// Log error but maybe continue with default amount? Or return error?
				// Let's log and continue with 1.0 for now, but store the first error.
				dlog("WARN: Invalid amount '%s' for ingredient '%s' in cell '%s'. Using 1.0. Error: %v", amtStr, ing, pos, err) // Assumes dlog is defined elsewhere
				amt = 1.0
				if firstError == nil { // Store only the first parsing error
					firstError = fmt.Errorf("invalid amount '%s' for ingredient '%s'", amtStr, ing)
				}
			} else {
				amt = parsedAmt
			}
		}
		ingredients[ing] += amt
	}
	// Return the map and the first error encountered during amount parsing (if any)
	return ingredients, firstError
}

// isInPath checks if a NORMALIZED item name is already in the current expansion path.
func isInPath(itemName string, path []ItemStep) bool {
	normalizedItemName := BAZAAR_ID(itemName) // Ensure comparison ID is normalized
	for _, step := range path {
		// Path should already contain normalized names
		if step.name == normalizedItemName {
			return true
		}
	}
	return false
}

// expandItem recursively expands an item into its base ingredients based on C10M cost,
// automatically detecting cycles and forcing expansion for non-Bazaar items with recipes.
// Returns a map of base ingredient IDs to their total required quantity, and any error.
// Assumes all dependent functions (BAZAAR_ID, dlog, getBestC10M, etc.) and structs are defined externally.
func expandItem(
	itemName string,
	quantity float64,
	path []ItemStep,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (map[string]float64, error) {

	itemName = BAZAAR_ID(itemName) // Normalize ID at the beginning
	dlog("Expanding %.2f x %s (Cost-Based), Path: %v", quantity, itemName, path)
	finalIngredients := make(map[string]float64)

	// --- Cycle Detection ---
	if isInPath(itemName, path) { // isInPath uses normalized name
		dlog("  Cycle detected for '%s'. Stopping expansion at this branch. Treating as base.", itemName)
		finalIngredients[itemName] = quantity // Add the item itself as a requirement
		return finalIngredients, nil
	}

	// Add current item (normalized) to the path *before* recursive calls
	currentPath := append(path, ItemStep{name: itemName, quantity: quantity})

	// --- Check for recipe file ---
	filePath := filepath.Join(itemFilesDir, itemName+".json") // Use normalized name
	recipeFileExists := true
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		dlog("  No recipe file found for '%s'. Treating as base ingredient.", itemName)
		recipeFileExists = false
	} else if err != nil {
		return nil, fmt.Errorf("error checking recipe file '%s': %w", filePath, err) // Propagate FS error
	}

	// If no recipe file exists, it's a base ingredient for this path.
	if !recipeFileExists {
		finalIngredients[itemName] = quantity
		return finalIngredients, nil
	}

	// --- Recipe file exists, read and parse it ---
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading recipe file '%s': %w", filePath, err)
	} // Propagate FS error

	var item Item // Assumes Item struct defined elsewhere
	if err := json.Unmarshal(data, &item); err != nil {
		// Treat as base if JSON is invalid, log warning
		dlog("WARN: Failed JSON parse for '%s'. Treating as base. Error: %v", itemName, err)
		finalIngredients[itemName] = quantity
		return finalIngredients, nil // Don't propagate JSON error, just treat as base
	}

	// --- Determine which recipe to use AND check if recipe has content ---
	var chosenRecipeCells map[string]string
	var craftedAmount float64 = 1.0
	recipeContentExists := false
	// Logic to select recipe (prefer 'recipes' array, fallback to 'recipe')
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
			recipeContentExists = true
			dlog("  Using 'recipes' array[0] for '%s'.", itemName)
		}
	}
	if !recipeContentExists && (item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" || item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" || item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "") {
		chosenRecipeCells = map[string]string{"A1": item.Recipe.A1, "A2": item.Recipe.A2, "A3": item.Recipe.A3, "B1": item.Recipe.B1, "B2": item.Recipe.B2, "B3": item.Recipe.B3, "C1": item.Recipe.C1, "C2": item.Recipe.C2, "C3": item.Recipe.C3}
		if item.Recipe.Count > 0 {
			craftedAmount = float64(item.Recipe.Count)
		}
		recipeContentExists = true
		dlog("  Using 'recipe' object for '%s'.", itemName)
	}

	// If recipe file exists but has no actual recipe content, treat as base
	if !recipeContentExists {
		dlog("  Recipe file '%s' has no usable recipe content. Treating as base.", itemName)
		finalIngredients[itemName] = quantity
		return finalIngredients, nil
	}

	// --- Aggregate ingredients (already normalized by aggregateCells) ---
	ingredientsInOneCraft, aggErr := aggregateCells(chosenRecipeCells)
	if aggErr != nil {
		// If ingredient amounts are invalid in the JSON
		dlog("WARN: Error parsing ingredient amounts for '%s': %v. Treating as base.", itemName, aggErr)
		finalIngredients[itemName] = quantity
		return finalIngredients, nil // Treat as base, don't propagate cell format error
	}
	if len(ingredientsInOneCraft) == 0 {
		dlog("  Recipe for '%s' yields zero ingredients. Treating as base.", itemName)
		finalIngredients[itemName] = quantity
		return finalIngredients, nil
	}

	// --- Cost Comparison & Forced Expansion Logic ---
	dlog("  Performing cost comparison/expansion check for '%s'...", itemName)

	// Cost to buy the target item directly
	costToBuy, buyMethod, _, _, errBuy := getBestC10M(itemName, quantity, apiResp, metricsMap) // Assumes getBestC10M defined externally
	isApiNotFoundError := false
	if errBuy != nil && strings.Contains(errBuy.Error(), "API data not found") { // Check specific error message
		isApiNotFoundError = true
		dlog("    Item '%s' not found in Bazaar API data.", itemName)
	}
	// Only log the generic errBuy if it wasn't the specific API not found error we already handled
	otherErrorBuy := errBuy
	if isApiNotFoundError {
		otherErrorBuy = nil
	}
	dlog("    Cost to buy %.2f x %s: %s (%s) (API Not Found: %v, Other Error: %v)", quantity, itemName, formatCost(costToBuy), buyMethod, isApiNotFoundError, otherErrorBuy) // Assumes formatCost defined elsewhere

	// Calculate total Best C10M cost to craft (if possible)
	totalCraftCost := 0.0
	canCalculateCraftCost := true
	numCraftsNeeded := math.Ceil(quantity / craftedAmount)
	dlog("    Need %.0f crafts.", numCraftsNeeded)

	for ingName, ingAmtPerCraft := range ingredientsInOneCraft { // ingName is already normalized from aggregateCells
		totalIngAmtNeeded := ingAmtPerCraft * numCraftsNeeded
		ingCost, ingMethod, _, _, errIng := getBestC10M(ingName, totalIngAmtNeeded, apiResp, metricsMap)
		dlog("      Cost of ingredient %.2f x %s: %s (%s) (Error: %v)", totalIngAmtNeeded, ingName, formatCost(ingCost), ingMethod, errIng)
		if errIng != nil || math.IsInf(ingCost, 1) || math.IsNaN(ingCost) {
			dlog("      Cannot determine valid cost for ingredient '%s'.", ingName)
			canCalculateCraftCost = false
		} else if canCalculateCraftCost { // Only add if no failure *yet* in this loop
			totalCraftCost += ingCost
		}
	}
	dlog("    Total finite craft cost components sum: %.2f", totalCraftCost)

	// --- Decision Logic ---
	expandDecision := false
	// Condition 1: Force expansion if item not on Bazaar BUT has a recipe
	// We already established recipeContentExists is true to reach here.
	if isApiNotFoundError {
		dlog("    Decision: Item '%s' not on Bazaar but has recipe. FORCING expansion.", itemName)
		expandDecision = true
	} else {
		// Condition 2: Normal cost comparison if item IS on Bazaar
		if !canCalculateCraftCost {
			dlog("    Decision: Cannot calc finite craft cost. NOT expanding '%s'.", itemName)
			expandDecision = false
		} else if math.IsInf(costToBuy, 1) || math.IsNaN(costToBuy) {
			// Buy cost is Inf/NaN (and not due to API error), and craft cost IS finite. Expand.
			dlog("    Decision: Buy cost Inf/NaN. Expanding '%s' (Craft Cost: %.2f).", itemName, totalCraftCost)
			expandDecision = true
		} else if totalCraftCost < costToBuy {
			dlog("    Decision: Craft cost (%.2f) < Buy cost (%.2f). Expanding '%s'.", totalCraftCost, costToBuy, itemName)
			expandDecision = true
		} else {
			dlog("    Decision: Craft cost (%.2f) >= Buy cost (%.2f). NOT expanding '%s'.", totalCraftCost, costToBuy, itemName)
			expandDecision = false
		}
	}

	// --- Process based on decision ---
	if expandDecision {
		dlog("  Expanding ingredients for '%s'...", itemName)
		var firstExpansionError error
		for ingName, ingAmtPerCraft := range ingredientsInOneCraft { // ingName is already normalized
			totalIngAmtNeeded := ingAmtPerCraft * numCraftsNeeded
			dlog("    Recursively expanding %.2f x %s", totalIngAmtNeeded, ingName)
			// Recursive call uses currentPath
			subIngredients, errExpand := expandItem(ingName, totalIngAmtNeeded, currentPath, apiResp, metricsMap, itemFilesDir)
			if errExpand != nil {
				dlog("ERROR: Critical error sub-expanding '%s' for '%s': %v.", ingName, itemName, errExpand)
				if firstExpansionError == nil {
					firstExpansionError = fmt.Errorf("failed expanding ingredient '%s' for '%s': %w", ingName, itemName, errExpand)
				}
				// Propagate critical error immediately
				return nil, firstExpansionError
			}
			// Add results from sub-expansion
			for subName, subAmt := range subIngredients {
				finalIngredients[subName] += subAmt
				dlog("      Aggregated %.2f x %s from %s expansion", subAmt, subName, ingName)
			}
		}
		// If loop completed without critical errors
		return finalIngredients, nil
	} else {
		// Not expanding this item
		dlog("  Adding %.2f x %s to final ingredients (not expanding).", quantity, itemName)
		finalIngredients[itemName] = quantity
		return finalIngredients, nil
	}
}

// NOTE: This file assumes BAZAAR_ID, dlog, formatCost, getBestC10M,
// and structs HypixelAPIResponse, ProductMetrics are defined externally.
