package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// --- Structs ---
type BaseIngredientDetail struct {
	Quantity       float64  `json:"quantity"`
	BestCost       float64  `json:"best_cost"`
	AssociatedCost float64  `json:"associated_cost"`
	Method         string   `json:"method"`
	RR             *float64 `json:"rr,omitempty"`
	IF             *float64 `json:"if,omitempty"` // Added IF field
}
type ExpansionResult struct {
	BaseIngredients     map[string]BaseIngredientDetail `json:"base_ingredients"`
	TotalCost           float64                         `json:"total_cost"`
	PerspectiveType     string                          `json:"perspective_type"`
	TopLevelAction      string                          `json:"top_level_action"`
	FinalCostMethod     string                          `json:"final_cost_method"`
	CalculationPossible bool                            `json:"calculation_possible"`
	ErrorMessage        string                          `json:"error_message,omitempty"`
	TopLevelCost        float64                         `json:"top_level_cost"`
	TopLevelRR          *float64                        `json:"top_level_rr,omitempty"`
	// Fields for slowest ingredient fill time (buy orders)
	SlowestIngredientBuyTimeSeconds float64 `json:"slowest_ingredient_buy_time_seconds"`
	SlowestIngredientName           string  `json:"slowest_ingredient_name,omitempty"`
	SlowestIngredientQuantity       float64 `json:"slowest_ingredient_quantity"`
}
type DualExpansionResult struct {
	ItemName                     string          `json:"item_name"`
	Quantity                     float64         `json:"quantity"`
	PrimaryBased                 ExpansionResult `json:"primary_based"`
	SecondaryBased               ExpansionResult `json:"secondary_based"`
	TopLevelInstasellTimeSeconds float64         `json:"top_level_instasell_time_seconds"`
}

// Helper
func float64Ptr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) { // Handles positive and negative infinity
		return nil
	}
	return &v
}

