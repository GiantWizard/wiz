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

// CraftingStepNode struct definition
type CraftingStepNode struct {
	ItemName         string                `json:"item_name"`
	QuantityNeeded   float64               `json:"quantity_needed"`
	QuantityPerCraft float64               `json:"quantity_per_craft"`
	NumCrafts        float64               `json:"num_crafts"`
	IsBaseComponent  bool                  `json:"is_base_component"`
	Acquisition      *BaseIngredientDetail `json:"acquisition,omitempty"` // BaseIngredientDetail is defined in expansion.go
	Ingredients      []*CraftingStepNode   `json:"ingredients,omitempty"`
	ErrorMessage     string                `json:"error_message,omitempty"`
	Depth            int                   `json:"depth"`
	MaxSubTreeDepth  int                   `json:"max_sub_tree_depth"`
}

// calculateC10MForNode is a helper to get C10M details for a node being treated as base.
func calculateC10MForNode(itemID string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) (
	cost float64, method string, assocCost float64, rr float64, ifVal float64, delta float64, err error) {
	cost, method, assocCost, rr, ifVal, err = getBestC10M(itemID, quantity, apiResp, metricsMap)
	delta = math.NaN()
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
	if metricsOk {
		delta = metricsData.SellSize*metricsData.SellFrequency - metricsData.OrderSize*metricsData.OrderFrequency
	}
	return
}

