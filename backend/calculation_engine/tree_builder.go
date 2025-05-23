// tree_builder.go
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// CraftingStepNode struct definition
type CraftingStepNode struct {
	ItemName         string                `json:"item_name"`
	QuantityNeeded   float64               `json:"quantity_needed"`
	QuantityPerCraft float64               `json:"quantity_per_craft"`
	NumCrafts        float64               `json:"num_crafts"`
	IsBaseComponent  bool                  `json:"is_base_component"`
	Acquisition      *BaseIngredientDetail `json:"acquisition,omitempty"` // BaseIngredientDetail uses JSONFloat64
	Ingredients      []*CraftingStepNode   `json:"ingredients,omitempty"`
	ErrorMessage     string                `json:"error_message,omitempty"`
	Depth            int                   `json:"depth"`
	MaxSubTreeDepth  int                   `json:"max_sub_tree_depth"`
}

func calculateC10MForNode(itemID string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) (
	cost float64, method string, assocCost float64, rr float64, ifVal float64, delta float64, err error) {
	cost, method, assocCost, rr, ifVal, err = getBestC10M(itemID, quantity, apiResp, metricsMap)
	calculatedDelta := math.NaN()
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
	if metricsOk {
		calculatedDelta = metricsData.SellSize*metricsData.SellFrequency - metricsData.OrderSize*metricsData.OrderFrequency
	}
	delta = calculatedDelta
	return
}