// --- Cost and Fill Time Helper ---
func calculateDetailedCostsAndFillTimes(
	baseMapInput map[string]float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (
	totalSumOfBestCosts float64,
	detailedMapOutput map[string]BaseIngredientDetail,
	slowestFillTimeSecs float64,
	slowestIngName string,
	slowestIngQty float64,
	isPossible bool,
	errorMsg string,
) {
	totalSumOfBestCosts = 0.0
	isPossible = true
	errorMsg = ""
	detailedMapOutput = make(map[string]BaseIngredientDetail)

	slowestFillTimeSecs = 0.0
	slowestIngName = ""
	slowestIngQty = 0.0

	if len(baseMapInput) == 0 {
		return 0.0, detailedMapOutput, 0.0, "", 0.0, true, ""
	}

	for itemID, quantity := range baseMapInput {
		if quantity <= 0 {
			continue
		}
		// getBestC10M now returns IF as the 5th value
		bestCost, method, assocCost, rr, ifVal, errC10M := getBestC10M(itemID, quantity, apiResp, metricsMap)
		ingredientDetail := BaseIngredientDetail{Quantity: quantity, BestCost: 0.0, AssociatedCost: 0.0, Method: "N/A", RR: nil, IF: nil}

		if errC10M != nil || method == "N/A" || math.IsInf(bestCost, 0) || math.IsNaN(bestCost) || bestCost < 0 {
			errorMsg = fmt.Sprintf("Cannot determine valid BEST cost for base ingredient '%s': BestC:%.2f, Method: %s, Err: %v", itemID, bestCost, method, errC10M)
			dlog("  WARN (calculateDetailedCostsAndFillTimes - Best): %s", errorMsg)
			isPossible = false
			totalSumOfBestCosts = math.Inf(1)
			detailedMapOutput[itemID] = ingredientDetail
			break
		} else {
			totalSumOfBestCosts += bestCost
			ingredientDetail.BestCost = sanitizeFloat(bestCost)
			ingredientDetail.AssociatedCost = sanitizeFloat(assocCost)
			ingredientDetail.Method = method
			if method == "Primary" { // IF and RR are primarily relevant for the "Primary" C10M path
				ingredientDetail.RR = float64Ptr(rr)
				ingredientDetail.IF = float64Ptr(ifVal) // Store IF
			}
			detailedMapOutput[itemID] = ingredientDetail
		}

		metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
		if metricsOk {
			buyTime, _, buyErr := calculateBuyOrderFillTime(itemID, quantity, metricsData)
			if buyErr == nil && !math.IsNaN(buyTime) && !math.IsInf(buyTime, -1) && buyTime >= 0 {
				if math.IsInf(buyTime, 1) {
					if !math.IsInf(slowestFillTimeSecs, 1) || slowestFillTimeSecs < buyTime {
						slowestFillTimeSecs = buyTime
						slowestIngName = itemID
						slowestIngQty = quantity
					}
				} else if !math.IsInf(slowestFillTimeSecs, 1) && buyTime > slowestFillTimeSecs {
					slowestFillTimeSecs = buyTime
					slowestIngName = itemID
					slowestIngQty = quantity
				}
			} else {
				dlog("  WARN (calculateDetailedCostsAndFillTimes - FillTime): Could not get valid fill time for %s (Value: %.2f, Err: %v)", itemID, buyTime, buyErr)
				if !math.IsInf(slowestFillTimeSecs, 1) {
					slowestFillTimeSecs = math.Inf(1)
					slowestIngName = itemID
					slowestIngQty = quantity
					dlog("    Setting slowestFillTime to Inf due to invalid fill time for %s", itemID)
				}
			}
		} else {
			dlog("  WARN (calculateDetailedCostsAndFillTimes - FillTime): Metrics not found for %s, cannot calculate fill time.", itemID)
			if !math.IsInf(slowestFillTimeSecs, 1) {
				slowestFillTimeSecs = math.Inf(1)
				slowestIngName = itemID
				slowestIngQty = quantity
				dlog("    Setting slowestFillTime to Inf due to missing metrics for %s", itemID)
			}
		}
	}

	if !isPossible {
		return sanitizeFloat(totalSumOfBestCosts), detailedMapOutput, sanitizeFloat(slowestFillTimeSecs), slowestIngName, sanitizeFloat(slowestIngQty), false, errorMsg
	}

	return sanitizeFloat(totalSumOfBestCosts), detailedMapOutput, sanitizeFloat(slowestFillTimeSecs), slowestIngName, sanitizeFloat(slowestIngQty), true, ""
}

// --- Orchestration Function (UPDATED P2 LOGIC) ---
func PerformDualExpansion(itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string) (*DualExpansionResult, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s <<<", quantity, itemNameNorm)
	result := &DualExpansionResult{
		ItemName: itemNameNorm,
		Quantity: quantity,
		PrimaryBased: ExpansionResult{
			PerspectiveType:                 "PrimaryBased",
			CalculationPossible:             false,
			BaseIngredients:                 make(map[string]BaseIngredientDetail),
			TotalCost:                       0.0,
			TopLevelCost:                    0.0,
			TopLevelRR:                      nil,
			SlowestIngredientBuyTimeSeconds: 0.0,
			SlowestIngredientName:           "",
			SlowestIngredientQuantity:       0.0,
		},
		SecondaryBased: ExpansionResult{
			PerspectiveType:                 "SecondaryBased",
			CalculationPossible:             false,
			BaseIngredients:                 make(map[string]BaseIngredientDetail),
			TotalCost:                       0.0,
			TopLevelCost:                    0.0,
			TopLevelRR:                      nil,
			SlowestIngredientBuyTimeSeconds: 0.0,
			SlowestIngredientName:           "",
			SlowestIngredientQuantity:       0.0,
		},
		TopLevelInstasellTimeSeconds: 0.0,
	}

	topLevelProductData, topProductOK := safeGetProductData(apiResp, itemNameNorm)
	if topProductOK {
		instaSellTime, errInstaSell := calculateInstasellFillTime(quantity, topLevelProductData)
		if errInstaSell == nil && !math.IsNaN(instaSellTime) && !math.IsInf(instaSellTime, 0) && instaSellTime >= 0 {
			result.TopLevelInstasellTimeSeconds = sanitizeFloat(instaSellTime)
		} else {
			dlog("  WARN: Could not calculate valid TopLevelInstasellTime for %s (Time: %.2f, Err: %v)", itemNameNorm, instaSellTime, errInstaSell)
			result.TopLevelInstasellTimeSeconds = sanitizeFloat(instaSellTime)
		}
	} else {
		dlog("  WARN: Product data not found for %s, cannot calculate TopLevelInstasellTime.", itemNameNorm)
		result.TopLevelInstasellTimeSeconds = 0.0
	}

	sellP := getSellPrice(apiResp, itemNameNorm)
	buyP := getBuyPrice(apiResp, itemNameNorm)
	metricsP := getMetrics(metricsMap, itemNameNorm)

	// calculateC10MInternal returns: c10mPrimary, c10mSecondary, ifValue, rrValue, deltaRatio, adjustment, err
	topC10mPrim, topC10mSec, topIF, topRR, _, _, errTopC10M := calculateC10MInternal( // Capture topIF
		itemNameNorm, quantity, sellP, buyP, metricsP,
	)

	result.PrimaryBased.TopLevelCost = sanitizeFloat(topC10mPrim)
	result.SecondaryBased.TopLevelCost = sanitizeFloat(topC10mSec)
	validTopC10mPrim := errTopC10M == nil && !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0
	if validTopC10mPrim {
		result.PrimaryBased.TopLevelRR = float64Ptr(topRR)
		// If Perspective 1 treats item as base using Primary, its IF would be topIF
		// This will be handled when populating BaseIngredients for res1.
	} else {
		result.PrimaryBased.TopLevelRR = nil
	}
	result.SecondaryBased.TopLevelRR = nil

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
		result.PrimaryBased.TopLevelAction = "Unknown"
		result.SecondaryBased.TopLevelAction = "Unknown"
		result.PrimaryBased.TotalCost = sanitizeFloat(result.PrimaryBased.TotalCost)
		result.SecondaryBased.TotalCost = sanitizeFloat(result.SecondaryBased.TotalCost)
		result.PrimaryBased.SlowestIngredientBuyTimeSeconds = sanitizeFloat(result.PrimaryBased.SlowestIngredientBuyTimeSeconds)
		result.SecondaryBased.SlowestIngredientBuyTimeSeconds = sanitizeFloat(result.SecondaryBased.SlowestIngredientBuyTimeSeconds)
		result.PrimaryBased.SlowestIngredientQuantity = sanitizeFloat(result.PrimaryBased.SlowestIngredientQuantity)
		result.SecondaryBased.SlowestIngredientQuantity = sanitizeFloat(result.SecondaryBased.SlowestIngredientQuantity)
		result.TopLevelInstasellTimeSeconds = sanitizeFloat(result.TopLevelInstasellTimeSeconds)
		return result, nil
	}

	dlog("  Top-Level C10M: Primary=%.2f (IF=%.2f, RR=%.2f), Secondary=%.2f. Recipe Exists: %v. Error: %v", topC10mPrim, topIF, topRR, topC10mSec, topLevelRecipeExists, errTopC10M)
	isApiNotFoundError := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")
	validTopC10mSec := errTopC10M == nil && !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0

	var costToCraftOptimal float64 = math.Inf(1)
	var baseIngredientsFromCraft map[string]BaseIngredientDetail
	var craftPossible bool = false
	var craftErrMsg string = ""
	var craftResultedInCycle bool = false
	var craftSlowestFillTime float64 = 0.0
	var craftSlowestIngName string = ""
	var craftSlowestIngQty float64 = 0.0

	if topLevelRecipeExists {
		baseMapQtyOnly, errExpand := ExpandItem(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)
		if errExpand != nil {
			craftErrMsg = fmt.Sprintf("Expansion failed critically: %v", errExpand)
			log.Printf("WARN (PerformDualExpansion): ExpandItem failed for %s: %v", itemNameNorm, errExpand)
		} else if len(baseMapQtyOnly) == 0 {
			craftErrMsg = "Expansion resulted in empty map (likely top-level item cycles to itself or recipe is empty)"
			craftResultedInCycle = true
			costToCraftOptimal = math.Inf(1)
			baseIngredientsFromCraft = make(map[string]BaseIngredientDetail)
		} else {
			var detailedMapTemp map[string]BaseIngredientDetail
			costToCraftOptimal, detailedMapTemp, craftSlowestFillTime, craftSlowestIngName, craftSlowestIngQty, craftPossible, craftErrMsg =
				calculateDetailedCostsAndFillTimes(baseMapQtyOnly, apiResp, metricsMap)

			if craftPossible {
				baseIngredientsFromCraft = detailedMapTemp
				dlog("  Cost to Craft (Optimal Ingredients) for %s: %.2f. Slowest Ing: %s (Qty: %.2f, Time: %.2fs)", itemNameNorm, costToCraftOptimal, craftSlowestIngName, craftSlowestIngQty, craftSlowestFillTime)
			} else {
				dlog("  Failed to calculate detailed costs/filltimes for crafting %s: %s", itemNameNorm, craftErrMsg)
				if detailedMapTemp != nil {
					baseIngredientsFromCraft = detailedMapTemp
				}
			}
		}
	} else {
		craftErrMsg = "No recipe found for top-level item."
		dlog("  No recipe for %s, cannot calculate cost to craft.", itemNameNorm)
	}

	dlog("--- Running Perspective 1 (Benchmark: Absolute Minimum Cost) ---")
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
		res1.TotalCost = costToCraftOptimal
		res1.CalculationPossible = true
		res1.FinalCostMethod = "SumBestC10M"
		res1.SlowestIngredientBuyTimeSeconds = craftSlowestFillTime
		res1.SlowestIngredientName = craftSlowestIngName
		res1.SlowestIngredientQuantity = craftSlowestIngQty
	} else if chosenMethodP1 == "Primary" {
		res1.TopLevelAction = "TreatedAsBase"
		res1.BaseIngredients = map[string]BaseIngredientDetail{
			itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(sellP * quantity), Method: "Primary", RR: float64Ptr(topRR), IF: float64Ptr(topIF)}, // Add IF here
		}
		res1.TotalCost = sanitizeFloat(topC10mPrim)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelPrimary"
		res1.SlowestIngredientBuyTimeSeconds = 0.0
		res1.SlowestIngredientName = ""
		res1.SlowestIngredientQuantity = 0.0
	} else if chosenMethodP1 == "Secondary" {
		res1.TopLevelAction = "TreatedAsBase"
		res1.BaseIngredients = map[string]BaseIngredientDetail{
			itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(buyP * quantity), Method: "Secondary", RR: nil, IF: nil}, // IF not applicable
		}
		res1.TotalCost = sanitizeFloat(topC10mSec)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelSecondary"
		res1.SlowestIngredientBuyTimeSeconds = 0.0
		res1.SlowestIngredientName = ""
		res1.SlowestIngredientQuantity = 0.0
	} else {
		res1.TopLevelAction = "TreatedAsBase (Unobtainable)"
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "P1: No valid acquisition method found."
			if topLevelRecipeExists && craftResultedInCycle {
				res1.ErrorMessage += " Crafting resulted in cycle (" + craftErrMsg + ")."
			} else if topLevelRecipeExists && !craftPossible {
				res1.ErrorMessage += " Crafting failed (" + craftErrMsg + ")."
			} else if !topLevelRecipeExists {
				res1.ErrorMessage += " No recipe and C10M methods invalid/unavailable."
			}
		}
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		res1.TotalCost = 0.0
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
		res1.SlowestIngredientBuyTimeSeconds = 0.0
		res1.SlowestIngredientName = ""
		res1.SlowestIngredientQuantity = 0.0
	}
	if chosenMethodP1 != "Craft" && craftResultedInCycle {
		msgToAdd := "Crafting path resulted in cycle."
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = msgToAdd
		} else if !strings.Contains(res1.ErrorMessage, msgToAdd) {
			res1.ErrorMessage += "; " + msgToAdd
		}
	}

	dlog("--- Running Perspective 2 (Benchmark: Primary C10M first, then Craft) ---")
	res2 := &result.SecondaryBased
	chosenMethodP2 := "N/A"

	if isApiNotFoundError {
		if topLevelRecipeExists {
			if craftPossible {
				dlog("  P2 Decision: Expand (Not on Bazaar, recipe & craft possible)")
				chosenMethodP2 = "Craft"
			} else {
				dlog("  P2 Decision: ExpansionFailed (Not on Bazaar, recipe exists, craft calculation failed. Msg: %s)", craftErrMsg)
				chosenMethodP2 = "ExpansionFailed"
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = craftErrMsg
				} else if craftErrMsg != "" && !strings.Contains(res2.ErrorMessage, craftErrMsg) {
					res2.ErrorMessage += "; " + craftErrMsg
				}
			}
		} else {
			dlog("  P2 Decision: TreatedAsBase (Unobtainable - Not on Bazaar, no recipe)")
			chosenMethodP2 = "N/A"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "Item not on Bazaar and no recipe exists."
			}
		}
	} else if validTopC10mPrim {
		if !craftPossible || topC10mPrim <= costToCraftOptimal {
			dlog("  P2 Decision: TreatedAsBase (Primary C10M %.2f is valid and preferred/only option vs Craft %.2f (craftPossible: %v))", topC10mPrim, costToCraftOptimal, craftPossible)
			chosenMethodP2 = "Primary"
		} else {
			dlog("  P2 Decision: Expand (Craft cost %.2f is better than Primary C10M %.2f, and craft is possible)", costToCraftOptimal, topC10mPrim)
			chosenMethodP2 = "Craft"
		}
	} else {
		if craftPossible {
			dlog("  P2 Decision: Expand (Primary C10M invalid/unavailable, Crafting possible with cost %.2f)", costToCraftOptimal)
			chosenMethodP2 = "Craft"
		} else {
			dlog("  P2 Decision: Fallback (Primary C10M invalid, Crafting not possible/failed. CraftMsg: %s)", craftErrMsg)
			if validTopC10mSec {
				dlog("  P2 Fallback: Using Secondary C10M %.2f", topC10mSec)
				chosenMethodP2 = "Secondary"
			} else {
				dlog("  P2 Fallback: Secondary C10M also invalid. Unobtainable.")
				chosenMethodP2 = "N/A"
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = "P2: Primary C10M invalid, Crafting not possible/failed (" + craftErrMsg + "), and Secondary C10M invalid."
				}
			}
		}
	}

	if chosenMethodP2 == "Craft" {
		if craftResultedInCycle {
			dlog("  P2 Adjustment: Craft was chosen but resulted in top-level cycle. Attempting fallback.")
			res2.TopLevelAction = "TreatedAsBase (Due to Cycle)"
			cycleMsg := "Crafting path resulted in top-level cycle."
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = cycleMsg
			} else if !strings.Contains(res2.ErrorMessage, cycleMsg) {
				res2.ErrorMessage += "; " + cycleMsg
			}

			if validTopC10mPrim {
				dlog("    P2 Cycle Fallback: Using Primary C10M %.2f", topC10mPrim)
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(sellP * quantity), Method: "Primary", RR: float64Ptr(topRR), IF: float64Ptr(topIF)}}
				res2.TotalCost = sanitizeFloat(topC10mPrim)
				res2.CalculationPossible = true
				res2.FinalCostMethod = "FixedTopLevelPrimary"
				res2.SlowestIngredientBuyTimeSeconds = 0.0
				res2.SlowestIngredientName = ""
				res2.SlowestIngredientQuantity = 0.0
			} else if validTopC10mSec {
				dlog("    P2 Cycle Fallback: Primary invalid, using Secondary C10M %.2f", topC10mSec)
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(buyP * quantity), Method: "Secondary", RR: nil, IF: nil}}
				res2.TotalCost = sanitizeFloat(topC10mSec)
				res2.CalculationPossible = true
				res2.FinalCostMethod = "FixedTopLevelSecondary"
				res2.SlowestIngredientBuyTimeSeconds = 0.0
				res2.SlowestIngredientName = ""
				res2.SlowestIngredientQuantity = 0.0
			} else {
				dlog("    P2 Cycle Fallback: Primary and Secondary C10M also invalid. Unobtainable.")
				res2.TotalCost = 0.0
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A (Cycle Fallback Failed)"}}
				res2.CalculationPossible = false
				res2.FinalCostMethod = "N/A"
				noFallbackMsg := "No valid C10M fallback for top-level item after cycle."
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = noFallbackMsg
				} else if !strings.Contains(res2.ErrorMessage, noFallbackMsg) {
					res2.ErrorMessage += "; " + noFallbackMsg
				}
				res2.SlowestIngredientBuyTimeSeconds = 0.0
				res2.SlowestIngredientName = ""
				res2.SlowestIngredientQuantity = 0.0
			}
		} else { // Craft chosen, and no top-level cycle
			res2.TopLevelAction = "Expanded"
			res2.BaseIngredients = baseIngredientsFromCraft
			res2.TotalCost = costToCraftOptimal
			res2.CalculationPossible = craftPossible
			res2.FinalCostMethod = "SumBestC10M"
			res2.SlowestIngredientBuyTimeSeconds = craftSlowestFillTime
			res2.SlowestIngredientName = craftSlowestIngName
			res2.SlowestIngredientQuantity = craftSlowestIngQty
			if !craftPossible {
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = craftErrMsg
				} else if craftErrMsg != "" && !strings.Contains(res2.ErrorMessage, craftErrMsg) {
					res2.ErrorMessage += "; Craft Error: " + craftErrMsg
				}
			}
		}
	} else if chosenMethodP2 == "Primary" {
		res2.TopLevelAction = "TreatedAsBase"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(sellP * quantity), Method: "Primary", RR: float64Ptr(topRR), IF: float64Ptr(topIF)}}
		res2.TotalCost = sanitizeFloat(topC10mPrim)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelPrimary"
		res2.SlowestIngredientBuyTimeSeconds = 0.0
		res2.SlowestIngredientName = ""
		res2.SlowestIngredientQuantity = 0.0
	} else if chosenMethodP2 == "Secondary" {
		res2.TopLevelAction = "TreatedAsBase"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(buyP * quantity), Method: "Secondary", RR: nil, IF: nil}}
		res2.TotalCost = sanitizeFloat(topC10mSec)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelSecondary"
		fallbackMsg := "Fell back to Secondary C10M cost."
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = fallbackMsg
		} else if !strings.Contains(res2.ErrorMessage, fallbackMsg) {
			res2.ErrorMessage += "; " + fallbackMsg
		}
		res2.SlowestIngredientBuyTimeSeconds = 0.0
		res2.SlowestIngredientName = ""
		res2.SlowestIngredientQuantity = 0.0
	} else if chosenMethodP2 == "ExpansionFailed" {
		res2.TopLevelAction = "ExpansionFailed"
		if baseIngredientsFromCraft != nil {
			res2.BaseIngredients = baseIngredientsFromCraft
		} else {
			res2.BaseIngredients = make(map[string]BaseIngredientDetail)
		}
		if res2.ErrorMessage == "" && craftErrMsg != "" {
			res2.ErrorMessage = craftErrMsg
		}

		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
		res2.SlowestIngredientBuyTimeSeconds = craftSlowestFillTime
		res2.SlowestIngredientName = craftSlowestIngName
		res2.SlowestIngredientQuantity = craftSlowestIngQty
	} else { // N/A or Unobtainable for P2
		res2.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = "P2: No valid acquisition method found."
		}
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
		res2.SlowestIngredientBuyTimeSeconds = 0.0
		res2.SlowestIngredientName = ""
		res2.SlowestIngredientQuantity = 0.0
	}

	result.PrimaryBased.TotalCost = sanitizeFloat(result.PrimaryBased.TotalCost)
	result.PrimaryBased.TopLevelCost = sanitizeFloat(result.PrimaryBased.TopLevelCost)
	result.PrimaryBased.SlowestIngredientBuyTimeSeconds = sanitizeFloat(result.PrimaryBased.SlowestIngredientBuyTimeSeconds)
	result.PrimaryBased.SlowestIngredientQuantity = sanitizeFloat(result.PrimaryBased.SlowestIngredientQuantity)

	result.SecondaryBased.TotalCost = sanitizeFloat(result.SecondaryBased.TotalCost)
	result.SecondaryBased.TopLevelCost = sanitizeFloat(result.SecondaryBased.TopLevelCost)
	result.SecondaryBased.SlowestIngredientBuyTimeSeconds = sanitizeFloat(result.SecondaryBased.SlowestIngredientBuyTimeSeconds)
	result.SecondaryBased.SlowestIngredientQuantity = sanitizeFloat(result.SecondaryBased.SlowestIngredientQuantity)

	result.TopLevelInstasellTimeSeconds = sanitizeFloat(result.TopLevelInstasellTimeSeconds)

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
