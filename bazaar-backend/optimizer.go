package main

import (
	"fmt"
	"log"
	"math"
	"sort"
)

// OptimizedItemResult stores the result of optimizing a single item.
type OptimizedItemResult struct {
	ItemName                    string   `json:"item_name"`
	MaxFeasibleQuantity         float64  `json:"max_feasible_quantity"`
	CostAtOptimalQty            float64  `json:"cost_at_optimal_qty"`    // From P1
	RevenueAtOptimalQty         float64  `json:"revenue_at_optimal_qty"` // Based on instasell
	MaxProfit                   float64  `json:"max_profit"`
	TotalCycleTimeAtOptimalQty  float64  `json:"total_cycle_time_at_optimal_qty"`           // Sanitized sum of acquisition + sale time
	AcquisitionTimeAtOptimalQty *float64 `json:"acquisition_time_at_optimal_qty,omitempty"` // P1's SlowestIngredientBuyTimeSeconds (pointer)
	SaleTimeAtOptimalQty        *float64 `json:"sale_time_at_optimal_qty,omitempty"`        // TopLevelInstasellTimeSeconds (pointer)
	BottleneckIngredient        string   `json:"bottleneck_ingredient,omitempty"`           // P1's SlowestIngredientName
	BottleneckIngredientQty     float64  `json:"bottleneck_ingredient_qty"`                 // P1's SlowestIngredientQuantity
	CalculationPossible         bool     `json:"calculation_possible"`
	ErrorMessage                string   `json:"error_message,omitempty"`
}

