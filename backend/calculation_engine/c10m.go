// c10m.go
package main

import (
	"fmt"
	"math"
)

// calculateC10MInternal is the core logic for C10M calculation.
// It takes all necessary pre-fetched and validated inputs.
func calculateC10MInternal(
	prodID string, // Normalized Product ID for logging
	qty float64, // Quantity needed
	sellP float64, // Sell Price (top sell order, i.e., price to place a buy order under)
	buyP float64, // Buy Price (top buy order, i.e., price to insta-buy at)
	pm ProductMetrics, // ProductMetrics for the item
) (c10mPrimary, c10mSecondary, ifValue, rrValue, deltaRatio, adjustment float64, err error) {

	dlog("  [Internal C10M Calc] For %.2f x %s", qty, prodID)

	// Validate inputs
	if qty <= 0 {
		err = fmt.Errorf("quantity must be positive (got %.2f for %s)", qty, prodID)
		// Return NaNs for values that depend on qty, Inf for costs
		return math.Inf(1), math.Inf(1), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}
	if sellP <= 0 || buyP <= 0 || math.IsNaN(sellP) || math.IsNaN(buyP) || math.IsInf(sellP, 0) || math.IsInf(buyP, 0) {
		err = fmt.Errorf("invalid (non-positive, NaN, or Inf) API price provided for %s (sP: %.2f, bP: %.2f)", prodID, sellP, buyP)
		return math.Inf(1), math.Inf(1), math.NaN(), math.NaN(), math.NaN(), math.NaN(), err
	}

	// Clamp metric values to be non-negative as negative values are not meaningful here
	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_f := math.Max(0, pm.OrderFrequency) // Order frequency from metrics
	o_s := math.Max(0, pm.OrderSize)      // Order size from metrics

	supplyRate := s_s * s_f
	demandRate := o_s * o_f // Demand based on buy orders being placed by others
	dlog("    Rates for %s: SupplyRate (s_s*s_f)=%.4f (ss:%.2f * sf:%.2f), DemandRate (o_s*o_f)=%.4f (os:%.2f * of:%.2f)", prodID, supplyRate, s_s, s_f, demandRate, o_s, o_f)

	// Delta Ratio: Ratio of supply to demand pressure
	if demandRate <= 0 { // Avoid division by zero
		if supplyRate <= 0 {
			deltaRatio = 1.0 // No flow either way, neutral
		} else {
			deltaRatio = math.Inf(1) // Supply exists, no demand = infinitely easy to fill (theoretically)
		}
	} else {
		deltaRatio = supplyRate / demandRate
	}
	dlog("    DeltaRatio (SR/DR) for %s: %.4f", prodID, deltaRatio)

	// Base cost for Primary C10M (cost if order fills instantly at sellP)
	baseCostPrimary := qty * sellP
	dlog("    Base Cost (Primary C10M) for %s (qty * sellP): %.2f * %.2f = %.2f", prodID, qty, sellP, baseCostPrimary)

	// --- Primary C10M Calculation (Buy Order Cost) ---
	if deltaRatio > 1.0 { // More supply than demand pressure: order likely fills fast
		dlog("    DeltaRatio > 1.0 for %s: Simplified logic (fast fill).", prodID)
		ifValue = math.Inf(1) // Effectively infinite insta-fills relative to order size
		rrValue = 1.0         // One round of orders needed
		adjustment = 0.0      // No upward adjustment needed
		c10mPrimary = baseCostPrimary
		dlog("    Primary C10M for %s = baseCostPrimary = %.2f", prodID, c10mPrimary)
	} else { // deltaRatio <= 1.0: Demand matches or exceeds supply pressure, slower fill
		dlog("    DeltaRatio <= 1.0 for %s: Full IF/RR logic.", prodID)

		// Calculate InstaFills (IF) per order cycle
		if o_f <= 0 { // If no orders are being placed by others (OrderFrequency is 0)
			ifValue = 0 // No opportunity for our order to be insta-filled by new sell orders
			dlog("    IF Calc for %s: OrderFrequency (o_f) <= 0. IF = 0.", prodID)
		} else {
			// IF = SellSize * (SellFrequency / OrderFrequency)
			// This represents how many items (s_s) are insta-sold by others during the typical lifetime of one of our buy orders.
			ifValue = s_s * (s_f / o_f)
			dlog("    IF Calc for %s: s_s * (s_f / o_f) = %.4f * (%.4f / %.4f) = %.4f", prodID, s_s, s_f, o_f, ifValue)
		}
		ifValue = math.Max(0, ifValue) // Ensure IF is not negative
		dlog("    Final Calculated IF for %s: %.4f", prodID, ifValue)

		// Calculate RelistRate (RR)
		if ifValue <= 0 { // If no items are insta-filled per order cycle
			// If there's also no general supply (supplyRate is 0), then RR is Inf (never fills)
			// If there IS supply, but IF is 0 (e.g., o_f was 0), it implies a complex situation.
			// For simplicity, if IF is 0, assume RR becomes effectively infinite for filling 'qty'.
			rrValue = math.Inf(1)
			dlog("    RR Calc for %s: IF <= 0 -> RR = Inf.", prodID)
		} else {
			rrValue = math.Ceil(qty / ifValue) // How many order cycles to fill 'qty'
			dlog("    RR Calc for %s: Ceil(qty / IF) = Ceil(%.2f / %.4f) = %.2f", prodID, qty, ifValue, rrValue)
		}
		// RR must be at least 1, unless it's already Inf (which means it'll never fill)
		if rrValue < 1 && !math.IsInf(rrValue, 1) {
			rrValue = 1.0
		}
		if math.IsNaN(rrValue) { // Should not happen if IF logic is correct, but defensive
			rrValue = math.Inf(1)
		}
		dlog("    Final RR for %s: %.2f", prodID, rrValue)

		// Calculate cost adjustment factor
		if math.IsInf(rrValue, 1) { // If RR is infinite, primary cost is infinite
			dlog("    RR is Infinite for %s, Primary C10M is Infinite.", prodID)
			c10mPrimary = math.Inf(1)
			adjustment = 0.0 // No meaningful adjustment if cost is already Inf
		} else {
			if rrValue <= 1.0 { // If fills in one round or less (deltaRatio > 1 case effectively)
				adjustment = 0.0
				dlog("    Adjustment factor for %s: RR <= 1.0 -> adj = 0.0", prodID)
			} else {
				// Adjustment factor: (1 - 1/RR), approaches 1 as RR increases
				adjustment = 1.0 - (1.0 / rrValue)
				dlog("    Adjustment factor for %s: 1.0 - (1.0 / %.2f) = %.4f", prodID, rrValue, adjustment)
			}

			// Calculate extra cost due to relisting (simplified model)
			// This "extra" part is a bit hand-wavy in the C10M model.
			// A simpler C10M might just be: Cost = Base + Adjustment_Factor * (Price_Range_Penalty)
			// The original C10M LaTeX implies a more complex "extra" related to sum of k.
			// For now, let's use a simplified interpretation or a placeholder for "extra".
			// A common simplification: if adjustment > 0, there's *some* penalty.
			// The original prompt's formula `sellP * (qty*rrValue - ifValue*sumK)` can be large.
			// Let's assume `extra` is a penalty related to the spread or a fixed percentage if relisting is high.
			// For this implementation, let's stick to the spirit of the adjustment factor.
			// If C10M = BaseCost * (1 + AdjustmentFactor * PenaltyFraction)
			// If PenaltyFraction is e.g. (BuyPrice - SellPrice)/SellPrice (the spread as fraction of sell price)
			// This can get complex. The original formula for `extra` might be too volatile.
			// Let's assume the adjustment applies to a portion of the base cost that represents risk/time.
			// For now, using a simplified adjustment logic: c10mPrimary = baseCostPrimary * (1 + adjustment_penalty)
			// where adjustment_penalty is related to `adjustment`. If `adjustment` is 0.5, maybe penalty is 0.1 (10%).
			// This part of C10M is often proprietary or heavily tweaked.
			// The provided C10MInternal code had: extra = sellP * (qty*rrValue - ifValue*sumK)
			// Let's re-evaluate sumK logic from the original context if available.
			// If sumK is sum of 1 to RR_int:
			var extraCalculatedPart float64 = 0.0
			if adjustment > 0 { // Only calculate if there's an adjustment
				RRint := int(math.Round(rrValue)) // Use rounded RR for sumK
				if RRint < 1 {
					RRint = 1
				}
				sumK := float64(RRint*(RRint+1)) / 2.0 // Sum of integers from 1 to RRint

				// The term (qty*rrValue - ifValue*sumK) can be problematic.
				// If ifValue*sumK is very large, this could go negative.
				// This "extra" cost needs careful interpretation.
				// Original formula might be: Cost = Base + Adj * (Cost_Of_Waiting_Or_Relisting_Penalty)
				// Let's use the formula structure as provided:
				extraTerm := (qty * rrValue) - (ifValue * sumK)
				// This extra term seems to represent a "cost beyond simple base * quantity"
				// due to multiple relists or waiting.
				// If this term is negative, it implies a "gain", which is counterintuitive for a cost.
				// So, clamp it at 0 if it goes negative.
				extraCalculatedPart = sellP * math.Max(0, extraTerm)
				dlog("    Extra Cost Part for %s: sellP * Max(0, (qty*RR - IF*sumK(RRint=%d))) = %.2f * Max(0, (%.2f*%.2f - %.4f*%.2f)) = %.2f",
					prodID, sellP, RRint, qty, rrValue, ifValue, sumK, extraCalculatedPart)
			} else {
				dlog("    Extra Cost Part for %s: Skipped (adjustment is 0).", prodID)
			}

			c10mPrimary = baseCostPrimary + (adjustment * extraCalculatedPart)
			// Validate c10mPrimary
			if math.IsInf(c10mPrimary, 0) || math.IsNaN(c10mPrimary) {
				dlog("    Primary C10M for %s calculation resulted in Inf/NaN.", prodID)
				c10mPrimary = math.Inf(1) // Ensure positive Inf for error
			} else if c10mPrimary < 0 { // Cost should not be negative
				dlog("    WARN: Primary C10M for %s calculation resulted in negative (%.2f). Clamping to base or Inf.", prodID, c10mPrimary)
				// If it's negative, it suggests an issue with the 'extra' calculation or parameters.
				// Fallback to baseCostPrimary or Inf if baseCostPrimary is also problematic.
				c10mPrimary = math.Max(baseCostPrimary, 0) // Ensure it's at least base, or 0 if base was also bad. More robust: math.Inf(1)
			} else {
				dlog("    Primary C10M for %s: baseCostPrimary + adjustment*extra = %.2f + %.4f*%.2f = %.2f", prodID, baseCostPrimary, adjustment, extraCalculatedPart, c10mPrimary)
			}
		}
	}

	// --- Secondary C10M Calculation (Insta-Buy Cost) ---
	c10mSecondary = qty * buyP // Cost to insta-buy 'qty' at the current top buy order price
	dlog("    Secondary C10M (Instabuy) for %s = qty * buyP = %.2f * %.2f = %.2f", prodID, qty, buyP, c10mSecondary)

	// Validate c10mSecondary
	if math.IsNaN(c10mSecondary) || math.IsInf(c10mSecondary, -1) || c10mSecondary < 0 { // Negative Inf or negative cost
		dlog("    Secondary C10M for %s validation failed (%.2f), setting to Inf.", prodID, c10mSecondary)
		c10mSecondary = math.Inf(1) // Ensure positive Inf for error
	}

	dlog("  [Internal C10M Calc] Results for %s: Prim=%.2f, Sec=%.2f, IF=%.4f, RR=%.2f, DeltaRatio=%.4f, AdjFactor=%.4f, Err=%v",
		prodID, c10mPrimary, c10mSecondary, ifValue, rrValue, deltaRatio, adjustment, err)

	return // Returns named variables
}

