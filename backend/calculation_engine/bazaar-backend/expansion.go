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
	BaseIngredients     map[string]BaseIngredientDetail `json:"base_ingredients"`
	TotalCost           float64                         `json:"total_cost"`
	PerspectiveType     string                          `json:"perspective_type"`
	TopLevelAction      string                          `json:"top_level_action"`
	FinalCostMethod     string                          `json:"final_cost_method"`
	CalculationPossible bool                            `json:"calculation_possible"`
	ErrorMessage        string                          `json:"error_message,omitempty"`
	TopLevelCost        float64                         `json:"top_level_cost"`
	TopLevelRR          *float64                        `json:"top_level_rr,omitempty"`
}
type DualExpansionResult struct {
	ItemName       string          `json:"item_name"`
	Quantity       float64         `json:"quantity"`
	PrimaryBased   ExpansionResult `json:"primary_based"`
	SecondaryBased ExpansionResult `json:"secondary_based"`
}

// Helper
func float64Ptr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// --- Cost Helper (calculateDetailedCosts - Sums BestCost for ingredients) ---
// This function remains the same, as it correctly sums the optimal C10M costs of ingredients.
func calculateDetailedCosts(baseMapInput map[string]float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) (float64, map[string]BaseIngredientDetail, bool, string) {
	sumOfBestCosts := 0.0
	possible := true
	errMsg := ""
	detailedMap := make(map[string]BaseIngredientDetail)
	if len(baseMapInput) == 0 {
		return 0.0, detailedMap, true, ""
	}
	for itemID, quantity := range baseMapInput {
		if quantity <= 0 {
			continue
		}
		bestCost, method, assocCost, rr, errC10M := getBestC10M(itemID, quantity, apiResp, metricsMap)
		ingredientDetail := BaseIngredientDetail{Quantity: quantity, BestCost: 0.0, AssociatedCost: 0.0, Method: "N/A", RR: nil}
		if errC10M != nil || method == "N/A" || math.IsInf(bestCost, 0) || math.IsNaN(bestCost) || bestCost < 0 {
			errMsg = fmt.Sprintf("Cannot determine valid BEST cost for base ingredient '%s': BestC:%.2f, Err: %v", itemID, bestCost, errC10M)
			dlog("  WARN (calculateDetailedCosts - Best): %s", errMsg)
			possible = false
			sumOfBestCosts = math.Inf(1)
			detailedMap[itemID] = ingredientDetail
			break
		} else {
			sumOfBestCosts += bestCost
			ingredientDetail.BestCost = sanitizeFloat(bestCost)
			ingredientDetail.AssociatedCost = sanitizeFloat(assocCost)
			ingredientDetail.Method = method
			if method == "Primary" {
				ingredientDetail.RR = float64Ptr(rr)
			}
			detailedMap[itemID] = ingredientDetail
		}
	}
	if !possible {
		return 0.0, detailedMap, false, errMsg
	}
	return sanitizeFloat(sumOfBestCosts), detailedMap, true, ""
}

