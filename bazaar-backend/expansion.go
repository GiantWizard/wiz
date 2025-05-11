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
	BestCost       float64  `json:"best_cost"`       // Sanitized
	AssociatedCost float64  `json:"associated_cost"` // Sanitized
	Method         string   `json:"method"`
	RR             *float64 `json:"rr,omitempty"`
	IF             *float64 `json:"if,omitempty"`
	Delta          *float64 `json:"delta,omitempty"` // Added Delta field
}
type ExpansionResult struct {
	BaseIngredients                 map[string]BaseIngredientDetail `json:"base_ingredients"`
	TotalCost                       float64                         `json:"total_cost"` // Sanitized
	PerspectiveType                 string                          `json:"perspective_type"`
	TopLevelAction                  string                          `json:"top_level_action"`
	FinalCostMethod                 string                          `json:"final_cost_method"`
	CalculationPossible             bool                            `json:"calculation_possible"`
	ErrorMessage                    string                          `json:"error_message,omitempty"`
	TopLevelCost                    float64                         `json:"top_level_cost"` // Sanitized
	TopLevelRR                      *float64                        `json:"top_level_rr,omitempty"`
	SlowestIngredientBuyTimeSeconds *float64                        `json:"slowest_ingredient_buy_time_seconds,omitempty"`
	SlowestIngredientName           string                          `json:"slowest_ingredient_name,omitempty"`
	SlowestIngredientQuantity       float64                         `json:"slowest_ingredient_quantity"` // Sanitized
}
type DualExpansionResult struct {
	ItemName                     string          `json:"item_name"`
	Quantity                     float64         `json:"quantity"`
	PrimaryBased                 ExpansionResult `json:"primary_based"`
	SecondaryBased               ExpansionResult `json:"secondary_based"`
	TopLevelInstasellTimeSeconds *float64        `json:"top_level_instasell_time_seconds,omitempty"`
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
		ingredientDetail := BaseIngredientDetail{Quantity: quantity, BestCost: 0.0, AssociatedCost: 0.0, Method: "N/A", RR: nil, IF: nil, Delta: nil}

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
			if method == "Primary" {
				ingredientDetail.RR = float64Ptr(rr)
				ingredientDetail.IF = float64Ptr(ifVal)
			} else {
				ingredientDetail.RR = nil
				ingredientDetail.IF = nil
			}

			// Calculate and store Delta for this ingredient
			metricsDataForDelta, metricsOkForDelta := safeGetMetricsData(metricsMap, itemID)
			if metricsOkForDelta {
				deltaVal := metricsDataForDelta.SellSize*metricsDataForDelta.SellFrequency - metricsDataForDelta.OrderSize*metricsDataForDelta.OrderFrequency
				ingredientDetail.Delta = float64Ptr(deltaVal)
			}
			detailedMapOutput[itemID] = ingredientDetail
		}

		metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID)
		if metricsOk {
			buyTime, _, buyErr := calculateBuyOrderFillTime(itemID, quantity, metricsData)
			if buyErr == nil && !math.IsNaN(buyTime) && !math.IsInf(buyTime, -1) && buyTime >= 0 {
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
			} else {
				dlog("  WARN (calculateDetailedCostsAndFillTimes - FillTime): Could not get valid fill time for %s (Value: %.2f, Err: %v)", itemID, buyTime, buyErr)
				if !math.IsInf(currentSlowestTime, 1) {
					currentSlowestTime = math.Inf(1)
					slowestIngName = itemID
					slowestIngQty = quantity
					dlog("    Setting slowestFillTime to Inf due to invalid fill time for %s", itemID)
				}
			}
		} else {
			dlog("  WARN (calculateDetailedCostsAndFillTimes - FillTime): Metrics not found for %s, cannot calculate fill time.", itemID)
			if !math.IsInf(currentSlowestTime, 1) {
				currentSlowestTime = math.Inf(1)
				slowestIngName = itemID
				slowestIngQty = quantity
				dlog("    Setting slowestFillTime to Inf due to missing metrics for %s", itemID)
			}
		}
	}

	slowestFillTimeSecs = float64Ptr(currentSlowestTime)

	if !isPossible {
		return sanitizeFloat(totalSumOfBestCosts), detailedMapOutput, slowestFillTimeSecs, slowestIngName, sanitizeFloat(slowestIngQty), false, errorMsg
	}
	return sanitizeFloat(totalSumOfBestCosts), detailedMapOutput, slowestFillTimeSecs, slowestIngName, sanitizeFloat(slowestIngQty), true, ""
}

