package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// --- Structs (Remain the same) ---
type BaseIngredientDetail struct {
	Quantity       float64  `json:"quantity"`
	BestCost       float64  `json:"best_cost"`
	AssociatedCost float64  `json:"associated_cost"`
	Method         string   `json:"method"`
	RR             *float64 `json:"rr,omitempty"`
}
type ExpansionResult struct {
	BaseIngredients              map[string]BaseIngredientDetail `json:"base_ingredients"`
	TotalCost                    float64                         `json:"total_cost"`
	PerspectiveType              string                          `json:"perspective_type"`
	TopLevelAction               string                          `json:"top_level_action"`
	FinalCostMethod              string                          `json:"final_cost_method"`
	CalculationPossible          bool                            `json:"calculation_possible"`
	ErrorMessage                 string                          `json:"error_message,omitempty"`
	TopLevelCost                 float64                         `json:"top_level_cost"`
	TopLevelRR                   *float64                        `json:"top_level_rr,omitempty"`
	SlowestIngredientBuyTimeName string                          `json:"slowest_ingredient_buy_time_name,omitempty"`
	SlowestIngredientBuyTimeQty  float64                         `json:"slowest_ingredient_buy_time_qty"`
	SlowestIngredientBuyTime     float64                         `json:"slowest_ingredient_buy_time"`
}
type DualExpansionResult struct {
	ItemName              string          `json:"item_name"`
	Quantity              float64         `json:"quantity"`
	PrimaryBased          ExpansionResult `json:"primary_based"`
	SecondaryBased        ExpansionResult `json:"secondary_based"`
	TopLevelInstasellTime float64         `json:"top_level_instasell_time"`
}

// Helper
func float64Ptr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// --- Cost and Fill Time Helper (NOW CORRECTLY CALLS fill_time.go) ---
func calculateDetailedCostsAndFillTimes(
	baseMapInput map[string]float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (
	totalCost float64, // Sanitized
	detailedMap map[string]BaseIngredientDetail,
	slowestBuyTimeName string,
	slowestBuyTimeQty float64,
	slowestBuyTimeVal float64, // Sanitized
	possible bool,
	errMsg string,
) {
	totalCost = 0.0
	possible = true
	detailedMap = make(map[string]BaseIngredientDetail)
	slowestBuyTimeVal = 0.0
	slowestBuyTimeName = ""
	slowestBuyTimeQty = 0.0

	if len(baseMapInput) == 0 {
		return 0.0, detailedMap, "", 0.0, 0.0, true, ""
	}

	dlog("Calculating detailed costs & fill times for %d base ingredient types...", len(baseMapInput))
	for itemID, quantity := range baseMapInput {
		if quantity <= 0 {
			continue
		}
		bestCost, method, assocCost, rr, errC10M := getBestC10M(itemID, quantity, apiResp, metricsMap)
		ingredientDetail := BaseIngredientDetail{Quantity: quantity, BestCost: 0.0, AssociatedCost: 0.0, Method: "N/A", RR: nil}

		if errC10M != nil || method == "N/A" || math.IsInf(bestCost, 0) || math.IsNaN(bestCost) || bestCost < 0 {
			errMsg = fmt.Sprintf("Cannot determine valid cost for base ingredient '%s': %v", itemID, errC10M)
			dlog("  WARN (calculateDetailedCostsAndFillTimes - Cost): %s", errMsg)
			possible = false
			totalCost = math.Inf(1)
			detailedMap[itemID] = ingredientDetail
			break
		} else {
			totalCost += bestCost
			ingredientDetail.BestCost = sanitizeFloat(bestCost)
			ingredientDetail.AssociatedCost = sanitizeFloat(assocCost)
			ingredientDetail.Method = method
			if method == "Primary" {
				ingredientDetail.RR = float64Ptr(rr)
			}
			detailedMap[itemID] = ingredientDetail

			// *** CORRECTED: Call calculateBuyOrderFillTime from fill_time.go ***
			metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID) // Assumes utils.go
			if metricsOk {
				// Call the function from fill_time.go
				buyTime, _, errFillTime := calculateBuyOrderFillTime(itemID, quantity, metricsData)
				dlog("  Fill time calc for %s (qty %.2f): time=%.2f, err=%v", itemID, quantity, buyTime, errFillTime)

				if errFillTime == nil && !math.IsNaN(buyTime) && !math.IsInf(buyTime, 0) && buyTime > 0 {
					if buyTime > slowestBuyTimeVal {
						slowestBuyTimeVal = buyTime
						slowestBuyTimeName = itemID
						slowestBuyTimeQty = quantity
						dlog("    New slowest buy time: %.2fs for %s", slowestBuyTimeVal, slowestBuyTimeName)
					}
				} else if errFillTime != nil {
					dlog("  WARN (calculateDetailedCostsAndFillTimes - FillTime): Error calculating buy order fill time for %s: %v", itemID, errFillTime)
				}
			} else {
				dlog("  WARN (calculateDetailedCostsAndFillTimes - FillTime): Metrics not found for %s, cannot calculate buy order fill time.", itemID)
			}
			// *** END CORRECTION ***
		}
	}

	if !possible {
		return 0.0, detailedMap, "", 0.0, 0.0, false, errMsg
	}
	return sanitizeFloat(totalCost), detailedMap, slowestBuyTimeName, slowestBuyTimeQty, sanitizeFloat(slowestBuyTimeVal), true, ""
}

