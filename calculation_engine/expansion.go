// expansion.go
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// valueOrNaN returns float64, using NaN for Inf/error states.
// The result of this needs to be cast to JSONFloat64 when assigned to struct fields.
func valueOrNaN(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return math.NaN()
	}
	return v
}

// --- Structs Definitions (using JSONFloat64) ---
// These structs are used by tree_builder.go and optimizer.go as well.
type BaseIngredientDetail struct {
	Quantity       float64     `json:"quantity"`
	BestCost       JSONFloat64 `json:"best_cost,omitempty"`
	AssociatedCost JSONFloat64 `json:"associated_cost,omitempty"`
	Method         string      `json:"method"`
	RR             JSONFloat64 `json:"rr,omitempty"`
	IF             JSONFloat64 `json:"if,omitempty"`
	Delta          JSONFloat64 `json:"delta,omitempty"`
}

type DualExpansionResult struct {
	ItemName                     string          `json:"item_name"`
	Quantity                     float64         `json:"quantity"`
	PrimaryBased                 ExpansionResult `json:"primary_based"`
	SecondaryBased               ExpansionResult `json:"secondary_based"`
	TopLevelInstasellTimeSeconds JSONFloat64     `json:"top_level_instasell_time_seconds,omitempty"`
}

type ExpansionResult struct {
	BaseIngredients                 map[string]BaseIngredientDetail `json:"base_ingredients"`
	TotalCost                       JSONFloat64                     `json:"total_cost,omitempty"`
	PerspectiveType                 string                          `json:"perspective_type"`
	TopLevelAction                  string                          `json:"top_level_action"`
	FinalCostMethod                 string                          `json:"final_cost_method"`
	CalculationPossible             bool                            `json:"calculation_possible"`
	ErrorMessage                    string                          `json:"error_message,omitempty"`
	TopLevelCost                    JSONFloat64                     `json:"top_level_cost,omitempty"`
	TopLevelRR                      JSONFloat64                     `json:"top_level_rr,omitempty"`
	SlowestIngredientBuyTimeSeconds JSONFloat64                     `json:"slowest_ingredient_buy_time_seconds,omitempty"`
	SlowestIngredientName           string                          `json:"slowest_ingredient_name,omitempty"`
	SlowestIngredientQuantity       float64                         `json:"slowest_ingredient_quantity"`
	RecipeTree                      *CraftingStepNode               `json:"recipe_tree,omitempty"` // Defined in tree_builder.go
}

func float64Ptr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	f := v
	return &f
}

