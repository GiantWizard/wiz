// optimizer.go
package main

import (
	"fmt"
	"log"
	"math"
	"sort"
)

type OptimizedItemResult struct {
	ItemName                    string            `json:"item_name"`
	MaxFeasibleQuantity         float64           `json:"max_feasible_quantity"`
	CostAtOptimalQty            *float64          `json:"cost_at_optimal_qty,omitempty"`
	RevenueAtOptimalQty         *float64          `json:"revenue_at_optimal_qty,omitempty"`
	MaxProfit                   *float64          `json:"max_profit,omitempty"`
	TotalCycleTimeAtOptimalQty  *float64          `json:"total_cycle_time_at_optimal_qty,omitempty"`
	AcquisitionTimeAtOptimalQty *float64          `json:"acquisition_time_at_optimal_qty,omitempty"`
	SaleTimeAtOptimalQty        *float64          `json:"sale_time_at_optimal_qty,omitempty"`
	BottleneckIngredient        string            `json:"bottleneck_ingredient,omitempty"`
	BottleneckIngredientQty     float64           `json:"bottleneck_ingredient_qty"`
	CalculationPossible         bool              `json:"calculation_possible"`
	ErrorMessage                string            `json:"error_message,omitempty"`
	RecipeTree                  *CraftingStepNode `json:"recipe_tree,omitempty"`
	MaxRecipeDepth              int               `json:"max_recipe_depth,omitempty"` // NEW FIELD for max depth
}

// ... (findMaxQuantityForTimeConstraint remains the same) ...
func findMaxQuantityForTimeConstraint(
	itemName string,
	maxAllowedFillTime float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleQty float64,
) (float64, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog("Optimizer: Finding max quantity for %s (Total Cycle Time Constraint: %.2f s, Initial Max Search Qty: %.2f)", itemNameNorm, maxAllowedFillTime, maxPossibleQty)

	low := 1.0
	high := maxPossibleQty
	if high < low {
		dlog("  Optimizer Search: Initial high (%.2f) < low (%.2f). Setting high = low = %.2f", maxPossibleQty, low, low)
		high = low
	}
	bestQty := 0.0
	iterations := 0
	const maxIterations = 50

	for iterations < maxIterations && high >= low {
		iterations++
		midQty := low + (high-low)/2
		if midQty < 1 {
			midQty = 1
		}
		midQty = math.Floor(midQty)

		if midQty == low && midQty == high && iterations > 1 {
			if bestQty == 0 && midQty > 0 {
				// Final check
			} else {
				dlog("  Optimizer Search: Range collapsed or midQty stuck. Low=%.2f, High=%.2f, Mid=%.2f. Breaking.", low, high, midQty)
				break
			}
		}

		dlog("  Optimizer Search: Iter %d, Low=%.2f, High=%.2f, Testing MidQty=%.2f for %s", iterations, low, high, midQty, itemNameNorm)

		dualResult, err := PerformDualExpansion(itemNameNorm, midQty, apiResp, metricsMap, itemFilesDir)
		if err != nil {
			dlog("  Optimizer Search: PerformDualExpansion error for %s qty %.2f: %v. Treating as too slow.", itemNameNorm, midQty, err)
			high = midQty - 1
			continue
		}

		if !dualResult.PrimaryBased.CalculationPossible {
			dlog("  Optimizer Search: P1 calculation not possible for %s qty %.2f. Error: '%s'. Treating as too slow.", itemNameNorm, midQty, dualResult.PrimaryBased.ErrorMessage)
			high = midQty - 1
			continue
		}

		acquisitionTimePtr := dualResult.PrimaryBased.SlowestIngredientBuyTimeSeconds
		saleTimePtr := dualResult.TopLevelInstasellTimeSeconds

		var acquisitionTimeValue float64 = math.Inf(1)
		if acquisitionTimePtr != nil && !math.IsNaN(*acquisitionTimePtr) {
			acquisitionTimeValue = *acquisitionTimePtr
		}
		var saleTimeValue float64 = math.Inf(1)
		if saleTimePtr != nil && !math.IsNaN(*saleTimePtr) {
			saleTimeValue = *saleTimePtr
		}

		totalEffectiveTime := acquisitionTimeValue + saleTimeValue
		if math.IsNaN(totalEffectiveTime) {
			totalEffectiveTime = math.Inf(1)
		}

		dlog("  Optimizer Search: %s Qty %.2f - AcqTime: %.2fs, SaleTime: %.2fs, TotalEffTime: %.2fs (vs Max: %.2fs)",
			itemNameNorm, midQty, acquisitionTimeValue, saleTimeValue, totalEffectiveTime, maxAllowedFillTime)

		isTooSlow := false
		if totalEffectiveTime > maxAllowedFillTime {
			isTooSlow = true
		}

		if isTooSlow {
			high = midQty - 1
		} else {
			bestQty = midQty
			low = midQty + 1
		}

		if high < low || math.Abs(high-low) < 1 && iterations > 5 {
			dlog("  Optimizer Search: Range small (high %.2f, low %.2f), breaking.", high, low)
			break
		}
	}

	dlog("Optimizer: Best feasible quantity for %s (Total Cycle Time Constraint %.2f s): %.2f (after %d iterations)", itemNameNorm, maxAllowedFillTime, bestQty, iterations)
	return sanitizeFloat(bestQty), nil
}

