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

type Recipe struct {
	Type  string `json:"type"`
	A1    string `json:"A1"`
	A2    string `json:"A2"`
	A3    string `json:"A3"`
	B1    string `json:"B1"`
	B2    string `json:"B2"`
	B3    string `json:"B3"`
	C1    string `json:"C1"`
	C2    string `json:"C2"`
	C3    string `json:"C3"`
	Count int    `json:"count"`
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
	Count int    `json:"count"`
}

type Item struct {
	ItemID  string       `json:"itemid"`
	Recipe  SingleRecipe `json:"recipe"`
	Recipes []Recipe     `json:"recipes"`
}

type ItemStep struct {
	name     string
	quantity float64
}

func expandItemRecursive(
	itemName string,
	quantity float64,
	path []ItemStep,
	originalTopLevelItemID string,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (map[string]float64, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog("  -> Recursive: Expanding %.2f x %s, Path: %v, TopLevel: %s", quantity, itemNameNorm, path, originalTopLevelItemID)
	finalIngredients := make(map[string]float64)

	if isInPath(itemNameNorm, path) {
		if itemNameNorm == originalTopLevelItemID {
			dlog("  <- Recursive: Cycle detected back to TOP LEVEL item '%s'. Pruning branch.", itemNameNorm)
			return make(map[string]float64), nil
		} else {
			dlog("  <- Recursive: Cycle detected to intermediate item '%s'. Treating as base for this branch.", itemNameNorm)
			finalIngredients[itemNameNorm] = quantity
			return finalIngredients, nil
		}
	}

	currentPath := append([]ItemStep{}, path...)
	currentPath = append(currentPath, ItemStep{name: itemNameNorm, quantity: quantity})

	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	recipeFileExists := false
	if _, err := os.Stat(filePath); err == nil {
		recipeFileExists = true
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("Recursive checking recipe file '%s': %w", filePath, err)
	}

	if !recipeFileExists {
		dlog("  <- Recursive: No recipe for '%s'. Treating as base.", itemNameNorm)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Recursive reading recipe file '%s': %w", filePath, err)
	}
	var itemData Item // Renamed from 'item' to avoid conflict with loop var if any
	if err := json.Unmarshal(data, &itemData); err != nil {
		dlog("  <- Recursive: Failed JSON parse for '%s'. Treating as base. Error: %v", itemNameNorm, err)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	var chosenRecipeCells map[string]string
	var craftedAmount float64 = 1.0
	recipeContentExists := false
	if len(itemData.Recipes) > 0 {
		firstRecipe := itemData.Recipes[0]
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
			}
			recipeContentExists = true
		}
	}
	if !recipeContentExists && (itemData.Recipe.A1 != "" || itemData.Recipe.A2 != "" || itemData.Recipe.A3 != "" || itemData.Recipe.B1 != "" || itemData.Recipe.B2 != "" || itemData.Recipe.B3 != "" || itemData.Recipe.C1 != "" || itemData.Recipe.C2 != "" || itemData.Recipe.C3 != "") {
		chosenRecipeCells = map[string]string{"A1": itemData.Recipe.A1, "A2": itemData.Recipe.A2, "A3": itemData.Recipe.A3, "B1": itemData.Recipe.B1, "B2": itemData.Recipe.B2, "B3": itemData.Recipe.B3, "C1": itemData.Recipe.C1, "C2": itemData.Recipe.C2, "C3": itemData.Recipe.C3}
		if itemData.Recipe.Count > 0 {
			craftedAmount = float64(itemData.Recipe.Count)
		} else {
			craftedAmount = 1.0
		}
		recipeContentExists = true
	}

	if !recipeContentExists {
		dlog("  <- Recursive: No recipe content for '%s'. Treating as base.", itemNameNorm)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	ingredientsInOneCraft, aggErr := aggregateCells(chosenRecipeCells)
	if aggErr != nil {
		dlog("  <- Recursive: Error parsing ingredients for '%s': %v. Treating as base.", itemNameNorm, aggErr)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}
	if len(ingredientsInOneCraft) == 0 {
		dlog("  <- Recursive: Recipe for '%s' yields zero ingredients. Treating as base.", itemNameNorm)
		finalIngredients[itemNameNorm] = quantity
		return finalIngredients, nil
	}

	numCraftsNeeded := math.Ceil(quantity / craftedAmount)
	dlog("  Recursive: Need %.0f crafts for %s.", numCraftsNeeded, itemNameNorm)

	for ingName, ingAmtPerCraft := range ingredientsInOneCraft {
		totalIngAmtNeeded := ingAmtPerCraft * numCraftsNeeded
		if totalIngAmtNeeded <= 0 {
			continue
		}
		ingNameNorm := BAZAAR_ID(ingName)
		dlog("    Recursive: Processing Ingredient: %.2f x %s", totalIngAmtNeeded, ingNameNorm)

		// CORRECTED: getBestC10M now returns 6 values, discard ifValue (5th one)
		ingCost, ingMethod, _, _, _, errIng := getBestC10M(ingNameNorm, totalIngAmtNeeded, apiResp, metricsMap)
		dlog("      Recursive: Ingredient C10M Check (%s): Cost=%.2f, Method=%s, Err=%v", ingNameNorm, ingCost, ingMethod, errIng)

		expandIngredient := false
		isApiNotFoundError := false
		if errIng != nil && strings.Contains(errIng.Error(), "API data not found") {
			isApiNotFoundError = true
		}

		if isApiNotFoundError {
			ingFilePath := filepath.Join(itemFilesDir, ingNameNorm+".json")
			if _, statErr := os.Stat(ingFilePath); statErr == nil {
				expandIngredient = true
				dlog("      Recursive: Decision for %s: FORCE EXPAND (Not on Bazaar, recipe exists)", ingNameNorm)
			} else if os.IsNotExist(statErr) {
				expandIngredient = false
				dlog("      Recursive: Decision for %s: TREAT AS BASE (Not on Bazaar, no recipe)", ingNameNorm)
				log.Printf("WARN (Recursive): Ingredient '%s' is not on Bazaar and has no recipe file. Path impossible if needed.", ingNameNorm)
			} else {
				expandIngredient = false
				log.Printf("ERROR (Recursive): FS error checking recipe for non-bazaar '%s': %v. Treating as base.", ingNameNorm, statErr)
			}
		} else if errIng == nil && ingMethod == "Primary" && !math.IsInf(ingCost, 0) && !math.IsNaN(ingCost) && ingCost >= 0 {
			expandIngredient = true
			dlog("      Recursive: Decision for %s: EXPAND (Primary C10M best)", ingNameNorm)
		} else {
			expandIngredient = false
			dlog("      Recursive: Decision for %s: BUY/BASE (Method: %s, Valid Cost: %v, Err: %v)", ingNameNorm, ingMethod, !math.IsInf(ingCost, 0) && !math.IsNaN(ingCost) && ingCost >= 0, errIng)
		}

		if !expandIngredient {
			finalIngredients[ingNameNorm] += totalIngAmtNeeded
			dlog("      Recursive: Adding to final map: %s -> %.2f", ingNameNorm, finalIngredients[ingNameNorm])
		} else {
			dlog("      Recursive: Calling recursive helper for %.2f x %s...", totalIngAmtNeeded, ingNameNorm)
			subIngredients, errExpand := expandItemRecursive(ingNameNorm, totalIngAmtNeeded, currentPath, originalTopLevelItemID, apiResp, metricsMap, itemFilesDir)
			if errExpand != nil {
				dlog("ERROR: Recursive: Critical error sub-expanding '%s': %v. Aborting branch.", ingNameNorm, errExpand)
				return nil, fmt.Errorf("failed recursive expansion for ingredient '%s': %w", ingNameNorm, errExpand)
			}

			dlog("      Recursive: Merging results from %s expansion:", ingNameNorm)
			for subName, subAmt := range subIngredients {
				subNameNorm := BAZAAR_ID(subName)
				finalIngredients[subNameNorm] += subAmt
				dlog("        - %s: %.2f", subNameNorm, finalIngredients[subNameNorm])
			}
		}
	}

	dlog("  <- Recursive: Exiting for %s. Returning map: %+v", itemNameNorm, finalIngredients)
	return finalIngredients, nil
}

func ExpandItem(
	itemName string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (map[string]float64, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog("-> ExpandItem called for %.2f x %s", quantity, itemNameNorm)

	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		dlog("<- ExpandItem: No recipe file for '%s'. Cannot expand. Returning item as base.", itemNameNorm)
		return map[string]float64{itemNameNorm: quantity}, nil
	} else if err != nil {
		return nil, fmt.Errorf("ExpandItem checking recipe file '%s': %w", filePath, err)
	}

	dlog("   ExpandItem: Recipe found. Calling recursive helper for %s...", itemNameNorm)
	baseMap, errRec := expandItemRecursive(itemNameNorm, quantity, nil, itemNameNorm, apiResp, metricsMap, itemFilesDir)

	if errRec != nil {
		log.Printf("ERROR (ExpandItem): Recursive helper failed for %s: %v", itemNameNorm, errRec)
		return nil, fmt.Errorf("recursive expansion failed within ExpandItem: %w", errRec)
	}

	dlog("<- ExpandItem: Expansion call for %s complete. Final map: %+v", itemNameNorm, baseMap)
	return baseMap, nil
}