// findMaxQuantityForTimeConstraint performs a binary search to find the max quantity
// of an item that can be crafted/acquired AND sold within the maxAllowedFillTime.
func findMaxQuantityForTimeConstraint(
	itemName string,
	maxAllowedFillTime float64, // in seconds
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
		high = low
	}
	bestQty := 0.0
	iterations := 0
	const maxIterations = 50

	for iterations < maxIterations && high >= low {
		iterations++
		midQty := math.Floor(low + (high-low)/2)
		if midQty < 1 {
			if low >= 1 {
				midQty = low
			} else {
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

		var acquisitionTimeValue float64 = math.Inf(1) // Default to Inf if nil
		if acquisitionTimePtr != nil {
			acquisitionTimeValue = *acquisitionTimePtr
		}
		var saleTimeValue float64 = math.Inf(1) // Default to Inf if nil
		if saleTimePtr != nil {
			saleTimeValue = *saleTimePtr
		}

		totalEffectiveTime := acquisitionTimeValue + saleTimeValue // Summing Inf with anything results in Inf
		// If either is NaN, sum will be NaN. math.IsInf will handle Inf correctly.
		// math.IsNaN(totalEffectiveTime) could also be a check for "too slow".

		dlog("  Optimizer Search: %s Qty %.2f - AcqTime: %.2fs, SaleTime: %.2fs, TotalEffTime: %.2fs (vs Max: %.2fs)",
			itemNameNorm, midQty, acquisitionTimeValue, saleTimeValue, totalEffectiveTime, maxAllowedFillTime)

		isTooSlow := false
		if math.IsNaN(totalEffectiveTime) || math.IsInf(totalEffectiveTime, 1) || totalEffectiveTime > maxAllowedFillTime {
			isTooSlow = true
		}

		if isTooSlow {
			high = midQty - 1
		} else {
			bestQty = midQty
			low = midQty + 1
		}
		if math.Abs(high-low) < 0.1 && iterations > maxIterations/2 {
			dlog("  Optimizer Search: Range very small (%.2f), breaking early.", math.Abs(high-low))
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
		// Attempt to get info for Qty=1 for better error reporting
		if maxFeasibleQty == 0 {
			dualResultCheckQty1, _ := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir)
			if dualResultCheckQty1 != nil {
				result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
				result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds

				var acqTime, saleTime float64 = math.Inf(1), math.Inf(1)
				if result.AcquisitionTimeAtOptimalQty != nil {
					acqTime = *result.AcquisitionTimeAtOptimalQty
				}
				if result.SaleTimeAtOptimalQty != nil {
					saleTime = *result.SaleTimeAtOptimalQty
				}
				result.TotalCycleTimeAtOptimalQty = sanitizeFloat(acqTime + saleTime)

				if dualResultCheckQty1.PrimaryBased.CalculationPossible {
					result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
					result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				}
			}
		}
		return result
	}
	result.MaxFeasibleQuantity = sanitizeFloat(maxFeasibleQty)

	if result.MaxFeasibleQuantity <= 0 {
		dualResultCheckQty1, errCheckQty1 := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir)
		var acqTime, saleTime, totalTime float64 = math.Inf(1), math.Inf(1), math.Inf(1)

		if errCheckQty1 != nil {
			if result.ErrorMessage == "" {
				result.ErrorMessage = fmt.Sprintf("Qty 1 check (PerformDualExpansion) failed: %v", errCheckQty1)
			}
		} else if dualResultCheckQty1 != nil {
			result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
			result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds
			if result.AcquisitionTimeAtOptimalQty != nil {
				acqTime = *result.AcquisitionTimeAtOptimalQty
			}
			if result.SaleTimeAtOptimalQty != nil {
				saleTime = *result.SaleTimeAtOptimalQty
			}
			totalTime = acqTime + saleTime
			result.TotalCycleTimeAtOptimalQty = sanitizeFloat(totalTime) // Sanitize for storage in float64 field

			if dualResultCheckQty1.PrimaryBased.CalculationPossible {
				result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
				result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				if result.ErrorMessage == "" {
					if math.IsNaN(totalTime) || math.IsInf(totalTime, 1) || totalTime > maxAllowedFillTime {
						result.ErrorMessage = fmt.Sprintf("Quantity 1 total cycle time (Acq:%.2fs + Sale:%.2fs = %.2fs) exceeds max (%.2fs). Bottleneck: %s.", acqTime, saleTime, totalTime, maxAllowedFillTime, result.BottleneckIngredient)
					} else {
						result.ErrorMessage = "No feasible quantity found by optimizer; Qty 1 was feasible by total cycle time but may not be profitable or other search issue."
					}
				}
			} else {
				if result.ErrorMessage == "" {
					errMsg := "P1 calculation failed for qty 1 check"
					if dualResultCheckQty1.PrimaryBased.ErrorMessage != "" {
						errMsg += ": " + dualResultCheckQty1.PrimaryBased.ErrorMessage
					}
					result.ErrorMessage = errMsg
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
		return result
	}

	resP1Final := dualResultFinal.PrimaryBased
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
		result.TotalCycleTimeAtOptimalQty = sanitizeFloat(acqTime + saleTime)

		result.BottleneckIngredient = resP1Final.SlowestIngredientName
		result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)
		return result
	}

	result.CostAtOptimalQty = sanitizeFloat(resP1Final.TotalCost)
	result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
	result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds

	var acqTimeFinal, saleTimeFinal float64 = math.Inf(1), math.Inf(1)
	if result.AcquisitionTimeAtOptimalQty != nil {
		acqTimeFinal = *result.AcquisitionTimeAtOptimalQty
	}
	if result.SaleTimeAtOptimalQty != nil {
		saleTimeFinal = *result.SaleTimeAtOptimalQty
	}
	result.TotalCycleTimeAtOptimalQty = sanitizeFloat(acqTimeFinal + saleTimeFinal) // Sanitize sum for storage

	result.BottleneckIngredient = resP1Final.SlowestIngredientName
	result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)

	instasellPrice := getBuyPrice(apiResp, itemNameNorm)
	if instasellPrice <= 0 || math.IsNaN(instasellPrice) || math.IsInf(instasellPrice, 0) {
		result.ErrorMessage = fmt.Sprintf("Cannot get valid instasell price for %s (Price: %.2f). Revenue calculation failed.", itemNameNorm, instasellPrice)
		result.RevenueAtOptimalQty = 0.0
		result.MaxProfit = sanitizeFloat(0.0 - result.CostAtOptimalQty)
		result.CalculationPossible = true
		return result
	}

	result.RevenueAtOptimalQty = sanitizeFloat(instasellPrice * result.MaxFeasibleQuantity)
	result.MaxProfit = sanitizeFloat(result.RevenueAtOptimalQty - result.CostAtOptimalQty)
	result.CalculationPossible = true

	logMsgAcqTime := "N/A (nil)"
	if result.AcquisitionTimeAtOptimalQty != nil {
		logMsgAcqTime = fmt.Sprintf("%.2fs", *result.AcquisitionTimeAtOptimalQty)
	}
	logMsgSaleTime := "N/A (nil)"
	if result.SaleTimeAtOptimalQty != nil {
		logMsgSaleTime = fmt.Sprintf("%.2fs", *result.SaleTimeAtOptimalQty)
	}

	dlog("Optimizer: Profit for %s @ qty %.2f: Cost=%.2f, Revenue=%.2f, Profit=%.2f. Bottleneck: %s (Qty: %.2f). Times: Acq=%s, Sale=%s, TotalCycleStoredSanitized=%.2fs",
		itemNameNorm, result.MaxFeasibleQuantity, result.CostAtOptimalQty, result.RevenueAtOptimalQty, result.MaxProfit,
		result.BottleneckIngredient, result.BottleneckIngredientQty,
		logMsgAcqTime, logMsgSaleTime, result.TotalCycleTimeAtOptimalQty)

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
		results = append(results, OptimizedItemResult{ItemName: "BATCH_ERROR", ErrorMessage: "Optimizer input error: API or Metrics data missing."})
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
			dlog("Optimizer: Item %s (Normalized: %s) not found in API product list, skipping.", itemID, normalizedID)
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

		result := optimizeItemProfit(itemID, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, currentMaxInitialQty)
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].CalculationPossible && !results[j].CalculationPossible {
			return true
		}
		if !results[i].CalculationPossible && results[j].CalculationPossible {
			return false
		}
		if results[i].CalculationPossible && results[j].CalculationPossible {
			return results[i].MaxProfit > results[j].MaxProfit
		}
		return results[i].ItemName < results[j].ItemName
	})

	dlog("Optimizer: Full optimization complete. Processed %d items.", len(results))
	return results
}