func expandItemRecursiveTree(
	itemName string, quantityNeeded float64, path []ItemStep, originalTopLevelItemID string, currentDepth int,
	apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string,
) (*CraftingStepNode, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	node := &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantityNeeded, Depth: currentDepth, MaxSubTreeDepth: currentDepth}

	if isInPath(itemNameNorm, path) {
		node.IsBaseComponent = true
		isTopLevelCycle := itemNameNorm == originalTopLevelItemID
		if isTopLevelCycle {
			node.ErrorMessage = "Cycle detected to top-level item"
		} else {
			node.ErrorMessage = "Cycle detected to intermediate item"
		}

		costRaw, method, assocCostRaw, rrRaw, ifValRaw, deltaRaw, errC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		node.Acquisition = &BaseIngredientDetail{
			Quantity: quantityNeeded, Method: method,
			BestCost:       toJSONFloat64(valueOrNaN(costRaw)),
			AssociatedCost: toJSONFloat64(valueOrNaN(assocCostRaw)),
			RR:             toJSONFloat64(valueOrNaN(rrRaw)),
			IF:             toJSONFloat64(valueOrNaN(ifValRaw)),
			Delta:          toJSONFloat64(valueOrNaN(deltaRaw)),
		}
		if errC10M != nil {
			if node.Acquisition.Method == "N/A" || node.Acquisition.Method == "" {
				node.Acquisition.Method = "ERROR (Cycle)"
			}
			if node.ErrorMessage == "" {
				node.ErrorMessage = errC10M.Error()
			} else if !strings.Contains(node.ErrorMessage, errC10M.Error()) {
				node.ErrorMessage += "; " + errC10M.Error()
			}
		}
		return node, nil
	}

	currentPath := append([]ItemStep{}, path...)
	currentPath = append(currentPath, ItemStep{name: itemNameNorm, quantity: quantityNeeded})
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	recipeFileExists := false
	var itemData Item

	if _, statErr := os.Stat(filePath); statErr == nil {
		recipeFileExists = true
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			node.IsBaseComponent = true
			node.ErrorMessage = fmt.Sprintf("Error reading recipe file '%s': %v", filePath, readErr)
			costR, mR, acR, rrR, ifR, dR, errR := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap) // Capture errR
			node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mR, BestCost: toJSONFloat64(valueOrNaN(costR)), AssociatedCost: toJSONFloat64(valueOrNaN(acR)), RR: toJSONFloat64(valueOrNaN(rrR)), IF: toJSONFloat64(valueOrNaN(ifR)), Delta: toJSONFloat64(valueOrNaN(dR))}
			if errR != nil && node.Acquisition != nil {
				node.Acquisition.Method = "ERROR (RecipeRead)"
			} // Use errR
			return node, nil
		}
		if err := json.Unmarshal(data, &itemData); err != nil {
			node.IsBaseComponent = true
			node.ErrorMessage = fmt.Sprintf("Error parsing recipe JSON for '%s': %v", itemNameNorm, err)
			costP, mP, acP, rrP, ifP, dP, errP := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap) // Capture errP
			node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mP, BestCost: toJSONFloat64(valueOrNaN(costP)), AssociatedCost: toJSONFloat64(valueOrNaN(acP)), RR: toJSONFloat64(valueOrNaN(rrP)), IF: toJSONFloat64(valueOrNaN(ifP)), Delta: toJSONFloat64(valueOrNaN(dP))}
			if errP != nil && node.Acquisition != nil {
				node.Acquisition.Method = "ERROR (RecipeParse)"
			} // Use errP
			return node, nil
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("RecursiveTree: error checking recipe file '%s': %w", filePath, statErr)
	}

	itemCostRaw, itemMethod, itemAssocCostRaw, itemRRRaw, itemIFRaw, itemDeltaRaw, itemErrC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
	shouldExpandThisItem := false
	isApiNotFoundError := false
	if itemErrC10M != nil && strings.Contains(itemErrC10M.Error(), "API data not found") {
		isApiNotFoundError = true
	}

	if isApiNotFoundError {
		if recipeFileExists {
			shouldExpandThisItem = true
		} else {
			if node.ErrorMessage == "" {
				node.ErrorMessage = "Not on Bazaar and no recipe file"
			}
		}
	} else if itemErrC10M == nil && (itemMethod == "Primary" || itemMethod == "N/A") {
		if recipeFileExists {
			shouldExpandThisItem = true
		} else {
			if node.ErrorMessage == "" {
				node.ErrorMessage = "No recipe file to expand further"
			}
		}
	} else {
		if itemErrC10M != nil && node.ErrorMessage == "" {
			node.ErrorMessage = itemErrC10M.Error()
		}
	}

	if !shouldExpandThisItem || !recipeFileExists {
		node.IsBaseComponent = true
		node.Acquisition = &BaseIngredientDetail{
			Quantity: quantityNeeded, Method: itemMethod, BestCost: toJSONFloat64(valueOrNaN(itemCostRaw)), AssociatedCost: toJSONFloat64(valueOrNaN(itemAssocCostRaw)),
			RR: toJSONFloat64(valueOrNaN(itemRRRaw)), IF: toJSONFloat64(valueOrNaN(itemIFRaw)), Delta: toJSONFloat64(valueOrNaN(itemDeltaRaw)),
		}
		if itemErrC10M != nil && node.ErrorMessage == "" {
			node.ErrorMessage = itemErrC10M.Error()
		}
		if !recipeFileExists && node.ErrorMessage == "" {
			node.ErrorMessage = "No recipe file"
		} else if !recipeFileExists && !strings.Contains(node.ErrorMessage, "No recipe file") {
			node.ErrorMessage += "; No recipe file"
		}
		return node, nil
	}

	var chosenRecipeCells map[string]string
	var craftedAmount float64 = 1.0
	recipeContentExists := false
	if len(itemData.Recipes) > 0 {
		firstRecipe := itemData.Recipes[0]
		tempCells := map[string]string{"A1": firstRecipe.A1, "A2": firstRecipe.A2, "A3": firstRecipe.A3, "B1": firstRecipe.B1, "B2": firstRecipe.B2, "B3": firstRecipe.B3, "C1": firstRecipe.C1, "C2": firstRecipe.C2, "C3": firstRecipe.C3}
		for _, v := range tempCells {
			if v != "" {
				recipeContentExists = true
				break
			}
		}
		if recipeContentExists {
			chosenRecipeCells = tempCells
			if firstRecipe.Count > 0 {
				craftedAmount = float64(firstRecipe.Count)
			}
		}
	}
	if !recipeContentExists && (itemData.Recipe.A1 != "" || itemData.Recipe.A2 != "" || itemData.Recipe.A3 != "" || itemData.Recipe.B1 != "" || itemData.Recipe.B2 != "" || itemData.Recipe.B3 != "" || itemData.Recipe.C1 != "" || itemData.Recipe.C2 != "" || itemData.Recipe.C3 != "") {
		chosenRecipeCells = map[string]string{"A1": itemData.Recipe.A1, "A2": itemData.Recipe.A2, "A3": itemData.Recipe.A3, "B1": itemData.Recipe.B1, "B2": itemData.Recipe.B2, "B3": itemData.Recipe.B3, "C1": itemData.Recipe.C1, "C2": itemData.Recipe.C2, "C3": itemData.Recipe.C3}
		if itemData.Recipe.Count > 0 {
			craftedAmount = float64(itemData.Recipe.Count)
		}
		recipeContentExists = true
	}

	if !recipeContentExists {
		node.IsBaseComponent = true
		node.ErrorMessage = "No usable recipe content in file."
		costN, mN, acN, rrN, ifN, dN, errN := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap) // Capture errN
		node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mN, BestCost: toJSONFloat64(valueOrNaN(costN)), AssociatedCost: toJSONFloat64(valueOrNaN(acN)), RR: toJSONFloat64(valueOrNaN(rrN)), IF: toJSONFloat64(valueOrNaN(ifN)), Delta: toJSONFloat64(valueOrNaN(dN))}
		if errN != nil && node.Acquisition != nil {
			node.Acquisition.Method = "ERROR (NoRecipeContent)"
		} // Use errN
		return node, nil
	}
	node.QuantityPerCraft = craftedAmount
	node.NumCrafts = math.Ceil(quantityNeeded / craftedAmount)
	node.IsBaseComponent = false
	ingredientsInOneCraft, aggErr := aggregateCells(chosenRecipeCells)
	if aggErr != nil { // CORRECTED: Check aggErr
		node.ErrorMessage = fmt.Sprintf("Error parsing recipe cells for expansion: %v", aggErr)
		// Return node with error message, but also propagate error if it's critical for caller
		return node, fmt.Errorf("failed parsing cells for %s: %w", itemNameNorm, aggErr)
	}
	if len(ingredientsInOneCraft) == 0 {
		node.ErrorMessage = "Recipe definition yields zero ingredients."
		return node, nil
	}

	maxChildSubTreeDepth := currentDepth
	for ingName, ingAmtPerCraft := range ingredientsInOneCraft {
		totalIngAmtNeededForParent := ingAmtPerCraft * node.NumCrafts
		if totalIngAmtNeededForParent <= 0 {
			continue
		}
		subNode, errExpandSub := expandItemRecursiveTree(ingName, totalIngAmtNeededForParent, currentPath, originalTopLevelItemID, currentDepth+1, apiResp, metricsMap, itemFilesDir)
		if errExpandSub != nil {
			errorSubNode := &CraftingStepNode{
				ItemName: BAZAAR_ID(ingName), QuantityNeeded: totalIngAmtNeededForParent, ErrorMessage: fmt.Sprintf("Sub-expansion failed critically: %v", errExpandSub),
				IsBaseComponent: true, Depth: currentDepth + 1, MaxSubTreeDepth: currentDepth + 1,
				Acquisition: &BaseIngredientDetail{Quantity: totalIngAmtNeededForParent, Method: "N/A (Sub-Expansion Critical Error)", BestCost: toJSONFloat64(math.NaN())},
			}
			node.Ingredients = append(node.Ingredients, errorSubNode)
			if errorSubNode.MaxSubTreeDepth > maxChildSubTreeDepth {
				maxChildSubTreeDepth = errorSubNode.MaxSubTreeDepth
			}
		} else if subNode != nil {
			node.Ingredients = append(node.Ingredients, subNode)
			if subNode.MaxSubTreeDepth > maxChildSubTreeDepth {
				maxChildSubTreeDepth = subNode.MaxSubTreeDepth
			}
		}
	}
	node.MaxSubTreeDepth = maxChildSubTreeDepth
	return node, nil
}