func PerformDualExpansion(itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string) (*DualExpansionResult, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s <<<", quantity, itemNameNorm)
	result := &DualExpansionResult{
		ItemName: itemNameNorm,
		Quantity: sanitizeFloat(quantity),
		PrimaryBased: ExpansionResult{
			PerspectiveType: "PrimaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail),
			TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil,
			SlowestIngredientBuyTimeSeconds: float64Ptr(0.0), SlowestIngredientName: "", SlowestIngredientQuantity: 0.0,
		},
		SecondaryBased: ExpansionResult{
			PerspectiveType: "SecondaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail),
			TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil,
			SlowestIngredientBuyTimeSeconds: float64Ptr(0.0), SlowestIngredientName: "", SlowestIngredientQuantity: 0.0,
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

	result.PrimaryBased.TopLevelCost = sanitizeFloat(topC10mPrim)
	result.SecondaryBased.TopLevelCost = sanitizeFloat(topC10mSec)
	validTopC10mPrim := errTopC10M == nil && !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0
	if validTopC10mPrim {
		result.PrimaryBased.TopLevelRR = float64Ptr(topRR)
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
		result.PrimaryBased.SlowestIngredientQuantity = sanitizeFloat(result.PrimaryBased.SlowestIngredientQuantity)
		result.SecondaryBased.SlowestIngredientQuantity = sanitizeFloat(result.SecondaryBased.SlowestIngredientQuantity)
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
	var craftSlowestFillTimePtr *float64
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
			craftSlowestFillTimePtr = float64Ptr(math.Inf(1))
		} else {
			var detailedMapTemp map[string]BaseIngredientDetail
			costToCraftOptimal, detailedMapTemp, craftSlowestFillTimePtr, craftSlowestIngName, craftSlowestIngQty, craftPossible, craftErrMsg =
				calculateDetailedCostsAndFillTimes(baseMapQtyOnly, apiResp, metricsMap)
			if craftPossible {
				baseIngredientsFromCraft = detailedMapTemp
				dlog("  Cost to Craft (Optimal Ingredients) for %s: %.2f. Slowest Ing: %s (Qty: %.2f, TimePtr: %v)", itemNameNorm, costToCraftOptimal, craftSlowestIngName, craftSlowestIngQty, craftSlowestFillTimePtr)
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
		craftSlowestFillTimePtr = float64Ptr(0.0)
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
		res1.TotalCost = costToCraftOptimal
		res1.CalculationPossible = true
		res1.FinalCostMethod = "SumBestC10M"
		res1.SlowestIngredientBuyTimeSeconds = craftSlowestFillTimePtr
		res1.SlowestIngredientName = craftSlowestIngName
		res1.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
	} else if chosenMethodP1 == "Primary" {
		res1.TopLevelAction = "TreatedAsBase"
		var topLevelBuyTimeP1Raw float64 = math.Inf(1)
		if metricsP.ProductID != "" {
			fillTime, _, errFill := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
			if errFill == nil && !math.IsNaN(fillTime) && !math.IsInf(fillTime, -1) && fillTime >= 0 {
				topLevelBuyTimeP1Raw = fillTime
			} else {
				dlog("  P1: WARN - Fill time for top-level (TreatedAsBase) %s Primary error: %.2f, %v", itemNameNorm, fillTime, errFill)
			}
		} else {
			dlog("  P1: Metrics not found for top-level (TreatedAsBase) %s Primary", itemNameNorm)
		}

		topLevelDeltaP1 := metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {
			Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(sellP * quantity), Method: "Primary",
			RR: float64Ptr(topRR), IF: float64Ptr(topIF), Delta: float64Ptr(topLevelDeltaP1),
		}}
		res1.TotalCost = sanitizeFloat(topC10mPrim)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelPrimary"
		res1.SlowestIngredientBuyTimeSeconds = float64Ptr(topLevelBuyTimeP1Raw)
		res1.SlowestIngredientName = itemNameNorm
		res1.SlowestIngredientQuantity = sanitizeFloat(quantity)
	} else if chosenMethodP1 == "Secondary" {
		res1.TopLevelAction = "TreatedAsBase"
		topLevelDeltaSec := metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {
			Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(buyP * quantity), Method: "Secondary",
			RR: nil, IF: nil, Delta: float64Ptr(topLevelDeltaSec),
		}}
		res1.TotalCost = sanitizeFloat(topC10mSec)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelSecondary"
		res1.SlowestIngredientBuyTimeSeconds = float64Ptr(0.0)
		res1.SlowestIngredientName = ""
		res1.SlowestIngredientQuantity = 0.0
	} else {
		res1.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A", Delta: float64Ptr(metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency)}}
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "P1: No valid acquisition method found."
		}
		if topLevelRecipeExists && craftResultedInCycle {
			if !strings.Contains(res1.ErrorMessage, "cycle") {
				res1.ErrorMessage += " Crafting resulted in cycle (" + craftErrMsg + ")."
			}
		} else if topLevelRecipeExists && !craftPossible {
			if !strings.Contains(res1.ErrorMessage, "failed") {
				res1.ErrorMessage += " Crafting failed (" + craftErrMsg + ")."
			}
		} else if !topLevelRecipeExists {
			if !strings.Contains(res1.ErrorMessage, "No recipe") {
				res1.ErrorMessage += " No recipe and C10M methods invalid/unavailable."
			}
		}
		res1.TotalCost = 0.0
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
		res1.SlowestIngredientBuyTimeSeconds = float64Ptr(math.Inf(1))
		res1.SlowestIngredientName = ""
		res1.SlowestIngredientQuantity = 0.0
	}
	if chosenMethodP1 != "Craft" && craftResultedInCycle {
		msg := "Crafting path resulted in cycle."
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = msg
		} else if !strings.Contains(res1.ErrorMessage, msg) {
			res1.ErrorMessage += "; " + msg
		}
	}

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
				} else if craftErrMsg != "" && !strings.Contains(res2.ErrorMessage, craftErrMsg) {
					res2.ErrorMessage += "; " + craftErrMsg
				}
			}
		} else {
			chosenMethodP2 = "N/A"
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "Item not on Bazaar and no recipe exists."
			}
		}
	} else if validTopC10mPrim {
		if !craftPossible || topC10mPrim <= costToCraftOptimal {
			chosenMethodP2 = "Primary"
		} else {
			chosenMethodP2 = "Craft"
		}
	} else {
		if craftPossible {
			chosenMethodP2 = "Craft"
		} else {
			if validTopC10mSec {
				chosenMethodP2 = "Secondary"
			} else {
				chosenMethodP2 = "N/A"
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = "P2: Primary C10M invalid, Crafting impossible/failed (" + craftErrMsg + "), and Secondary C10M invalid."
				}
			}
		}
	}
	dlog("  P2 Decision: %s", chosenMethodP2)

	if chosenMethodP2 == "Craft" {
		if craftResultedInCycle {
			res2.TopLevelAction = "TreatedAsBase (Due to Cycle)"
			cycleMsg := "Crafting path resulted in top-level cycle."
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = cycleMsg
			} else if !strings.Contains(res2.ErrorMessage, cycleMsg) {
				res2.ErrorMessage += "; " + cycleMsg
			}

			var topLevelBuyTimeForCycleFallbackP2Raw float64 = math.Inf(1)
			if validTopC10mPrim {
				if metricsP.ProductID != "" {
					fillTime, _, errFill := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
					if errFill == nil && !math.IsNaN(fillTime) && !math.IsInf(fillTime, 0) && fillTime >= 0 {
						topLevelBuyTimeForCycleFallbackP2Raw = fillTime
					} else {
						dlog("  P2 Cycle Fallback Primary WARN - Fill time for %s err: %.2f, %v", itemNameNorm, fillTime, errFill)
					}
				} else {
					dlog("  P2 Cycle Fallback Primary: Metrics not found for %s", itemNameNorm)
				}
			}

			if validTopC10mPrim {
				topLevelDeltaCycleP2Prim := metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {
					Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(sellP * quantity), Method: "Primary",
					RR: float64Ptr(topRR), IF: float64Ptr(topIF), Delta: float64Ptr(topLevelDeltaCycleP2Prim),
				}}
				res2.TotalCost = sanitizeFloat(topC10mPrim)
				res2.CalculationPossible = true
				res2.FinalCostMethod = "FixedTopLevelPrimary"
				res2.SlowestIngredientBuyTimeSeconds = float64Ptr(topLevelBuyTimeForCycleFallbackP2Raw)
				res2.SlowestIngredientName = itemNameNorm
				res2.SlowestIngredientQuantity = sanitizeFloat(quantity)
			} else if validTopC10mSec {
				topLevelDeltaCycleP2Sec := metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {
					Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(buyP * quantity), Method: "Secondary",
					RR: nil, IF: nil, Delta: float64Ptr(topLevelDeltaCycleP2Sec),
				}}
				res2.TotalCost = sanitizeFloat(topC10mSec)
				res2.CalculationPossible = true
				res2.FinalCostMethod = "FixedTopLevelSecondary"
				res2.SlowestIngredientBuyTimeSeconds = float64Ptr(0.0)
				res2.SlowestIngredientName = ""
				res2.SlowestIngredientQuantity = 0.0
			} else {
				res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A (Cycle Fallback Failed)", Delta: float64Ptr(metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency)}}
				res2.TotalCost = 0.0
				res2.CalculationPossible = false
				res2.FinalCostMethod = "N/A"
				noFallbackMsg := "No valid C10M fallback for top-level item after cycle."
				if res2.ErrorMessage == "" {
					res2.ErrorMessage = noFallbackMsg
				} else if !strings.Contains(res2.ErrorMessage, noFallbackMsg) {
					res2.ErrorMessage += "; " + noFallbackMsg
				}
				res2.SlowestIngredientBuyTimeSeconds = float64Ptr(math.Inf(1))
				res2.SlowestIngredientName = ""
				res2.SlowestIngredientQuantity = 0.0
			}
		} else {
			res2.TopLevelAction = "Expanded"
			res2.BaseIngredients = baseIngredientsFromCraft
			res2.TotalCost = costToCraftOptimal
			res2.CalculationPossible = craftPossible
			res2.FinalCostMethod = "SumBestC10M"
			res2.SlowestIngredientBuyTimeSeconds = craftSlowestFillTimePtr
			res2.SlowestIngredientName = craftSlowestIngName
			res2.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
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
		var topLevelBuyTimeP2Raw float64 = math.Inf(1)
		if metricsP.ProductID != "" {
			fillTime, _, errFill := calculateBuyOrderFillTime(itemNameNorm, quantity, metricsP)
			if errFill == nil && !math.IsNaN(fillTime) && !math.IsInf(fillTime, 0) && fillTime >= 0 {
				topLevelBuyTimeP2Raw = fillTime
			} else {
				dlog("  P2 WARN - Fill time for top-level (TreatedAsBase) %s Primary err: %.2f, %v", itemNameNorm, fillTime, errFill)
			}
		} else {
			dlog("  P2: Metrics not found for top-level (TreatedAsBase) %s Primary", itemNameNorm)
		}
		topLevelDeltaP2Prim := metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {
			Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(sellP * quantity), Method: "Primary",
			RR: float64Ptr(topRR), IF: float64Ptr(topIF), Delta: float64Ptr(topLevelDeltaP2Prim),
		}}
		res2.TotalCost = sanitizeFloat(topC10mPrim)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelPrimary"
		res2.SlowestIngredientBuyTimeSeconds = float64Ptr(topLevelBuyTimeP2Raw)
		res2.SlowestIngredientName = itemNameNorm
		res2.SlowestIngredientQuantity = sanitizeFloat(quantity)
	} else if chosenMethodP2 == "Secondary" {
		res2.TopLevelAction = "TreatedAsBase"
		topLevelDeltaP2Sec := metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {
			Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(buyP * quantity), Method: "Secondary",
			RR: nil, IF: nil, Delta: float64Ptr(topLevelDeltaP2Sec),
		}}
		res2.TotalCost = sanitizeFloat(topC10mSec)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelSecondary"
		fallbackMsg := "Fell back to Secondary C10M cost."
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = fallbackMsg
		} else if !strings.Contains(res2.ErrorMessage, fallbackMsg) {
			res2.ErrorMessage += "; " + fallbackMsg
		}
		res2.SlowestIngredientBuyTimeSeconds = float64Ptr(0.0)
		res2.SlowestIngredientName = ""
		res2.SlowestIngredientQuantity = 0.0
	} else if chosenMethodP2 == "ExpansionFailed" {
		res2.TopLevelAction = "ExpansionFailed"
		if baseIngredientsFromCraft != nil {
			res2.BaseIngredients = baseIngredientsFromCraft
		} else {
			res2.BaseIngredients = make(map[string]BaseIngredientDetail)
		} // Ensure map exists
		if res2.ErrorMessage == "" && craftErrMsg != "" {
			res2.ErrorMessage = craftErrMsg
		}
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
		res2.SlowestIngredientBuyTimeSeconds = craftSlowestFillTimePtr
		res2.SlowestIngredientName = craftSlowestIngName
		res2.SlowestIngredientQuantity = sanitizeFloat(craftSlowestIngQty)
	} else { // N/A
		res2.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A", Delta: float64Ptr(metricsP.SellSize*metricsP.SellFrequency - metricsP.OrderSize*metricsP.OrderFrequency)}}
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = "P2: No valid acquisition method found."
		}
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
		res2.SlowestIngredientBuyTimeSeconds = float64Ptr(math.Inf(1))
		res2.SlowestIngredientName = ""
		res2.SlowestIngredientQuantity = 0.0
	}

	result.PrimaryBased.TotalCost = sanitizeFloat(result.PrimaryBased.TotalCost)
	result.PrimaryBased.TopLevelCost = sanitizeFloat(result.PrimaryBased.TopLevelCost)
	result.PrimaryBased.SlowestIngredientQuantity = sanitizeFloat(result.PrimaryBased.SlowestIngredientQuantity)

	result.SecondaryBased.TotalCost = sanitizeFloat(result.SecondaryBased.TotalCost)
	result.SecondaryBased.TopLevelCost = sanitizeFloat(result.SecondaryBased.TopLevelCost)
	result.SecondaryBased.SlowestIngredientQuantity = sanitizeFloat(result.SecondaryBased.SlowestIngredientQuantity)

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
