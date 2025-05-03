// c10m.go

package main

import (
	"fmt"
	"math"
)

// calculateC10M computes primary and secondary C10M values using pre-fetched data.
// ... (calculateC10M function remains the same) ...
func calculateC10M(
	prodID string,
	qty float64,
	productData HypixelProduct, // Use the combined struct
	metricsData ProductMetrics, // Use the metrics struct
) (c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment float64, err error) {

	dlog("Calculating C10M for %.2f x %s", qty, prodID)
	if qty <= 0 {
		err = fmt.Errorf("quantity must be positive (got %.2f)", qty)
		return math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}

	if len(productData.SellSummary) == 0 || len(productData.BuySummary) == 0 {
		err = fmt.Errorf("API data for '%s' missing sell_summary or buy_summary", prodID)
		return math.Inf(1), math.Inf(1), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}
	sellP := productData.SellSummary[0].PricePerUnit // Buy order price (user sells to this)
	buyP := productData.BuySummary[0].PricePerUnit   // Instabuy price (user buys at this)

	if sellP <= 0 || buyP <= 0 {
		err = fmt.Errorf("invalid (non-positive) API price for '%s' (sP: %.2f, bP: %.2f)", prodID, sellP, buyP)
		return math.Inf(1), math.Inf(1), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}

	pm := metricsData

	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_f := math.Max(0, pm.OrderFrequency)
	o_s := math.Max(0, pm.OrderSize)

	supplyRate := s_s * s_f
	demandRate := o_s * o_f
	dlog("  [%s] Rates: Supply=%.4f (ss:%.2f * sf:%.2f), Demand=%.4f (os:%.2f * of:%.2f)", prodID, supplyRate, s_s, s_f, demandRate, o_s, o_f)

	if demandRate <= 0 {
		if supplyRate <= 0 {
			DeltaRatio = 1.0
		} else {
			DeltaRatio = math.Inf(1)
		}
	} else {
		DeltaRatio = supplyRate / demandRate
	}
	dlog("  [%s] DeltaRatio (SR/DR): %.4f", prodID, DeltaRatio)

	base := qty * sellP
	dlog("  [%s] Base Cost (qty * sellP): %.2f", prodID, base)

	if DeltaRatio > 1.0 {
		dlog("  [%s] DeltaRatio > 1.0: Using simplified logic.", prodID)
		IF = math.Inf(1)
		RR = 1.0
		adjustment = 0.0
		c10mPrimary = base
		dlog("  [%s] Set IF=Inf, RR=1.0, Adjustment=0.0", prodID)
		dlog("  [%s] Primary C10M = base = %.2f", prodID, c10mPrimary)
	} else {
		dlog("  [%s] DeltaRatio <= 1.0: Using full IF/RR logic.", prodID)
		if o_f <= 0 {
			IF = 0
			dlog("  [%s] IF Calc: OF <= 0. IF = 0.", prodID)
		} else {
			IF = s_s * (s_f / o_f)
			dlog("  [%s] IF Calc: ss*(sf/of) = %.4f*(%.4f/%.4f) = %.4f", prodID, s_s, s_f, o_f, IF)
		}
		if IF < 0 {
			IF = 0
		}
		dlog("  [%s] Final Calculated IF: %.4f", prodID, IF)

		if IF <= 0 {
			if supplyRate <= 0 {
				RR = math.Inf(1)
				dlog("  [%s] RR Calc: IF=0, SR=0 -> RR=Inf.", prodID)
			} else {
				RR = 1.0
				dlog("  [%s] RR Calc: IF <= 0, SR>0 -> RR = 1.0", prodID)
			}
		} else {
			RR = math.Ceil(qty / IF)
			dlog("  [%s] RR Calc: IF > 0. RR = Ceil(%.2f/%.4f) = %.2f", prodID, qty, IF, RR)
		}
		if RR < 1 && !math.IsInf(RR, 1) {
			RR = 1.0
		}
		if math.IsNaN(RR) {
			RR = math.Inf(1)
		}
		dlog("  [%s] Final RR for this branch: %.2f", prodID, RR)

		if math.IsInf(RR, 1) {
			dlog("  [%s] RR is Infinite, Primary C10M is Infinite.", prodID)
			c10mPrimary = math.Inf(1)
			adjustment = 0.0
		} else {
			if RR <= 1.0 {
				adjustment = 0.0
				dlog("  [%s] Adj: RR <= 1.0 -> adj = 0.0", prodID)
			} else {
				adjustment = 1.0 - 1.0/RR
				dlog("  [%s] Adj: 1.0-1.0/%.2f = %.4f", prodID, RR, adjustment)
			}
			var extra float64 = 0.0
			if adjustment > 0 {
				RRint := int(RR)
				sumK := float64(RRint*(RRint+1)) / 2.0
				extra = sellP * (qty*RR - IF*sumK)
				if extra < 0 {
					dlog("  [%s] WARN: Negative Extra Cost (%.2f). Clamping.", prodID, extra)
					extra = 0
				}
				dlog("  [%s] Extra Cost: sellP*(qty*RR - IF*sumK(RR=%d)) = %.2f", prodID, RRint, extra)
			} else {
				dlog("  [%s] Extra Cost: Skipped (adj=0).", prodID)
			}
			c10mPrimary = base + adjustment*extra
			if math.IsInf(c10mPrimary, 1) {
				dlog("  [%s] Primary C10M calculation resulted in Inf.", prodID)
			} else {
				dlog("  [%s] Primary C10M: base+adj*extra=%.2f+%.4f*%.2f=%.2f", prodID, base, adjustment, extra, c10mPrimary)
			}
		}
	}

	c10mSecondary = qty * buyP
	dlog("  [%s] Secondary C10M (Instabuy) = qty * buyP = %.2f * %.2f = %.2f", prodID, qty, buyP, c10mSecondary)

	if math.IsNaN(c10mPrimary) || math.IsInf(c10mPrimary, -1) || c10mPrimary < 0 {
		dlog("  [%s] Primary C10M validation failed (%.2f), setting to Inf.", prodID, c10mPrimary)
		c10mPrimary = math.Inf(1)
	}
	if math.IsNaN(c10mSecondary) || math.IsInf(c10mSecondary, -1) || c10mSecondary < 0 {
		dlog("  [%s] Secondary C10M validation failed (%.2f), setting to Inf.", prodID, c10mSecondary)
		c10mSecondary = math.Inf(1)
	}
	dlog("  [%s] Returning C10M: Prim=%.2f, Sec=%.2f, IF=%.4f, RR=%.2f, Delta=%.4f, Adj=%.4f",
		prodID, c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment)
	return c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment, nil
}

