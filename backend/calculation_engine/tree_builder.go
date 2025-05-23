// tree_builder.go
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
	QuantityPerCraft float64               `json:"quantity_per_craft,omitempty"` // omitempty if not crafting
	NumCrafts        float64               `json:"num_crafts,omitempty"`         // omitempty if not crafting
	IsBaseComponent  bool                  `json:"is_base_component"`
	Acquisition      *BaseIngredientDetail `json:"acquisition,omitempty"` // Populated for base components or if treated as base
	Ingredients      []*CraftingStepNode   `json:"ingredients,omitempty"` // Children if expanded
	ErrorMessage     string                `json:"error_message,omitempty"`
	Depth            int                   `json:"depth"`
	MaxSubTreeDepth  int                   `json:"max_sub_tree_depth"` // Max depth of this node or any of its children
}

func calculateC10MForNode(itemID string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) (
	cost float64, method string, assocCost float64, rr float64, ifVal float64, delta float64, err error) {
	// This function directly calls getBestC10M and then adds Delta calculation.
	// getBestC10M returns: bestCost, bestMethod, associatedCost, rrValue, ifValue, err
	cost, method, assocCost, rr, ifVal, err = getBestC10M(itemID, quantity, apiResp, metricsMap)

	// Calculate Delta separately as it's not part of getBestC10M's direct return for this purpose.
	// Delta is more of a property of the item's market dynamics, not strictly part of its acquisition cost.
	calculatedDelta := math.NaN() // Default to NaN if metrics not found
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
	if metricsOk {
		calculatedDelta = (metricsData.SellSize * metricsData.SellFrequency) - (metricsData.OrderSize * metricsData.OrderFrequency)
	}
	delta = calculatedDelta // Assign to the return variable

	return // Return all captured values
}