func expandItemRecursiveTree(
	itemName string,
	quantityNeeded float64,
	path []ItemStep,
	originalTopLevelItemID string,
	currentDepth int,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (*CraftingStepNode, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog("  -> RecursiveTree (Depth %d): Expanding %.2f x %s", currentDepth, quantityNeeded, itemNameNorm)

	node := &CraftingStepNode{
		ItemName:        itemNameNorm,
		QuantityNeeded:  quantityNeeded,
		Depth:           currentDepth,
		MaxSubTreeDepth: currentDepth,
	}

	if isInPath(itemNameNorm, path) {
		isTopLevelCycle := itemNameNorm == originalTopLevelItemID
		cycleMsg := ""
		if isTopLevelCycle {
			cycleMsg = fmt.Sprintf("Cycle detected back to TOP LEVEL item '%s'. Treating as base here.", itemNameNorm)
			node.ErrorMessage = "Cycle detected to top-level item"
		} else {
			cycleMsg = fmt.Sprintf("Cycle detected to intermediate item '%s'. Treating as base here.", itemNameNorm)
			node.ErrorMessage = "Cycle detected to intermediate item"
		}
		dlog("  <- RecursiveTree (Depth %d): %s", currentDepth, cycleMsg)
		node.IsBaseComponent = true
		cost, method, assocCost, rr, ifVal, delta, errC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		if errC10M == nil {
			node.Acquisition = &BaseIngredientDetail{
				Quantity: quantityNeeded, BestCost: float64Ptr(cost), AssociatedCost: float64Ptr(assocCost), Method: method,
				RR: float64Ptr(rr), IF: float64Ptr(ifVal), Delta: float64Ptr(delta),
			}
		} else {
			node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: "N/A (Cycle Error)", BestCost: float64Ptr(math.Inf(1))}
			if node.ErrorMessage == "" {
				node.ErrorMessage = errC10M.Error()
			} else {
				node.ErrorMessage += "; " + errC10M.Error()
			}
		}
		return node, nil
	}

	currentPath := append([]ItemStep{}, path...)
	currentPath = append(currentPath, ItemStep{name: itemNameNorm, quantity: quantityNeeded})

	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	recipeFileExists := false
	var itemData Item // Item struct from recipe.go
	if _, statErr := os.Stat(filePath); statErr == nil {
		recipeFileExists = true
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			node.IsBaseComponent = true
			node.ErrorMessage = fmt.Sprintf("Error reading recipe file '%s': %v", filePath, readErr)
			cost, method, assocCost, rr, ifVal, delta, errC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
			node.Acquisition = &BaseIngredientDetail{
				Quantity: quantityNeeded, BestCost: float64Ptr(cost), AssociatedCost: float64Ptr(assocCost), Method: method,
				RR: float64Ptr(rr), IF: float64Ptr(ifVal), Delta: float64Ptr(delta),
			}
			if errC10M != nil && node.Acquisition != nil {
				node.Acquisition.Method += " (RecipeReadError)"
			}
			return node, nil
		}
		if err := json.Unmarshal(data, &itemData); err != nil {
			node.IsBaseComponent = true
			node.ErrorMessage = fmt.Sprintf("Error parsing recipe JSON for '%s': %v", itemNameNorm, err)
			cost, method, assocCost, rr, ifVal, delta, errC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
			node.Acquisition = &BaseIngredientDetail{
				Quantity: quantityNeeded, BestCost: float64Ptr(cost), AssociatedCost: float64Ptr(assocCost), Method: method,
				RR: float64Ptr(rr), IF: float64Ptr(ifVal), Delta: float64Ptr(delta),
			}
			if errC10M != nil && node.Acquisition != nil {
				node.Acquisition.Method += " (RecipeParseError)"
			}
			return node, nil
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("RecursiveTree checking recipe file '%s': %w", filePath, statErr)
	}

	itemCost, itemMethod, itemAssocCost, itemRR, itemIF, itemDelta, itemErrC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)

	shouldExpandThisItem := false
	isApiNotFoundError := false
	if itemErrC10M != nil && strings.Contains(itemErrC10M.Error(), "API data not found") {
		isApiNotFoundError = true
	}

	if isApiNotFoundError {
		if recipeFileExists {
			shouldExpandThisItem = true
			dlog("      RecursiveTree (Depth %d): Decision for %s: FORCE EXPAND (Not on Bazaar, recipe exists)", currentDepth, itemNameNorm)
		} else {
			shouldExpandThisItem = false
			dlog("      RecursiveTree (Depth %d): Decision for %s: TREAT AS BASE (Not on Bazaar, no recipe)", currentDepth, itemNameNorm)
			if node.ErrorMessage == "" {
				node.ErrorMessage = "Not on Bazaar and no recipe file"
			}
		}
	} else if itemErrC10M == nil && (itemMethod == "Primary" || itemMethod == "N/A") {
		if recipeFileExists {
			shouldExpandThisItem = true
			dlog("      RecursiveTree (Depth %d): Decision for %s: EXPAND (Method %s, recipe exists)", currentDepth, itemNameNorm, itemMethod)
		} else {
			shouldExpandThisItem = false
			dlog("      RecursiveTree (Depth %d): Decision for %s: TREAT AS BASE (Method %s, but no recipe)", currentDepth, itemNameNorm, itemMethod)
			if node.ErrorMessage == "" {
				node.ErrorMessage = "No recipe file"
			}
		}
	} else {
		shouldExpandThisItem = false
		dlog("      RecursiveTree (Depth %d): Decision for %s: TREAT AS BASE (Method: %s, Valid Cost: %v, Err: %v)", currentDepth, itemNameNorm, itemMethod, !math.IsInf(itemCost, 0) && !math.IsNaN(itemCost) && itemCost >= 0, itemErrC10M)
		if itemErrC10M != nil && node.ErrorMessage == "" {
			node.ErrorMessage = itemErrC10M.Error()
		}
	}

	if !shouldExpandThisItem || !recipeFileExists {
		node.IsBaseComponent = true
		node.Acquisition = &BaseIngredientDetail{
			Quantity: quantityNeeded, BestCost: float64Ptr(itemCost), AssociatedCost: float64Ptr(itemAssocCost), Method: itemMethod,
			RR: float64Ptr(itemRR), IF: float64Ptr(itemIF), Delta: float64Ptr(itemDelta),
		}
		if itemErrC10M != nil && node.ErrorMessage == "" {
			node.ErrorMessage = itemErrC10M.Error()
		}
		if !recipeFileExists && node.ErrorMessage == "" {
			node.ErrorMessage = "No recipe file"
		}
		dlog("  <- RecursiveTree (Depth %d): Treating %s as base. Node Err: %s", currentDepth, itemNameNorm, node.ErrorMessage)
		return node, nil
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
			}
			recipeContentExists = true
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
		node.ErrorMessage = "No usable recipe content found in file"
		cost, method, assocCost, rr, ifVal, delta, errC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		node.Acquisition = &BaseIngredientDetail{
			Quantity: quantityNeeded, BestCost: float64Ptr(cost), AssociatedCost: float64Ptr(assocCost), Method: method,
			RR: float64Ptr(rr), IF: float64Ptr(ifVal), Delta: float64Ptr(delta),
		}
		if errC10M != nil && node.Acquisition != nil {
			node.Acquisition.Method += " (NoRecipeContent)"
		}
		return node, nil
	}

	node.QuantityPerCraft = craftedAmount
	node.NumCrafts = math.Ceil(quantityNeeded / craftedAmount)
	node.IsBaseComponent = false

	ingredientsInOneCraft, aggErr := aggregateCells(chosenRecipeCells)
	if aggErr != nil {
		node.ErrorMessage = fmt.Sprintf("Error parsing recipe cells: %v", aggErr)
		return node, fmt.Errorf("failed parsing cells for %s: %w", itemNameNorm, aggErr)
	}
	if len(ingredientsInOneCraft) == 0 {
		node.ErrorMessage = "Recipe yields zero ingredients"
		return node, nil
	}

	maxChildDepth := currentDepth
	for ingName, ingAmtPerCraft := range ingredientsInOneCraft {
		totalIngAmtNeededForParent := ingAmtPerCraft * node.NumCrafts
		if totalIngAmtNeededForParent <= 0 {
			continue
		}

		subNode, errExpand := expandItemRecursiveTree(
			ingName,
			totalIngAmtNeededForParent,
			currentPath,
			originalTopLevelItemID,
			currentDepth+1,
			apiResp, metricsMap, itemFilesDir,
		)

		if errExpand != nil {
			log.Printf("ERROR: RecursiveTree: Critical sub-expansion error for '%s' (ingredient of '%s'): %v", ingName, itemNameNorm, errExpand)
			errorSubNode := &CraftingStepNode{
				ItemName:        BAZAAR_ID(ingName),
				QuantityNeeded:  totalIngAmtNeededForParent,
				ErrorMessage:    fmt.Sprintf("Sub-expansion failed: %v", errExpand),
				IsBaseComponent: true,
				Depth:           currentDepth + 1,
				MaxSubTreeDepth: currentDepth + 1,
				Acquisition:     &BaseIngredientDetail{Quantity: totalIngAmtNeededForParent, Method: "N/A (Sub-Expansion Error)", BestCost: float64Ptr(math.Inf(1))},
			}
			node.Ingredients = append(node.Ingredients, errorSubNode)
			if errorSubNode.MaxSubTreeDepth > maxChildDepth {
				maxChildDepth = errorSubNode.MaxSubTreeDepth
			}
		} else if subNode != nil {
			node.Ingredients = append(node.Ingredients, subNode)
			if subNode.MaxSubTreeDepth > maxChildDepth {
				maxChildDepth = subNode.MaxSubTreeDepth
			}
		}
	}
	node.MaxSubTreeDepth = maxChildDepth

	dlog("  <- RecursiveTree (Depth %d, MaxSub %d): Exiting for %s.", currentDepth, node.MaxSubTreeDepth, itemNameNorm)
	return node, nil
}