func ExpandItemToTree(
	itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string,
) (*CraftingStepNode, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		rootNode := &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0}
		costR, mR, acR, rrR, ifR, dR, errC10M := calculateC10MForNode(itemNameNorm, quantity, apiResp, metricsMap) // Capture errC10M
		rootNode.Acquisition = &BaseIngredientDetail{
			Quantity: quantity, Method: mR, BestCost: toJSONFloat64(valueOrNaN(costR)), AssociatedCost: toJSONFloat64(valueOrNaN(acR)),
			RR: toJSONFloat64(valueOrNaN(rrR)), IF: toJSONFloat64(valueOrNaN(ifR)), Delta: toJSONFloat64(valueOrNaN(dR)),
		}
		if errC10M != nil { // Use errC10M
			rootNode.Acquisition.Method = "ERROR (NoRecipe)"
			rootNode.ErrorMessage = fmt.Sprintf("No recipe file and C10M error: %v", errC10M)
		} else if mR == "N/A" {
			rootNode.ErrorMessage = "No recipe file and item acquisition is N/A via C10M."
		}
		return rootNode, nil
	} else if statErr != nil {
		return nil, fmt.Errorf("ExpandItemToTree: %w", statErr)
	}

	rootNode, errRec := expandItemRecursiveTree(itemNameNorm, quantity, nil, itemNameNorm, 0, apiResp, metricsMap, itemFilesDir)
	if errRec != nil {
		if rootNode == nil {
			rootNode = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0, ErrorMessage: fmt.Sprintf("Recursive expansion failed critically: %v", errRec),
				Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A (Critical Expansion Error)", BestCost: toJSONFloat64(math.NaN())},
			}
		} // ...
		return rootNode, errRec
	}
	if rootNode == nil {
		rootNode = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0, ErrorMessage: "Expansion resulted in nil node.",
			Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A (Nil Node Error)", BestCost: toJSONFloat64(math.NaN())},
		}
		return rootNode, fmt.Errorf("nil node from expansion")
	}
	return rootNode, nil
}

