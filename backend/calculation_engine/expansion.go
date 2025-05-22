// expansion.go
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// --- Structs Definitions (ensure these are at the top of expansion.go) ---

// BaseIngredientDetail describes a base component's acquisition details.
type BaseIngredientDetail struct {
	Quantity       float64  `json:"quantity"`
	BestCost       *float64 `json:"best_cost,omitempty"`       // Pointer
	AssociatedCost *float64 `json:"associated_cost,omitempty"` // Pointer
	Method         string   `json:"method"`
	RR             *float64 `json:"rr,omitempty"`
	IF             *float64 `json:"if,omitempty"`
	Delta          *float64 `json:"delta,omitempty"`
}

// DualExpansionResult is the top-level result for dual perspective expansion.
type DualExpansionResult struct {
	ItemName                     string          `json:"item_name"`
	Quantity                     float64         `json:"quantity"`
	PrimaryBased                 ExpansionResult `json:"primary_based"`
	SecondaryBased               ExpansionResult `json:"secondary_based"`
	TopLevelInstasellTimeSeconds *float64        `json:"top_level_instasell_time_seconds,omitempty"`
}

// ExpansionResult holds details for one perspective of expansion (Primary or Secondary based).
type ExpansionResult struct {
	BaseIngredients                 map[string]BaseIngredientDetail `json:"base_ingredients"`     // Retained for summary/compatibility
	TotalCost                       *float64                        `json:"total_cost,omitempty"` // Pointer
	PerspectiveType                 string                          `json:"perspective_type"`
	TopLevelAction                  string                          `json:"top_level_action"`
	FinalCostMethod                 string                          `json:"final_cost_method"`
	CalculationPossible             bool                            `json:"calculation_possible"`
	ErrorMessage                    string                          `json:"error_message,omitempty"`
	TopLevelCost                    *float64                        `json:"top_level_cost,omitempty"` // Pointer
	TopLevelRR                      *float64                        `json:"top_level_rr,omitempty"`
	SlowestIngredientBuyTimeSeconds *float64                        `json:"slowest_ingredient_buy_time_seconds,omitempty"`
	SlowestIngredientName           string                          `json:"slowest_ingredient_name,omitempty"`
	SlowestIngredientQuantity       float64                         `json:"slowest_ingredient_quantity"`
	RecipeTree                      *CraftingStepNode               `json:"recipe_tree,omitempty"` // From tree_builder.go
}

// --- Utility Functions ---

// float64Ptr returns a pointer to a float64, or nil if the value is NaN or Inf.
func float64Ptr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	f := v
	return &f
}