func ExpandItemToTree(
	itemName string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
) (*CraftingStepNode, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog("-> ExpandItemToTree called for %.2f x %s", quantity, itemNameNorm)

	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		dlog("<- ExpandItemToTree: No recipe file for top-level '%s'. Returning as base node.", itemNameNorm)
		rootNode := &CraftingStepNode{
			ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0,
		}
		cost, method, assocCost, rr, ifVal, delta, errC10M := calculateC10MForNode(itemNameNorm, quantity, apiResp, metricsMap)
		if errC10M == nil {
			rootNode.Acquisition = &BaseIngredientDetail{
				Quantity: quantity, BestCost: float64Ptr(cost), AssociatedCost: float64Ptr(assocCost), Method: method,
				RR: float64Ptr(rr), IF: float64Ptr(ifVal), Delta: float64Ptr(delta),
			}
		} else {
			rootNode.Acquisition = &BaseIngredientDetail{Quantity: quantity, Method: "N/A (Error)", BestCost: float64Ptr(math.Inf(1))}
			rootNode.ErrorMessage = fmt.Sprintf("No recipe file and C10M error: %v", errC10M)
		}
		return rootNode, nil
	} else if statErr != nil {
		return nil, fmt.Errorf("ExpandItemToTree checking recipe file '%s': %w", filePath, statErr)
	}

	rootNode, errRec := expandItemRecursiveTree(itemNameNorm, quantity, nil, itemNameNorm, 0, apiResp, metricsMap, itemFilesDir)
	if errRec != nil {
		log.Printf("ERROR (ExpandItemToTree): Recursive helper failed for %s: %v", itemNameNorm, errRec)
		if rootNode == nil {
			rootNode = &CraftingStepNode{
				ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0, ErrorMessage: fmt.Sprintf("Recursive expansion failed critically: %v", errRec),
				Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A (Critical Error)", BestCost: float64Ptr(math.Inf(1))},
			}
		} else if rootNode.ErrorMessage == "" {
			rootNode.ErrorMessage = fmt.Sprintf("Recursive expansion encountered an error: %v", errRec)
		}
		return rootNode, errRec
	}

	dlog("<- ExpandItemToTree: Expansion call for %s complete. Max Depth: %d", itemNameNorm, rootNode.MaxSubTreeDepth)
	return rootNode, nil
}