func expandItemRecursiveTree(
	itemName string, quantityNeeded float64, path []ItemStep, originalTopLevelItemID string, currentDepth int,
	apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string,
) (*CraftingStepNode, error) { // Returns node and potentially a critical error for the caller
	itemNameNorm := BAZAAR_ID(itemName)
	node := &CraftingStepNode{
		ItemName:        itemNameNorm,
		QuantityNeeded:  quantityNeeded,
		Depth:           currentDepth,
		MaxSubTreeDepth: currentDepth, // Initial assumption, will be updated by children
	}

	// Cycle Detection
	if isInPath(itemNameNorm, path) {
		node.IsBaseComponent = true // Treat as base due to cycle
		isTopLevelCycle := itemNameNorm == originalTopLevelItemID
		if isTopLevelCycle {
			node.ErrorMessage = "Cycle detected to top-level item '" + originalTopLevelItemID + "'"
		} else {
			node.ErrorMessage = "Cycle detected to intermediate item '" + itemNameNorm + "'"
		}
		// Even in a cycle, try to get its acquisition cost as if it were a base component
		costRaw, method, assocCostRaw, rrRaw, ifValRaw, deltaRaw, errC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		node.Acquisition = &BaseIngredientDetail{
			Quantity:       quantityNeeded,
			Method:         method,
			BestCost:       toJSONFloat64(valueOrNaN(costRaw)),
			AssociatedCost: toJSONFloat64(valueOrNaN(assocCostRaw)),
			RR:             toJSONFloat64(valueOrNaN(rrRaw)),
			IF:             toJSONFloat64(valueOrNaN(ifValRaw)),
			Delta:          toJSONFloat64(valueOrNaN(deltaRaw)),
		}
		if errC10M != nil {
			if node.Acquisition.Method == "N/A" || node.Acquisition.Method == "" { // If method wasn't set due to error
				node.Acquisition.Method = "ERROR (Cycle/C10M)"
			}
			if node.ErrorMessage == "" { // If no cycle message was set (shouldn't happen here)
				node.ErrorMessage = errC10M.Error()
			} else if !strings.Contains(node.ErrorMessage, errC10M.Error()) { // Append C10M error if distinct
				node.ErrorMessage += "; C10M Error: " + errC10M.Error()
			}
		}
		return node, nil // Return the node, cycle is not a critical error for recursion itself
	}

	// Add current item to path for sub-expansions
	currentPath := append([]ItemStep{}, path...) // Create a copy of the path
	currentPath = append(currentPath, ItemStep{name: itemNameNorm, quantity: quantityNeeded})

	// Recipe File Handling
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	recipeFileExists := false
	var itemData Item // To store parsed recipe data

	if _, statErr := os.Stat(filePath); statErr == nil {
		recipeFileExists = true
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			// Error reading the file, treat as base and record error
			node.IsBaseComponent = true
			node.ErrorMessage = fmt.Sprintf("Error reading recipe file '%s': %v", filePath, readErr)
			// Attempt to get C10M cost even if recipe read fails
			costR, mR, acR, rrR, ifR, dR, errR := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
			node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mR, BestCost: toJSONFloat64(valueOrNaN(costR)), AssociatedCost: toJSONFloat64(valueOrNaN(acR)), RR: toJSONFloat64(valueOrNaN(rrR)), IF: toJSONFloat64(valueOrNaN(ifR)), Delta: toJSONFloat64(valueOrNaN(dR))}
			if errR != nil && node.Acquisition != nil {
				node.Acquisition.Method = "ERROR (RecipeReadFail/C10M)"
			}
			return node, nil // Not a critical error for recursion, just this node can't expand
		}
		if err := json.Unmarshal(data, &itemData); err != nil {
			// Error parsing JSON, treat as base and record error
			node.IsBaseComponent = true
			node.ErrorMessage = fmt.Sprintf("Error parsing recipe JSON for '%s': %v", itemNameNorm, err)
			costP, mP, acP, rrP, ifP, dP, errP := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
			node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mP, BestCost: toJSONFloat64(valueOrNaN(costP)), AssociatedCost: toJSONFloat64(valueOrNaN(acP)), RR: toJSONFloat64(valueOrNaN(rrP)), IF: toJSONFloat64(valueOrNaN(ifP)), Delta: toJSONFloat64(valueOrNaN(dP))}
			if errP != nil && node.Acquisition != nil {
				node.Acquisition.Method = "ERROR (RecipeParseFail/C10M)"
			}
			return node, nil
		}
	} else if !os.IsNotExist(statErr) { // If os.Stat returned an error other than "not exist"
		// This is a more critical file system error
		return nil, fmt.Errorf("expandItemRecursiveTree: error checking recipe file '%s': %w", filePath, statErr)
	}

	// Decision to Expand vs. Treat as Base (based on C10M of current item)
	itemCostRaw, itemMethod, itemAssocCostRaw, itemRRRaw, itemIFRaw, itemDeltaRaw, itemErrC10M := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
	shouldExpandThisItem := false // Default to not expanding

	isApiNotFoundError := false
	if itemErrC10M != nil && strings.Contains(itemErrC10M.Error(), "API data not found") {
		isApiNotFoundError = true
	}

	if isApiNotFoundError { // If item is not on Bazaar
		if recipeFileExists {
			shouldExpandThisItem = true // Must expand if possible, as it can't be bought
			dlog("  Item %s not on Bazaar, but recipe exists. Will expand.", itemNameNorm)
		} else {
			if node.ErrorMessage == "" {
				node.ErrorMessage = "Not on Bazaar and no recipe file."
			}
		}
	} else if itemErrC10M == nil && (itemMethod == "Primary" || itemMethod == "N/A" || itemMethod == "ERROR") {
		// If C10M results in Primary (meaning it's generally cheaper to buy order),
		// or N/A (cannot determine a good buy price), or ERROR, then we *should* try to expand if a recipe exists.
		// If itemMethod was "Secondary", it implies instabuy is better than buy order, making it a candidate for base component.
		if recipeFileExists {
			shouldExpandThisItem = true
			dlog("  Item %s C10M is %s. Recipe exists. Will expand.", itemNameNorm, itemMethod)
		} else {
			if node.ErrorMessage == "" {
				node.ErrorMessage = "C10M indicates buy/N/A, but no recipe file to expand further."
			}
		}
	} else { // C10M was successful and method was e.g. "Secondary" (instabuy)
		dlog("  Item %s C10M is %s. Treating as base without expanding further.", itemNameNorm, itemMethod)
		// Error message for C10M might already be on 'node' if expansion isn't chosen
		if itemErrC10M != nil && node.ErrorMessage == "" {
			node.ErrorMessage = itemErrC10M.Error()
		}
	}

	if !shouldExpandThisItem || !recipeFileExists {
		node.IsBaseComponent = true
		// Acquisition details are from the C10M calculated for this item itself
		node.Acquisition = &BaseIngredientDetail{
			Quantity:       quantityNeeded,
			Method:         itemMethod, // This is the C10M method for the item itself
			BestCost:       toJSONFloat64(valueOrNaN(itemCostRaw)),
			AssociatedCost: toJSONFloat64(valueOrNaN(itemAssocCostRaw)),
			RR:             toJSONFloat64(valueOrNaN(itemRRRaw)),
			IF:             toJSONFloat64(valueOrNaN(itemIFRaw)),
			Delta:          toJSONFloat64(valueOrNaN(itemDeltaRaw)),
		}
		// Append C10M error to node error message if not already there
		if itemErrC10M != nil {
			if node.ErrorMessage == "" {
				node.ErrorMessage = itemErrC10M.Error()
			} else if !strings.Contains(node.ErrorMessage, itemErrC10M.Error()) {
				node.ErrorMessage += "; C10M: " + itemErrC10M.Error()
			}
		}
		// Add note if no recipe file was the reason for not expanding
		if !recipeFileExists && node.ErrorMessage == "" {
			node.ErrorMessage = "No recipe file found."
		} else if !recipeFileExists && !strings.Contains(node.ErrorMessage, "No recipe file") {
			node.ErrorMessage += "; No recipe file."
		}
		return node, nil
	}

	// --- Expand this item: Process its recipe ---
	var chosenRecipeCells map[string]string
	var craftedAmount float64 = 1.0
	recipeContentExists := false

	// Logic to choose recipe (from Item.Recipes or Item.Recipe) - same as level_cost_expansion.go
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

	if !recipeContentExists { // Should not be reached if shouldExpandThisItem is true and recipeFileExists
		node.IsBaseComponent = true
		node.ErrorMessage = "No usable recipe content in file, despite initial check."
		// Get C10M as fallback
		costN, mN, acN, rrN, ifN, dN, errN := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mN, BestCost: toJSONFloat64(valueOrNaN(costN)), AssociatedCost: toJSONFloat64(valueOrNaN(acN)), RR: toJSONFloat64(valueOrNaN(rrN)), IF: toJSONFloat64(valueOrNaN(ifN)), Delta: toJSONFloat64(valueOrNaN(dN))}
		if errN != nil && node.Acquisition != nil {
			node.Acquisition.Method = "ERROR (NoRecipeContent/C10M)"
		}
		return node, nil
	}

	node.QuantityPerCraft = craftedAmount
	node.NumCrafts = math.Ceil(quantityNeeded / craftedAmount)
	node.IsBaseComponent = false // Mark as expanded, not base

	ingredientsInOneCraft, aggErr := aggregateCells(chosenRecipeCells)
	if aggErr != nil {
		node.ErrorMessage = fmt.Sprintf("Error parsing recipe cells for '%s' during expansion: %v", itemNameNorm, aggErr)
		// This is a problem with the recipe data itself. Node is returned with error.
		// It won't have ingredients. It's not BaseComponent=true yet, but effectively is now.
		node.IsBaseComponent = true // Correcting its state
		// Try to get C10M for it as a fallback.
		costAgg, mAgg, acAgg, rrAgg, ifAgg, dAgg, errAgg := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mAgg, BestCost: toJSONFloat64(valueOrNaN(costAgg)), AssociatedCost: toJSONFloat64(valueOrNaN(acAgg)), RR: toJSONFloat64(valueOrNaN(rrAgg)), IF: toJSONFloat64(valueOrNaN(ifAgg)), Delta: toJSONFloat64(valueOrNaN(dAgg))}
		if errAgg != nil && node.Acquisition != nil {
			node.Acquisition.Method = "ERROR (AggFail/C10M)"
		}
		return node, nil // Return node with error, not a critical path error for caller typically
	}
	if len(ingredientsInOneCraft) == 0 {
		node.ErrorMessage = "Recipe for '" + itemNameNorm + "' yields zero ingredients."
		node.IsBaseComponent = true // No ingredients means it's effectively base for this path.
		costZero, mZero, acZero, rrZero, ifZero, dZero, errZero := calculateC10MForNode(itemNameNorm, quantityNeeded, apiResp, metricsMap)
		node.Acquisition = &BaseIngredientDetail{Quantity: quantityNeeded, Method: mZero, BestCost: toJSONFloat64(valueOrNaN(costZero)), AssociatedCost: toJSONFloat64(valueOrNaN(acZero)), RR: toJSONFloat64(valueOrNaN(rrZero)), IF: toJSONFloat64(valueOrNaN(ifZero)), Delta: toJSONFloat64(valueOrNaN(dZero))}
		if errZero != nil && node.Acquisition != nil {
			node.Acquisition.Method = "ERROR (ZeroIng/C10M)"
		}
		return node, nil
	}

	// Recursively expand ingredients
	maxChildSubTreeDepth := currentDepth // Initialize with current depth
	for ingName, ingAmtPerCraft := range ingredientsInOneCraft {
		totalIngAmtNeededForParent := ingAmtPerCraft * node.NumCrafts
		if totalIngAmtNeededForParent <= 0 { // Should not happen if ingAmtPerCraft > 0
			continue
		}

		subNode, errExpandSub := expandItemRecursiveTree(ingName, totalIngAmtNeededForParent, currentPath, originalTopLevelItemID, currentDepth+1, apiResp, metricsMap, itemFilesDir)

		if errExpandSub != nil {
			// A critical error occurred in a sub-expansion (e.g., file system error)
			// Create an error placeholder node for this ingredient
			errorSubNode := &CraftingStepNode{
				ItemName:        BAZAAR_ID(ingName), // Normalize
				QuantityNeeded:  totalIngAmtNeededForParent,
				ErrorMessage:    fmt.Sprintf("Sub-expansion for '%s' failed critically: %v", ingName, errExpandSub),
				IsBaseComponent: true, // Treat as base due to sub-expansion error
				Depth:           currentDepth + 1,
				MaxSubTreeDepth: currentDepth + 1,
				Acquisition:     &BaseIngredientDetail{Quantity: totalIngAmtNeededForParent, Method: "N/A (Sub-Expansion Critical Error)", BestCost: toJSONFloat64(math.NaN())},
			}
			node.Ingredients = append(node.Ingredients, errorSubNode)
			// Propagate the critical error up
			// If one sub-expansion fails critically, the whole parent expansion might be considered failed.
			// For now, let's just log it and return the partially built tree with the error node.
			// The caller (ExpandItemToTree) can decide how to handle this.
			// No, this function should return the error to its caller to stop recursion if critical.
			return node, fmt.Errorf("critical failure in sub-expansion of %s for %s: %w", ingName, itemNameNorm, errExpandSub)
		}

		if subNode != nil { // If sub-expansion was successful (even if subNode itself has errors/is base)
			node.Ingredients = append(node.Ingredients, subNode)
			if subNode.MaxSubTreeDepth > maxChildSubTreeDepth {
				maxChildSubTreeDepth = subNode.MaxSubTreeDepth
			}
		}
	}
	node.MaxSubTreeDepth = maxChildSubTreeDepth // Update node's max depth based on children
	return node, nil                            // Success for this level of expansion
}