// calculateDetailedCostsAndFillTimes: This function calculates costs based on a flat map.
// It's kept for now as analyzeTreeForCostsAndTimes uses extractBaseIngredientsFromTree
// to generate a compatible map.
func calculateDetailedCostsAndFillTimes(
	baseMapInput map[string]float64, // This is map[itemID] -> quantity
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (
	totalSumOfBestCosts float64, // This will be summed from *float64, so it should be finite or Inf
	detailedMapOutput map[string]BaseIngredientDetail,
	slowestFillTimeSecs *float64,
	slowestIngName string,
	slowestIngQty float64,
	isPossible bool,
	errorMsg string,
) {
	totalSumOfBestCosts = 0.0
	isPossible = true
	errorMsg = ""
	detailedMapOutput = make(map[string]BaseIngredientDetail)

	var currentSlowestTime float64 = 0.0
	slowestIngName = ""
	slowestIngQty = 0.0

	if len(baseMapInput) == 0 {
		return 0.0, detailedMapOutput, float64Ptr(0.0), "", 0.0, true, ""
	}

	for itemID, quantity := range baseMapInput {
		if quantity <= 0 {
			continue
		}
		bestCost, method, assocCost, rr, ifVal, errC10M := getBestC10M(itemID, quantity, apiResp, metricsMap)

		ingredientDetail := BaseIngredientDetail{Quantity: quantity, Method: "N/A"} // BestCost & AssocCost are nil by default

		if errC10M != nil || method == "N/A" || math.IsInf(bestCost, 0) || math.IsNaN(bestCost) || bestCost < 0 {
			currentErrMsg := fmt.Sprintf("Cannot determine valid BEST cost for base ingredient '%s': BestC:%.2f, Method: %s, Err: %v", itemID, bestCost, method, errC10M)
			dlog("  WARN (calculateDetailedCostsAndFillTimes - Best): %s", currentErrMsg)
			if errorMsg == "" {
				errorMsg = currentErrMsg
			} else {
				errorMsg += "; " + currentErrMsg
			}
			isPossible = false
			totalSumOfBestCosts = math.Inf(1)
			ingredientDetail.Method = method
			if errC10M != nil {
				ingredientDetail.Method = "ERROR"
			}
			ingredientDetail.BestCost = float64Ptr(math.Inf(1))
			ingredientDetail.AssociatedCost = float64Ptr(math.NaN())
			detailedMapOutput[itemID] = ingredientDetail
		} else {
			if !math.IsInf(totalSumOfBestCosts, 1) {
				totalSumOfBestCosts += bestCost
			}
			ingredientDetail.BestCost = float64Ptr(bestCost)
			ingredientDetail.AssociatedCost = float64Ptr(assocCost)
			ingredientDetail.Method = method
			if method == "Primary" {
				ingredientDetail.RR = float64Ptr(rr)
				ingredientDetail.IF = float64Ptr(ifVal)
			}
		}

		metricsDataForDelta, metricsOkForDelta := safeGetMetricsData(metricsMap, itemID)
		if metricsOkForDelta {
			deltaVal := metricsDataForDelta.SellSize*metricsDataForDelta.SellFrequency - metricsDataForDelta.OrderSize*metricsDataForDelta.OrderFrequency
			ingredientDetail.Delta = float64Ptr(deltaVal)
		}
		detailedMapOutput[itemID] = ingredientDetail

		metricsDataForFill, metricsOkForFill := safeGetMetricsData(metricsMap, itemID)
		var buyTime float64 = 0.0
		if method == "Primary" {
			if metricsOkForFill {
				calculatedTime, _, buyErr := calculateBuyOrderFillTime(itemID, quantity, metricsDataForFill)
				if buyErr == nil && !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, -1) && calculatedTime >= 0 {
					buyTime = calculatedTime
				} else {
					buyTime = math.Inf(1)
					currentErrMsg := fmt.Sprintf("Fill time calculation error for '%s'", itemID)
					if errorMsg == "" {
						errorMsg = currentErrMsg
					} else {
						errorMsg += "; " + currentErrMsg
					}
					isPossible = false
				}
			} else {
				buyTime = math.Inf(1)
				currentErrMsg := fmt.Sprintf("Metrics not found for primary buy fill time of '%s'", itemID)
				if errorMsg == "" {
					errorMsg = currentErrMsg
				} else {
					errorMsg += "; " + currentErrMsg
				}
				isPossible = false
			}
		}

		if math.IsInf(buyTime, 1) {
			if !math.IsInf(currentSlowestTime, 1) {
				currentSlowestTime = buyTime
				slowestIngName = itemID
				slowestIngQty = quantity
			}
		} else if !math.IsInf(currentSlowestTime, 1) && buyTime > currentSlowestTime {
			currentSlowestTime = buyTime
			slowestIngName = itemID
			slowestIngQty = quantity
		}
	}

	slowestFillTimeSecs = float64Ptr(currentSlowestTime)

	if !isPossible {
		if errorMsg == "" {
			errorMsg = "Calculation of detailed costs/fill times failed for an unknown reason."
		}
		return totalSumOfBestCosts, detailedMapOutput, slowestFillTimeSecs, slowestIngName, sanitizeFloat(slowestIngQty), false, errorMsg
	}
	return totalSumOfBestCosts, detailedMapOutput, slowestFillTimeSecs, slowestIngName, sanitizeFloat(slowestIngQty), true, ""
}

