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

// --- Cost Helper ---
func calculateDetailedCosts(baseMapInput map[string]float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) (float64, map[string]BaseIngredientDetail, bool, string) { /* ... Same as before ... */
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

// --- Orchestration Function (UPDATED P2 LOGIC) ---
func PerformDualExpansion(itemName string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics, itemFilesDir string) (*DualExpansionResult, error) {

	itemNameNorm := BAZAAR_ID(itemName)
	dlog(">>> Performing Dual Expansion for %.2f x %s <<<", quantity, itemNameNorm)
	result := &DualExpansionResult{ItemName: itemNameNorm, Quantity: quantity, PrimaryBased: ExpansionResult{PerspectiveType: "PrimaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail), TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil}, SecondaryBased: ExpansionResult{PerspectiveType: "SecondaryBased", CalculationPossible: false, BaseIngredients: make(map[string]BaseIngredientDetail), TotalCost: 0.0, TopLevelCost: 0.0, TopLevelRR: nil}}

	// --- 1. Pre-calculate Top-Level Costs & Check Recipe ---
	topC10mPrim, topC10mSec, _, topRR, _, _, errTopC10M := calculateC10MInternal( /* ... params ... */
		itemNameNorm, quantity, getSellPrice(apiResp, itemNameNorm), getBuyPrice(apiResp, itemNameNorm), getMetrics(metricsMap, itemNameNorm),
	)
	result.PrimaryBased.TopLevelCost = sanitizeFloat(topC10mPrim)
	result.SecondaryBased.TopLevelCost = sanitizeFloat(topC10mSec) // Store P2 benchmark cost
	validTopC10mPrim := errTopC10M == nil && !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0
	if validTopC10mPrim {
		result.PrimaryBased.TopLevelRR = float64Ptr(topRR)
	}
	result.SecondaryBased.TopLevelRR = nil // P2 doesn't use primary RR as its defining cost

	topLevelRecipeExists := false
	var topLevelRecipeCheckErr error
	filePath := filepath.Join(itemFilesDir, itemNameNorm+".json")
	if _, statErr := os.Stat(filePath); statErr == nil {
		topLevelRecipeExists = true
	} else if !os.IsNotExist(statErr) {
		topLevelRecipeCheckErr = fmt.Errorf("checking recipe file '%s': %w", filePath, statErr)
	}
	if topLevelRecipeCheckErr != nil { /* Critical error */
		errMsg := fmt.Sprintf("Failed to check top-level recipe: %v", topLevelRecipeCheckErr)
		result.PrimaryBased.ErrorMessage = errMsg
		result.SecondaryBased.ErrorMessage = errMsg
		result.PrimaryBased.TopLevelAction = "Unknown"
		result.SecondaryBased.TopLevelAction = "Unknown"
		return result, nil
	}
	dlog("  Top-Level C10M: Primary=%.2f, Secondary=%.2f, TopRR=%.2f. Recipe Exists: %v", topC10mPrim, topC10mSec, topRR, topLevelRecipeExists)
	isApiNotFoundError := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")
	validTopC10mSec := errTopC10M == nil && !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0

	// --- 2. Calculate Cost to Craft (Optimal Ingredients) - Done ONCE if recipe exists ---
	var costToCraftOptimal float64 = math.Inf(1)
	var baseIngredientsFromCraft map[string]BaseIngredientDetail
	var craftPossible bool = false
	var craftErrMsg string = ""
	var craftResultedInCycle bool = false

	if topLevelRecipeExists {
		baseMapQtyOnly, errExpand := ExpandItem(itemNameNorm, quantity, apiResp, metricsMap, itemFilesDir)
		if errExpand != nil {
			craftErrMsg = fmt.Sprintf("Expansion failed: %v", errExpand)
			log.Printf("WARN (PerformDualExpansion): ExpandItem failed for %s: %v", itemNameNorm, errExpand)
		} else if len(baseMapQtyOnly) == 0 {
			craftErrMsg = "Expansion resulted in empty map (likely cycles to self)"
			craftResultedInCycle = true
			costToCraftOptimal = math.Inf(1)
			baseIngredientsFromCraft = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A (Cycle)", RR: nil}}
		} else {
			var detailedMapTemp map[string]BaseIngredientDetail
			costToCraftOptimal, detailedMapTemp, craftPossible, craftErrMsg = calculateDetailedCosts(baseMapQtyOnly, apiResp, metricsMap)
			if craftPossible {
				baseIngredientsFromCraft = detailedMapTemp
				dlog("  Cost to Craft (Optimal Ingredients) for %s: %.2f", itemNameNorm, costToCraftOptimal)
			} else {
				dlog("  Failed to calculate detailed costs for crafting %s: %s", itemNameNorm, craftErrMsg)
				if detailedMapTemp != nil {
					baseIngredientsFromCraft = detailedMapTemp
				}
			}
		}
	} else {
		craftErrMsg = "No recipe found for top-level item."
		dlog("  No recipe for %s, cannot calculate cost to craft.", itemNameNorm)
	}

	// --- 3. Process Perspective 1 (Primary Benchmark - Absolute Minimum) ---
	dlog("--- Running Perspective 1 (Benchmark: Absolute Minimum Cost) ---")
	res1 := &result.PrimaryBased
	minCostP1 := math.Inf(1)
	chosenMethodP1 := "N/A"

	// Consider valid costs only
	if craftPossible && costToCraftOptimal < minCostP1 {
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
	} else if chosenMethodP1 == "Primary" {
		res1.TopLevelAction = "TreatedAsBase"
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(getSellPrice(apiResp, itemNameNorm) * quantity), Method: "Primary", RR: float64Ptr(topRR)}}
		res1.TotalCost = sanitizeFloat(topC10mPrim)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelPrimary"
	} else if chosenMethodP1 == "Secondary" {
		res1.TopLevelAction = "TreatedAsBase"
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(getBuyPrice(apiResp, itemNameNorm) * quantity), Method: "Secondary", RR: nil}}
		res1.TotalCost = sanitizeFloat(topC10mSec)
		res1.CalculationPossible = true
		res1.FinalCostMethod = "FixedTopLevelSecondary"
	} else { // No valid option
		res1.TopLevelAction = "TreatedAsBase (Unobtainable)"
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "No valid acquisition method found"
		}
		res1.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		res1.TotalCost = 0.0
		res1.CalculationPossible = false
		res1.FinalCostMethod = "N/A"
	}
	// Add note if craft cycled but wasn't chosen
	if chosenMethodP1 != "Craft" && craftResultedInCycle {
		if res1.ErrorMessage == "" {
			res1.ErrorMessage = "Crafting path resulted in cycle."
		} else {
			res1.ErrorMessage += "; Crafting path resulted in cycle."
		}
	}

	// --- 4. Process Perspective 2 (Benchmark: Primary First, then Craft, Fallback Secondary) ---
	dlog("--- Running Perspective 2 (Benchmark: Primary C10M first, then Craft) ---")
	res2 := &result.SecondaryBased
	chosenMethodP2 := "N/A" // Track which path P2 ends up taking

	if isApiNotFoundError && topLevelRecipeExists && craftPossible { // Force craft if not on bazaar
		dlog("  P2 Decision: Expand (Not on Bazaar, recipe & craft possible)")
		chosenMethodP2 = "Craft"
	} else if isApiNotFoundError && topLevelRecipeExists && !craftPossible { // Not on bazaar, craft failed
		dlog("  P2 Decision: ExpansionFailed (Not on Bazaar, recipe exists, craft cost failed)")
		chosenMethodP2 = "ExpansionFailed" // Mark as failed craft attempt
	} else if isApiNotFoundError && !topLevelRecipeExists { // Not on bazaar, no recipe
		dlog("  P2 Decision: TreatedAsBase (Not on Bazaar, no recipe)")
		chosenMethodP2 = "N/A" // Unobtainable
	} else if validTopC10mPrim && (!craftPossible || topC10mPrim <= costToCraftOptimal) {
		// Choose Primary if valid AND (craft impossible OR primary <= craft)
		dlog("  P2 Decision: TreatedAsBase (Primary C10M %.2f is valid and preferred/only option vs Craft %.2f)", topC10mPrim, costToCraftOptimal)
		chosenMethodP2 = "Primary"
	} else if craftPossible {
		// Choose Craft if Primary wasn't chosen AND craft is possible
		dlog("  P2 Decision: Expand (Craft cost %.2f is better than Primary C10M %.2f or Primary invalid)", costToCraftOptimal, topC10mPrim)
		chosenMethodP2 = "Craft"
	} else {
		// Fallback: Neither Primary nor Craft chosen/possible. Use Secondary if valid.
		dlog("  P2 Decision: Fallback - Primary invalid/worse and Crafting impossible/failed.")
		if validTopC10mSec {
			dlog("  P2 Fallback: Using Secondary C10M %.2f", topC10mSec)
			chosenMethodP2 = "Secondary"
		} else {
			dlog("  P2 Fallback: Secondary C10M also invalid. Unobtainable.")
			chosenMethodP2 = "N/A" // Unobtainable
		}
	}

	// Populate P2 results based on chosenMethodP2
	if chosenMethodP2 == "Craft" {
		res2.TopLevelAction = "Expanded"
		res2.BaseIngredients = baseIngredientsFromCraft
		res2.TotalCost = costToCraftOptimal
		res2.CalculationPossible = true
		res2.FinalCostMethod = "SumBestC10M"
	} else if chosenMethodP2 == "Primary" {
		res2.TopLevelAction = "TreatedAsBase"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(getSellPrice(apiResp, itemNameNorm) * quantity), Method: "Primary", RR: float64Ptr(topRR)}}
		res2.TotalCost = sanitizeFloat(topC10mPrim)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelPrimary"
	} else if chosenMethodP2 == "Secondary" { // Fallback case
		res2.TopLevelAction = "TreatedAsBase"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(getBuyPrice(apiResp, itemNameNorm) * quantity), Method: "Secondary", RR: nil}}
		res2.TotalCost = sanitizeFloat(topC10mSec)
		res2.CalculationPossible = true
		res2.FinalCostMethod = "FixedTopLevelSecondary"
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = "Fell back to Secondary C10M cost."
		} else {
			res2.ErrorMessage += "; Fell back to Secondary C10M cost."
		}
	} else if chosenMethodP2 == "ExpansionFailed" {
		res2.TopLevelAction = "ExpansionFailed"
		res2.BaseIngredients = baseIngredientsFromCraft
		res2.ErrorMessage = craftErrMsg
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
	} else { // N/A or Unobtainable
		res2.TopLevelAction = "TreatedAsBase (Unobtainable)"
		res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: 0, AssociatedCost: 0, Method: "N/A"}}
		if res2.ErrorMessage == "" {
			res2.ErrorMessage = "No valid acquisition method found for P2."
		}
		res2.TotalCost = 0.0
		res2.CalculationPossible = false
		res2.FinalCostMethod = "N/A"
	}
	// Handle case where P2 *chose* Craft, but craft resulted in cycle
	if chosenMethodP2 == "Craft" && craftResultedInCycle {
		res2.TopLevelAction = "TreatedAsBase (Due to Cycle)"
		// Fallback to Primary if valid, otherwise Secondary if valid, else impossible
		if validTopC10mPrim {
			dlog("  P2 Adjustment: Craft cycled, falling back to valid Primary C10M cost.")
			res2.TotalCost = sanitizeFloat(topC10mPrim)
			res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mPrim), AssociatedCost: sanitizeFloat(getSellPrice(apiResp, itemNameNorm) * quantity), Method: "Primary", RR: float64Ptr(topRR)}}
			res2.FinalCostMethod = "FixedTopLevelPrimary"
			res2.CalculationPossible = true
		} else if validTopC10mSec {
			dlog("  P2 Adjustment: Craft cycled, Primary invalid, falling back to valid Secondary C10M cost.")
			res2.TotalCost = sanitizeFloat(topC10mSec)
			res2.BaseIngredients = map[string]BaseIngredientDetail{itemNameNorm: {Quantity: quantity, BestCost: sanitizeFloat(topC10mSec), AssociatedCost: sanitizeFloat(getBuyPrice(apiResp, itemNameNorm) * quantity), Method: "Secondary", RR: nil}}
			res2.FinalCostMethod = "FixedTopLevelSecondary"
			res2.CalculationPossible = true
		} else {
			dlog("  P2 Adjustment: Craft cycled, Primary & Secondary C10M also invalid.")
			if res2.ErrorMessage == "" {
				res2.ErrorMessage = "Crafting path resulted in cycle; Top-level costs also invalid."
			} else {
				res2.ErrorMessage += "; Crafting path resulted in cycle; Top-level costs also invalid."
			}
			res2.TotalCost = 0.0
			res2.CalculationPossible = false
			res2.FinalCostMethod = "N/A"
		}
	}

	// Final cleanup
	result.PrimaryBased.TotalCost = sanitizeFloat(result.PrimaryBased.TotalCost)
	result.SecondaryBased.TotalCost = sanitizeFloat(result.SecondaryBased.TotalCost)

	dlog(">>> Dual Expansion Complete for %s <<<", itemNameNorm)
	return result, nil
}