func ExpandItemToTree(
	itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string,
) (*CraftingStepNode, error) { // Returns root node of the tree, and any critical error during expansion
	itemNameNorm := BAZAAR_ID(itemName)
	dlog("ExpandItemToTree: Starting expansion for %.2f x %s", quantity, itemNameNorm)

	// Check if recipe file exists. If not, treat as a base component.
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		dlog("  No recipe file for %s, treating as base component.", itemNameNorm)
		rootNode := &CraftingStepNode{
			ItemName:        itemNameNorm,
			QuantityNeeded:  quantity,
			IsBaseComponent: true,
			Depth:           0,
			MaxSubTreeDepth: 0,
		}
		// Get C10M acquisition details for this base item
		costR, mR, acR, rrR, ifR, dR, errC10M := calculateC10MForNode(itemNameNorm, quantity, apiResp, metricsMap)
		rootNode.Acquisition = &BaseIngredientDetail{
			Quantity:       quantity,
			Method:         mR,
			BestCost:       toJSONFloat64(valueOrNaN(costR)),
			AssociatedCost: toJSONFloat64(valueOrNaN(acR)),
			RR:             toJSONFloat64(valueOrNaN(rrR)),
			IF:             toJSONFloat64(valueOrNaN(ifR)),
			Delta:          toJSONFloat64(valueOrNaN(dR)),
		}
		if errC10M != nil {
			rootNode.Acquisition.Method = "ERROR (NoRecipe/C10M)" // Mark method due to C10M error
			rootNode.ErrorMessage = fmt.Sprintf("No recipe file and C10M error: %v", errC10M)
		} else if mR == "N/A" && quantity > 0 { // Added quantity > 0 to avoid for 0 qty calls.
			rootNode.ErrorMessage = "No recipe file and item acquisition method is N/A via C10M."
		}
		return rootNode, nil // No critical error, just no expansion possible
	} else if statErr != nil { // Other file system error
		return nil, fmt.Errorf("ExpandItemToTree: error checking recipe file '%s': %w", filePath, statErr)
	}

	// Recipe file exists, proceed with recursive expansion
	rootNode, errRec := expandItemRecursiveTree(itemNameNorm, quantity, nil, itemNameNorm, 0, apiResp, metricsMap, itemFilesDir)

	if errRec != nil {
		// A critical error occurred during recursive expansion (e.g., fs error in sub-call)
		log.Printf("ERROR: Critical error during recursive expansion of %s: %v", itemNameNorm, errRec)
		if rootNode == nil { // If error was so early that rootNode wasn't even formed
			rootNode = &CraftingStepNode{
				ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0,
				ErrorMessage: fmt.Sprintf("Recursive expansion failed critically early: %v", errRec),
				Acquisition:  &BaseIngredientDetail{Quantity: quantity, Method: "N/A (Critical Expansion Error)", BestCost: toJSONFloat64(math.NaN())},
			}
		} else { // Root node exists but sub-expansion failed critically
			if rootNode.ErrorMessage == "" {
				rootNode.ErrorMessage = fmt.Sprintf("Recursive expansion encountered a critical sub-error: %v", errRec)
			} else if !strings.Contains(rootNode.ErrorMessage, errRec.Error()) {
				rootNode.ErrorMessage += "; Critical sub-error: " + errRec.Error()
			}
		}
		return rootNode, errRec // Propagate the critical error
	}

	if rootNode == nil { // Should ideally not happen if errRec is nil
		log.Printf("ERROR: Expansion of %s resulted in nil root node without explicit error.", itemNameNorm)
		rootNode = &CraftingStepNode{
			ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Depth: 0, MaxSubTreeDepth: 0,
			ErrorMessage: "Expansion resulted in nil node unexpectedly.",
			Acquisition:  &BaseIngredientDetail{Quantity: quantity, Method: "N/A (Nil Node Error)", BestCost: toJSONFloat64(math.NaN())},
		}
		return rootNode, fmt.Errorf("nil node from expansion for %s", itemNameNorm) // Return an error for this unexpected state
	}

	dlog("ExpandItemToTree: Expansion complete for %s. Root node depth: %d, MaxSubTreeDepth: %d", itemNameNorm, rootNode.Depth, rootNode.MaxSubTreeDepth)
	return rootNode, nil // Success
}