// --- Main Expansion Function ---
func PerformDualExpansion(itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string) (*DualExpansionResult, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s <<<", quantity, itemNameNorm)
	result := &DualExpansionResult{
		ItemName: itemNameNorm,
		Quantity: sanitizeFloat(quantity),
		PrimaryBased: ExpansionResult{
			PerspectiveType:                 "PrimaryBased",
			CalculationPossible:             false,
			BaseIngredients:                 make(map[string]BaseIngredientDetail),
			TotalCost:                       float64Ptr(math.Inf(1)),
			TopLevelCost:                    float64Ptr(math.Inf(1)),
			SlowestIngredientBuyTimeSeconds: float64Ptr(math.Inf(1)),
		},
		SecondaryBased: ExpansionResult{
			PerspectiveType:                 "SecondaryBased",
			CalculationPossible:             false,
			BaseIngredients:                 make(map[string]BaseIngredientDetail),
			TotalCost:                       float64Ptr(math.Inf(1)),
			TopLevelCost:                    float64Ptr(math.Inf(1)),
			SlowestIngredientBuyTimeSeconds: float64Ptr(math.Inf(1)),
		},
		TopLevelInstasellTimeSeconds: float64Ptr(0.0),
	}

	var instaSellTimeRaw float64 = math.Inf(1)
	topLevelProductData, topProductOK := safeGetProductData(apiResp, itemNameNorm)
	if topProductOK {
		calculatedTime, errInstaSell := calculateInstasellFillTime(quantity, topLevelProductData)
		if errInstaSell == nil && !math.IsNaN(calculatedTime) && !math.IsInf(calculatedTime, -1) && calculatedTime >= 0 {
			instaSellTimeRaw = calculatedTime
		} else {
			dlog("  WARN: Could not calculate valid TopLevelInstasellTime for %s (Time: %.2f, Err: %v)", itemNameNorm, calculatedTime, errInstaSell)
		}
	} else {
		dlog("  WARN: Product data not found for %s, cannot calculate TopLevelInstasellTime.", itemNameNorm)
	}
	result.TopLevelInstasellTimeSeconds = float64Ptr(instaSellTimeRaw)

	sellP := getSellPrice(apiResp, itemNameNorm)
	buyP := getBuyPrice(apiResp, itemNameNorm)
	metricsP := getMetrics(metricsMap, itemNameNorm)
	topC10mPrim, topC10mSec, topIF, topRR, _, _, errTopC10M := calculateC10MInternal(
		itemNameNorm, quantity, sellP, buyP, metricsP,
	)
	result.PrimaryBased.TopLevelCost = float64Ptr(topC10mPrim)
	result.SecondaryBased.TopLevelCost = float64Ptr(topC10mSec)
	validTopC10mPrim := errTopC10M == nil && !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0
	if validTopC10mPrim {
		result.PrimaryBased.TopLevelRR = float64Ptr(topRR)
	}
	validTopC10mSec := errTopC10M == nil && !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0

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
		result.PrimaryBased.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: errMsg, IsBaseComponent: true, Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1))}}
		result.SecondaryBased.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: errMsg, IsBaseComponent: true, Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1))}}
		return result, nil
	}

	dlog("  Top-Level C10M: Primary=%.2f (IF=%.2f, RR=%.2f), Secondary=%.2f. Recipe Exists: %v. Error: %v", topC10mPrim, topIF, topRR, topC10mSec, topLevelRecipeExists, errTopC10M)
	isApiNotFoundError := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")

	var costToCraftOptimal float64 = math.Inf(1)
	var craftRecipeTree *CraftingStepNode
	var craftPossible bool = false
	var craftErrMsg string = ""
	var craftResultedInCycle bool = false
	var craftSlowestFillTimePtr *float64
	var craftSlowestIngName string = ""
	var craftSlowestIngQty float64 = 0.0
	var baseIngredientsFromCraft map[string]BaseIngredientDetail

	if topLevelRecipeExists {
		var errExpand error
		craftRecipeTree, errExpand = ExpandItemToTree(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)

		if errExpand != nil {
			craftErrMsg = fmt.Sprintf("Expansion to tree failed: %v", errExpand)
			log.Printf("WARN (PerformDualExpansion): ExpandItemToTree failed for %s: %v", itemNameNorm, errExpand)
		}
		if craftRecipeTree == nil {
			craftErrMsg = "Expansion to tree resulted in nil root node"
			craftRecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: craftErrMsg, IsBaseComponent: true, Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1))}}
		} else {
			if craftRecipeTree.IsBaseComponent && strings.Contains(craftRecipeTree.ErrorMessage, "Cycle detected to top-level item") {
				craftResultedInCycle = true
				if craftErrMsg == "" {
					craftErrMsg = "Expansion resulted in top-level cycle"
				}
				costToCraftOptimal = math.Inf(1)
				craftPossible = false
			} else {
				var analysisErrorMsg string
				costToCraftOptimal, craftSlowestFillTimePtr, craftSlowestIngName, craftSlowestIngQty, craftPossible, analysisErrorMsg =
					analyzeTreeForCostsAndTimes(craftRecipeTree, apiResp, metricsMap)

				if !craftPossible {
					if craftErrMsg == "" {
						craftErrMsg = "Failed to calculate detailed costs/times from tree"
					}
					if analysisErrorMsg != "" {
						craftErrMsg += "; Analysis: " + analysisErrorMsg
					}
				} else {
					dlog("  Cost to Craft (from Tree) for %s: %.2f. Slowest Ing: %s (Qty: %.2f, TimePtr: %v)", itemNameNorm, costToCraftOptimal, craftSlowestIngName, craftSlowestIngQty, craftSlowestFillTimePtr)
				}
			}
			baseIngredientsFromCraft = extractBaseIngredientsFromTree(craftRecipeTree)
		}
	} else {
		craftErrMsg = "No recipe found for top-level item."
		dlog("  No recipe for %s, cannot calculate cost to craft.", itemNameNorm)
		craftSlowestFillTimePtr = float64Ptr(0.0)
		craftRecipeTree = &CraftingStepNode{
			ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, ErrorMessage: craftErrMsg,
		}
		cost, method, assocCost, rrVal, ifVal, deltaVal, c10mErr := calculateC10MForNode(itemNameNorm, quantity, apiResp, metricsMap)
		if c10mErr == nil {
			craftRecipeTree.Acquisition = &BaseIngredientDetail{
				Quantity: quantity, BestCost: float64Ptr(cost), AssociatedCost: float64Ptr(assocCost), Method: method,
				RR: float64Ptr(rrVal), IF: float64Ptr(ifVal), Delta: float64Ptr(deltaVal),
			}
		} else {
			craftRecipeTree.Acquisition = &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1))}
			if craftRecipeTree.ErrorMessage == "" {
				craftRecipeTree.ErrorMessage = c10mErr.Error()
			} else {
				craftRecipeTree.ErrorMessage += "; C10M Error: " + c10mErr.Error()
			}
		}
	}

	res1 := &result.PrimaryBased
	minCostP1 := math.Inf(1)
	chosenMethodP1 := "N/A"

	if craftPossible && !math.IsInf(costToCraftOptimal, 0) && costToCraftOptimal < minCostP1 {
		minCostP1 = costToCraftOptimal
		chosenMethodP1 = "Craft"
	}
	if validTopC10mPrim && topC10mPrim < minCostP1 {
		minCostP1 = topC10mPrim
		chosenMethodP1 = "Primary"
	}
	if validTopC10mSec && topC10mSec < minCostP1 {
		minCostP1 = topC10mSec
		chosenMethodP1 = "Secondary"
	}
	dlog("  P1 Minimum Cost Choice: %s (%.2f)", chosenMethodP1, minCostP1)

	if chosenMethodP1 == "Craft" {
		res1.TopLevelAction = "Expanded"
		res1.BaseIngredients = baseIngredientsFromCraft
		res1.RecipeTree = craftRecipeTree
		res1.TotalCost = float64Ptr(costToCraftOptimal)
		res1.CalculationPossible = craftPossible
		res1.FinalCostMethod = "SumBestC10MFromTree"
		res1.SlowestIngredientBuyTimeSeconds = craftSlowestFillTimePtr
		res1.SlowestIngredientName = craftSlowestIngName
		res1.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
		if !craftPossible && res1.ErrorMessage == "" {
			res1.ErrorMessage = craftErrMsg
		}
	} else if chosenMethodP1 == "Primary" || chosenMethodP1 == "Secondary" {
		res1.TopLevelAction = "TreatedAsBase"
		var acqCost, acqAssocCostRaw, acqRR, acqIF, acqDelta float64
		var acqMethod string
		var fillTimeForBase float64 = 0.0

		if chosenMethodP1 == "Primary" {
			acqCost, acqMethod, acqAssocCostRaw, acqRR, acqIF = topC10mPrim, "Primary", sellP*quantity, topRR, topIF
			acqDelta = math.NaN()
			if metricsP.ProductID != "" {
				acqDelta = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
			}

			if metricsP.ProductID != "" {
				fillTimeVal, _, errFill := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
				if errFill == nil && !math.IsNaN(fillTimeVal) && !math.IsInf(fillTimeVal, 0) && fillTimeVal >= 0 {
					fillTimeForBase = fillTimeVal
				} else {
					fillTimeForBase = math.Inf(1)
				}
			} else {
				fillTimeForBase = math.Inf(1)
			}
			res1.FinalCostMethod = "FixedTopLevelPrimary"
			res1.TotalCost = float64Ptr(topC10mPrim)
		} else { // Secondary
			acqCost, acqMethod, acqAssocCostRaw = topC10mSec, "Secondary", sellP*quantity // Instabuy uses sellP
			acqRR, acqIF = math.NaN(), math.NaN()
			acqDelta = math.NaN()
			if metricsP.ProductID != "" {
				acqDelta = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
			}
			res1.FinalCostMethod = "FixedTopLevelSecondary"
			res1.TotalCost = float64Ptr(topC10mSec)
		}

		currentBaseDetailP1 := BaseIngredientDetail{ // CORRECTED
			Quantity:       quantity,
			BestCost:       float64Ptr(acqCost),
			AssociatedCost: float64Ptr(acqAssocCostRaw),
			Method:         acqMethod,
			RR:             float64Ptr(acqRR),
			IF:             float64Ptr(acqIF),
			Delta:          float64Ptr(acqDelta),
		}
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP1}
		res1.RecipeTree = &CraftingStepNode{
			ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true,
			Acquisition: &currentBaseDetailP1,
		}
		res1.CalculationPossible = true
		res1.SlowestIngredientBuyTimeSeconds = float64Ptr(fillTimeForBase)
		res1.SlowestIngredientName = itemNameNorm
		res1.SlowestIngredientQuantity = sanitizeFloat(quantity)
	} else { // N/A
		res1.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res1.TotalCost = float64Ptr(math.Inf(1))
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "P1: No valid acquisition method found."
		}
		if topLevelRecipeExists && craftResultedInCycle {
			res1.ErrorMessage += " Crafting resulted in cycle."
		}
		if topLevelRecipeExists && !craftPossible && !craftResultedInCycle {
			res1.ErrorMessage += " Crafting failed (" + craftErrMsg + ")."
		}
		if !topLevelRecipeExists {
			res1.ErrorMessage += " No recipe and C10M methods invalid."
		}

		acqDeltaUnobtainable := math.NaN()
		if metricsP.ProductID != "" {
			acqDeltaUnobtainable = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		}
		res1.RecipeTree = &CraftingStepNode{
			ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, ErrorMessage: res1.ErrorMessage,
			Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1)), Delta: float64Ptr(acqDeltaUnobtainable)},
		}
		res1.SlowestIngredientBuyTimeSeconds = float64Ptr(math.Inf(1))
	}
	if chosenMethodP1 != "Craft" && craftResultedInCycle {
		msg := "Crafting path (not chosen) would result in cycle."
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = msg
		} else if !strings.Contains(res1.ErrorMessage, msg) {
			res1.ErrorMessage += "; " + msg
		}
	}

	// --- SecondaryBased Decision (res2) ---
	res2 := &result.SecondaryBased
	chosenMethodP2 := "N/A"
	if isApiNotFoundError {
		if topLevelRecipeExists {
			if craftPossible {
				chosenMethodP2 = "Craft"
			} else {
				chosenMethodP2 = "ExpansionFailed"
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = craftErrMsg
				}
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = "Item not on Bazaar and crafting failed/impossible."
				}
			}
		} else {
			chosenMethodP2 = "N/A"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "Item not on Bazaar and no recipe exists."
			}
		}
	} else {
		if craftPossible && validTopC10mPrim {
			if costToCraftOptimal <= topC10mPrim {
				chosenMethodP2 = "Craft"
			} else {
				chosenMethodP2 = "Primary"
			}
		} else if craftPossible {
			chosenMethodP2 = "Craft"
		} else if validTopC10mPrim {
			chosenMethodP2 = "Primary"
		} else if validTopC10mSec {
			chosenMethodP2 = "Secondary"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "Fell back to Secondary C10M; craft and Primary C10M not viable."
			}
			if craftErrMsg != "" && !craftResultedInCycle {
				res2.ErrorMessage += " Crafting failed: " + craftErrMsg + "."
			}
			if craftResultedInCycle {
				res2.ErrorMessage += " Crafting resulted in cycle."
			}
		} else {
			chosenMethodP2 = "N/A"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "P2: No valid acquisition method (Craft, Primary, or Secondary C10M)."
			}
			if craftErrMsg != "" {
				res2.ErrorMessage += " Crafting details: " + craftErrMsg + "."
			}
		}
	}
	dlog("  P2 Decision: %s", chosenMethodP2)

	if chosenMethodP2 == "Craft" {
		if craftResultedInCycle {
			res2.TopLevelAction = "TreatedAsBase (Due to Cycle)"
			cycleFallbackMsg := "Craft chosen for P2 but was cycle; attempting fallback."
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = cycleFallbackMsg
			} else {
				res2.ErrorMessage += "; " + cycleFallbackMsg
			}

			if validTopC10mPrim {
				res2.TotalCost = float64Ptr(topC10mPrim)
				res2.FinalCostMethod = "FixedTopLevelPrimary (Cycle Fallback)"
				acqDeltaP2CyclePrim := math.NaN()
				if metricsP.ProductID != "" {
					acqDeltaP2CyclePrim = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
				}
				// CORRECTED
				currentBaseDetailP2CyclePrim := BaseIngredientDetail{Quantity: quantity, BestCost: float64Ptr(topC10mPrim), AssociatedCost: float64Ptr(sellP * quantity), Method: "Primary", RR: float64Ptr(topRR), IF: float64Ptr(topIF), Delta: float64Ptr(acqDeltaP2CyclePrim)}
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP2CyclePrim}
				res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Acquisition: &currentBaseDetailP2CyclePrim, ErrorMessage: "Fell back to Primary C10M due to cycle"}
				res2.CalculationPossible = true
				fillTimeP2CyclePrim, _, _ := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
				res2.SlowestIngredientBuyTimeSeconds = float64Ptr(fillTimeP2CyclePrim)
				res2.SlowestIngredientName = itemNameNorm
				res2.SlowestIngredientQuantity = sanitizeFloat(quantity)
			} else if validTopC10mSec {
				res2.TotalCost = float64Ptr(topC10mSec)
				res2.FinalCostMethod = "FixedTopLevelSecondary (Cycle Fallback)"
				acqDeltaP2CycleSec := math.NaN()
				if metricsP.ProductID != "" {
					acqDeltaP2CycleSec = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
				}
				// CORRECTED
				currentBaseDetailP2CycleSec := BaseIngredientDetail{Quantity: quantity, BestCost: float64Ptr(topC10mSec), AssociatedCost: float64Ptr(sellP * quantity), Method: "Secondary", Delta: float64Ptr(acqDeltaP2CycleSec)} // Instabuy uses sellP
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP2CycleSec}
				res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Acquisition: &currentBaseDetailP2CycleSec, ErrorMessage: "Fell back to Secondary C10M due to cycle"}
				res2.CalculationPossible = true
				res2.SlowestIngredientBuyTimeSeconds = float64Ptr(0.0)
			} else {
				res2.TotalCost = float64Ptr(math.Inf(1))
				res2.FinalCostMethod = "N/A (Cycle Fallback Failed)"
				res2.CalculationPossible = false
				res2.RecipeTree = craftRecipeTree
				if res2.RecipeTree != nil {
					if res2.RecipeTree.ErrorMessage == "" {
						res2.RecipeTree.ErrorMessage = "No C10M fallback from cycle."
					} else {
						res2.RecipeTree.ErrorMessage += "; No C10M fallback from cycle."
					}
				} else {
					res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, ErrorMessage: "Cycle and no tree", IsBaseComponent: true, Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1))}}
				}
			}
		} else { // Normal Craft for P2
			res2.TopLevelAction = "Expanded"
			res2.BaseIngredients = baseIngredientsFromCraft
			res2.RecipeTree = craftRecipeTree
			res2.TotalCost = float64Ptr(costToCraftOptimal)
			res2.CalculationPossible = craftPossible
			res2.FinalCostMethod = "SumBestC10MFromTree"
			res2.SlowestIngredientBuyTimeSeconds = craftSlowestFillTimePtr
			res2.SlowestIngredientName = craftSlowestIngName
			res2.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
			if !craftPossible && res2.ErrorMessage == "" {
				res2.ErrorMessage = craftErrMsg
			}
		}
	} else if chosenMethodP2 == "Primary" {
		res2.TopLevelAction = "TreatedAsBase"
		res2.TotalCost = float64Ptr(topC10mPrim)
		res2.FinalCostMethod = "FixedTopLevelPrimary"
		acqDeltaP2Prim := math.NaN()
		if metricsP.ProductID != "" {
			acqDeltaP2Prim = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		}
		// CORRECTED
		currentBaseDetailP2PrimMethod := BaseIngredientDetail{
			Quantity: quantity, BestCost: float64Ptr(topC10mPrim), AssociatedCost: float64Ptr(sellP * quantity), Method: "Primary",
			RR: float64Ptr(topRR), IF: float64Ptr(topIF), Delta: float64Ptr(acqDeltaP2Prim),
		}
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP2PrimMethod}
		res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Acquisition: &currentBaseDetailP2PrimMethod}
		res2.CalculationPossible = true
		fillTimeP2Prim, _, _ := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
		res2.SlowestIngredientBuyTimeSeconds = float64Ptr(fillTimeP2Prim)
		res2.SlowestIngredientName = itemNameNorm
		res2.SlowestIngredientQuantity = sanitizeFloat(quantity)
	} else if chosenMethodP2 == "Secondary" {
		res2.TopLevelAction = "TreatedAsBase"
		res2.TotalCost = float64Ptr(topC10mSec)
		res2.FinalCostMethod = "FixedTopLevelSecondary"
		acqDeltaP2Sec := math.NaN()
		if metricsP.ProductID != "" {
			acqDeltaP2Sec = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		}
		// CORRECTED
		currentBaseDetailP2SecMethod := BaseIngredientDetail{
			Quantity: quantity, BestCost: float64Ptr(topC10mSec), AssociatedCost: float64Ptr(sellP * quantity), Method: "Secondary", // Instabuy uses sellP
			Delta: float64Ptr(acqDeltaP2Sec),
		}
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: currentBaseDetailP2SecMethod}
		res2.RecipeTree = &CraftingStepNode{ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, Acquisition: &currentBaseDetailP2SecMethod}
		res2.CalculationPossible = true
		res2.SlowestIngredientBuyTimeSeconds = float64Ptr(0.0)
	} else { // N/A or ExpansionFailed
		res2.TopLevelAction = chosenMethodP2
		res2.TotalCost = float64Ptr(math.Inf(1))
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
		if res2.ErrorMessage == "" {
			if chosenMethodP2 == "ExpansionFailed" {
				res2.ErrorMessage = craftErrMsg
			} else {
				res2.ErrorMessage = "P2: No valid acquisition method."
			}
		}
		if craftRecipeTree != nil && chosenMethodP2 == "ExpansionFailed" {
			res2.RecipeTree = craftRecipeTree
		} else {
			acqDeltaP2NA := math.NaN()
			if metricsP.ProductID != "" {
				acqDeltaP2NA = metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
			}
			res2.RecipeTree = &CraftingStepNode{
				ItemName: itemNameNorm, QuantityNeeded: quantity, IsBaseComponent: true, ErrorMessage: res2.ErrorMessage,
				Acquisition: &BaseIngredientDetail{Quantity: quantity, Method: "N/A", BestCost: float64Ptr(math.Inf(1)), Delta: float64Ptr(acqDeltaP2NA)},
			}
		}
		res2.SlowestIngredientBuyTimeSeconds = float64Ptr(math.Inf(1))
		if chosenMethodP2 == "ExpansionFailed" {
			res2.SlowestIngredientBuyTimeSeconds = craftSlowestFillTimePtr
			res2.SlowestIngredientName = craftSlowestIngName
			res2.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
		}
	}

	result.PrimaryBased.SlowestIngredientQuantity = sanitizeFloat(result.PrimaryBased.SlowestIngredientQuantity)
	result.SecondaryBased.SlowestIngredientQuantity = sanitizeFloat(result.SecondaryBased.SlowestIngredientQuantity)

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