func optimizeItemProfit(
	itemName string,
	maxAllowedFillTime float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleInitialQty float64,
) OptimizedItemResult {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog("Optimizer: Optimizing profit for %s (Total Cycle Time Constraint: %.2fs)", itemNameNorm, maxAllowedFillTime)

	result := OptimizedItemResult{
		ItemName:            itemNameNorm,
		CalculationPossible: false,
	}

	maxFeasibleQty, errFeasible := findMaxQuantityForTimeConstraint(itemNameNorm, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, maxPossibleInitialQty)
	if errFeasible != nil {
		result.ErrorMessage = fmt.Sprintf("Error finding max feasible quantity: %v", errFeasible)
		if maxFeasibleQty == 0 {
			dualResultCheckQty1, _ := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir)
			if dualResultCheckQty1 != nil {
				result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
				result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds
				result.RecipeTree = dualResultCheckQty1.PrimaryBased.RecipeTree
				if result.RecipeTree != nil {
					result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
				} // Get max depth

				var acqTime, saleTime float64 = math.Inf(1), math.Inf(1)
				if result.AcquisitionTimeAtOptimalQty != nil {
					acqTime = *result.AcquisitionTimeAtOptimalQty
				}
				if result.SaleTimeAtOptimalQty != nil {
					saleTime = *result.SaleTimeAtOptimalQty
				}
				result.TotalCycleTimeAtOptimalQty = float64Ptr(acqTime + saleTime)

				if dualResultCheckQty1.PrimaryBased.CalculationPossible {
					result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
					result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				}
				if result.ErrorMessage == "" {
					result.ErrorMessage += "; Qty 1 check performed."
				} else {
					result.ErrorMessage = "Qty 1 check performed."
				}
			}
		}
		return result
	}
	result.MaxFeasibleQuantity = maxFeasibleQty

	if result.MaxFeasibleQuantity <= 0 {
		dualResultCheckQty1, errCheckQty1 := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir)
		var acqTime, saleTime, totalTime float64 = math.Inf(1), math.Inf(1), math.Inf(1)

		if errCheckQty1 != nil {
			if result.ErrorMessage == "" {
				result.ErrorMessage = fmt.Sprintf("Qty 1 check (PerformDualExpansion) failed: %v", errCheckQty1)
			}
		} else if dualResultCheckQty1 != nil {
			result.RecipeTree = dualResultCheckQty1.PrimaryBased.RecipeTree
			if result.RecipeTree != nil {
				result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
			} // Get max depth
			result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
			result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds
			if result.AcquisitionTimeAtOptimalQty != nil {
				acqTime = *result.AcquisitionTimeAtOptimalQty
			}
			if result.SaleTimeAtOptimalQty != nil {
				saleTime = *result.SaleTimeAtOptimalQty
			}
			totalTime = acqTime + saleTime
			result.TotalCycleTimeAtOptimalQty = float64Ptr(totalTime)

			if dualResultCheckQty1.PrimaryBased.CalculationPossible {
				result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
				result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				if result.ErrorMessage == "" {
					if math.IsNaN(totalTime) || math.IsInf(totalTime, 1) || totalTime > maxAllowedFillTime {
						result.ErrorMessage = fmt.Sprintf("Quantity 1 total cycle time (Acq:%.2fs + Sale:%.2fs = %.2fs) exceeds max (%.2fs). Bottleneck: %s.", acqTime, saleTime, totalTime, maxAllowedFillTime, result.BottleneckIngredient)
					} else {
						result.ErrorMessage = "No feasible quantity found by optimizer (max feasible qty is 0)."
					}
				}
			} else {
				if result.ErrorMessage == "" {
					result.ErrorMessage = "P1 calculation failed for qty 1 check"
				}
				if dualResultCheckQty1.PrimaryBased.ErrorMessage != "" {
					result.ErrorMessage += ": " + dualResultCheckQty1.PrimaryBased.ErrorMessage
				}
			}
		} else {
			if result.ErrorMessage == "" {
				result.ErrorMessage = "No feasible quantity found and qty 1 check also failed or yielded no result."
			}
		}
		return result
	}

	dualResultFinal, errExpansionFinal := PerformDualExpansion(itemNameNorm, result.MaxFeasibleQuantity, apiResp, metricsMap, itemFilesDir)
	if errExpansionFinal != nil {
		result.ErrorMessage = fmt.Sprintf("Error performing dual expansion for optimal qty %.2f: %v", result.MaxFeasibleQuantity, errExpansionFinal)
		if dualResultFinal != nil {
			result.RecipeTree = dualResultFinal.PrimaryBased.RecipeTree
			if result.RecipeTree != nil {
				result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
			}
		}
		return result
	}
	if dualResultFinal == nil {
		result.ErrorMessage = fmt.Sprintf("Dual expansion returned nil for optimal qty %.2f", result.MaxFeasibleQuantity)
		return result
	}

	resP1Final := dualResultFinal.PrimaryBased
	result.RecipeTree = resP1Final.RecipeTree
	if result.RecipeTree != nil {
		result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
	} // Get max depth

	if !resP1Final.CalculationPossible {
		result.ErrorMessage = fmt.Sprintf("PrimaryBased calculation not possible for optimal qty %.2f: %s", result.MaxFeasibleQuantity, resP1Final.ErrorMessage)
		result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
		result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds
		var acqTime, saleTime float64 = math.Inf(1), math.Inf(1)
		if result.AcquisitionTimeAtOptimalQty != nil {
			acqTime = *result.AcquisitionTimeAtOptimalQty
		}
		if result.SaleTimeAtOptimalQty != nil {
			saleTime = *result.SaleTimeAtOptimalQty
		}
		result.TotalCycleTimeAtOptimalQty = float64Ptr(acqTime + saleTime)
		result.BottleneckIngredient = resP1Final.SlowestIngredientName
		result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)
		return result
	}

	var costAtOptimal float64 = math.Inf(1)
	if resP1Final.TotalCost != nil {
		costAtOptimal = *resP1Final.TotalCost
	}
	result.CostAtOptimalQty = float64Ptr(costAtOptimal)

	result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
	result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds

	var acqTimeFinal, saleTimeFinal float64 = math.Inf(1), math.Inf(1)
	if result.AcquisitionTimeAtOptimalQty != nil {
		acqTimeFinal = *result.AcquisitionTimeAtOptimalQty
	}
	if result.SaleTimeAtOptimalQty != nil {
		saleTimeFinal = *result.SaleTimeAtOptimalQty
	}
	result.TotalCycleTimeAtOptimalQty = float64Ptr(acqTimeFinal + saleTimeFinal)

	result.BottleneckIngredient = resP1Final.SlowestIngredientName
	result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)

	instasellPrice := getBuyPrice(apiResp, itemNameNorm)
	var revenueAtOptimal float64 = 0
	var maxProfit float64 = -math.Inf(1)

	if instasellPrice <= 0 || math.IsNaN(instasellPrice) || math.IsInf(instasellPrice, 0) {
		errMsgRevenue := fmt.Sprintf("Cannot get valid instasell price for %s (Price: %.2f). Revenue calculation failed.", itemNameNorm, instasellPrice)
		if result.ErrorMessage == "" {
			result.ErrorMessage = errMsgRevenue
		} else {
			result.ErrorMessage += "; " + errMsgRevenue
		}
		if !math.IsInf(costAtOptimal, 0) {
			maxProfit = 0 - costAtOptimal
		}
	} else {
		revenueAtOptimal = instasellPrice * result.MaxFeasibleQuantity
		if !math.IsInf(costAtOptimal, 0) {
			maxProfit = revenueAtOptimal - costAtOptimal
		}
	}
	result.RevenueAtOptimalQty = float64Ptr(revenueAtOptimal)
	result.MaxProfit = float64Ptr(maxProfit)
	result.CalculationPossible = true

	return result
}