// getBestC10M calculates both C10M values and returns the better one AND its associated simple cost.
// Returns: bestCost, method ("Primary", "Secondary", "N/A"), associatedCost, calculated RR, error
func getBestC10M(
	itemID string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (bestCost float64, bestMethod string, associatedCost float64, rrValue float64, err error) {

	dlog("Getting Best C10M for %.2f x %s", quantity, itemID)
	bestCost = math.Inf(1)
	bestMethod = "N/A"
	associatedCost = math.NaN() // Initialize associated cost
	rrValue = math.NaN()        // Initialize RR

	if quantity <= 0 {
		err = fmt.Errorf("quantity must be positive")
		return 0, "N/A", 0, 0, err // Or NaN? Let's return 0s for cost/RR on invalid qty.
	}

	// Get data from caches
	productData, apiOk := safeGetProductData(apiResp, itemID)        // Assumes defined in utils.go
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemID) // Assumes defined in utils.go

	var sellP, buyP float64 = math.NaN(), math.NaN() // Store prices for associated cost calc

	if !apiOk {
		dlog("  [%s] API data not found.", itemID)
		err = fmt.Errorf("API data not found for %s", itemID)
		return math.Inf(1), "N/A", math.NaN(), math.NaN(), err
	} else {
		// Extract prices if API data exists
		if len(productData.SellSummary) > 0 {
			sellP = productData.SellSummary[0].PricePerUnit
		}
		if len(productData.BuySummary) > 0 {
			buyP = productData.BuySummary[0].PricePerUnit
		}
		// Check if prices are valid *after* extraction
		if sellP <= 0 || buyP <= 0 {
			dlog("  [%s] Invalid prices from API (sP: %.2f, bP: %.2f)", itemID, sellP, buyP)
			// If prices are invalid, C10M can't be calculated correctly anyway
			err = fmt.Errorf("invalid API prices for %s", itemID)
			return math.Inf(1), "N/A", math.NaN(), math.NaN(), err
		}
		dlog("  [%s] Prices - SellP: %.2f, BuyP: %.2f", itemID, sellP, buyP)
	}

	if !metricsOk {
		dlog("  [%s] Metrics data not found. Primary C10M calculation skipped.", itemID)
		// We can still calculate Secondary C10M and its associated cost if prices are valid
		if buyP > 0 { // Check validity again just in case
			bestCost = quantity * buyP
			bestMethod = "Secondary"
			associatedCost = bestCost // For secondary, C10M cost and associated cost are the same
			// RR is N/A as primary calc was skipped
			if math.IsNaN(bestCost) || bestCost < 0 || math.IsInf(bestCost, 0) { // Validation
				err = fmt.Errorf("secondary C10M calculation failed for %s despite missing metrics", itemID)
				return math.Inf(1), "N/A", math.NaN(), math.NaN(), err
			}
			dlog("  [%s] Using Secondary C10M (%.2f) due to missing metrics.", itemID, bestCost)
			// Return error indicating partial calculation
			err = fmt.Errorf("metrics not found for %s (used secondary C10M)", itemID)
			return bestCost, bestMethod, associatedCost, math.NaN(), err
		} else {
			dlog("  [%s] Metrics missing and buy price invalid/missing. Cannot calculate C10M.", itemID)
			err = fmt.Errorf("metrics and valid buy price not found for %s", itemID)
			return math.Inf(1), "N/A", math.NaN(), math.NaN(), err
		}
	}

	// Both API (with valid prices) and Metrics data available, calculate C10M
	var c10mPrim, c10mSec float64
	var calcErr error
	// Pass extracted prices directly to calculateC10M to avoid redundant lookups inside
	// NOTE: We need calculateC10M to be refactored to accept prices or we need the old lookup logic back
	// Let's revert calculateC10M call signature temporarily for simplicity, assuming it handles lookup
	// Ideally, refactor calculateC10M to take sellP, buyP as args.
	// Using original call structure for now:
	// c10mPrim, c10mSec, _, rrValue, _, _, calcErr = calculateC10M(itemID, quantity, sellP, buyP, metricsMap) // Assuming metricsMap is []ProductMetrics locally
	// Correction: Pass the single found metricsData
	c10mPrim, c10mSec, _, rrValue, _, _, calcErr = calculateC10MInternal(itemID, quantity, sellP, buyP, metricsData) // Use internal helper

	if calcErr != nil {
		dlog("  [%s] Error during C10M calculation: %v", itemID, calcErr)
		err = calcErr // Preserve the first error encountered
		// Fall through to comparison logic, Inf values will be handled
	}

	// Determine Best C10M and Associated Cost
	isPrimInf := math.IsInf(c10mPrim, 0)
	isSecInf := math.IsInf(c10mSec, 0)
	isPrimNaN := math.IsNaN(c10mPrim)
	isSecNaN := math.IsNaN(c10mSec)

	if (isPrimInf || isPrimNaN) && (isSecInf || isSecNaN) {
		bestCost = math.Inf(1)
		bestMethod = "N/A (Both Invalid)"
		associatedCost = math.NaN()
		dlog("  [%s] Both C10M invalid.", itemID)
	} else if isPrimInf || isPrimNaN {
		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = quantity * buyP // Use simple buy price
		dlog("  [%s] Primary Invalid, using Secondary (%.2f). Assoc=%.2f", itemID, bestCost, associatedCost)
	} else if isSecInf || isSecNaN {
		bestCost = c10mPrim
		bestMethod = "Primary"
		associatedCost = quantity * sellP // Use simple sell price
		dlog("  [%s] Secondary Invalid, using Primary (%.2f). Assoc=%.2f", itemID, bestCost, associatedCost)
	} else {
		// Both are valid numbers
		if c10mPrim <= c10mSec {
			bestCost = c10mPrim
			bestMethod = "Primary"
			associatedCost = quantity * sellP // Use simple sell price
			dlog("  [%s] Primary (%.2f) <= Secondary (%.2f). Using Primary. Assoc=%.2f", itemID, c10mPrim, c10mSec, associatedCost)
		} else {
			bestCost = c10mSec
			bestMethod = "Secondary"
			associatedCost = quantity * buyP // Use simple buy price
			dlog("  [%s] Secondary (%.2f) < Primary (%.2f). Using Secondary. Assoc=%.2f", itemID, c10mSec, c10mPrim, associatedCost)
		}
	}

	// Final validation on costs
	if math.IsNaN(bestCost) || bestCost < 0 {
		bestCost = math.Inf(1)
		bestMethod = "N/A (Calc Failed)"
		associatedCost = math.NaN()
	}
	if math.IsNaN(associatedCost) || associatedCost < 0 {
		associatedCost = math.NaN()
	} // Keep NaN if invalid, don't make Inf

	// Update error status if no valid cost determined
	if bestMethod == "N/A" || math.IsInf(bestCost, 1) {
		if err == nil { // If no specific calc error, create a generic one
			err = fmt.Errorf("failed to determine valid C10M for %s (result: %s)", itemID, bestMethod)
		}
	}

	dlog("  [%s] Best C10M: %.2f (%s), AssocCost: %.2f, RR: %.2f", itemID, bestCost, bestMethod, associatedCost, rrValue)
	return bestCost, bestMethod, associatedCost, rrValue, err // Return error status
}

