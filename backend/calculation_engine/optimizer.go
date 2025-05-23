// optimizer.go
package main

import (
	"fmt"
	"log"
	"math"
	"sort"
)

// OptimizedItemResult uses JSONFloat64 for NaN-able fields
type OptimizedItemResult struct {
	ItemName                    string            `json:"item_name"`
	MaxFeasibleQuantity         float64           `json:"max_feasible_quantity"` // Assuming never NaN
	CostAtOptimalQty            JSONFloat64       `json:"cost_at_optimal_qty,omitempty"`
	RevenueAtOptimalQty         JSONFloat64       `json:"revenue_at_optimal_qty,omitempty"`
	MaxProfit                   JSONFloat64       `json:"max_profit,omitempty"`
	TotalCycleTimeAtOptimalQty  JSONFloat64       `json:"total_cycle_time_at_optimal_qty,omitempty"`
	AcquisitionTimeAtOptimalQty JSONFloat64       `json:"acquisition_time_at_optimal_qty,omitempty"`
	SaleTimeAtOptimalQty        JSONFloat64       `json:"sale_time_at_optimal_qty,omitempty"`
	BottleneckIngredient        string            `json:"bottleneck_ingredient,omitempty"`
	BottleneckIngredientQty     float64           `json:"bottleneck_ingredient_qty"` // Assuming never NaN
	CalculationPossible         bool              `json:"calculation_possible"`
	ErrorMessage                string            `json:"error_message,omitempty"`
	RecipeTree                  *CraftingStepNode `json:"recipe_tree,omitempty"` // Defined in tree_builder.go
	MaxRecipeDepth              int               `json:"max_recipe_depth,omitempty"`
}