// ... (RunFullOptimization remains the same) ...
func RunFullOptimization(
	itemIDs []string,
	maxAllowedFillTime float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleInitialQtyPerItem float64,
) []OptimizedItemResult {
	dlog("Optimizer: Starting full optimization for %d items. Total Cycle Time Constraint: %.2fs, Max Initial Qty: %.2f", len(itemIDs), maxAllowedFillTime, maxPossibleInitialQtyPerItem)
	var results []OptimizedItemResult

	if apiResp == nil || metricsMap == nil {
		log.Println("ERROR (Optimizer): API response or metrics map is nil. Cannot run full optimization.")
		results = append(results, OptimizedItemResult{ItemName: "BATCH_ERROR", ErrorMessage: "Optimizer input error: API or Metrics data missing.", CalculationPossible: false})
		return results
	}
	if len(itemIDs) == 0 {
		log.Println("Optimizer: No item IDs provided for optimization.")
		return results
	}

	for i, itemID := range itemIDs {
		dlog("Optimizer: Optimizing item %d/%d: %s", i+1, len(itemIDs), itemID)
		normalizedID := BAZAAR_ID(itemID)

		if _, exists := apiResp.Products[normalizedID]; !exists {
			dlog("Optimizer: Item %s (Normalized: %s) not found in API product list for this run, skipping.", itemID, normalizedID)
			results = append(results, OptimizedItemResult{
				ItemName:            normalizedID,
				CalculationPossible: false,
				ErrorMessage:        "Item not found in current Bazaar API data.",
			})
			continue
		}

		currentMaxInitialQty := maxPossibleInitialQtyPerItem
		if currentMaxInitialQty <= 0 {
			currentMaxInitialQty = 1000000.0
			dlog("Optimizer: maxPossibleInitialQtyPerItem was <=0, using default %.2f for %s", currentMaxInitialQty, normalizedID)
		}

		result := optimizeItemProfit(normalizedID, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, currentMaxInitialQty)
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].CalculationPossible && !results[j].CalculationPossible {
			return true
		}
		if !results[i].CalculationPossible && results[j].CalculationPossible {
			return false
		}
		if results[i].CalculationPossible == results[j].CalculationPossible {
			profitI := -math.Inf(1)
			if results[i].MaxProfit != nil {
				profitI = *results[i].MaxProfit
			}
			profitJ := -math.Inf(1)
			if results[j].MaxProfit != nil {
				profitJ = *results[j].MaxProfit
			}

			if profitI != profitJ {
				return profitI > profitJ
			}
		}
		return results[i].ItemName < results[j].ItemName
	})

	dlog("Optimizer: Full optimization complete. Processed %d items.", len(results))
	return results
}