// --- Orchestration Function (No changes needed here from previous version, already calls the helper correctly) ---
func PerformDualExpansion(itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string) (*DualExpansionResult, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s <<<", quantity, itemNameNorm)
	result := &DualExpansionResult{ItemName: itemNameNorm, Quantity: quantity, PrimaryBased: ExpansionResult{PerspectiveType: "PrimaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail), TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil, SlowestIngredientBuyTime: 0.0, SlowestIngredientBuyTimeQty: 0.0}, SecondaryBased: ExpansionResult{PerspectiveType: "SecondaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail), TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil, SlowestIngredientBuyTime: 0.0, SlowestIngredientBuyTimeQty: 0.0}, TopLevelInstasellTime: 0.0}

	// --- Pre-calculate Top-Level Costs & Check Recipe ONCE ---
	topC10mPrim, topC10mSec, _, topRR, _, _, errTopC10M := calculateC10MInternal( /* ... params ... */
		itemNameNorm, quantity, getSellPrice(apiResp, itemNameNorm), getBuyPrice(apiResp, itemNameNorm), getMetrics(metricsMap, itemNameNorm),
	)
	result.PrimaryBased.TopLevelCost = sanitizeFloat(topC10mPrim)
	result.SecondaryBased.TopLevelCost = sanitizeFloat(topC10mSec)
	if !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0 {
		result.PrimaryBased.TopLevelRR = float64Ptr(topRR)
	}
	topLevelRecipeExists := false
	var topLevelRecipeCheckErr error
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); statErr == nil {
		topLevelRecipeExists = true
	} else if !os.IsNotExist(statErr) {
		topLevelRecipeCheckErr = fmt.Errorf("checking recipe file '%s': %w", filePath, statErr)
		log.Printf("ERROR (PerformDualExpansion): Cannot check recipe file for %s: %v", itemNameNorm, topLevelRecipeCheckErr)
	}
	if errTopC10M != nil && topLevelRecipeCheckErr == nil {
		log.Printf("WARN (PerformDualExpansion): Failed C10M (%v) for %s. Proceeding based on recipe existence.", errTopC10M, itemNameNorm)
	} else if topLevelRecipeCheckErr != nil {
		errMsg := fmt.Sprintf("Failed to check top-level recipe: %v", topLevelRecipeCheckErr)
		result.PrimaryBased.ErrorMessage = errMsg
		result.SecondaryBased.ErrorMessage = errMsg
		result.PrimaryBased.TopLevelAction = "Unknown"
		result.SecondaryBased.TopLevelAction = "Unknown"
		return result, nil
	} else {
		dlog("  Pre-calculated Top-Level C10M: Primary=%.2f, Secondary=%.2f, TopRR=%.2f", topC10mPrim, topC10mSec, topRR)
	}
	dlog("  Top-Level Recipe Exists: %v", topLevelRecipeExists)

	// --- Calculate Top-Level Instasell Time ---
	topProductData, apiOk := safeGetProductData(apiResp, itemNameNorm)
	var tempInstaTime float64 = math.NaN()
	if apiOk {
		instaTimeCalc, errInsta := calculateInstasellFillTime(quantity, topProductData)
		if errInsta == nil {
			tempInstaTime = instaTimeCalc
		} else {
			log.Printf("WARN (PerformDualExpansion): Failed to calculate top-level instasell time for %s: %v", itemNameNorm, errInsta)
		}
	} else {
		log.Printf("WARN (PerformDualExpansion): API data for %s not found for top-level instasell time.", itemNameNorm)
	}
	result.TopLevelInstasellTime = sanitizeFloat(tempInstaTime)

	// --- Perspective 1: Primary-Based ---
	dlog("--- Running Perspective 1 (Primary-Based Decision) ---")
	res1 := &result.PrimaryBased
	res1.FinalCostMethod = "SumBestC10M"
	shouldExpandP1 := false
	var validPrim, validSec bool
	if errTopC10M == nil {
		validPrim = !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0
		validSec = !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0
	}
	isApiNotFoundErrorP1 := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")
	if isApiNotFoundErrorP1 {
		if topLevelRecipeExists {
			shouldExpandP1 = true
		} else {
			shouldExpandP1 = false
			if res1.ErrorMessage == "" {
				res1.ErrorMessage = "Item not on Bazaar and no recipe found"
			}
		}
	} else if errTopC10M != nil {
		shouldExpandP1 = false
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = fmt.Sprintf("Top-level C10M failed: %v", errTopC10M)
		}
	} else {
		if validPrim && validSec {
			shouldExpandP1 = topC10mPrim <= topC10mSec
		} else if validPrim {
			shouldExpandP1 = true
		} else {
			shouldExpandP1 = false
		}
	}

	if shouldExpandP1 {
		baseMapP1_qtyOnly, errExpandP1 := ExpandItem(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)
		if errExpandP1 != nil {
			res1.ErrorMessage = fmt.Sprintf("Expansion failed: %v", errExpandP1)
			res1.TopLevelAction = "ExpansionFailed"
		} else {
			if len(baseMapP1_qtyOnly) == 0 { /* Handle cycles */
				res1.TopLevelAction = "TreatedAsBase (Due to Cycle)"
				costP1 := math.Inf(1)
				methodP1 := "N/A"
				assocCostP1 := math.NaN()
				rrP1 := math.NaN()
				if validPrim && validSec {
					costP1 = math.Min(topC10mPrim, topC10mSec)
					if topC10mPrim <= topC10mSec {
						methodP1 = "Primary"
						assocCostP1 = getSellPrice(apiResp, itemNameNorm) * quantity
						rrP1 = topRR
					} else {
						methodP1 = "Secondary"
						assocCostP1 = getBuyPrice(apiResp, itemNameNorm) * quantity
					}
				} else if validPrim {
					costP1 = topC10mPrim
					methodP1 = "Primary"
					assocCostP1 = getSellPrice(apiResp, itemNameNorm) * quantity
					rrP1 = topRR
				} else if validSec {
					costP1 = topC10mSec
					methodP1 = "Secondary"
					assocCostP1 = getBuyPrice(apiResp, itemNameNorm) * quantity
				}
				res1.BaseIngredients[itemNameNorm] = BaseIngredientDetail{Quantity: quantity, BestCost: sanitizeFloat(costP1), AssociatedCost: sanitizeFloat(assocCostP1), Method: methodP1, RR: float64Ptr(rrP1)}
				if math.IsInf(costP1, 0) || math.IsNaN(costP1) {
					res1.ErrorMessage = "Valid top-level cost undetermined after cycle pruning"
					res1.TotalCost = 0.0
				} else {
					res1.TotalCost = sanitizeFloat(costP1)
					res1.CalculationPossible = true
				}
				res1.FinalCostMethod = "TopLevelMinimumC10M"
			} else { /* Handle success */
				res1.TopLevelAction = "Expanded"
				totalCostP1, detailedMapP1, sbtName, sbtQty, sbtVal, possibleP1, errMsgP1 := calculateDetailedCostsAndFillTimes(baseMapP1_qtyOnly, apiResp, metricsMap)
				res1.BaseIngredients = detailedMapP1
				res1.SlowestIngredientBuyTimeName = sbtName
				res1.SlowestIngredientBuyTimeQty = sbtQty
				res1.SlowestIngredientBuyTime = sbtVal
				if !possibleP1 {
					res1.ErrorMessage = fmt.Sprintf("Cost calculation failed: %s", errMsgP1)
					res1.TotalCost = 0.0
				} else {
					res1.TotalCost = totalCostP1
					res1.CalculationPossible = true
				}
			}
		}
	} else { // shouldExpandP1 is false (Treat As Base)
		res1.TopLevelAction = "TreatedAsBase"
		costP1 := math.Inf(1)
		methodP1 := "N/A"
		assocCostP1 := math.NaN()
		rrP1 := math.NaN()
		if validPrim && validSec {
			costP1 = math.Min(topC10mPrim, topC10mSec)
			if topC10mPrim <= topC10mSec {
				methodP1 = "Primary"
				assocCostP1 = getSellPrice(apiResp, itemNameNorm) * quantity
				rrP1 = topRR
			} else {
				methodP1 = "Secondary"
				assocCostP1 = getBuyPrice(apiResp, itemNameNorm) * quantity
			}
		} else if validPrim {
			costP1 = topC10mPrim
			methodP1 = "Primary"
			assocCostP1 = getSellPrice(apiResp, itemNameNorm) * quantity
			rrP1 = topRR
		} else if validSec {
			costP1 = topC10mSec
			methodP1 = "Secondary"
			assocCostP1 = getBuyPrice(apiResp, itemNameNorm) * quantity
		}
		res1.BaseIngredients[itemNameNorm] = BaseIngredientDetail{Quantity: quantity, BestCost: sanitizeFloat(costP1), AssociatedCost: sanitizeFloat(assocCostP1), Method: methodP1, RR: float64Ptr(rrP1)}
		if math.IsInf(costP1, 0) || math.IsNaN(costP1) {
			if res1.ErrorMessage == "" {
				res1.ErrorMessage = "Failed to determine a valid top-level cost"
			}
			res1.TotalCost = 0.0
		} else {
			res1.TotalCost = sanitizeFloat(costP1)
			res1.CalculationPossible = true
		}
		res1.FinalCostMethod = "TopLevelMinimumC10M"
	}

	// --- Perspective 2: Secondary-Based ---
	dlog("--- Running Perspective 2 (Benchmark: Top-Level Secondary C10M) ---")
	res2 := &result.SecondaryBased
	res2.FinalCostMethod = "SumBestC10M"
	validTopC10mSec := errTopC10M == nil && !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0

	if !topLevelRecipeExists { /* No recipe */
		res2.TopLevelAction = "TreatedAsBase (No Recipe)"
		res2.BaseIngredients[itemNameNorm] = BaseIngredientDetail{Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(topC10mSec), Method: "Secondary", RR: nil}
		if !validTopC10mSec {
			res2.ErrorMessage = fmt.Sprintf("Top-level Secondary C10M invalid (%.2f) and no recipe.", topC10mSec)
			res2.TotalCost = 0.0
			res2.CalculationPossible = false
		} else {
			res2.TotalCost = sanitizeFloat(topC10mSec)
			res2.CalculationPossible = true
		}
		res2.FinalCostMethod = "FixedTopLevelSecondary"
	} else { // Recipe exists, attempt expansion
		baseMapP2_qtyOnly, errExpandP2 := ExpandItem(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)
		if errExpandP2 != nil { /* Expansion failed */
			res2.ErrorMessage = fmt.Sprintf("Forced expansion attempt failed: %v", errExpandP2)
			res2.TopLevelAction = "ExpansionFailed"
			if !validTopC10mSec {
				res2.TotalCost = 0.0
				res2.CalculationPossible = false
				res2.ErrorMessage += "; Top-level Secondary C10M also invalid."
			} else {
				res2.TotalCost = sanitizeFloat(topC10mSec)
				res2.CalculationPossible = true
				res2.FinalCostMethod = "FixedTopLevelSecondary"
			}
		} else { // Expansion succeeded
			if len(baseMapP2_qtyOnly) == 0 { /* Cycled to self */
				res2.TopLevelAction = "TreatedAsBase (Due to Cycle)"
				res2.BaseIngredients[itemNameNorm] = BaseIngredientDetail{Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(topC10mSec), Method: "Secondary", RR: nil}
				if !validTopC10mSec {
					res2.ErrorMessage = "Top-level Secondary C10M invalid after cycle pruning"
					res2.TotalCost = 0.0
					res2.CalculationPossible = false
				} else {
					res2.TotalCost = sanitizeFloat(topC10mSec)
					res2.CalculationPossible = true
				}
				res2.FinalCostMethod = "FixedTopLevelSecondary"
			} else { /* Successfully expanded, sum optimal ingredient costs */
				res2.TopLevelAction = "Expanded"
				totalCostP2, detailedMapP2, sbtName, sbtQty, sbtVal, possibleP2, errMsgP2 := calculateDetailedCostsAndFillTimes(baseMapP2_qtyOnly, apiResp, metricsMap)
				res2.BaseIngredients = detailedMapP2
				res2.SlowestIngredientBuyTimeName = sbtName
				res2.SlowestIngredientBuyTimeQty = sbtQty
				res2.SlowestIngredientBuyTime = sbtVal
				if !possibleP2 {
					res2.ErrorMessage = fmt.Sprintf("Ingredient cost calculation failed: %s", errMsgP2)
					res2.TotalCost = 0.0
				} else {
					res2.TotalCost = totalCostP2
					res2.CalculationPossible = true
				}
			}
		}
	}

	// Final sanitization
	res1.TotalCost = sanitizeFloat(res1.TotalCost)
	res1.SlowestIngredientBuyTime = sanitizeFloat(res1.SlowestIngredientBuyTime)
	res2.TotalCost = sanitizeFloat(res2.TotalCost)
	res2.SlowestIngredientBuyTime = sanitizeFloat(res2.SlowestIngredientBuyTime)

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
