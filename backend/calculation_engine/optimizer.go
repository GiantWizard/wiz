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
	RecipeTree                  *CraftingStepNode `json:"recipe_tree,omitempty"` // Will be nil in final JSON output
	MaxRecipeDepth              int               `json:"max_recipe_depth,omitempty"`
}

// Helper to safely get P1 error message from DualExpansionResult
func safeGetP1Error(dr *DualExpansionResult) string {
	if dr == nil {
		return "DualResult nil"
	}
	return dr.PrimaryBased.ErrorMessage
}

func findMaxQuantityForTimeConstraint(
	itemName string,
	maxAllowedFillTime float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleQty float64, // This is the initial upper bound for the search
) (float64, error) {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog("Optimizer: Finding max quantity for %s (Total Cycle Time Constraint: %.2f s, Initial Max Search Qty: %.2f)", itemNameNorm, maxAllowedFillTime, maxPossibleQty)

	if maxPossibleQty < 1.0 {
		dlog("  Optimizer Search: maxPossibleQty (%.2f) is less than 1. Cannot find feasible quantity. Returning 0.", maxPossibleQty)
		return 0.0, nil // No search possible or meaningful if upper bound is less than 1
	}

	low := 1.0
	high := math.Floor(maxPossibleQty) // Ensure high is an integer and within bounds
	if high < low {                    // If maxPossibleQty was < 1, high might be < low
		high = low
	}

	bestQty := 0.0 // Stores the highest quantity found so far that meets the time constraint
	iterations := 0
	const maxIterations = 50 // Limit iterations to prevent infinite loops in edge cases

	for iterations < maxIterations && high >= low {
		iterations++
		midQty := math.Floor(low + (high-low)/2) // Calculate midpoint, ensure integer
		if midQty < 1 {                          // Should not happen if low is 1, but defensive
			midQty = 1
		}

		// Convergence/Stuck check
		if iterations > 1 && midQty <= low && low >= high && bestQty == midQty {
			// If midQty is not advancing and is same as bestQty, likely converged
			dlog("  Optimizer Search: Converged or stuck at Low=%.0f, High=%.0f, MidQty=%.0f, BestQty=%.0f. Breaking.", low, high, midQty, bestQty)
			break
		}
		if midQty == low && midQty == high && iterations > 5 { // Stuck on a single value for too long
			dlog("  Optimizer Search: Stuck on MidQty=%.0f for several iterations. Breaking.", midQty)
			break
		}

		dlog("  Optimizer Search: Iter %d, Low=%.0f, High=%.0f, Testing MidQty=%.0f for %s", iterations, low, high, midQty, itemNameNorm)

		// Call PerformDualExpansion with includeTreeInExpansionResult = false (RAM Optimization)
		dualResult, err := PerformDualExpansion(itemNameNorm, midQty, apiResp, metricsMap, itemFilesDir, false)
		if err != nil {
			dlog("  Optimizer Search: Error in PerformDualExpansion for %s Qty %.0f: %v. Assuming time constraint exceeded (treat as too high).", itemNameNorm, midQty, err)
			high = midQty - 1 // Treat error as if it's too slow/costly
			continue
		}
		if dualResult == nil || !dualResult.PrimaryBased.CalculationPossible {
			errMsg := safeGetP1Error(dualResult)
			dlog("  Optimizer Search: P1 calculation not possible for %s Qty %.0f (ErrMsg: %s). Assuming time constraint exceeded.", itemNameNorm, midQty, errMsg)
			high = midQty - 1 // Treat as too slow/costly
			continue
		}

		acquisitionTimeRaw := float64(dualResult.PrimaryBased.SlowestIngredientBuyTimeSeconds)
		saleTimeRaw := float64(dualResult.TopLevelInstasellTimeSeconds)

		// Handle NaN times as Infinite for comparison
		if math.IsNaN(acquisitionTimeRaw) {
			acquisitionTimeRaw = math.Inf(1)
		}
		if math.IsNaN(saleTimeRaw) {
			saleTimeRaw = math.Inf(1)
		}
		totalEffectiveTime := acquisitionTimeRaw + saleTimeRaw
		dlog("  Optimizer Search: %s Qty %.0f - AcqTime: %.2fs, SaleTime: %.2fs, TotalEffTime: %.2fs (vs MaxAllowed: %.2fs)", itemNameNorm, midQty, acquisitionTimeRaw, saleTimeRaw, totalEffectiveTime, maxAllowedFillTime)

		if totalEffectiveTime <= maxAllowedFillTime && totalEffectiveTime >= 0 { // Check if it meets the constraint (and not negative infinity)
			bestQty = midQty // This quantity is feasible
			low = midQty + 1 // Try for a higher quantity
		} else { // Time constraint exceeded or invalid time
			high = midQty - 1 // Quantity is too high, try lower
		}
	}
	dlog("Optimizer: Best feasible quantity for %s (Total Cycle Time Constraint %.2f s): %.0f (after %d iterations)", itemNameNorm, maxAllowedFillTime, bestQty, iterations)
	return sanitizeFloat(bestQty), nil // SanitizeFloat will handle NaN/Inf if bestQty remained 0.0 (which is fine)
}