// calculateC10MInternal - temporary helper mirroring original calculateC10M logic
// but taking a single ProductMetrics struct instead of a slice.
// This avoids modifying the public calculateC10M signature yet.
// TODO: Refactor calculateC10M properly to accept prices and single metrics struct.
func calculateC10MInternal(
	prodID string,
	qty float64,
	sellP float64, // Now takes prices directly
	buyP float64, // Now takes prices directly
	pm ProductMetrics, // Takes the single metrics struct
) (c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment float64, err error) { // Added error return

	// Use provided prices directly
	// Use provided pm directly

	// 2) Clamp Metrics & Calculate Rates / Delta Ratio
	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_f := math.Max(0, pm.OrderFrequency)
	o_s := math.Max(0, pm.OrderSize)

	supplyRate := s_s * s_f
	demandRate := o_s * o_f
	// dlog internal logs might be duplicated by getBestC10M, maybe remove later
	dlog("  [Internal] Rates: Supply=%.4f, Demand=%.4f", supplyRate, demandRate)

	if demandRate <= 0 {
		if supplyRate <= 0 {
			DeltaRatio = 1.0
		} else {
			DeltaRatio = math.Inf(1)
		}
	} else {
		DeltaRatio = supplyRate / demandRate
	}
	dlog("  [Internal] DeltaRatio (SR/DR): %.4f", DeltaRatio)

	// 3) Calculate IF, RR, Adjustment, and Primary C10M based on DeltaRatio
	base := qty * sellP // Calculate base cost once
	dlog("  [Internal] Base Cost: %.2f", base)

	if DeltaRatio > 1.0 {
		IF = math.Inf(1)
		RR = 1.0
		adjustment = 0.0
		c10mPrimary = base
		dlog("  [Internal] Delta > 1: Prim=%.2f", c10mPrimary)
	} else {
		if o_f <= 0 {
			IF = 0
		} else {
			IF = s_s * (s_f / o_f)
		}
		if IF < 0 {
			IF = 0
		}
		dlog("  [Internal] IF=%.4f", IF)

		if IF <= 0 {
			if supplyRate <= 0 {
				RR = math.Inf(1)
			} else {
				RR = 1.0
			}
		} else {
			RR = math.Ceil(qty / IF)
		}
		if RR < 1 && !math.IsInf(RR, 1) {
			RR = 1.0
		}
		if math.IsNaN(RR) {
			RR = math.Inf(1)
		}
		dlog("  [Internal] RR=%.2f", RR)

		if math.IsInf(RR, 1) {
			c10mPrimary = math.Inf(1)
			adjustment = 0.0
		} else {
			if RR <= 1.0 {
				adjustment = 0.0
			} else {
				adjustment = 1.0 - 1.0/RR
			}
			var extra float64 = 0.0
			if adjustment > 0 {
				RRint := int(RR)
				sumK := float64(RRint*(RRint+1)) / 2.0
				extra = sellP * (qty*RR - IF*sumK)
				if extra < 0 {
					extra = 0
				}
			}
			c10mPrimary = base + adjustment*extra
		}
		dlog("  [Internal] Delta <= 1: Prim=%.2f", c10mPrimary)
	}
	c10mSecondary = qty * buyP
	dlog("  [Internal] Sec=%.2f", c10mSecondary)

	// 5) Final Validation
	if math.IsNaN(c10mPrimary) || math.IsInf(c10mPrimary, -1) || c10mPrimary < 0 {
		c10mPrimary = math.Inf(1)
	}
	if math.IsNaN(c10mSecondary) || math.IsInf(c10mSecondary, -1) || c10mSecondary < 0 {
		c10mSecondary = math.Inf(1)
	}

	return // Returns named variables including the new 'err' (which is nil here)
}