func findMaxQuantityForTimeConstraint(
	itemName string,
	maxAllowedFillTime float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleQty float64,
) (float64, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	// ... (rest of findMaxQuantityForTimeConstraint logic as previously corrected,
	// ensuring it uses float64(jsonFloatField) when reading for calculations) ...
	// Example when reading from dualResult:
	// acquisitionTimeRaw := float64(dualResult.PrimaryBased.SlowestIngredientBuyTimeSeconds)
	// saleTimeRaw := float64(dualResult.TopLevelInstasellTimeSeconds)
	// This part was correct in the previous full version.

	// --- Start of copy from previous correct version for this function ---
	dlog("Optimizer: Finding max quantity for %s (Total Cycle Time Constraint: %.2f s, Initial Max Search Qty: %.2f)", itemNameNorm, maxAllowedFillTime, maxPossibleQty)
	low := 1.0
	high := maxPossibleQty
	if high < low {
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
			if bestQty == 0 && midQty == 1 {
			} else if math.Abs(high-low) < 1e-9 && iterations > 1 {
				break
			} else if midQty == low && iterations > 5 {
				break
			}
		}
		dlog("  Optimizer Search: Iter %d, Low=%.2f, High=%.2f, Testing MidQty=%.2f for %s", iterations, low, high, midQty, itemNameNorm)
		dualResult, err := PerformDualExpansion(itemNameNorm, midQty, apiResp, metricsMap, itemFilesDir)
		if err != nil {
			high = midQty - 1
			continue
		}
		if !dualResult.PrimaryBased.CalculationPossible {
			high = midQty - 1
			continue
		}

		acquisitionTimeRaw := float64(dualResult.PrimaryBased.SlowestIngredientBuyTimeSeconds) // Cast from JSONFloat64
		saleTimeRaw := float64(dualResult.TopLevelInstasellTimeSeconds)                        // Cast from JSONFloat64
		if math.IsNaN(acquisitionTimeRaw) {
			acquisitionTimeRaw = math.Inf(1)
		}
		if math.IsNaN(saleTimeRaw) {
			saleTimeRaw = math.Inf(1)
		}
		totalEffectiveTime := acquisitionTimeRaw + saleTimeRaw
		dlog("  Optimizer Search: %s Qty %.2f - AcqTime: %.2fs, SaleTime: %.2fs, TotalEffTime: %.2fs (vs Max: %.2fs)", itemNameNorm, midQty, acquisitionTimeRaw, saleTimeRaw, totalEffectiveTime, maxAllowedFillTime)
		if totalEffectiveTime > maxAllowedFillTime {
			high = midQty - 1
		} else {
			bestQty = midQty
			low = midQty + 1
		}
	}
	dlog("Optimizer: Best feasible quantity for %s (Total Cycle Time Constraint %.2f s): %.2f (after %d iterations)", itemNameNorm, maxAllowedFillTime, bestQty, iterations)
	return sanitizeFloat(bestQty), nil
	// --- End of copy from previous correct version for this function ---
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
		ItemName:                    itemNameNorm,
		CalculationPossible:         false,
		CostAtOptimalQty:            toJSONFloat64(math.NaN()), // Use toJSONFloat64 for initialization
		RevenueAtOptimalQty:         toJSONFloat64(math.NaN()),
		MaxProfit:                   toJSONFloat64(math.NaN()),
		TotalCycleTimeAtOptimalQty:  toJSONFloat64(math.NaN()),
		AcquisitionTimeAtOptimalQty: toJSONFloat64(math.NaN()),
		SaleTimeAtOptimalQty:        toJSONFloat64(math.NaN()),
	}

	maxFeasibleQty, errFeasible := findMaxQuantityForTimeConstraint(itemNameNorm, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, maxPossibleInitialQty)
	if errFeasible != nil {
		result.ErrorMessage = fmt.Sprintf("Error finding max feasible quantity: %v", errFeasible)
		if maxFeasibleQty == 0 {
			dualResultCheckQty1, _ := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir)
			if dualResultCheckQty1 != nil {
				result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds // Already JSONFloat64
				result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds                        // Already JSONFloat64
				result.RecipeTree = dualResultCheckQty1.PrimaryBased.RecipeTree
				if result.RecipeTree != nil {
					result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
				}

				acqTime := float64(result.AcquisitionTimeAtOptimalQty)
				saleTime := float64(result.SaleTimeAtOptimalQty)
				if !math.IsNaN(acqTime) && !math.IsNaN(saleTime) {
					result.TotalCycleTimeAtOptimalQty = toJSONFloat64(acqTime + saleTime)
				} // else TotalCycleTime remains NaN via toJSONFloat64(math.NaN())

				if dualResultCheckQty1.PrimaryBased.CalculationPossible {
					result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
					result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				}
				if result.ErrorMessage == "" {
					result.ErrorMessage = "Qty 1 check performed after optimizer error."
				} else {
					result.ErrorMessage += "; Qty 1 check performed."
				}
			}
		}
		return result
	}
	result.MaxFeasibleQuantity = maxFeasibleQty

	if result.MaxFeasibleQuantity <= 0 {
		dualResultCheckQty1, errCheckQty1 := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir)
		acqTimeRaw := math.Inf(1)
		saleTimeRaw := math.Inf(1)

		if errCheckQty1 != nil {
			if result.ErrorMessage == "" {
				result.ErrorMessage = fmt.Sprintf("Qty 1 check (PerformDualExpansion) failed: %v", errCheckQty1)
			}
		} else if dualResultCheckQty1 != nil {
			result.RecipeTree = dualResultCheckQty1.PrimaryBased.RecipeTree
			if result.RecipeTree != nil {
				result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
			}

			result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
			result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds

			acqVal := float64(result.AcquisitionTimeAtOptimalQty)
			saleVal := float64(result.SaleTimeAtOptimalQty)
			if !math.IsNaN(acqVal) {
				acqTimeRaw = acqVal
			}
			if !math.IsNaN(saleVal) {
				saleTimeRaw = saleVal
			}

			totalTimeRaw := acqTimeRaw + saleTimeRaw
			result.TotalCycleTimeAtOptimalQty = toJSONFloat64(totalTimeRaw)

			if dualResultCheckQty1.PrimaryBased.CalculationPossible {
				result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
				result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				if result.ErrorMessage == "" {
					if math.IsInf(totalTimeRaw, 0) || totalTimeRaw > maxAllowedFillTime {
						result.ErrorMessage = fmt.Sprintf("Quantity 1 total cycle time (Acq:%.2fs + Sale:%.2fs = %.2fs) exceeds max (%.2fs). Bottleneck: %s.", acqTimeRaw, saleTimeRaw, totalTimeRaw, maxAllowedFillTime, result.BottleneckIngredient)
					} else {
						result.ErrorMessage = "No feasible quantity > 0 found by optimizer."
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
				result.ErrorMessage = "No feasible quantity found and qty 1 check (PerformDualExpansion) also yielded no result."
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
		result.ErrorMessage = fmt.Sprintf("Dual expansion returned nil for optimal qty %.2f without error.", result.MaxFeasibleQuantity)
		return result
	}

	resP1Final := dualResultFinal.PrimaryBased
	result.RecipeTree = resP1Final.RecipeTree
	if result.RecipeTree != nil {
		result.MaxRecipeDepth = result.RecipeTree.MaxSubTreeDepth
	}

	if !resP1Final.CalculationPossible {
		result.ErrorMessage = fmt.Sprintf("PrimaryBased calculation not possible for optimal qty %.2f: %s", result.MaxFeasibleQuantity, resP1Final.ErrorMessage)
		result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
		result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds

		acqTimeRaw := float64(resP1Final.SlowestIngredientBuyTimeSeconds)
		saleTimeRaw := float64(dualResultFinal.TopLevelInstasellTimeSeconds)
		if !math.IsNaN(acqTimeRaw) && !math.IsNaN(saleTimeRaw) {
			result.TotalCycleTimeAtOptimalQty = toJSONFloat64(acqTimeRaw + saleTimeRaw)
		}

		result.BottleneckIngredient = resP1Final.SlowestIngredientName
		result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)
		return result
	}

	result.CostAtOptimalQty = resP1Final.TotalCost // Already JSONFloat64 from PerformDualExpansion
	result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
	result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds

	acqTimeFinalRaw := float64(result.AcquisitionTimeAtOptimalQty)
	saleTimeFinalRaw := float64(result.SaleTimeAtOptimalQty)
	if !math.IsNaN(acqTimeFinalRaw) && !math.IsNaN(saleTimeFinalRaw) {
		result.TotalCycleTimeAtOptimalQty = toJSONFloat64(acqTimeFinalRaw + saleTimeFinalRaw)
	}

	result.BottleneckIngredient = resP1Final.SlowestIngredientName
	result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)

	instasellPrice := getBuyPrice(apiResp, itemNameNorm)
	var revenueAtOptimalRaw float64 = 0
	var maxProfitRaw float64 = -math.Inf(1)
	costAtOptimalVal := float64(result.CostAtOptimalQty) // Cast for calculation

	if instasellPrice <= 0 || math.IsNaN(instasellPrice) || math.IsInf(instasellPrice, 0) {
		errMsgRevenue := fmt.Sprintf("Cannot get valid instasell price for %s (Price: %.2f). Revenue calculation failed.", itemNameNorm, instasellPrice)
		if result.ErrorMessage == "" {
			result.ErrorMessage = errMsgRevenue
		} else {
			result.ErrorMessage += "; " + errMsgRevenue
		}

		if !math.IsNaN(costAtOptimalVal) {
			maxProfitRaw = 0 - costAtOptimalVal
		} else {
			maxProfitRaw = math.NaN()
		}
	} else {
		revenueAtOptimalRaw = instasellPrice * result.MaxFeasibleQuantity
		if !math.IsNaN(costAtOptimalVal) {
			maxProfitRaw = revenueAtOptimalRaw - costAtOptimalVal
		} else {
			maxProfitRaw = math.NaN()
		}
	}
	result.RevenueAtOptimalQty = toJSONFloat64(revenueAtOptimalRaw)
	result.MaxProfit = toJSONFloat64(maxProfitRaw)
	result.CalculationPossible = true
	return result
}

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
		results = append(results, OptimizedItemResult{
			ItemName: "BATCH_ERROR", ErrorMessage: "Optimizer input error: API or Metrics data missing.", CalculationPossible: false,
			CostAtOptimalQty: toJSONFloat64(math.NaN()), RevenueAtOptimalQty: toJSONFloat64(math.NaN()), MaxProfit: toJSONFloat64(math.NaN()),
			TotalCycleTimeAtOptimalQty: toJSONFloat64(math.NaN()), AcquisitionTimeAtOptimalQty: toJSONFloat64(math.NaN()), SaleTimeAtOptimalQty: toJSONFloat64(math.NaN()),
		})
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
				ItemName: normalizedID, CalculationPossible: false, ErrorMessage: "Item not found in current Bazaar API data.",
				CostAtOptimalQty: toJSONFloat64(math.NaN()), RevenueAtOptimalQty: toJSONFloat64(math.NaN()), MaxProfit: toJSONFloat64(math.NaN()),
				TotalCycleTimeAtOptimalQty: toJSONFloat64(math.NaN()), AcquisitionTimeAtOptimalQty: toJSONFloat64(math.NaN()), SaleTimeAtOptimalQty: toJSONFloat64(math.NaN()),
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
		resI := results[i]
		resJ := results[j]
		if resI.CalculationPossible && !resJ.CalculationPossible {
			return true
		}
		if !resI.CalculationPossible && resJ.CalculationPossible {
			return false
		}

		profitI := float64(resI.MaxProfit)
		profitJ := float64(resJ.MaxProfit)

		isProfitINaN := math.IsNaN(profitI)
		isProfitJNaN := math.IsNaN(profitJ)
		if isProfitINaN && !isProfitJNaN {
			return false
		}
		if !isProfitINaN && isProfitJNaN {
			return true
		}
		if isProfitINaN && isProfitJNaN {
			return resI.ItemName < resJ.ItemName
		}
		if profitI != profitJ {
			return profitI > profitJ
		}
		return resI.ItemName < resJ.ItemName
	})

	dlog("Optimizer: Full optimization complete. Processed %d items.", len(results))
	return results
}