func extractBaseIngredientsFromTree(rootNode *CraftingStepNode) map[string]BaseIngredientDetail {
	baseMapDetails := make(map[string]BaseIngredientDetail)
	if rootNode == nil {
		return baseMapDetails
	}
	var q []*CraftingStepNode
	q = append(q, rootNode)
	visited := make(map[*CraftingStepNode]bool)

	for len(q) > 0 {
		curr := q[0]
		q = q[1:]
		if curr == nil || visited[curr] {
			continue
		}
		visited[curr] = true
		if curr.IsBaseComponent {
			if curr.Acquisition != nil {
				existing, found := baseMapDetails[curr.ItemName]
				if found {
					updatedDetail := existing
					updatedDetail.Quantity += curr.Acquisition.Quantity
					baseMapDetails[curr.ItemName] = updatedDetail
				} else {
					baseMapDetails[curr.ItemName] = *curr.Acquisition
				}
			} else {
				if _, exists := baseMapDetails[curr.ItemName]; !exists {
					baseMapDetails[curr.ItemName] = BaseIngredientDetail{
						Quantity: curr.QuantityNeeded, Method: "ERROR (MissingAcquisition)", BestCost: toJSONFloat64(math.NaN()),
						AssociatedCost: toJSONFloat64(math.NaN()), RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(math.NaN()),
					}
				} else {
					detail := baseMapDetails[curr.ItemName]
					detail.Quantity += curr.QuantityNeeded
					baseMapDetails[curr.ItemName] = detail
				}
			}
		} else {
			for _, child := range curr.Ingredients {
				if child != nil {
					q = append(q, child)
				}
			}
		}
	}
	return baseMapDetails
}