func extractBaseIngredientsFromTree(rootNode *CraftingStepNode) map[string]BaseIngredientDetail {
	baseMapDetails := make(map[string]BaseIngredientDetail)
	if rootNode == nil {
		return baseMapDetails
	}

	var q []*CraftingStepNode
	q = append(q, rootNode)

	for len(q) > 0 {
		curr := q[0]
		q = q[1:]

		if curr.IsBaseComponent {
			if curr.Acquisition != nil {
				existing, found := baseMapDetails[curr.ItemName]
				if found {
					newQty := existing.Quantity + curr.Acquisition.Quantity
					existing.Quantity = newQty
					baseMapDetails[curr.ItemName] = existing
				} else {
					copiedAcquisition := *curr.Acquisition
					baseMapDetails[curr.ItemName] = copiedAcquisition
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
	rootNode *CraftingStepNode,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (float64, *float64, string, float64, bool, string) {
	if rootNode == nil {
		return math.Inf(1), float64Ptr(math.Inf(1)), "", 0.0, false, "Root node is nil"
	}

	if rootNode.IsBaseComponent {
		if rootNode.Acquisition != nil && rootNode.Acquisition.Method != "N/A" && rootNode.Acquisition.BestCost != nil && !math.IsInf(*rootNode.Acquisition.BestCost, 0) && *rootNode.Acquisition.BestCost >= 0 {
			metricsData, metricsOk := safeGetMetricsData(metricsMap, rootNode.ItemName)
			var fillTime float64 = 0.0
			if rootNode.Acquisition.Method == "Primary" && metricsOk {
				calculatedTime, _, _ := calculateBuyOrderFillTime(rootNode.ItemName, rootNode.Acquisition.Quantity, metricsData)
				if !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, 0) && calculatedTime >= 0 {
					fillTime = calculatedTime
				} else {
					fillTime = math.Inf(1)
				}
			} else if rootNode.Acquisition.Method == "Primary" && !metricsOk {
				fillTime = math.Inf(1)
			}
			return *rootNode.Acquisition.BestCost, float64Ptr(fillTime), rootNode.ItemName, rootNode.Acquisition.Quantity, true, rootNode.ErrorMessage
		} else {
			errMsg := fmt.Sprintf("Top-level item '%s' is base but acquisition failed or is N/A", rootNode.ItemName)
			if rootNode.ErrorMessage != "" {
				errMsg += "; " + rootNode.ErrorMessage
			} else if rootNode.Acquisition != nil {
				costStr := "nil"
				if rootNode.Acquisition.BestCost != nil {
					costStr = fmt.Sprintf("%.2f", *rootNode.Acquisition.BestCost)
				}
				errMsg += fmt.Sprintf(" (Method: %s, Cost: %s)", rootNode.Acquisition.Method, costStr)
			}
			return math.Inf(1), float64Ptr(math.Inf(1)), rootNode.ItemName, rootNode.QuantityNeeded, false, errMsg
		}
	}

	baseIngredientDetailsMap := extractBaseIngredientsFromTree(rootNode)
	if len(baseIngredientDetailsMap) == 0 {
		errMsg := "Expansion resulted in no valid base ingredients"
		if rootNode.ErrorMessage != "" {
			errMsg = fmt.Sprintf("%s (Root Node Error: %s)", errMsg, rootNode.ErrorMessage)
		}
		return math.Inf(1), float64Ptr(math.Inf(1)), "", 0.0, false, errMsg
	}

	totalSumOfBestCosts := 0.0
	var currentSlowestTime float64 = 0.0
	slowestIngName := ""
	slowestIngQty := 0.0
	isPossible := true
	var errorMessages []string

	for itemID, detail := range baseIngredientDetailsMap {
		if detail.Method == "N/A" || detail.BestCost == nil || math.IsInf(*detail.BestCost, 0) || math.IsNaN(*detail.BestCost) || *detail.BestCost < 0 {
			costValStr := "nil"
			if detail.BestCost != nil {
				costValStr = fmt.Sprintf("%.2f", *detail.BestCost)
			}
			errorMessages = append(errorMessages, fmt.Sprintf("Invalid cost for base ingredient '%s': Cost=%s, Method=%s", itemID, costValStr, detail.Method))
			isPossible = false
			totalSumOfBestCosts = math.Inf(1)
		}
		if !math.IsInf(totalSumOfBestCosts, 1) && detail.BestCost != nil {
			totalSumOfBestCosts += *detail.BestCost
		}

		metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
		var fillTimeForIngredient float64 = 0.0
		if detail.Method == "Primary" {
			if metricsOk {
				buyTime, _, buyErr := calculateBuyOrderFillTime(itemID, detail.Quantity, metricsData)
				if buyErr == nil && !math.IsNaN(buyTime) && !math.IsInf(buyTime, -1) && buyTime >= 0 {
					fillTimeForIngredient = buyTime
				} else {
					fillTimeForIngredient = math.Inf(1)
					errorMessages = append(errorMessages, fmt.Sprintf("Fill time calculation error for '%s'", itemID))
					if isPossible {
						isPossible = false
					}
				}
			} else {
				fillTimeForIngredient = math.Inf(1)
				errorMessages = append(errorMessages, fmt.Sprintf("Metrics not found for primary buy fill time of '%s'", itemID))
				if isPossible {
					isPossible = false
				}
			}
		}

		if math.IsInf(fillTimeForIngredient, 1) {
			if !math.IsInf(currentSlowestTime, 1) {
				currentSlowestTime = fillTimeForIngredient
				slowestIngName = itemID
				slowestIngQty = detail.Quantity
			}
		} else if !math.IsInf(currentSlowestTime, 1) && fillTimeForIngredient > currentSlowestTime {
			currentSlowestTime = fillTimeForIngredient
			slowestIngName = itemID
			slowestIngQty = detail.Quantity
		}
	}

	finalErrorMsg := strings.Join(errorMessages, "; ")
	if !isPossible && finalErrorMsg == "" {
		finalErrorMsg = "Calculation of summed costs/times failed due to problematic base ingredients"
	}
	if rootNode.ErrorMessage != "" {
		if finalErrorMsg == "" {
			finalErrorMsg = rootNode.ErrorMessage
		} else {
			finalErrorMsg += "; RootError: " + rootNode.ErrorMessage
		}
	}
	return totalSumOfBestCosts, float64Ptr(currentSlowestTime), slowestIngName, sanitizeFloat(slowestIngQty), isPossible, finalErrorMsg
}
