package main

import (
	"fmt"
	"math" // Needed for Ceil, Max, IsInf, IsNaN
	// "log" // Only needed if adding extra logging inside
)

// calculateC10MInternal - calculates both primary and secondary C10M values.
// This version takes pre-fetched prices and metrics.
// Assumes dlog is defined in utils.go
func calculateC10MInternal(
	prodID string,
	qty float64,
	sellP float64, // Now takes prices directly
	buyP float64, // Now takes prices directly
	pm ProductMetrics, // Takes the single metrics struct
) (c10mPrimary, c10mSecondary, ifValue, rrValue, deltaRatio, adjustment float64, err error) { // Added ifValue, err return

	dlog("  [Internal C10M Calc] For %.2f x %s", qty, prodID)
	// Input validation
	if qty <= 0 {
		err = fmt.Errorf("quantity must be positive (got %.2f)", qty)
		// Return NaNs for costs/metrics if qty is invalid
		return math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}
	if sellP <= 0 || buyP <= 0 {
		err = fmt.Errorf("invalid (non-positive) API price provided (sP: %.2f, bP: %.2f)", sellP, buyP)
		// Return Infs for costs if prices are invalid
		return math.Inf(1), math.Inf(1), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}

	// 2) Clamp Metrics & Calculate Rates / DeltaRatio
	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_f := math.Max(0, pm.OrderFrequency)
	o_s := math.Max(0, pm.OrderSize)

	supplyRate := s_s * s_f
	demandRate := o_s * o_f
	dlog("    Rates: Supply=%.4f (ss:%.2f * sf:%.2f), Demand=%.4f (os:%.2f * of:%.2f)", supplyRate, s_s, s_f, demandRate, o_s, o_f)

	if demandRate <= 0 {
		if supplyRate <= 0 {
			deltaRatio = 1.0 // No activity
		} else {
			deltaRatio = math.Inf(1) // Infinite supply relative to demand
		}
	} else {
		deltaRatio = supplyRate / demandRate
	}
	dlog("    DeltaRatio (SR/DR): %.4f", deltaRatio)

	// 3) Calculate IF, RR, Adjustment, and Primary C10M based on DeltaRatio
	base := qty * sellP // Calculate base cost once (cost if sell order filled instantly)
	dlog("    Base Cost (qty * sellP): %.2f", base)

	if deltaRatio > 1.0 {
		// Supply exceeds demand, order likely fills quickly. C10M is just the base cost.
		dlog("    DeltaRatio > 1.0: Simplified logic.")
		ifValue = math.Inf(1) // Effectively infinite fill rate from supply side
		rrValue = 1.0         // Only one 'round' needed
		adjustment = 0.0      // No adjustment needed
		c10mPrimary = base
		dlog("    Primary C10M = base = %.2f", c10mPrimary)
	} else {
		// Demand meets or exceeds supply. Use the more complex IF/RR logic.
		dlog("    DeltaRatio <= 1.0: Full IF/RR logic.")
		// Calculate IF (InstaFill equivalent based on relative frequencies)
		if o_f <= 0 { // Avoid division by zero if order frequency is zero
			ifValue = 0 // Cannot fill if no orders are placed/filled
			dlog("    IF Calc: OF <= 0. IF = 0.")
		} else {
			ifValue = s_s * (s_f / o_f) // Items supplied per order cycle
			dlog("    IF Calc: ss*(sf/of) = %.4f*(%.4f/%.4f) = %.4f", s_s, s_f, o_f, ifValue)
		}
		if ifValue < 0 { // Ensure IF is not negative
			ifValue = 0
		}
		dlog("    Final Calculated IF: %.4f", ifValue)

		// Calculate RR (Refill Rounds needed)
		if ifValue <= 0 { // Cannot fill if IF is zero
			if supplyRate <= 0 {
				rrValue = math.Inf(1) // Infinite rounds if no supply at all
				dlog("    RR Calc: IF=0, SR=0 -> RR=Inf.")
			} else {
				rrValue = math.Inf(1)
				dlog("    RR Calc: IF <= 0 but SR > 0 -> RR=Inf (mechanism missing).")
			}
		} else {
			rrValue = math.Ceil(qty / ifValue) // Rounds needed = total qty / qty per round (IF)
			dlog("    RR Calc: IF > 0. RR = Ceil(%.2f/%.4f) = %.2f", qty, ifValue, rrValue)
		}
		// Validate RR
		if rrValue < 1 && !math.IsInf(rrValue, 1) { // RR must be at least 1 if not infinite
			rrValue = 1.0
		}
		if math.IsNaN(rrValue) { // Should not happen with checks above, but safeguard
			rrValue = math.Inf(1)
		}
		dlog("    Final RR: %.2f", rrValue)

		// Calculate Primary C10M using RR
		if math.IsInf(rrValue, 1) {
			dlog("    RR is Infinite, Primary C10M is Infinite.")
			c10mPrimary = math.Inf(1)
			adjustment = 0.0
		} else {
			if rrValue <= 1.0 {
				adjustment = 0.0
				dlog("    Adj: RR <= 1.0 -> adj = 0.0")
			} else {
				adjustment = 1.0 - (1.0 / rrValue)
				dlog("    Adj: 1.0 - 1.0/%.2f = %.4f", rrValue, adjustment)
			}

			var extra float64 = 0.0
			if adjustment > 0 {
				RRint := int(rrValue)
				sumK := float64(RRint*(RRint+1)) / 2.0
				extra = sellP * (qty*rrValue - ifValue*sumK)
				if extra < 0 {
					dlog("    WARN: Negative Extra Cost (%.2f). Clamping to 0.", extra)
					extra = 0
				}
				dlog("    Extra Cost: sellP*(qty*RR - IF*sumK(RR=%d)) = %.2f", RRint, extra)
			} else {
				dlog("    Extra Cost: Skipped (adj=0).")
			}
			c10mPrimary = base + adjustment*extra
			if math.IsInf(c10mPrimary, 0) || math.IsNaN(c10mPrimary) {
				dlog("    Primary C10M calculation resulted in Inf/NaN.")
				c10mPrimary = math.Inf(1)
			} else if c10mPrimary < 0 {
				dlog("    WARN: Primary C10M calculation resulted in negative (%.2f). Setting to Inf.", c10mPrimary)
				c10mPrimary = math.Inf(1)
			} else {
				dlog("    Primary C10M: base + adj*extra = %.2f + %.4f*%.2f = %.2f", base, adjustment, extra, c10mPrimary)
			}
		}
	}

	// 4) Calculate Secondary C10M (Instabuy cost)
	c10mSecondary = qty * buyP
	dlog("    Secondary C10M (Instabuy) = qty * buyP = %.2f * %.2f = %.2f", qty, buyP, c10mSecondary)

	// 5) Final Validation on Secondary C10M
	if math.IsNaN(c10mSecondary) || math.IsInf(c10mSecondary, -1) || c10mSecondary < 0 {
		dlog("    Secondary C10M validation failed (%.2f), setting to Inf.", c10mSecondary)
		c10mSecondary = math.Inf(1)
	}

	dlog("  [Internal C10M Calc] Returning: Prim=%.2f, Sec=%.2f, IF=%.4f, RR=%.2f, Delta=%.4f, Adj=%.4f",
		c10mPrimary, c10mSecondary, ifValue, rrValue, deltaRatio, adjustment)

	return // Returns named variables
}