// getBestC10M determines the best C10M (Primary or Secondary) for acquiring an item.
// It returns the cost, method, associated cost (contextual), RR, IF, and any error.
func getBestC10M(
	itemID string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (bestCost float64, bestMethod string, associatedCost float64, rrValue float64, ifValue float64, err error) {

	itemIDNorm := BAZAAR_ID(itemID)
	dlog("Getting Best C10M for %.2f x %s", quantity, itemIDNorm)

	// Initialize return values for error cases or N/A
	bestCost = math.Inf(1)      // Default to infinite cost
	bestMethod = "N/A"          // Default to Not Applicable
	associatedCost = math.NaN() // Contextual cost, NaN if not applicable
	rrValue = math.NaN()        // RelistRate, NaN if not applicable (e.g., for Secondary)
	ifValue = math.NaN()        // InstaFills, NaN if not applicable

	if quantity <= 0 {
		err = fmt.Errorf("quantity must be positive (got %.2f for %s)", quantity, itemIDNorm)
		// For 0 quantity, cost is 0, method N/A, others 0 or NaN.
		return 0, "N/A", 0, 0, 0, err // Or specific values for 0 quantity if defined.
	}

	// Get API and Metrics data
	productData, apiOk := safeGetProductData(apiResp, itemIDNorm)
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemIDNorm)

	var sellP, buyP float64 = math.NaN(), math.NaN() // Prices from API

	if !apiOk {
		dlog("  [%s] API data not found.", itemIDNorm)
		err = fmt.Errorf("API data not found for %s", itemIDNorm)
		// All return values remain at their error/default state
		return // bestCost=Inf, bestMethod="N/A", etc.
	}

	// Extract prices from API data
	if len(productData.SellSummary) > 0 {
		sellP = productData.SellSummary[0].PricePerUnit
	}
	if len(productData.BuySummary) > 0 {
		buyP = productData.BuySummary[0].PricePerUnit
	}

	// Validate extracted prices
	if sellP <= 0 || buyP <= 0 || math.IsNaN(sellP) || math.IsNaN(buyP) || math.IsInf(sellP, 0) || math.IsInf(buyP, 0) {
		errMsg := fmt.Sprintf("invalid prices from API for %s (sP: %.2f, bP: %.2f)", itemIDNorm, sellP, buyP)
		dlog("  [%s] %s", itemIDNorm, errMsg)
		err = fmt.Errorf(errMsg) // Set the error
		return                   // Return with error defaults
	}
	dlog("  [%s] Prices from API - SellP (for buy order): %.2f, BuyP (for instabuy): %.2f", itemIDNorm, sellP, buyP)

	// Handle case where metrics data is missing
	if !metricsOk {
		dlog("  [%s] Metrics data not found. Primary C10M calculation skipped. Evaluating Secondary C10M only.", itemIDNorm)
		// Only Secondary C10M (instabuy) is possible if no metrics for Primary C10M
		c10mSec := quantity * buyP                                        // Instabuy cost
		if math.IsNaN(c10mSec) || c10mSec < 0 || math.IsInf(c10mSec, 0) { // Validate instabuy cost
			dlog("  [%s] Secondary C10M calculation failed (%.2f) even without metrics.", itemIDNorm, c10mSec)
			errMsg := fmt.Sprintf("secondary C10M failed for %s", itemIDNorm)
			if err != nil { // Append to existing API error if any (though unlikely here)
				err = fmt.Errorf("%v; and %s", err, errMsg)
			} else {
				err = fmt.Errorf("metrics missing and %s", errMsg)
			}
			return // Return with error defaults
		}
		// If Secondary C10M is valid, it's the best/only option
		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = bestCost // For Secondary, associated cost is the cost itself
		// RR and IF are not applicable to Secondary method
		rrValue = math.NaN()
		ifValue = math.NaN()
		dlog("  [%s] Using Secondary C10M (%.2f) due to missing metrics.", itemIDNorm, bestCost)
		// Set error to indicate why only Secondary was chosen, if no other error exists
		if err == nil {
			err = fmt.Errorf("metrics not found for %s, only Secondary C10M available", itemIDNorm)
		}
		return // Return with Secondary as best
	}

	// Both API and Metrics data are available, proceed with full C10M calculation
	dlog("  [%s] Both API and Metrics data available. Calculating full C10M...", itemIDNorm)
	var c10mPrim, c10mSec float64
	var calcIF, calcRR float64 // Capture IF/RR from internal calculation
	var calcErr error          // Error from internal calculation

	c10mPrim, c10mSec, calcIF, calcRR, _, _, calcErr = calculateC10MInternal(itemIDNorm, quantity, sellP, buyP, metricsData)

	if calcErr != nil {
		dlog("  [%s] Error during C10M internal calculation: %v", itemIDNorm, calcErr)
		if err == nil { // If no prior error (e.g. API price validation)
			err = calcErr
		} else { // Append to existing error
			err = fmt.Errorf("%v; additionally C10M internal calc failed: %w", err, calcErr)
		}
		// Even if internal calc fails, it might have returned some partial (e.g. Inf) costs.
		// The logic below will try to pick the best of what's available.
	}

	// Determine best method based on calculated Primary and Secondary C10M
	validPrim := !math.IsInf(c10mPrim, 0) && !math.IsNaN(c10mPrim) && c10mPrim >= 0
	validSec := !math.IsInf(c10mSec, 0) && !math.IsNaN(c10mSec) && c10mSec >= 0

	if validPrim && validSec {
		if c10mPrim <= c10mSec {
			bestCost = c10mPrim
			bestMethod = "Primary"
			associatedCost = quantity * sellP // Cost if order placed at sellP
			rrValue = calcRR                  // Use RR from internal calculation
			ifValue = calcIF                  // Use IF from internal calculation
			dlog("  [%s] Primary (%.2f) <= Secondary (%.2f). Using Primary. AssocCost=%.2f, RR=%.2f, IF=%.4f", itemIDNorm, c10mPrim, c10mSec, associatedCost, rrValue, ifValue)
		} else {
			bestCost = c10mSec
			bestMethod = "Secondary"
			associatedCost = quantity * buyP // Cost if instabought at buyP
			rrValue = math.NaN()             // RR not applicable for Secondary
			ifValue = math.NaN()             // IF not applicable for Secondary
			dlog("  [%s] Secondary (%.2f) < Primary (%.2f). Using Secondary. AssocCost=%.2f", itemIDNorm, c10mSec, c10mPrim, associatedCost)
		}
	} else if validPrim { // Only Primary is valid
		bestCost = c10mPrim
		bestMethod = "Primary"
		associatedCost = quantity * sellP
		rrValue = calcRR
		ifValue = calcIF
		dlog("  [%s] Secondary C10M Invalid, using Primary C10M (%.2f). AssocCost=%.2f, RR=%.2f, IF=%.4f", itemIDNorm, bestCost, associatedCost, rrValue, ifValue)
	} else if validSec { // Only Secondary is valid
		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = quantity * buyP
		rrValue = math.NaN()
		ifValue = math.NaN()
		dlog("  [%s] Primary C10M Invalid, using Secondary C10M (%.2f). AssocCost=%.2f", itemIDNorm, bestCost, associatedCost)
	} else { // Neither is valid
		bestCost = math.Inf(1) // Already default, but explicit
		bestMethod = "N/A"     // Already default
		associatedCost = math.NaN()
		rrValue = math.NaN()
		ifValue = math.NaN()
		dlog("  [%s] Both Primary and Secondary C10M results are invalid.", itemIDNorm)
		if err == nil { // If no specific error yet, create one
			err = fmt.Errorf("failed to determine any valid C10M for %s (both Primary/Secondary results invalid)", itemIDNorm)
		}
	}

	// Final sanity checks on output values for consistency, especially if method is N/A
	if bestMethod == "N/A" {
		associatedCost = math.NaN()
		rrValue = math.NaN()
		ifValue = math.NaN()
	} else { // For Primary/Secondary, ensure associatedCost is not negative or NaN without reason
		if math.IsNaN(associatedCost) || associatedCost < 0 {
			// This might happen if sellP/buyP were problematic initially but somehow passed.
			// Or if quantity was manipulated.
			// If bestCost is valid, associatedCost should ideally be related.
			// For now, if it's bad, set to NaN.
			associatedCost = math.NaN()
		}
	}
	// Ensure RR/IF are NaN if not Primary or if their values are Inf/NaN from calc
	if bestMethod != "Primary" || math.IsInf(rrValue, 0) || math.IsNaN(rrValue) {
		rrValue = math.NaN()
	}
	if bestMethod != "Primary" || math.IsInf(ifValue, 0) || math.IsNaN(ifValue) {
		ifValue = math.NaN()
	}

	dlog("  [%s] Best C10M Final Result: Cost=%.2f, Method=%s, AssocCost=%.2f, RR=%.2f, IF=%.4f, Err=%v", itemIDNorm, bestCost, bestMethod, associatedCost, rrValue, ifValue, err)
	return // Return named variables
}
