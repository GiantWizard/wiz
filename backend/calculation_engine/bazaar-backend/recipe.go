// recipe.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// --- Recipe Structs (Defined here) ---
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
	ItemID  string       `json:"itemid"`  // Field representing the canonical ID in JSON
	Recipe  SingleRecipe `json:"recipe"`  // Primary recipe object
	Recipes []Recipe     `json:"recipes"` // Array for alternative recipes
	// Add other fields present in your JSONs if needed (DisplayName, Lore, etc.)
}

// ItemStep is used for cycle detection during recursive expansion
// Needs to be accessible (e.g., defined here or globally if needed)
type ItemStep struct {
	name     string // Should store the NORMALIZED item ID
	quantity float64
}

// --- Recursive Helper Function ---
// expandItemRecursive performs the recursive part of the expansion.
// It handles ingredient decisions based on getBestC10M, force-expands non-bazaar items,
// and prunes cycles back to the original top-level item.
// Assumes BAZAAR_ID, isInPath, aggregateCells are defined in utils.go
// Assumes getBestC10M is defined in c10m.go
func expandItemRecursive(
	itemName string, // The item being crafted in this step
	quantity float64, // Quantity needed
	path []ItemStep, // Current path for cycle detection
	originalTopLevelItemID string, // Normalized ID of the initial item request
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (map[string]float64, error) { // Returns base map or critical error

	itemNameNorm := BAZAAR_ID(itemName)
	dlog("  -> Recursive: Expanding %.2f x %s, Path: %v, TopLevel: %s", quantity, itemNameNorm, path, originalTopLevelItemID)
	finalIngredients := make(map[string]float64)

	// --- Cycle Detection (Handles Top-Level Pruning) ---
	if isInPath(itemNameNorm, path) { // Assumes isInPath in utils.go
		if itemNameNorm == originalTopLevelItemID {
			// CYCLE BACK TO TOP LEVEL
			dlog("  <- Recursive: Cycle detected back to TOP LEVEL item '%s'. Pruning branch.", itemNameNorm)
			return make(map[string]float64), nil // Return empty map, prune this path
		} else {
			// CYCLE TO INTERMEDIATE ITEM
			dlog("  <- Recursive: Cycle detected to intermediate item '%s'. Treating as base for this branch.", itemNameNorm)
			finalIngredients[itemNameNorm] = quantity // Add intermediate item as base for this path
			return finalIngredients, nil
		}
	}
	// --- End Cycle Detection ---

	// Create new path slice for this recursion level
	currentPath := append([]ItemStep{}, path...) // Copy path
	currentPath = append(currentPath, ItemStep{name: itemNameNorm, quantity: quantity})

	// --- Load Recipe ---
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	recipeFileExists := false
	if _, err := os.Stat(filePath); err == nil {
		recipeFileExists = true
	} else if !os.IsNotExist(err) {
		// Filesystem error checking recipe is critical
		return nil, fmt.Errorf("Recursive checking recipe file '%s': %w", filePath, err)
	}

	if !recipeFileExists {
		// If no recipe file, treat this item as a base ingredient for the parent caller
		dlog("  <- Recursive: No recipe for '%s'. Treating as base.", itemNameNorm)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	// --- Read/Parse Recipe ---
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Filesystem error reading file is critical
		return nil, fmt.Errorf("Recursive reading recipe file '%s': %w", filePath, err)
	}
	var item Item // Uses struct defined in this file
	if err := json.Unmarshal(data, &item); err != nil {
		// If JSON is invalid, treat as base, but log warning
		dlog("  <- Recursive: Failed JSON parse for '%s'. Treating as base. Error: %v", itemNameNorm, err)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil // Not critical, but can't proceed down this path
	}

	// --- Select Recipe and Aggregate Ingredients ---
	var chosenRecipeCells map[string]string
	var craftedAmount float64 = 1.0
	recipeContentExists := false
	// Recipe selection logic (use first valid from recipes array, fallback to recipe object)
	if len(item.Recipes) > 0 {
		firstRecipe := item.Recipes[0]
		// Check if the first recipe has any actual content
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
			} else {
				craftedAmount = 1.0
			} // Default to 1 if count missing/zero
			recipeContentExists = true
		}
	}
	// Fallback to the single recipe object if recipes array was empty or first entry had no content
	if !recipeContentExists && (item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" || item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" || item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "") {
		chosenRecipeCells = map[string]string{"A1": item.Recipe.A1, "A2": item.Recipe.A2, "A3": item.Recipe.A3, "B1": item.Recipe.B1, "B2": item.Recipe.B2, "B3": item.Recipe.B3, "C1": item.Recipe.C1, "C2": item.Recipe.C2, "C3": item.Recipe.C3}
		if item.Recipe.Count > 0 {
			craftedAmount = float64(item.Recipe.Count)
		} else {
			craftedAmount = 1.0
		} // Default to 1
		recipeContentExists = true
	}

	if !recipeContentExists {
		// If file exists but has no usable recipe content
		dlog("  <- Recursive: No recipe content for '%s'. Treating as base.", itemNameNorm)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	ingredientsInOneCraft, aggErr := aggregateCells(chosenRecipeCells) // Assumes aggregateCells in utils.go
	if aggErr != nil {
		// Treat as base if ingredient amounts are invalid
		dlog("  <- Recursive: Error parsing ingredients for '%s': %v. Treating as base.", itemNameNorm, aggErr)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}
	if len(ingredientsInOneCraft) == 0 {
		// Treat as base if recipe yields nothing
		dlog("  <- Recursive: Recipe for '%s' yields zero ingredients. Treating as base.", itemNameNorm)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	// --- Process Ingredients ---
	numCraftsNeeded := math.Ceil(quantity / craftedAmount)
	dlog("  Recursive: Need %.0f crafts for %s.", numCraftsNeeded, itemNameNorm)

	for ingName, ingAmtPerCraft := range ingredientsInOneCraft {
		totalIngAmtNeeded := ingAmtPerCraft * numCraftsNeeded
		if totalIngAmtNeeded <= 0 {
			continue
		}
		ingNameNorm := BAZAAR_ID(ingName) // Normalize ingredient name
		dlog("    Recursive: Processing Ingredient: %.2f x %s", totalIngAmtNeeded, ingNameNorm)

		// --- C10M Check & Forced Expansion Logic for the INGREDIENT ---
		ingCost, ingMethod, _, _, errIng := getBestC10M(ingNameNorm, totalIngAmtNeeded, apiResp, metricsMap) // Assumes getBestC10M in c10m.go
		dlog("      Recursive: Ingredient C10M Check (%s): Cost=%.2f, Method=%s, Err=%v", ingNameNorm, ingCost, ingMethod, errIng)

		expandIngredient := false // Default: Buy/Base
		isApiNotFoundError := false
		if errIng != nil && strings.Contains(errIng.Error(), "API data not found") {
			isApiNotFoundError = true
		}

		// Decision Logic: Expand if Primary is best OR if not on Bazaar but recipe exists
		if isApiNotFoundError {
			// Check if recipe exists for this non-bazaar ingredient
			ingFilePath := filepath.Join(itemFilesDir, ingNameNorm+".json")
			if _, statErr := os.Stat(ingFilePath); statErr == nil {
				// Recipe exists, force expansion attempt
				expandIngredient = true
				dlog("      Recursive: Decision for %s: FORCE EXPAND (Not on Bazaar, recipe exists)", ingNameNorm)
			} else if os.IsNotExist(statErr) {
				// Not on Bazaar AND no recipe file -> Treat as base
				expandIngredient = false
				dlog("      Recursive: Decision for %s: TREAT AS BASE (Not on Bazaar, no recipe)", ingNameNorm)
				log.Printf("WARN (Recursive): Ingredient '%s' is not on Bazaar and has no recipe file. Path impossible if needed.", ingNameNorm)
			} else {
				// Filesystem error checking recipe -> Treat as base, log error
				expandIngredient = false
				log.Printf("ERROR (Recursive): FS error checking recipe for non-bazaar '%s': %v. Treating as base.", ingNameNorm, statErr)
			}
		} else if errIng == nil && ingMethod == "Primary" && !math.IsInf(ingCost, 0) && !math.IsNaN(ingCost) && ingCost >= 0 {
			// Standard case: Primary is best and valid cost
			expandIngredient = true
			dlog("      Recursive: Decision for %s: EXPAND (Primary C10M best)", ingNameNorm)
		} else {
			// All other cases: Buy or Treat as Base
			expandIngredient = false
			dlog("      Recursive: Decision for %s: BUY/BASE (Method: %s, Valid Cost: %v, Err: %v)", ingNameNorm, ingMethod, !math.IsInf(ingCost, 0) && !math.IsNaN(ingCost) && ingCost >= 0, errIng)
		}
		// --- End Ingredient Decision Logic ---

		// --- Process Ingredient based on decision ---
		if !expandIngredient {
			// Add ingredient to the current map
			currentAmt := finalIngredients[ingNameNorm]
			finalIngredients[ingNameNorm] += totalIngAmtNeeded
			dlog("      Recursive: Adding to final map: %s -> %.2f + %.2f = %.2f", ingNameNorm, currentAmt, totalIngAmtNeeded, finalIngredients[ingNameNorm])
		} else {
			// Recursively expand the ingredient
			dlog("      Recursive: Calling recursive helper for %.2f x %s...", totalIngAmtNeeded, ingNameNorm)
			// Pass the originalTopLevelItemID down the recursive call unchanged
			subIngredients, errExpand := expandItemRecursive(ingNameNorm, totalIngAmtNeeded, currentPath, originalTopLevelItemID, apiResp, metricsMap, itemFilesDir)
			if errExpand != nil {
				// If a sub-expansion fails critically, propagate the error up
				dlog("ERROR: Recursive: Critical error sub-expanding '%s': %v. Aborting branch.", ingNameNorm, errExpand)
				return nil, fmt.Errorf("failed recursive expansion for ingredient '%s': %w", ingNameNorm, errExpand)
			}

			// Merge results from the sub-expansion into the current finalIngredients map
			dlog("      Recursive: Merging results from %s expansion:", ingNameNorm)
			for subName, subAmt := range subIngredients { // subIngredients will be EMPTY if the sub-call hit a top-level cycle
				subNameNorm := BAZAAR_ID(subName) // Ensure key is normalized
				currentAmt := finalIngredients[subNameNorm]
				finalIngredients[subNameNorm] += subAmt // Add the amount from the sub-expansion
				dlog("        - %s: %.2f + %.2f = %.2f", subNameNorm, currentAmt, subAmt, finalIngredients[subNameNorm])
			}
		}
	} // End ingredient loop

	dlog("  <- Recursive: Exiting for %s. Returning map: %+v", itemNameNorm, finalIngredients)
	return finalIngredients, nil // Success
}

// --- Main Expansion Function for this File ---
// This is the entry point called by expansion.go.
// It checks if the top-level item has a recipe and then calls the recursive helper.
func ExpandItem(
	itemName string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (map[string]float64, error) { // Returns base map or critical error

	itemNameNorm := BAZAAR_ID(itemName) // Normalize the top-level item ID
	dlog("-> ExpandItem called for %.2f x %s", quantity, itemNameNorm)

	// Check for recipe existence first for the top-level item.
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// If the top-level item has NO recipe, it *is* the base ingredient.
		dlog("<- ExpandItem: No recipe file for '%s'. Cannot expand. Returning item as base.", itemNameNorm)
		// Return the item itself as the only base ingredient.
		return map[string]float64{itemNameNorm: quantity}, nil
	} else if err != nil {
		// Filesystem error checking the recipe file is critical.
		return nil, fmt.Errorf("ExpandItem checking recipe file '%s': %w", filePath, err)
	}

	// Recipe exists, call the recursive helper to start the expansion process.
	// Pass itemNameNorm as the originalTopLevelItemID. Start with an empty path.
	dlog("   ExpandItem: Recipe found. Calling recursive helper for %s...", itemNameNorm)
	baseMap, errRec := expandItemRecursive(itemNameNorm, quantity, nil, itemNameNorm, apiResp, metricsMap, itemFilesDir)

	if errRec != nil {
		log.Printf("ERROR (ExpandItem): Recursive helper failed for %s: %v", itemNameNorm, errRec)
		// Propagate critical error from recursion.
		return nil, fmt.Errorf("recursive expansion failed within ExpandItem: %w", errRec)
	}

	// Recursion completed (successfully or hit non-critical bases/cycles)
	dlog("<- ExpandItem: Expansion call for %s complete. Final map: %+v", itemNameNorm, baseMap)
	return baseMap, nil // Return the final map of base ingredients
}