// extractBaseIngredientsFromTree traverses the tree and collects all unique base components
// and their total quantities and acquisition details.
func extractBaseIngredientsFromTree(rootNode *CraftingStepNode) map[string]BaseIngredientDetail {
	baseMapDetails := make(map[string]BaseIngredientDetail)
	if rootNode == nil {
		return baseMapDetails
	}

	var queue []*CraftingStepNode
	queue = append(queue, rootNode)
	visited := make(map[*CraftingStepNode]bool) // To handle shared sub-nodes if trees were graphs (not strictly needed for pure trees)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr == nil || visited[curr] {
			continue
		}
		visited[curr] = true

		if curr.IsBaseComponent {
			if curr.Acquisition != nil {
				// Aggregate quantities for the same base ingredient from different paths
				existing, found := baseMapDetails[curr.ItemName]
				if found {
					updatedDetail := existing
					updatedDetail.Quantity += curr.Acquisition.Quantity // Sum quantities
					// Costs, RR, IF are per-instance and should not be summed directly.
					// The `analyzeTreeForCostsAndTimes` will sum the BestCost from these details.
					// Method might differ if acquired differently in parts of tree; this simplifies to first encountered.
					// This aggregation is primarily for quantity. analyzeTree should use the individual acquisition costs.
					baseMapDetails[curr.ItemName] = updatedDetail
				} else {
					// Store a copy of the acquisition detail
					acqCopy := *curr.Acquisition
					baseMapDetails[curr.ItemName] = acqCopy
				}
			} else { // Base component but no acquisition details (error state)
				if _, exists := baseMapDetails[curr.ItemName]; !exists { // Add error entry if not already there
					baseMapDetails[curr.ItemName] = BaseIngredientDetail{
						Quantity:       curr.QuantityNeeded, // Use QuantityNeeded from the node
						Method:         "ERROR (MissingAcquisitionInBase)",
						BestCost:       toJSONFloat64(math.NaN()),
						AssociatedCost: toJSONFloat64(math.NaN()), RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(math.NaN()),
					}
				} else { // If exists, just sum quantity, keep error state
					detail := baseMapDetails[curr.ItemName]
					detail.Quantity += curr.QuantityNeeded
					baseMapDetails[curr.ItemName] = detail
				}
			}
		} else { // Not a base component, add its ingredients to the queue
			for _, child := range curr.Ingredients {
				if child != nil {
					queue = append(queue, child)
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
		return math.Inf(1), math.NaN(), "", 0.0, false, "Root node is nil for analysis"
	}

	// If the root node itself is marked as a base component (e.g., no recipe, or cycle to self)
	if rootNode.IsBaseComponent {
		if rootNode.Acquisition != nil {
			cost := float64(rootNode.Acquisition.BestCost) // Convert JSONFloat64
			if !math.IsNaN(cost) && cost >= 0 {
				fillTimeRaw := 0.0 // Default for non-Primary or calculable zero time
				if rootNode.Acquisition.Method == "Primary" {
					metricsData, metricsOk := safeGetMetricsData(metricsMap, rootNode.ItemName)
					if metricsOk {
						// Use Acquisition.Quantity for fill time calculation for this base node
						calculatedTime, _, buyErr := calculateBuyOrderFillTime(rootNode.ItemName, rootNode.Acquisition.Quantity, metricsData)
						if buyErr == nil && !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, 0) && calculatedTime >= 0 {
							fillTimeRaw = calculatedTime
						} else {
							fillTimeRaw = math.Inf(1) // Error in fill time calculation
						}
					} else {
						fillTimeRaw = math.Inf(1) // Metrics not found for fill time
					}
				}
				// Error message from the node itself is important context
				return cost, valueOrNaN(fillTimeRaw), rootNode.ItemName, rootNode.Acquisition.Quantity, true, rootNode.ErrorMessage
			} else { // Invalid cost for base item
				errMsg := rootNode.ErrorMessage
				if errMsg == "" {
					errMsg = fmt.Sprintf("Base item '%s' acquisition cost is invalid/NaN (Cost: %.2f)", rootNode.ItemName, cost)
				} else if !strings.Contains(errMsg, "cost invalid") {
					errMsg += fmt.Sprintf("; Cost invalid (%.2f)", cost)
				}
				return math.Inf(1), math.NaN(), rootNode.ItemName, rootNode.QuantityNeeded, false, errMsg
			}
		} else { // No acquisition details for a base component node (error state)
			errMsg := rootNode.ErrorMessage
			if errMsg == "" {
				errMsg = fmt.Sprintf("Base item '%s' has no acquisition details.", rootNode.ItemName)
			}
			return math.Inf(1), math.NaN(), rootNode.ItemName, rootNode.QuantityNeeded, false, errMsg
		}
	}

	// If not a base component, extract all base ingredients from its sub-tree
	baseIngredientsDetailMap := extractBaseIngredientsFromTree(rootNode)
	if len(baseIngredientsDetailMap) == 0 {
		errMsg := rootNode.ErrorMessage
		if errMsg == "" {
			errMsg = fmt.Sprintf("No base ingredients found from expanded tree of '%s'.", rootNode.ItemName)
		} else {
			errMsg += "; No base ingredients found from expansion."
		}
		// Check if the root node itself had an issue making it effectively uncraftable
		// For example, if it's not base, but has no ingredients list.
		if len(rootNode.Ingredients) == 0 && !rootNode.IsBaseComponent {
			if !strings.Contains(errMsg, "yields zero") { // if not already about zero ingredients
				errMsg += " Root node has no ingredients despite not being base."
			}
		}
		return math.Inf(1), math.NaN(), "", 0.0, false, errMsg
	}

	currentTotalSumOfBestCosts := 0.0
	currentSlowestTimeRaw := 0.0 // Represents the slowest time among valid primary buy orders
	currentIsPossible := true    // Overall possibility of crafting
	var currentSlowestIngName string = ""
	var currentSlowestIngQty float64 = 0.0
	var errorMsgBuilder strings.Builder // For accumulating errors from base ingredients

	for itemID, detail := range baseIngredientsDetailMap {
		costVal := float64(detail.BestCost) // Convert JSONFloat64
		if math.IsNaN(costVal) || costVal < 0 || detail.Method == "N/A" || detail.Method == "ERROR" {
			if errorMsgBuilder.Len() > 0 {
				errorMsgBuilder.WriteString("; ")
			}
			errorMsgBuilder.WriteString(fmt.Sprintf("Invalid/unacquirable base ingredient '%s' (Cost: %.2f, Method: %s)", itemID, costVal, detail.Method))
			currentIsPossible = false
			currentTotalSumOfBestCosts = math.Inf(1) // Propagate impossibility for total cost
		}

		// Accumulate cost only if overall calculation is still possible and current cost is valid
		if !math.IsInf(currentTotalSumOfBestCosts, 1) && !math.IsNaN(costVal) && costVal >= 0 {
			currentTotalSumOfBestCosts += costVal
		} else if !math.IsInf(currentTotalSumOfBestCosts, 1) && (math.IsNaN(costVal) || costVal < 0) {
			// If a specific item's cost is bad, the whole sum becomes impossible (Inf)
			currentTotalSumOfBestCosts = math.Inf(1)
			currentIsPossible = false // Ensure overall is marked impossible
		}

		// Calculate fill time for this base ingredient if acquired via Primary buy order
		fillTimeForIngredientRaw := 0.0 // Default for non-Primary or if time is zero
		if detail.Method == "Primary" {
			metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
			if metricsOk {
				buyTime, _, buyErr := calculateBuyOrderFillTime(itemID, detail.Quantity, metricsData)
				if buyErr == nil && !math.IsNaN(buyTime) && !math.IsInf(buyTime, 0) && buyTime >= 0 {
					fillTimeForIngredientRaw = buyTime
				} else {
					fillTimeForIngredientRaw = math.Inf(1) // Mark this ingredient's fill time as Inf
					if errorMsgBuilder.Len() > 0 {
						errorMsgBuilder.WriteString("; ")
					}
					errorMsgBuilder.WriteString(fmt.Sprintf("fill time error for base '%s' (Err: %v, Time: %.2f)", itemID, buyErr, buyTime))
					currentIsPossible = false // Overall crafting becomes impossible if a primary buy fails
				}
			} else { // Metrics not found for a primary buy
				fillTimeForIngredientRaw = math.Inf(1)
				if errorMsgBuilder.Len() > 0 {
					errorMsgBuilder.WriteString("; ")
				}
				errorMsgBuilder.WriteString(fmt.Sprintf("metrics missing for primary fill time of base '%s'", itemID))
				currentIsPossible = false
			}
		}

		// Update overall slowest time among primary-bought ingredients
		if math.IsInf(fillTimeForIngredientRaw, 1) { // If current ingredient's fill time is Inf
			if !math.IsInf(currentSlowestTimeRaw, 1) { // And overall slowest wasn't Inf yet
				currentSlowestTimeRaw = fillTimeForIngredientRaw // Then overall slowest becomes Inf
				currentSlowestIngName = itemID
				currentSlowestIngQty = detail.Quantity
			}
		} else if !math.IsInf(currentSlowestTimeRaw, 1) && fillTimeForIngredientRaw > currentSlowestTimeRaw { // If neither is Inf and current is slower
			currentSlowestTimeRaw = fillTimeForIngredientRaw
			currentSlowestIngName = itemID
			currentSlowestIngQty = detail.Quantity
		}
	}

	finalErrorMsgStr := errorMsgBuilder.String()
	// Append root node's error message if it exists and is relevant
	if rootNode.ErrorMessage != "" {
		if finalErrorMsgStr == "" {
			finalErrorMsgStr = "TreeRoot: " + rootNode.ErrorMessage
		} else if !strings.Contains(finalErrorMsgStr, rootNode.ErrorMessage) { // Avoid duplicating root error
			finalErrorMsgStr += "; TreeRoot: " + rootNode.ErrorMessage
		}
	}
	// If calculation became impossible but no specific errors were added by loop, use root error or generic
	if !currentIsPossible && finalErrorMsgStr == "" {
		if rootNode.ErrorMessage != "" {
			finalErrorMsgStr = "TreeRoot: " + rootNode.ErrorMessage
		} else {
			finalErrorMsgStr = "Crafting became impossible during base ingredient analysis (unspecified reason)."
		}
	}

	return currentTotalSumOfBestCosts, valueOrNaN(currentSlowestTimeRaw), currentSlowestIngName, sanitizeFloat(currentSlowestIngQty), currentIsPossible, finalErrorMsgStr
}