func calculateDetailedCostsAndFillTimes(
	baseMapInput map[string]float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (
	totalSumOfBestCosts float64,
	detailedMapOutput map[string]BaseIngredientDetail,
	slowestFillTimeSecsRaw float64, // Returns raw float64
	slowestIngName string,
	slowestIngQty float64,
	isPossible bool,
	errorMsg string,
) {
	totalSumOfBestCosts = 0.0
	isPossible = true
	// Use strings.Builder for potentially long error messages
	var errorMsgBuilder strings.Builder
	detailedMapOutput = make(map[string]BaseIngredientDetail)
	currentSlowestTimeRaw := 0.0
	slowestIngName = ""
	slowestIngQty = 0.0

	if len(baseMapInput) == 0 {
		return 0.0, detailedMapOutput, 0.0, "", 0.0, true, ""
	}

	for itemID, quantity := range baseMapInput {
		if quantity <= 0 {
			continue
		}
		bestCostRaw, method, assocCostRaw, rrRaw, ifValRaw, errC10M := getBestC10M(itemID, quantity, apiResp, metricsMap)

		ingredientDetail := BaseIngredientDetail{
			Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN()), AssociatedCost: toJSONFloat64(math.NaN()),
			RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(math.NaN()),
		}

		if errC10M != nil || method == "N/A" || math.IsInf(bestCostRaw, 0) || math.IsNaN(bestCostRaw) || bestCostRaw < 0 {
			currentErrMsg := fmt.Sprintf("Cannot determine valid BEST cost for base ingredient '%s': BestC:%.2f, Method: %s, Err: %v", itemID, bestCostRaw, method, errC10M)
			dlog("  WARN (calculateDetailedCostsAndFillTimes - Best): %s", currentErrMsg)
			if errorMsgBuilder.Len() > 0 {
				errorMsgBuilder.WriteString("; ")
			}
			errorMsgBuilder.WriteString(currentErrMsg)
			isPossible = false
			totalSumOfBestCosts = math.Inf(1) // Mark total cost as impossible
			ingredientDetail.Method = method
			if errC10M != nil { // If specific error, mark method as ERROR
				ingredientDetail.Method = "ERROR"
			}
		} else { // Valid cost obtained
			if !math.IsInf(totalSumOfBestCosts, 1) { // Only add if total cost is still considered possible
				totalSumOfBestCosts += bestCostRaw
			}
			ingredientDetail.BestCost = toJSONFloat64(valueOrNaN(bestCostRaw))
			ingredientDetail.AssociatedCost = toJSONFloat64(valueOrNaN(assocCostRaw))
			ingredientDetail.Method = method
			if method == "Primary" {
				ingredientDetail.RR = toJSONFloat64(valueOrNaN(rrRaw))
				ingredientDetail.IF = toJSONFloat64(valueOrNaN(ifValRaw))
			}
		}
		metricsDataForDelta, metricsOkForDelta := safeGetMetricsData(metricsMap, itemID)
		if metricsOkForDelta {
			deltaValRaw := metricsDataForDelta.SellSize*metricsDataForDelta.SellFrequency - metricsDataForDelta.OrderSize*metricsDataForDelta.OrderFrequency
			ingredientDetail.Delta = toJSONFloat64(valueOrNaN(deltaValRaw))
		}
		detailedMapOutput[itemID] = ingredientDetail

		// Fill Time Calculation for Primary method
		buyTimeRaw := 0.0 // Default for non-Primary or calculable zero time
		if method == "Primary" {
			metricsDataForFill, metricsOkForFill := safeGetMetricsData(metricsMap, itemID)
			if metricsOkForFill {
				calculatedTime, _, buyErr := calculateBuyOrderFillTime(itemID, quantity, metricsDataForFill)
				if buyErr == nil && !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, 0) && calculatedTime >= 0 {
					buyTimeRaw = calculatedTime
				} else {
					buyTimeRaw = math.Inf(1) // Mark this ingredient's time as Inf
					currentErrMsg := fmt.Sprintf("Fill time calculation error for '%s': Err: %v, Time: %.2f", itemID, buyErr, calculatedTime)
					if errorMsgBuilder.Len() > 0 {
						errorMsgBuilder.WriteString("; ")
					}
					errorMsgBuilder.WriteString(currentErrMsg)
					isPossible = false // Overall calculation no longer possible
				}
			} else { // Metrics not found for fill time
				buyTimeRaw = math.Inf(1)
				currentErrMsg := fmt.Sprintf("Metrics not found for primary buy fill time of '%s'", itemID)
				if errorMsgBuilder.Len() > 0 {
					errorMsgBuilder.WriteString("; ")
				}
				errorMsgBuilder.WriteString(currentErrMsg)
				isPossible = false
			}
		}
		// Update overall slowest time
		if math.IsInf(buyTimeRaw, 1) { // If current ingredient's time is Inf
			if !math.IsInf(currentSlowestTimeRaw, 1) { // And overall slowest wasn't Inf yet
				currentSlowestTimeRaw = buyTimeRaw // Then overall becomes Inf
				slowestIngName = itemID
				slowestIngQty = quantity
			}
		} else if !math.IsInf(currentSlowestTimeRaw, 1) && buyTimeRaw > currentSlowestTimeRaw { // If neither is Inf and current is slower
			currentSlowestTimeRaw = buyTimeRaw
			slowestIngName = itemID
			slowestIngQty = quantity
		}
	}
	return totalSumOfBestCosts, detailedMapOutput, currentSlowestTimeRaw, slowestIngName, sanitizeFloat(slowestIngQty), isPossible, errorMsgBuilder.String()
}