func analyzeTreeForCostsAndTimes(
	rootNode *CraftingStepNode, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics,
) (totalCost float64, slowestFillTimeSecs float64, slowestIngName string, slowestIngQty float64, isPossible bool, errorMsg string) {
	if rootNode == nil {
		return math.Inf(1), math.NaN(), "", 0.0, false, "Root node is nil"
	}

	if rootNode.IsBaseComponent {
		if rootNode.Acquisition != nil {
			cost := float64(rootNode.Acquisition.BestCost)
			if !math.IsNaN(cost) && cost >= 0 {
				fillTimeRaw := 0.0
				if rootNode.Acquisition.Method == "Primary" {
					metricsData, metricsOk := safeGetMetricsData(metricsMap, rootNode.ItemName)
					if metricsOk {
						calculatedTime, _, _ := calculateBuyOrderFillTime(rootNode.ItemName, rootNode.Acquisition.Quantity, metricsData)
						if !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, 0) && calculatedTime >= 0 {
							fillTimeRaw = calculatedTime
						} else {
							fillTimeRaw = math.Inf(1)
						}
					} else {
						fillTimeRaw = math.Inf(1)
					}
				}
				return cost, valueOrNaN(fillTimeRaw), rootNode.ItemName, rootNode.Acquisition.Quantity, true, rootNode.ErrorMessage
			} else {
				return math.Inf(1), math.NaN(), rootNode.ItemName, rootNode.QuantityNeeded, false, "Base item acquisition cost invalid/NaN"
			}
		} else {
			return math.Inf(1), math.NaN(), rootNode.ItemName, rootNode.QuantityNeeded, false, "Base item no acquisition details"
		}
	}

	baseIngredientsMap := extractBaseIngredientsFromTree(rootNode)
	if len(baseIngredientsMap) == 0 {
		return math.Inf(1), math.NaN(), "", 0.0, false, "No base ingredients found"
	}

	currentTotalSumOfBestCosts := 0.0
	currentSlowestTimeRaw := 0.0
	currentIsPossible := true
	// CORRECTED: Declare these variables outside the loop
	var currentSlowestIngName string = ""
	var currentSlowestIngQty float64 = 0.0
	var errorMessages []string

	for itemID, detail := range baseIngredientsMap {
		costVal := float64(detail.BestCost)
		if math.IsNaN(costVal) || costVal < 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("Invalid cost for base '%s'", itemID))
			currentIsPossible = false
			currentTotalSumOfBestCosts = math.Inf(1)
		}
		// This check should be outside the above if, to sum valid costs even if another ing is impossible
		if !math.IsInf(currentTotalSumOfBestCosts, 1) && !math.IsNaN(costVal) && costVal >= 0 {
			currentTotalSumOfBestCosts += costVal
		}

		fillTimeForIngredientRaw := 0.0
		if detail.Method == "Primary" {
			metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
			if metricsOk {
				buyTime, _, buyErr := calculateBuyOrderFillTime(itemID, detail.Quantity, metricsData)
				if buyErr == nil && !math.IsNaN(buyTime) && !math.IsInf(buyTime, 0) && buyTime >= 0 {
					fillTimeForIngredientRaw = buyTime
				} else {
					fillTimeForIngredientRaw = math.Inf(1)
					errorMessages = append(errorMessages, "fill time err for "+itemID)
					currentIsPossible = false
				}
			} else {
				fillTimeForIngredientRaw = math.Inf(1)
				errorMessages = append(errorMessages, "metrics missing for "+itemID)
				currentIsPossible = false
			}
		}
		// Update slowest time logic
		if math.IsInf(fillTimeForIngredientRaw, 1) { // If current ingredient's fill time is Inf
			if !math.IsInf(currentSlowestTimeRaw, 1) { // And overall slowest wasn't Inf yet
				currentSlowestTimeRaw = fillTimeForIngredientRaw // Then overall becomes Inf
				currentSlowestIngName = itemID                   // CORRECTED
				currentSlowestIngQty = detail.Quantity           // CORRECTED
			}
		} else if !math.IsInf(currentSlowestTimeRaw, 1) && fillTimeForIngredientRaw > currentSlowestTimeRaw { // If neither is Inf and current is slower
			currentSlowestTimeRaw = fillTimeForIngredientRaw
			currentSlowestIngName = itemID         // CORRECTED
			currentSlowestIngQty = detail.Quantity // CORRECTED
		}
	}
	finalErrorMsg := strings.Join(errorMessages, "; ")
	if rootNode.ErrorMessage != "" {
		if finalErrorMsg == "" {
			finalErrorMsg = "TreeRoot: " + rootNode.ErrorMessage
		} else {
			finalErrorMsg += "; TreeRoot: " + rootNode.ErrorMessage
		}
	}

	// Return the values that were updated throughout the loop
	return currentTotalSumOfBestCosts, valueOrNaN(currentSlowestTimeRaw), currentSlowestIngName, sanitizeFloat(currentSlowestIngQty), currentIsPossible, finalErrorMsg
}