// getBestC10M calculates both C10M values and returns the better one AND its associated simple cost.
// Returns: bestCost, method ("Primary", "Secondary", "N/A"), associatedCost, calculated RR for Primary path, calculated IF for Primary path, error
func getBestC10M(
	itemID string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (bestCost float64, bestMethod string, associatedCost float64, rrValue float64, ifValue float64, err error) { // Added ifValue

	itemIDNorm := BAZAAR_ID(itemID)
	dlog("Getting Best C10M for %.2f x %s", quantity, itemIDNorm)

	bestCost = math.Inf(1)
	bestMethod = "N/A"
	associatedCost = math.NaN()
	rrValue = math.NaN()
	ifValue = math.NaN() // Initialize IF

	if quantity <= 0 {
		err = fmt.Errorf("quantity must be positive (got %.2f)", quantity)
		return 0, "N/A", 0, 0, 0, err // Return 0s for cost/RR/IF on invalid qty.
	}

	productData, apiOk := safeGetProductData(apiResp, itemIDNorm)
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemIDNorm)

	var sellP, buyP float64 = math.NaN(), math.NaN()

	if !apiOk {
		dlog("  [%s] API data not found.", itemIDNorm)
		err = fmt.Errorf("API data not found for %s", itemIDNorm)
		return math.Inf(1), "N/A", math.NaN(), math.NaN(), math.NaN(), err
	} else {
		if len(productData.SellSummary) > 0 {
			sellP = productData.SellSummary[0].PricePerUnit
		}
		if len(productData.BuySummary) > 0 {
			buyP = productData.BuySummary[0].PricePerUnit
		}
		if sellP <= 0 || buyP <= 0 || math.IsNaN(sellP) || math.IsNaN(buyP) {
			errMsg := fmt.Sprintf("invalid prices from API (sP: %.2f, bP: %.2f)", sellP, buyP)
			dlog("  [%s] %s", itemIDNorm, errMsg)
			err = fmt.Errorf(errMsg+" for %s", itemIDNorm)
			return math.Inf(1), "N/A", math.NaN(), math.NaN(), math.NaN(), err
		}
		dlog("  [%s] Prices - SellP: %.2f, BuyP: %.2f", itemIDNorm, sellP, buyP)
	}

	if !metricsOk {
		dlog("  [%s] Metrics data not found. Primary C10M calculation skipped.", itemIDNorm)
		// err = fmt.Errorf("metrics not found for %s", itemIDNorm) // Keep this error if it's critical path, or allow fallback. Let's assume it's not fatal for getBestC10M to attempt Secondary.
		// If metrics are missing, we can ONLY calculate Secondary C10M if API prices are valid.
		// Primary C10M (and its IF/RR) becomes NaN/Inf.

		c10mSec := quantity * buyP
		if math.IsNaN(c10mSec) || c10mSec < 0 || math.IsInf(c10mSec, 0) {
			dlog("  [%s] Secondary C10M calculation failed (%.2f) even without metrics.", itemIDNorm, c10mSec)
			// Combine error messages if err was already set
			errMsg := "secondary C10M failed"
			if err != nil {
				err = fmt.Errorf("%v; and %s for %s", err, errMsg, itemIDNorm)
			} else {
				err = fmt.Errorf("metrics missing and %s for %s", errMsg, itemIDNorm)
			}
			return math.Inf(1), "N/A", math.NaN(), math.NaN(), math.NaN(), err
		}

		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = bestCost
		rrValue = math.NaN()
		ifValue = math.NaN() // IF not applicable to secondary path
		dlog("  [%s] Using Secondary C10M (%.2f) due to missing metrics.", itemIDNorm, bestCost)
		if err == nil { // If no prior API error, set the metrics missing error
			err = fmt.Errorf("metrics not found for %s, using Secondary C10M", itemIDNorm)
		}
		return bestCost, bestMethod, associatedCost, rrValue, ifValue, err
	}

	dlog("  [%s] Both API and Metrics data available. Calculating C10M...", itemIDNorm)
	var c10mPrim, c10mSec float64
	var calcErr error
	// calculateC10MInternal returns IF as the 3rd value, RR as 4th
	c10mPrim, c10mSec, ifValue, rrValue, _, _, calcErr = calculateC10MInternal(itemIDNorm, quantity, sellP, buyP, metricsData)

	if calcErr != nil {
		dlog("  [%s] Error during C10M internal calculation: %v", itemIDNorm, calcErr)
		if err == nil {
			err = calcErr
		} else {
			err = fmt.Errorf("%v; additionally C10M internal calc failed: %w", err, calcErr)
		}
	}

	validPrim := !math.IsInf(c10mPrim, 0) && !math.IsNaN(c10mPrim) && c10mPrim >= 0
	validSec := !math.IsInf(c10mSec, 0) && !math.IsNaN(c10mSec) && c10mSec >= 0

	if validPrim && validSec {
		if c10mPrim <= c10mSec {
			bestCost = c10mPrim
			bestMethod = "Primary"
			associatedCost = quantity * sellP
			// rrValue and ifValue are already set from calculateC10MInternal for Primary path
			dlog("  [%s] Primary (%.2f) <= Secondary (%.2f). Using Primary. Assoc=%.2f, RR=%.2f, IF=%.4f", itemIDNorm, c10mPrim, c10mSec, associatedCost, rrValue, ifValue)
		} else {
			bestCost = c10mSec
			bestMethod = "Secondary"
			associatedCost = quantity * buyP
			rrValue = math.NaN() // RR doesn't apply if Secondary is chosen
			ifValue = math.NaN() // IF doesn't apply if Secondary is chosen
			dlog("  [%s] Secondary (%.2f) < Primary (%.2f). Using Secondary. Assoc=%.2f", itemIDNorm, c10mSec, c10mPrim, associatedCost)
		}
	} else if validPrim {
		bestCost = c10mPrim
		bestMethod = "Primary"
		associatedCost = quantity * sellP
		// rrValue and ifValue are set
		dlog("  [%s] Secondary Invalid, using Primary (%.2f). Assoc=%.2f, RR=%.2f, IF=%.4f", itemIDNorm, bestCost, associatedCost, rrValue, ifValue)
	} else if validSec {
		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = quantity * buyP
		rrValue = math.NaN()
		ifValue = math.NaN()
		dlog("  [%s] Primary Invalid, using Secondary (%.2f). Assoc=%.2f", itemIDNorm, bestCost, associatedCost)
	} else {
		bestCost = math.Inf(1)
		bestMethod = "N/A"
		associatedCost = math.NaN()
		rrValue = math.NaN()
		ifValue = math.NaN()
		dlog("  [%s] Both C10M results invalid.", itemIDNorm)
		if err == nil { // If no specific calc error, create a generic one
			err = fmt.Errorf("failed to determine valid C10M for %s (results invalid)", itemIDNorm)
		}
	}

	if math.IsNaN(associatedCost) || associatedCost < 0 {
		associatedCost = math.NaN()
	}
	// Sanitize RR and IF before returning
	if math.IsNaN(rrValue) || math.IsInf(rrValue, 0) { // Catches positive and negative infinity for RR
		rrValue = math.NaN()
	}
	if math.IsNaN(ifValue) || math.IsInf(ifValue, 0) { // Catches positive and negative infinity for IF
		ifValue = math.NaN()
	}

	dlog("  [%s] Best C10M Result: Cost=%.2f, Method=%s, AssocCost=%.2f, RR=%.2f, IF=%.4f, Err=%v", itemIDNorm, bestCost, bestMethod, associatedCost, rrValue, ifValue, err)
	return bestCost, bestMethod, associatedCost, rrValue, ifValue, err
}