// --- Orchestration Function (UPDATED for new P1/P2 decision logic) ---
func PerformDualExpansion(itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string) (*DualExpansionResult, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s <<<", quantity, itemNameNorm)
	result := &DualExpansionResult{ItemName: itemNameNorm, Quantity: quantity, PrimaryBased: ExpansionResult{PerspectiveType: "PrimaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail), TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil}, SecondaryBased: ExpansionResult{PerspectiveType: "SecondaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail), TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil}}

	// --- 1. Pre-calculate Top-Level Direct Acquisition Costs (Primary & Secondary C10M) ---
	topC10mPrim, topC10mSec, _, topRR, _, _, errTopC10M := calculateC10MInternal(
		itemNameNorm, quantity, getSellPrice(apiResp, itemNameNorm), getBuyPrice(apiResp, itemNameNorm), getMetrics(metricsMap, itemNameNorm),
	)
	// Store these as the "(Top-Level ... Cost)" for display, sanitized
	result.PrimaryBased.TopLevelCost = sanitizeFloat(topC10mPrim)  // This is the benchmark for P1 decision
	result.SecondaryBased.TopLevelCost = sanitizeFloat(topC10mSec) // This is the benchmark for P2 decision
	if !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0 {
		result.PrimaryBased.TopLevelRR = float64Ptr(topRR)
	}
	// P2 doesn't have a top-level RR in this model, as its benchmark is Secondary C10M
	result.SecondaryBased.TopLevelRR = nil

	topLevelRecipeExists := false
	var topLevelRecipeCheckErr error
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); statErr == nil {
		topLevelRecipeExists = true
	} else if !os.IsNotExist(statErr) {
		topLevelRecipeCheckErr = fmt.Errorf("checking recipe file '%s': %w", filePath, statErr)
	}

	if topLevelRecipeCheckErr != nil { /* Critical error checking recipe, fail both */
		errMsg := fmt.Sprintf("Failed to check top-level recipe: %v", topLevelRecipeCheckErr)
		result.PrimaryBased.ErrorMessage = errMsg
		result.SecondaryBased.ErrorMessage = errMsg
		result.PrimaryBased.TopLevelAction = "Unknown"
		result.SecondaryBased.TopLevelAction = "Unknown"
		return result, nil
	}
	dlog("  Top-Level C10M: Primary=%.2f, Secondary=%.2f, TopRR=%.2f. Recipe Exists: %v", topC10mPrim, topC10mSec, topRR, topLevelRecipeExists)

	// --- 2. Calculate Cost to Craft (Optimal Ingredients) - Done ONCE if recipe exists ---
	var costToCraftOptimal float64 = math.Inf(1)
	var baseIngredientsFromCraft map[string]BaseIngredientDetail // Will hold detailed ingredients if crafting happens
	var craftPossible bool = false
	var craftErrMsg string = ""

	if topLevelRecipeExists {
		baseMapQtyOnly, errExpand := ExpandItem(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)
		if errExpand != nil {
			craftErrMsg = fmt.Sprintf("Expansion failed: %v", errExpand)
			log.Printf("WARN (PerformDualExpansion): ExpandItem failed for %s: %v", itemNameNorm, errExpand)
			// craftPossible remains false
		} else if len(baseMapQtyOnly) == 0 { // Cycled back to itself with no other ingredients
			craftErrMsg = "Expansion resulted in empty map (likely cycles to self)"
			// In this case, "cost to craft" is effectively infinite as it can't be made from other things.
			// Or, we can represent it as the item requiring itself, which means cost is its own acquisition cost.
			// For comparison, let's keep costToCraftOptimal as Inf.
			dlog("  Crafting resulted in empty base map for %s (cycles).", itemNameNorm)
			baseIngredientsFromCraft = map[string]BaseIngredientDetail{
				itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A (Cycle)", RR: nil},
			} // Show itself as base
			// craftPossible remains false for summing costs, but true for showing base item.
		} else {
			// Successfully expanded, now get the sum of optimal costs for these ingredients
			var detailedMapTemp map[string]BaseIngredientDetail
			costToCraftOptimal, detailedMapTemp, craftPossible, craftErrMsg = calculateDetailedCosts(baseMapQtyOnly, apiResp, metricsMap)
			if craftPossible {
				baseIngredientsFromCraft = detailedMapTemp
				dlog("  Cost to Craft (Optimal Ingredients) for %s: %.2f", itemNameNorm, costToCraftOptimal)
			} else {
				dlog("  Failed to calculate detailed costs for crafting %s: %s", itemNameNorm, craftErrMsg)
				// baseIngredientsFromCraft might still have some data from calculateDetailedCosts if it partially failed
				if detailedMapTemp != nil {
					baseIngredientsFromCraft = detailedMapTemp
				}
			}
		}
	} else {
		craftErrMsg = "No recipe found for top-level item."
		dlog("  No recipe for %s, cannot calculate cost to craft.", itemNameNorm)
		// craftPossible remains false
	}

	// --- 3. Process Perspective 1 (Primary Benchmark) ---
	dlog("--- Running Perspective 1 (Benchmark: Top-Level Primary C10M) ---")
	res1 := &result.PrimaryBased
	res1.FinalCostMethod = "SumBestC10M" // Default if crafting
	isApiNotFoundErrorP1 := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")
	validTopC10mPrim := errTopC10M == nil && !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0

	// Decision for P1:
	if isApiNotFoundErrorP1 && topLevelRecipeExists && craftPossible { // Not on bazaar, but can craft
		dlog("  P1 Decision: Expand (Not on Bazaar, recipe & craft possible)")
		res1.TopLevelAction = "Expanded"
		res1.BaseIngredients = baseIngredientsFromCraft
		res1.TotalCost = costToCraftOptimal
		res1.CalculationPossible = true
	} else if isApiNotFoundErrorP1 && topLevelRecipeExists && !craftPossible { // Not on bazaar, recipe exists, but craft cost failed
		dlog("  P1 Decision: ExpansionFailed (Not on Bazaar, recipe exists, craft cost failed)")
		res1.TopLevelAction = "ExpansionFailed"
		res1.BaseIngredients = baseIngredientsFromCraft // Show whatever ingredients were found
		res1.ErrorMessage = craftErrMsg
		res1.TotalCost = 0.0 // Or Inf? Depends on how we want to show total.
		res1.CalculationPossible = false
	} else if isApiNotFoundErrorP1 && !topLevelRecipeExists { // Not on bazaar, no recipe
		dlog("  P1 Decision: TreatedAsBase (Not on Bazaar, no recipe)")
		res1.TopLevelAction = "TreatedAsBase (No Recipe)"
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		res1.ErrorMessage = "Item not on Bazaar and no recipe found."
		res1.TotalCost = 0.0 // Unobtainable
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
	} else if !validTopC10mPrim && topLevelRecipeExists && craftPossible { // Primary C10M invalid, but can craft
		dlog("  P1 Decision: Expand (Primary C10M invalid, but craft possible)")
		res1.TopLevelAction = "Expanded"
		res1.BaseIngredients = baseIngredientsFromCraft
		res1.TotalCost = costToCraftOptimal
		res1.CalculationPossible = true
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "Top-level Primary C10M invalid, using craft cost."
		}
	} else if !validTopC10mPrim && (!topLevelRecipeExists || !craftPossible) { // Primary C10M invalid, cannot craft
		dlog("  P1 Decision: TreatedAsBase (Primary C10M invalid, cannot craft)")
		res1.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = fmt.Sprintf("Top-level Primary C10M invalid (%.2f) and cannot craft.", topC10mPrim)
		}
		res1.TotalCost = 0.0
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
	} else if craftPossible && costToCraftOptimal <= topC10mPrim { // Crafting is possible and cheaper/equal to Primary C10M
		dlog("  P1 Decision: Expand (Craft cost %.2f <= Primary C10M %.2f)", costToCraftOptimal, topC10mPrim)
		res1.TopLevelAction = "Expanded"
		res1.BaseIngredients = baseIngredientsFromCraft
		res1.TotalCost = costToCraftOptimal
		res1.CalculationPossible = true
	} else { // Buying via Primary C10M is cheaper, or crafting not possible/more expensive
		dlog("  P1 Decision: TreatedAsBase (Primary C10M %.2f is best or crafting not viable)", topC10mPrim)
		res1.TopLevelAction = "TreatedAsBase"
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: getSellPrice(apiResp, itemNameNorm) * quantity, Method: "Primary", RR: float64Ptr(topRR)}}
		res1.TotalCost = sanitizeFloat(topC10mPrim)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelPrimary"
	}

	// --- 4. Process Perspective 2 (Secondary Benchmark) ---
	dlog("--- Running Perspective 2 (Benchmark: Top-Level Secondary C10M) ---")
	res2 := &result.SecondaryBased
	res2.FinalCostMethod = "SumBestC10M" // Default if crafting
	validTopC10mSec := errTopC10M == nil && !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0

	// Decision for P2:
	if isApiNotFoundErrorP1 && topLevelRecipeExists && craftPossible { // Not on bazaar, but can craft (same as P1 here)
		dlog("  P2 Decision: Expand (Not on Bazaar, recipe & craft possible)")
		res2.TopLevelAction = "Expanded"
		res2.BaseIngredients = baseIngredientsFromCraft
		res2.TotalCost = costToCraftOptimal
		res2.CalculationPossible = true
	} else if isApiNotFoundErrorP1 && topLevelRecipeExists && !craftPossible {
		dlog("  P2 Decision: ExpansionFailed (Not on Bazaar, recipe exists, craft cost failed)")
		res2.TopLevelAction = "ExpansionFailed"
		res2.BaseIngredients = baseIngredientsFromCraft // Show whatever ingredients were found
		res2.ErrorMessage = craftErrMsg
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
	} else if isApiNotFoundErrorP1 && !topLevelRecipeExists {
		dlog("  P2 Decision: TreatedAsBase (Not on Bazaar, no recipe)")
		res2.TopLevelAction = "TreatedAsBase (No Recipe)"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		res2.ErrorMessage = "Item not on Bazaar and no recipe found."
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
	} else if !validTopC10mSec && topLevelRecipeExists && craftPossible { // Secondary C10M invalid, but can craft
		dlog("  P2 Decision: Expand (Secondary C10M invalid, but craft possible)")
		res2.TopLevelAction = "Expanded"
		res2.BaseIngredients = baseIngredientsFromCraft
		res2.TotalCost = costToCraftOptimal
		res2.CalculationPossible = true
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = "Top-level Secondary C10M invalid, using craft cost."
		}
	} else if !validTopC10mSec && (!topLevelRecipeExists || !craftPossible) { // Secondary C10M invalid, cannot craft
		dlog("  P2 Decision: TreatedAsBase (Secondary C10M invalid, cannot craft)")
		res2.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = fmt.Sprintf("Top-level Secondary C10M invalid (%.2f) and cannot craft.", topC10mSec)
		}
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
	} else if craftPossible && costToCraftOptimal <= topC10mSec { // Crafting is possible and cheaper/equal to Secondary C10M
		dlog("  P2 Decision: Expand (Craft cost %.2f <= Secondary C10M %.2f)", costToCraftOptimal, topC10mSec)
		res2.TopLevelAction = "Expanded"
		res2.BaseIngredients = baseIngredientsFromCraft
		res2.TotalCost = costToCraftOptimal
		res2.CalculationPossible = true
	} else { // Buying via Secondary C10M is cheaper, or crafting not viable/more expensive
		dlog("  P2 Decision: TreatedAsBase (Secondary C10M %.2f is best or crafting not viable)", topC10mSec)
		res2.TopLevelAction = "TreatedAsBase"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: getBuyPrice(apiResp, itemNameNorm) * quantity, Method: "Secondary", RR: nil}}
		res2.TotalCost = sanitizeFloat(topC10mSec)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelSecondary"
	}

	// Final sanitization
	res1.TotalCost = sanitizeFloat(res1.TotalCost)
	res2.TotalCost = sanitizeFloat(res2.TotalCost)

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