func PerformDualExpansion(
	itemName string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	includeTreeInExpansionResult bool, // New parameter
) (*DualExpansionResult, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s (IncludeTree: %v) <<<", quantity, itemNameNorm, includeTreeInExpansionResult)
	result := &DualExpansionResult{
		ItemName: itemNameNorm, Quantity: sanitizeFloat(quantity),
		PrimaryBased: ExpansionResult{
			PerspectiveType: "PrimaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail),
			TotalCost: toJSONFloat64(math.NaN()), TopLevelCost: toJSONFloat64(math.NaN()), TopLevelRR: toJSONFloat64(math.NaN()), SlowestIngredientBuyTimeSeconds: toJSONFloat64(math.NaN()),
			RecipeTree: nil, // Initialize to nil
		},
		SecondaryBased: ExpansionResult{
			PerspectiveType: "SecondaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail),
			TotalCost: toJSONFloat64(math.NaN()), TopLevelCost: toJSONFloat64(math.NaN()), TopLevelRR: toJSONFloat64(math.NaN()), SlowestIngredientBuyTimeSeconds: toJSONFloat64(math.NaN()),
			RecipeTree: nil, // Initialize to nil
		},
		TopLevelInstasellTimeSeconds: toJSONFloat64(math.NaN()),
	}

	instaSellTimeRaw := math.Inf(1)
	topLevelProductData, topProductOK := safeGetProductData(apiResp, itemNameNorm)
	if topProductOK {
		calculatedTime, errInstaSell := calculateInstasellFillTime(quantity, topLevelProductData)
		if errInstaSell == nil && !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, 0) && calculatedTime >= 0 {
			instaSellTimeRaw = calculatedTime
		} else {
			dlog("  WARN: Could not calculate valid TopLevelInstasellTime for %s (Time: %.2f, Err: %v)", itemNameNorm, calculatedTime, errInstaSell)
		}
	} else {
		dlog("  WARN: Product data not found for %s, cannot calculate TopLevelInstasellTime.", itemNameNorm)
	}
	result.TopLevelInstasellTimeSeconds = toJSONFloat64(valueOrNaN(instaSellTimeRaw))

	sellP := getSellPrice(apiResp, itemNameNorm)
	buyP := getBuyPrice(apiResp, itemNameNorm)
	metricsP := getMetrics(metricsMap, itemNameNorm)
	topC10mPrimRaw, topC10mSecRaw, topIFRaw, topRRRaw, _, _, errTopC10M := calculateC10MInternal(itemNameNorm, quantity, sellP, buyP, metricsP)

	result.PrimaryBased.TopLevelCost = toJSONFloat64(valueOrNaN(topC10mPrimRaw))
	result.SecondaryBased.TopLevelCost = toJSONFloat64(valueOrNaN(topC10mSecRaw))

	validTopC10mPrim := errTopC10M == nil && !math.IsInf(topC10mPrimRaw, 0) && !math.IsNaN(topC10mPrimRaw) && topC10mPrimRaw >= 0
	if validTopC10mPrim {
		result.PrimaryBased.TopLevelRR = toJSONFloat64(valueOrNaN(topRRRaw))
	}
	validTopC10mSec := errTopC10M == nil && !math.IsInf(topC10mSecRaw, 0) && !math.IsNaN(topC10mSecRaw) && topC10mSecRaw >= 0

	topLevelRecipeExists := false
	var topLevelRecipeCheckErr error
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); statErr == nil {
		topLevelRecipeExists = true
	} else if !os.IsNotExist(statErr) {
		topLevelRecipeCheckErr = fmt.Errorf("checking recipe file '%s': %w", filePath, statErr)
	}

	if topLevelRecipeCheckErr != nil {
		errMsg := fmt.Sprintf("Failed to check top-level recipe: %v", topLevelRecipeCheckErr)
		result.PrimaryBased.ErrorMessage = errMsg
		result.SecondaryBased.ErrorMessage = errMsg
		baseAcqError := BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN()), AssociatedCost: toJSONFloat64(math.NaN()), RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(math.NaN())}

		// Create minimal error trees if tree is requested, otherwise they stay nil
		if includeTreeInExpansionResult {
			result.PrimaryBased.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: errMsg, IsBaseComponent: true, Acquisition: &baseAcqError, Depth: 0, MaxSubTreeDepth: 0}
			result.SecondaryBased.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: errMsg, IsBaseComponent: true, Acquisition: &baseAcqError, Depth: 0, MaxSubTreeDepth: 0}
		}
		// This is an early return, so explicit nilling if !includeTree is not needed beyond initialization.
		return result, nil
	}

	dlog("  Top-Level C10M: Primary=%.2f (IF=%.2f, RR=%.2f), Secondary=%.2f. Recipe Exists: %v. Error: %v", topC10mPrimRaw, topIFRaw, topRRRaw, topC10mSecRaw, topLevelRecipeExists, errTopC10M)
	isApiNotFoundError := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")

	costToCraftOptimalRaw := math.Inf(1)
	var craftRecipeTree *CraftingStepNode // This is the fully expanded tree for crafting
	craftPossible := false
	craftErrMsg := ""
	craftResultedInCycle := false
	craftSlowestFillTimeRaw := math.Inf(1)
	craftSlowestIngName := ""
	craftSlowestIngQty := 0.0
	var baseIngredientsFromCraft map[string]BaseIngredientDetail

	if topLevelRecipeExists {
		var errExpand error
		craftRecipeTree, errExpand = ExpandItemToTree(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)
		baseAcqTreeError := BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN()), AssociatedCost: toJSONFloat64(math.NaN()), RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(math.NaN())}
		if errExpand != nil {
			craftErrMsg = fmt.Sprintf("Expansion to tree failed: %v", errExpand)
			if craftRecipeTree == nil { // Ensure craftRecipeTree is not nil if error occurred
				craftRecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: craftErrMsg, IsBaseComponent: true, Acquisition: &baseAcqTreeError, Depth: 0, MaxSubTreeDepth: 0}
			} else { // If tree exists but had an error, ensure error message is on it
				if craftRecipeTree.ErrorMessage == "" {
					craftRecipeTree.ErrorMessage = craftErrMsg
				}
			}
		}
		if craftRecipeTree == nil { // Should be redundant due to above, but safety
			craftErrMsg = "Expansion to tree resulted in nil root node"
			craftRecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: craftErrMsg, IsBaseComponent: true, Acquisition: &baseAcqTreeError, Depth: 0, MaxSubTreeDepth: 0}
		} else {
			if craftRecipeTree.IsBaseComponent && strings.Contains(craftRecipeTree.ErrorMessage, "Cycle detected to top-level item") {
				craftResultedInCycle = true
				if craftErrMsg == "" {
					craftErrMsg = "Expansion resulted in top-level cycle"
				} else if !strings.Contains(craftErrMsg, "cycle") {
					craftErrMsg += "; Expansion resulted in top-level cycle"
				}
				costToCraftOptimalRaw = math.Inf(1)
				craftPossible = false
				craftSlowestFillTimeRaw = math.Inf(1)
			} else {
				var analysisErrorMsg string
				costToCraftOptimalRaw, craftSlowestFillTimeRaw, craftSlowestIngName, craftSlowestIngQty, craftPossible, analysisErrorMsg = analyzeTreeForCostsAndTimes(craftRecipeTree, apiResp, metricsMap)
				if !craftPossible {
					if craftErrMsg == "" {
						craftErrMsg = "Failed to calculate detailed costs/times from tree"
					}
					if analysisErrorMsg != "" {
						craftErrMsg += "; Analysis: " + analysisErrorMsg
					}
				} else {
					dlog("  Cost to Craft (from Tree) for %s: %.2f. Slowest Ing: %s (Qty: %.2f, TimeRaw: %.2f)", itemNameNorm, costToCraftOptimalRaw, craftSlowestIngName, craftSlowestIngQty, craftSlowestFillTimeRaw)
				}
			}
			baseIngredientsFromCraft = extractBaseIngredientsFromTree(craftRecipeTree)
		}
	} else { // No recipe file exists
		craftErrMsg = "No recipe found for top-level item."
		costToCraftOptimalRaw = math.Inf(1)
		craftPossible = false
		craftSlowestFillTimeRaw = math.Inf(1)
		baseAcqNoRecipe := BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN()), AssociatedCost: toJSONFloat64(math.NaN()), RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(math.NaN())}
		// Create a minimal tree node for this case
		craftRecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, ErrorMessage: craftErrMsg, Acquisition: &baseAcqNoRecipe, Depth: 0, MaxSubTreeDepth: 0}
		// Try to get C10M for this "base" item if no recipe
		costRawVal, method, assocCostRawVal, rrValRaw, ifValRaw, deltaValRaw, c10mErr := calculateC10MForNode(itemNameNorm, quantity, apiResp, metricsMap)
		if c10mErr == nil && !math.IsInf(costRawVal, 0) && !math.IsNaN(costRawVal) && costRawVal >= 0 {
			craftRecipeTree.Acquisition = &BaseIngredientDetail{
				Quantity: quantity, BestCost: toJSONFloat64(valueOrNaN(costRawVal)), AssociatedCost: toJSONFloat64(valueOrNaN(assocCostRawVal)), Method: method,
				RR: toJSONFloat64(valueOrNaN(rrValRaw)), IF: toJSONFloat64(valueOrNaN(ifValRaw)), Delta: toJSONFloat64(valueOrNaN(deltaValRaw)),
			}
		} else { // C10M failed for non-craftable item
			if craftRecipeTree.Acquisition == nil { // Should not happen if initialized above
				craftRecipeTree.Acquisition = &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN())}
			}
			craftRecipeTree.Acquisition.BestCost = toJSONFloat64(math.NaN()) // Mark cost as NaN
			craftRecipeTree.Acquisition.Method = "N/A"
			if c10mErr != nil {
				if craftRecipeTree.ErrorMessage == "" {
					craftRecipeTree.ErrorMessage = c10mErr.Error()
				} else if !strings.Contains(craftRecipeTree.ErrorMessage, c10mErr.Error()) {
					craftRecipeTree.ErrorMessage += "; C10M Error: " + c10mErr.Error()
				}
			}
		}
	}

	// --- PrimaryBased (res1) Logic ---
	res1 := &result.PrimaryBased
	minCostP1Raw := math.Inf(1)
	chosenMethodP1 := "N/A"

	if craftPossible && !math.IsInf(costToCraftOptimalRaw, 0) && !math.IsNaN(costToCraftOptimalRaw) && costToCraftOptimalRaw >= 0 {
		if costToCraftOptimalRaw < minCostP1Raw {
			minCostP1Raw = costToCraftOptimalRaw
			chosenMethodP1 = "Craft"
		}
	}
	if validTopC10mPrim {
		if topC10mPrimRaw < minCostP1Raw {
			minCostP1Raw = topC10mPrimRaw
			chosenMethodP1 = "Primary"
		}
	}
	if validTopC10mSec {
		if topC10mSecRaw < minCostP1Raw {
			minCostP1Raw = topC10mSecRaw
			chosenMethodP1 = "Secondary"
		}
	}
	dlog("  P1 Minimum Cost Choice: %s (Raw Min Cost: %.2f)", chosenMethodP1, minCostP1Raw)

	if chosenMethodP1 == "Craft" {
		res1.TopLevelAction = "Expanded"
		res1.BaseIngredients = baseIngredientsFromCraft
		if includeTreeInExpansionResult {
			res1.RecipeTree = craftRecipeTree
		}
		res1.TotalCost = toJSONFloat64(valueOrNaN(costToCraftOptimalRaw))
		res1.CalculationPossible = craftPossible
		res1.FinalCostMethod = "SumBestC10MFromTree"
		res1.SlowestIngredientBuyTimeSeconds = toJSONFloat64(valueOrNaN(craftSlowestFillTimeRaw))
		res1.SlowestIngredientName = craftSlowestIngName
		res1.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
		if !craftPossible && res1.ErrorMessage == "" {
			res1.ErrorMessage = craftErrMsg
		} else if !craftPossible && craftErrMsg != "" && !strings.Contains(res1.ErrorMessage, craftErrMsg) {
			res1.ErrorMessage += "; " + craftErrMsg
		}
	} else if chosenMethodP1 == "Primary" || chosenMethodP1 == "Secondary" {
		res1.TopLevelAction = "TreatedAsBase"
		var acqCostRawVal, acqAssocCostRawVal, acqRRRawVal, acqIFRawVal, acqDeltaRawVal float64
		var acqMethod string
		fillTimeForBaseRawVal := math.Inf(1)

		if chosenMethodP1 == "Primary" {
			acqCostRawVal, acqMethod, acqAssocCostRawVal, acqRRRawVal, acqIFRawVal = topC10mPrimRaw, "Primary", sellP*quantity, topRRRaw, topIFRaw
			acqDeltaRawVal = math.NaN() // Initialize
			if metricsP.ProductID != "" {
				acqDeltaRawVal = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
			}
			if metricsP.ProductID != "" { // Check again for fill time specifically
				fillTimeVal, _, errFill := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
				if errFill == nil && !math.IsNaN(fillTimeVal) && !math.IsInf(fillTimeVal, 0) && fillTimeVal >= 0 {
					fillTimeForBaseRawVal = fillTimeVal
				}
			}
			res1.FinalCostMethod = "FixedTopLevelPrimary"
			res1.TotalCost = toJSONFloat64(valueOrNaN(topC10mPrimRaw))
		} else { // Secondary
			acqCostRawVal, acqMethod, acqAssocCostRawVal = topC10mSecRaw, "Secondary", sellP*quantity
			acqRRRawVal, acqIFRawVal = math.NaN(), math.NaN()
			acqDeltaRawVal = math.NaN()
			if metricsP.ProductID != "" {
				acqDeltaRawVal = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
			}
			res1.FinalCostMethod = "FixedTopLevelSecondary"
			res1.TotalCost = toJSONFloat64(valueOrNaN(topC10mSecRaw))
			fillTimeForBaseRawVal = 0.0 // Instabuy is instant
		}
		currentBaseDetailP1 := BaseIngredientDetail{
			Quantity: quantity, BestCost: toJSONFloat64(valueOrNaN(acqCostRawVal)), AssociatedCost: toJSONFloat64(valueOrNaN(acqAssocCostRawVal)), Method: acqMethod,
			RR: toJSONFloat64(valueOrNaN(acqRRRawVal)), IF: toJSONFloat64(valueOrNaN(acqIFRawVal)), Delta: toJSONFloat64(valueOrNaN(acqDeltaRawVal)),
		}
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP1}
		if includeTreeInExpansionResult {
			res1.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Acquisition: &currentBaseDetailP1, Depth: 0, MaxSubTreeDepth: 0}
		}
		res1.CalculationPossible = true
		res1.SlowestIngredientBuyTimeSeconds = toJSONFloat64(valueOrNaN(fillTimeForBaseRawVal))
		res1.SlowestIngredientName = itemNameNorm
		res1.SlowestIngredientQuantity = sanitizeFloat(quantity)
	} else { // N/A
		res1.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res1.TotalCost = toJSONFloat64(math.NaN())
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "P1: No valid acquisition method."
		}
		if errTopC10M != nil && !strings.Contains(res1.ErrorMessage, errTopC10M.Error()) {
			res1.ErrorMessage += "; TopC10M Err: " + errTopC10M.Error()
		}
		if craftErrMsg != "" && !strings.Contains(res1.ErrorMessage, craftErrMsg) {
			res1.ErrorMessage += "; Craft Err: " + craftErrMsg
		}

		acqDeltaUnobtainableRaw := math.NaN()
		if metricsP.ProductID != "" {
			acqDeltaUnobtainableRaw = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		}
		baseAcqUnobtainable := BaseIngredientDetail{
			Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN()), AssociatedCost: toJSONFloat64(math.NaN()),
			RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(valueOrNaN(acqDeltaUnobtainableRaw)),
		}

		if includeTreeInExpansionResult {
			// If crafting was attempted and failed/cycled, use that tree for context
			if craftRecipeTree != nil && (craftResultedInCycle || !craftPossible || craftRecipeTree.ErrorMessage != "") {
				res1.RecipeTree = craftRecipeTree
				if res1.RecipeTree.ErrorMessage == "" { // Ensure error message propagates
					res1.RecipeTree.ErrorMessage = res1.ErrorMessage
				} else if res1.ErrorMessage != "" && !strings.Contains(res1.RecipeTree.ErrorMessage, res1.ErrorMessage) {
					res1.RecipeTree.ErrorMessage += "; " + res1.ErrorMessage
				}
			} else { // Otherwise, a simple error node
				res1.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, ErrorMessage: res1.ErrorMessage, Acquisition: &baseAcqUnobtainable, Depth: 0, MaxSubTreeDepth: 0}
			}
		}
		res1.SlowestIngredientBuyTimeSeconds = toJSONFloat64(math.NaN())
	}
	if chosenMethodP1 != "Craft" && craftResultedInCycle {
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "Crafting resulted in a cycle, but another acquisition method was chosen for P1."
		} else if !strings.Contains(res1.ErrorMessage, "cycle") {
			res1.ErrorMessage += "; Crafting resulted in a cycle."
		}
		if includeTreeInExpansionResult && craftRecipeTree != nil {
			res1.RecipeTree = craftRecipeTree
		}
	}

	// --- SecondaryBased (res2) Logic ---
	res2 := &result.SecondaryBased
	chosenMethodP2 := "N/A" // Default

	if isApiNotFoundError {
		if craftPossible && !craftResultedInCycle && !math.IsInf(costToCraftOptimalRaw, 0) && !math.IsNaN(costToCraftOptimalRaw) && costToCraftOptimalRaw >= 0 {
			chosenMethodP2 = "Craft"
		} else {
			chosenMethodP2 = "ExpansionFailed"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "API data missing for top-level item and crafting not viable"
			}
			if craftErrMsg != "" && !strings.Contains(res2.ErrorMessage, craftErrMsg) {
				res2.ErrorMessage += "; " + craftErrMsg
			}
		}
	} else {
		if craftPossible && !craftResultedInCycle && !math.IsInf(costToCraftOptimalRaw, 0) && !math.IsNaN(costToCraftOptimalRaw) && costToCraftOptimalRaw >= 0 {
			if validTopC10mPrim {
				if costToCraftOptimalRaw <= topC10mPrimRaw {
					chosenMethodP2 = "Craft"
				} else {
					chosenMethodP2 = "Primary"
				}
			} else {
				chosenMethodP2 = "Craft"
			}
		} else if validTopC10mPrim {
			chosenMethodP2 = "Primary"
		} else {
			chosenMethodP2 = "ExpansionFailed"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "P2: Neither Craft nor Primary acquisition is viable."
			}
			if craftErrMsg != "" && !strings.Contains(res2.ErrorMessage, craftErrMsg) {
				res2.ErrorMessage += "; " + craftErrMsg
			}
			if errTopC10M != nil && !strings.Contains(res2.ErrorMessage, errTopC10M.Error()) {
				res2.ErrorMessage += "; TopC10M Err: " + errTopC10M.Error()
			}
		}
	}
	dlog("  P2 Decision: %s", chosenMethodP2)

	if chosenMethodP2 == "Craft" {
		res2.TopLevelAction = "Expanded"
		res2.BaseIngredients = baseIngredientsFromCraft
		if includeTreeInExpansionResult {
			res2.RecipeTree = craftRecipeTree
		}
		res2.TotalCost = toJSONFloat64(valueOrNaN(costToCraftOptimalRaw))
		res2.CalculationPossible = craftPossible
		res2.FinalCostMethod = "SumBestC10MFromTree"
		res2.SlowestIngredientBuyTimeSeconds = toJSONFloat64(valueOrNaN(craftSlowestFillTimeRaw))
		res2.SlowestIngredientName = craftSlowestIngName
		res2.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
		if !craftPossible && res2.ErrorMessage == "" {
			res2.ErrorMessage = craftErrMsg
		} else if !craftPossible && craftErrMsg != "" && !strings.Contains(res2.ErrorMessage, craftErrMsg) {
			res2.ErrorMessage += "; " + craftErrMsg
		}
	} else if chosenMethodP2 == "Primary" {
		res2.TopLevelAction = "TreatedAsBase"
		res2.TotalCost = toJSONFloat64(valueOrNaN(topC10mPrimRaw))
		res2.FinalCostMethod = "FixedTopLevelPrimary"
		acqDeltaP2PrimRaw := math.NaN()
		if metricsP.ProductID != "" {
			acqDeltaP2PrimRaw = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		}
		currentBaseDetailP2PrimMethod := BaseIngredientDetail{
			Quantity: quantity, BestCost: toJSONFloat64(valueOrNaN(topC10mPrimRaw)), AssociatedCost: toJSONFloat64(valueOrNaN(sellP * quantity)), Method: "Primary",
			RR: toJSONFloat64(valueOrNaN(topRRRaw)), IF: toJSONFloat64(valueOrNaN(topIFRaw)), Delta: toJSONFloat64(valueOrNaN(acqDeltaP2PrimRaw)),
		}
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP2PrimMethod}
		if includeTreeInExpansionResult {
			res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Acquisition: &currentBaseDetailP2PrimMethod, Depth: 0, MaxSubTreeDepth: 0}
		}
		res2.CalculationPossible = true
		fillTimeP2PrimRawVal := math.Inf(1)
		if metricsP.ProductID != "" {
			f, _, errF := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
			if errF == nil && !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0 {
				fillTimeP2PrimRawVal = f
			}
		}
		res2.SlowestIngredientBuyTimeSeconds = toJSONFloat64(valueOrNaN(fillTimeP2PrimRawVal))
		res2.SlowestIngredientName = itemNameNorm
		res2.SlowestIngredientQuantity = sanitizeFloat(quantity)
	} else {
		res2.TopLevelAction = chosenMethodP2
		res2.TotalCost = toJSONFloat64(math.NaN())
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"

		if includeTreeInExpansionResult {
			if chosenMethodP2 == "ExpansionFailed" && craftRecipeTree != nil {
				res2.RecipeTree = craftRecipeTree
				if res2.RecipeTree.ErrorMessage == "" {
					res2.RecipeTree.ErrorMessage = res2.ErrorMessage
				} else if res2.ErrorMessage != "" && !strings.Contains(res2.RecipeTree.ErrorMessage, res2.ErrorMessage) {
					res2.RecipeTree.ErrorMessage += "; " + res2.ErrorMessage
				}
			} else {
				acqDeltaP2NARaw := math.NaN()
				if metricsP.ProductID != "" {
					acqDeltaP2NARaw = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
				}
				baseAcqP2NA := BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: toJSONFloat64(math.NaN()), AssociatedCost: toJSONFloat64(math.NaN()), RR: toJSONFloat64(math.NaN()), IF: toJSONFloat64(math.NaN()), Delta: toJSONFloat64(valueOrNaN(acqDeltaP2NARaw))}
				res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, ErrorMessage: res2.ErrorMessage, Acquisition: &baseAcqP2NA, Depth: 0, MaxSubTreeDepth: 0}
			}
		}

		if chosenMethodP2 == "ExpansionFailed" {
			res2.SlowestIngredientBuyTimeSeconds = toJSONFloat64(valueOrNaN(craftSlowestFillTimeRaw))
			res2.SlowestIngredientName = craftSlowestIngName
			res2.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
		} else {
			res2.SlowestIngredientBuyTimeSeconds = toJSONFloat64(math.NaN())
		}
	}

	result.PrimaryBased.SlowestIngredientQuantity = sanitizeFloat(result.PrimaryBased.SlowestIngredientQuantity)
	result.SecondaryBased.SlowestIngredientQuantity = sanitizeFloat(result.SecondaryBased.SlowestIngredientQuantity)

	if !includeTreeInExpansionResult {
		if result.PrimaryBased.RecipeTree != nil {
			dlog("  Final Nilling P1 RecipeTree for %s as per request.", itemNameNorm)
			result.PrimaryBased.RecipeTree = nil
		}
		if result.SecondaryBased.RecipeTree != nil {
			dlog("  Final Nilling P2 RecipeTree for %s as per request.", itemNameNorm)
			result.SecondaryBased.RecipeTree = nil
		}
	} else {
		dlog("  Retaining P1/P2 RecipeTrees for %s as per request (if they were set).", itemNameNorm)
	}

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