func optimizeItemProfit(
	itemName string,
	maxAllowedFillTime float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleInitialQty float64, // Initial upper bound for findMaxQuantityForTimeConstraint
) OptimizedItemResult {
	itemNameNorm := BAZAAR_ID(itemName)
	dlog("Optimizer: Optimizing profit for %s (Total Cycle Time Constraint: %.2fs, MaxInitialSearchQty: %.2f)", itemNameNorm, maxAllowedFillTime, maxPossibleInitialQty)

	result := OptimizedItemResult{
		ItemName:                    itemNameNorm,
		CalculationPossible:         false, // Default to false
		MaxFeasibleQuantity:         0,     // Default
		CostAtOptimalQty:            toJSONFloat64(math.NaN()),
		RevenueAtOptimalQty:         toJSONFloat64(math.NaN()),
		MaxProfit:                   toJSONFloat64(math.NaN()),
		TotalCycleTimeAtOptimalQty:  toJSONFloat64(math.NaN()),
		AcquisitionTimeAtOptimalQty: toJSONFloat64(math.NaN()),
		SaleTimeAtOptimalQty:        toJSONFloat64(math.NaN()),
		RecipeTree:                  nil, // IMPORTANT: Ensure RecipeTree is not stored in the final result
		MaxRecipeDepth:              0,   // Will be populated if tree is processed
	}

	// Step 1: Find the maximum feasible quantity under the time constraint
	maxFeasibleQty, errFeasible := findMaxQuantityForTimeConstraint(itemNameNorm, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, maxPossibleInitialQty)
	if errFeasible != nil {
		result.ErrorMessage = fmt.Sprintf("Error finding max feasible quantity: %v", errFeasible)
		// If findMaxQuantityForTimeConstraint itself errors, we might not have a qty.
		// Try to get Qty 1 data for context if maxFeasibleQty is 0 (or wasn't determined).
		if maxFeasibleQty == 0 { // Or if errFeasible implies no qty found
			// Call PerformDualExpansion with includeTreeInExpansionResult = true for Qty 1 check
			// to get MaxRecipeDepth and other details for the error report.
			dualResultCheckQty1, _ := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir, true)
			if dualResultCheckQty1 != nil {
				result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
				result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds
				if dualResultCheckQty1.PrimaryBased.RecipeTree != nil {
					result.MaxRecipeDepth = dualResultCheckQty1.PrimaryBased.RecipeTree.MaxSubTreeDepth
				}
				// Combine error messages
				acqTime := float64(result.AcquisitionTimeAtOptimalQty)
				saleTime := float64(result.SaleTimeAtOptimalQty)
				if !math.IsNaN(acqTime) && !math.IsNaN(saleTime) && (acqTime >= 0 && saleTime >= 0) { // ensure valid times
					result.TotalCycleTimeAtOptimalQty = toJSONFloat64(acqTime + saleTime)
				}

				if dualResultCheckQty1.PrimaryBased.CalculationPossible {
					result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
					result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				}
				currentMsg := result.ErrorMessage
				if currentMsg == "" {
					result.ErrorMessage = "Optimizer error during feasibility search; Qty 1 check performed."
				} else {
					result.ErrorMessage += "; Qty 1 check performed."
				}
				if dualResultCheckQty1.PrimaryBased.ErrorMessage != "" {
					result.ErrorMessage += " Qty1 P1 Err: " + dualResultCheckQty1.PrimaryBased.ErrorMessage
				}
			}
		}
		return result // Return with error message and any Qty 1 context
	}
	result.MaxFeasibleQuantity = maxFeasibleQty

	// Step 2: If no feasible quantity found (or 0), get details for Qty=1 for error reporting
	if result.MaxFeasibleQuantity <= 0 {
		dlog("Optimizer: No feasible quantity > 0 found for %s. Performing Qty=1 check for details.", itemNameNorm)
		// Call PerformDualExpansion with includeTreeInExpansionResult = true for Qty 1
		dualResultCheckQty1, errCheckQty1 := PerformDualExpansion(itemNameNorm, 1, apiResp, metricsMap, itemFilesDir, true)

		acqTimeRaw := math.Inf(1)  // Default to Inf
		saleTimeRaw := math.Inf(1) // Default to Inf

		if errCheckQty1 != nil {
			if result.ErrorMessage == "" {
				result.ErrorMessage = fmt.Sprintf("No feasible quantity found; Qty 1 check (PerformDualExpansion) also failed: %v", errCheckQty1)
			} else {
				result.ErrorMessage += fmt.Sprintf("; Qty 1 check (PerformDualExpansion) failed: %v", errCheckQty1)
			}
		} else if dualResultCheckQty1 != nil {
			// Populate details from Qty 1 check
			if dualResultCheckQty1.PrimaryBased.RecipeTree != nil {
				result.MaxRecipeDepth = dualResultCheckQty1.PrimaryBased.RecipeTree.MaxSubTreeDepth
			}
			result.AcquisitionTimeAtOptimalQty = dualResultCheckQty1.PrimaryBased.SlowestIngredientBuyTimeSeconds
			result.SaleTimeAtOptimalQty = dualResultCheckQty1.TopLevelInstasellTimeSeconds

			acqVal := float64(result.AcquisitionTimeAtOptimalQty)
			saleVal := float64(result.SaleTimeAtOptimalQty)
			if !math.IsNaN(acqVal) && acqVal >= 0 {
				acqTimeRaw = acqVal
			} // Only use if valid non-negative
			if !math.IsNaN(saleVal) && saleVal >= 0 {
				saleTimeRaw = saleVal
			}

			totalTimeRaw := acqTimeRaw + saleTimeRaw
			result.TotalCycleTimeAtOptimalQty = toJSONFloat64(totalTimeRaw) // Can be NaN if inputs are Inf

			if dualResultCheckQty1.PrimaryBased.CalculationPossible {
				result.BottleneckIngredient = dualResultCheckQty1.PrimaryBased.SlowestIngredientName
				result.BottleneckIngredientQty = sanitizeFloat(dualResultCheckQty1.PrimaryBased.SlowestIngredientQuantity)
				if result.ErrorMessage == "" { // Only set error if not already set by findMaxQuantity
					if math.IsInf(totalTimeRaw, 0) || totalTimeRaw > maxAllowedFillTime {
						result.ErrorMessage = fmt.Sprintf("Quantity 1 total cycle time (Acq:%.2fs + Sale:%.2fs = %.2fs) exceeds max (%.2fs). Bottleneck: %s.", acqTimeRaw, saleTimeRaw, totalTimeRaw, maxAllowedFillTime, result.BottleneckIngredient)
					} else {
						result.ErrorMessage = "No feasible quantity > 0 found by optimizer (detailed Qty 1 check complete)."
					}
				}
			} else { // P1 calculation not possible for Qty 1
				if result.ErrorMessage == "" {
					result.ErrorMessage = "P1 calculation failed for Qty 1 check"
				}
				if dualResultCheckQty1.PrimaryBased.ErrorMessage != "" {
					result.ErrorMessage += ": " + dualResultCheckQty1.PrimaryBased.ErrorMessage
				}
			}
		} else { // dualResultCheckQty1 was nil
			if result.ErrorMessage == "" {
				result.ErrorMessage = "No feasible quantity found and Qty 1 check (PerformDualExpansion) also yielded nil result."
			}
		}
		return result // Return with Qty 1 context
	}

	// Step 3: Max feasible quantity > 0. Perform final expansion for this quantity.
	dlog("Optimizer: Max feasible quantity for %s is %.2f. Performing final expansion.", itemNameNorm, result.MaxFeasibleQuantity)
	// Call PerformDualExpansion with includeTreeInExpansionResult = true to get MaxRecipeDepth
	dualResultFinal, errExpansionFinal := PerformDualExpansion(itemNameNorm, result.MaxFeasibleQuantity, apiResp, metricsMap, itemFilesDir, true)

	if errExpansionFinal != nil {
		result.ErrorMessage = fmt.Sprintf("Error performing dual expansion for optimal qty %.2f: %v", result.MaxFeasibleQuantity, errExpansionFinal)
		if dualResultFinal != nil && dualResultFinal.PrimaryBased.RecipeTree != nil { // Still try to get depth
			result.MaxRecipeDepth = dualResultFinal.PrimaryBased.RecipeTree.MaxSubTreeDepth
		}
		return result
	}
	if dualResultFinal == nil { // Should not happen if no error, but defensive
		result.ErrorMessage = fmt.Sprintf("Dual expansion returned nil for optimal qty %.2f without error.", result.MaxFeasibleQuantity)
		return result
	}

	// Process the final expansion result (PrimaryBased perspective)
	resP1Final := dualResultFinal.PrimaryBased
	if resP1Final.RecipeTree != nil {
		result.MaxRecipeDepth = resP1Final.RecipeTree.MaxSubTreeDepth
	}
	// result.RecipeTree remains nil for the OptimizedItemResult struct itself.

	if !resP1Final.CalculationPossible {
		result.ErrorMessage = fmt.Sprintf("PrimaryBased calculation not possible for optimal qty %.2f: %s", result.MaxFeasibleQuantity, resP1Final.ErrorMessage)
		// Populate times and bottleneck info even if calculation wasn't fully possible
		result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
		result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds // Use top-level sale time

		acqTimeRaw := float64(resP1Final.SlowestIngredientBuyTimeSeconds)
		saleTimeRaw := float64(dualResultFinal.TopLevelInstasellTimeSeconds)
		if !math.IsNaN(acqTimeRaw) && !math.IsNaN(saleTimeRaw) && (acqTimeRaw >= 0 && saleTimeRaw >= 0) {
			result.TotalCycleTimeAtOptimalQty = toJSONFloat64(acqTimeRaw + saleTimeRaw)
		}
		result.BottleneckIngredient = resP1Final.SlowestIngredientName
		result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)
		return result
	}

	// Populate result with data from the successful final expansion
	result.CostAtOptimalQty = resP1Final.TotalCost // This is already JSONFloat64
	result.AcquisitionTimeAtOptimalQty = resP1Final.SlowestIngredientBuyTimeSeconds
	result.SaleTimeAtOptimalQty = dualResultFinal.TopLevelInstasellTimeSeconds

	acqTimeFinalRaw := float64(result.AcquisitionTimeAtOptimalQty)
	saleTimeFinalRaw := float64(result.SaleTimeAtOptimalQty)
	if !math.IsNaN(acqTimeFinalRaw) && !math.IsNaN(saleTimeFinalRaw) && (acqTimeFinalRaw >= 0 && saleTimeFinalRaw >= 0) {
		result.TotalCycleTimeAtOptimalQty = toJSONFloat64(acqTimeFinalRaw + saleTimeFinalRaw)
	} else {
		result.TotalCycleTimeAtOptimalQty = toJSONFloat64(math.NaN()) // Ensure NaN if components are invalid
	}

	result.BottleneckIngredient = resP1Final.SlowestIngredientName
	result.BottleneckIngredientQty = sanitizeFloat(resP1Final.SlowestIngredientQuantity)

	// Calculate revenue and profit
	instasellPrice := getBuyPrice(apiResp, itemNameNorm) // Instasell price is buy price from API perspective
	var revenueAtOptimalRaw float64 = 0.0
	var maxProfitRaw float64 = -math.Inf(1)              // Default to very low profit
	costAtOptimalVal := float64(result.CostAtOptimalQty) // Convert JSONFloat64 to float64 for calculation

	if instasellPrice <= 0 || math.IsNaN(instasellPrice) || math.IsInf(instasellPrice, 0) {
		errMsgRevenue := fmt.Sprintf("Cannot get valid instasell price for %s (Price: %.2f). Revenue/Profit calculation failed.", itemNameNorm, instasellPrice)
		if result.ErrorMessage == "" {
			result.ErrorMessage = errMsgRevenue
		} else {
			result.ErrorMessage += "; " + errMsgRevenue
		}
		// Profit is -Cost if revenue is 0 or undefined
		if !math.IsNaN(costAtOptimalVal) {
			maxProfitRaw = 0 - costAtOptimalVal
		} else {
			maxProfitRaw = math.NaN() // If cost is also NaN, profit is NaN
		}
	} else {
		revenueAtOptimalRaw = instasellPrice * result.MaxFeasibleQuantity
		if !math.IsNaN(costAtOptimalVal) { // Ensure cost is a valid number
			maxProfitRaw = revenueAtOptimalRaw - costAtOptimalVal
		} else { // Cost was NaN
			maxProfitRaw = math.NaN() // Profit becomes NaN if cost is NaN
		}
	}
	result.RevenueAtOptimalQty = toJSONFloat64(revenueAtOptimalRaw)
	result.MaxProfit = toJSONFloat64(maxProfitRaw)
	result.CalculationPossible = true // Mark as successful if we reach here with valid numbers

	if result.ErrorMessage != "" { // If there was a non-fatal error message accumulated
		dlog("Optimizer: %s finished with non-fatal error: %s", itemNameNorm, result.ErrorMessage)
	}

	return result
}

