package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
)

// --- Level Cost Calculation ---

// Calculate the total best C10M cost for a map of ingredients.
// Returns the total cost. Returns Inf if any ingredient cost is Inf or calculation fails.
func calculateTotalC10MForLevel(
	ingredients map[string]float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) float64 {
	totalCost := 0.0
	calculationPossible := true

	if len(ingredients) == 0 {
		return 0.0
	}

	dlog("Calculating total C10M for level with %d ingredient types", len(ingredients))

	for itemID, quantity := range ingredients {
		if quantity <= 0 {
			continue
		}
		cost, method, _, _, _, err := getBestC10M(itemID, quantity, apiResp, metricsMap)

		if err != nil || math.IsInf(cost, 1) || math.IsNaN(cost) {
			log.Printf("Level Cost Debug: Item '%s' (Qty: %.2f) has invalid cost (Cost: %s, Method: %s, Err: %v)",
				itemID, quantity, formatCost(cost), method, err)
		}

		if err != nil {
			dlog("  WARN: Error getting C10M for %.2f x %s: %v. Cost assumed Infinite.", quantity, itemID, err)
			cost = math.Inf(1)
		}

		if math.IsInf(cost, 1) || math.IsNaN(cost) {
			dlog("  Infinite/NaN cost detected for %s. Total level cost will be Infinite.", itemID)
			calculationPossible = false
			totalCost = math.Inf(1)
			break
		}

		totalCost += cost
	}

	if !calculationPossible {
		dlog("  Total C10M for level: Infinite")
		return math.Inf(1)
	}
	dlog("  Total C10M for level: %.2f", totalCost)
	return totalCost
}

// --- Level Expansion Helpers ---

// expandSingleItemOneLevel expands a single item one level down, returning its direct ingredients.
func expandSingleItemOneLevel(
	itemName string,
	itemFilesDir string,
) (map[string]float64, float64, bool, error) {

	filePath := filepath.Join(itemFilesDir, itemName+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		dlog("expandSingleItemOneLevel: No recipe file for %s", itemName)
		return nil, 1.0, false, nil // No file = cannot expand
	} else if err != nil {
		return nil, 1.0, false, fmt.Errorf("checking recipe file '%s': %w", filePath, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 1.0, false, fmt.Errorf("reading recipe file '%s': %w", filePath, err)
	}

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
		}
	}

	if !recipeContentFound && (item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" || item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" || item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "") {
		chosenRecipeCells = map[string]string{"A1": item.Recipe.A1, "A2": item.Recipe.A2, "A3": item.Recipe.A3, "B1": item.Recipe.B1, "B2": item.Recipe.B2, "B3": item.Recipe.B3, "C1": item.Recipe.C1, "C2": item.Recipe.C2, "C3": item.Recipe.C3}
		if item.Recipe.Count > 0 {
			craftedAmount = float64(item.Recipe.Count)
		}
		recipeContentFound = true
		dlog("  Using 'recipe' object for '%s'.", itemName)
	}

	if !recipeContentFound {
		dlog("expandSingleItemOneLevel: No usable recipe content for '%s'.", itemName)
		return nil, 1.0, false, nil
	}

	ingredientsInOneCraft, err := aggregateCells(chosenRecipeCells)
	if err != nil {
		return nil, craftedAmount, false, fmt.Errorf("parsing recipe cells for '%s': %w", itemName, err)
	}
	if len(ingredientsInOneCraft) == 0 {
		dlog("expandSingleItemOneLevel: Recipe for '%s' yields zero ingredients.", itemName)
		return nil, craftedAmount, false, nil
	}

	return ingredientsInOneCraft, craftedAmount, true, nil // Success, was expanded
}

// expandIngredientsOneLevel skips items without a valid recipe defined.
func expandIngredientsOneLevel(
	currentIngredients map[string]float64,
	itemFilesDir string,
) (map[string]float64, bool, error) { // Returns next ingredients, if any expansion occurred, first critical error

	nextLevelIngredients := make(map[string]float64)
	anyItemWasExpanded := false
	var firstCriticalError error

	dlog("Expanding ingredients one level deeper...")
	for itemID, quantity := range currentIngredients {
		if quantity <= 0 {
			continue
		}

		dlog("  Checking item: %.2f x %s", quantity, itemID)
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
			dlog("  Item '%s' was not expanded (no recipe/empty). Treating as base.", itemID)
			nextLevelIngredients[itemID] += quantity
		} else {
			dlog("  Expanded '%s' (%.2f per craft). Found %d ingredients.", itemID, craftedAmount, len(ingredientsPerCraft))
			anyItemWasExpanded = true
			numCraftsNeeded := math.Ceil(quantity / craftedAmount)
			dlog("    Need %.0f crafts.", numCraftsNeeded)

			for subIngID, subIngQtyPerCraft := range ingredientsPerCraft {
				totalSubIngNeeded := subIngQtyPerCraft * numCraftsNeeded
				nextLevelIngredients[subIngID] += totalSubIngNeeded
				dlog("    Added %.2f x %s to next level.", totalSubIngNeeded, subIngID)
			}
		}
	}
	dlog("One level expansion complete. Any item expanded this level: %v", anyItemWasExpanded)

	if mapsAreEqual(currentIngredients, nextLevelIngredients) {
		dlog("Next level ingredients identical. Forcing stop.")
		anyItemWasExpanded = false
	}

	return nextLevelIngredients, anyItemWasExpanded, firstCriticalError
}