func RunFullOptimization(
	itemIDs []string,
	maxAllowedFillTime float64, // Max total cycle time (acquisition + sale)
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
	itemFilesDir string,
	maxPossibleInitialQtyPerItem float64, // Max quantity for initial search in findMaxQuantityForTimeConstraint
) []OptimizedItemResult {
	dlog("Optimizer: Starting full optimization for %d items. Total Cycle Time Constraint: %.2fs, Max Initial Search Qty: %.2f", len(itemIDs), maxAllowedFillTime, maxPossibleInitialQtyPerItem)
	var results []OptimizedItemResult

	if apiResp == nil || metricsMap == nil {
		log.Println("ERROR (Optimizer): API response or metrics map is nil. Cannot run full optimization.")
		// Return a single error result to indicate batch failure
		results = append(results, OptimizedItemResult{
			ItemName: "BATCH_OPTIMIZATION_ERROR", ErrorMessage: "Optimizer input error: API response or Metrics map was nil.", CalculationPossible: false,
			CostAtOptimalQty: toJSONFloat64(math.NaN()), RevenueAtOptimalQty: toJSONFloat64(math.NaN()), MaxProfit: toJSONFloat64(math.NaN()),
			TotalCycleTimeAtOptimalQty: toJSONFloat64(math.NaN()), AcquisitionTimeAtOptimalQty: toJSONFloat64(math.NaN()), SaleTimeAtOptimalQty: toJSONFloat64(math.NaN()),
			RecipeTree: nil, // Ensure nil
		})
		return results
	}
	if len(itemIDs) == 0 {
		log.Println("Optimizer: No item IDs provided for optimization.")
		return results // Return empty slice, not an error
	}

	for i, itemID := range itemIDs {
		dlog("Optimizer: Optimizing item %d/%d: %s", i+1, len(itemIDs), itemID)
		normalizedID := BAZAAR_ID(itemID) // Normalize ID

		// Check if item exists in API data (quick sanity check)
		if _, exists := apiResp.Products[normalizedID]; !exists {
			dlog("Optimizer: Item %s (Normalized: %s) not found in API product list for this run, skipping.", itemID, normalizedID)
			results = append(results, OptimizedItemResult{
				ItemName: normalizedID, CalculationPossible: false, ErrorMessage: "Item not found in current Bazaar API data.",
				MaxFeasibleQuantity: 0,
				CostAtOptimalQty:    toJSONFloat64(math.NaN()), RevenueAtOptimalQty: toJSONFloat64(math.NaN()), MaxProfit: toJSONFloat64(math.NaN()),
				TotalCycleTimeAtOptimalQty: toJSONFloat64(math.NaN()), AcquisitionTimeAtOptimalQty: toJSONFloat64(math.NaN()), SaleTimeAtOptimalQty: toJSONFloat64(math.NaN()),
				RecipeTree: nil, // Ensure nil
			})
			continue
		}

		currentMaxInitialQty := maxPossibleInitialQtyPerItem
		if currentMaxInitialQty <= 0 { // Safety for this parameter
			currentMaxInitialQty = 1000000.0 // Default large search quantity
			dlog("Optimizer: maxPossibleInitialQtyPerItem was <=0, using default %.2f for %s", currentMaxInitialQty, normalizedID)
		}

		// optimizeItemProfit now handles RAM for RecipeTree internally
		result := optimizeItemProfit(normalizedID, maxAllowedFillTime, apiResp, metricsMap, itemFilesDir, currentMaxInitialQty)
		results = append(results, result)
	}

	// Sort results: CalculationPossible=true first, then by MaxProfit descending.
	sort.Slice(results, func(i, j int) bool {
		resI := results[i]
		resJ := results[j]

		// Prioritize CalculationPossible = true
		if resI.CalculationPossible && !resJ.CalculationPossible {
			return true // resI comes first
		}
		if !resI.CalculationPossible && resJ.CalculationPossible {
			return false // resJ comes first
		}
		// If both have same CalculationPossible status (either both true or both false)

		profitI := float64(resI.MaxProfit) // Convert JSONFloat64 to float64 for comparison
		profitJ := float64(resJ.MaxProfit)

		isProfitINaN := math.IsNaN(profitI)
		isProfitJNaN := math.IsNaN(profitJ)

		// Handle NaN profits: NaNs go to the end when sorting descending
		if isProfitINaN && !isProfitJNaN {
			return false // I is NaN, J is not -> J comes before I (I goes to end)
		}
		if !isProfitINaN && isProfitJNaN {
			return true // I is not NaN, J is -> I comes before J (J goes to end)
		}
		if isProfitINaN && isProfitJNaN { // Both are NaN
			return resI.ItemName < resJ.ItemName // Tie-break by name
		}

		// Both profits are valid numbers, sort by profit descending
		if profitI != profitJ {
			return profitI > profitJ
		}

		// If profits are equal (and CalculationPossible is same), tie-break by item name ascending
		return resI.ItemName < resJ.ItemName
	})

	dlog("Optimizer: Full optimization complete. Processed %d items, generated %d results.", len(itemIDs), len(results))
	return results
}
