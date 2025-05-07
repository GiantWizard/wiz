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
) (c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment float64, err error) { // Added error return

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

	// 2) Clamp Metrics & Calculate Rates / Delta Ratio
	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_f := math.Max(0, pm.OrderFrequency)
	o_s := math.Max(0, pm.OrderSize)

	supplyRate := s_s * s_f
	demandRate := o_s * o_f
	dlog("    Rates: Supply=%.4f (ss:%.2f * sf:%.2f), Demand=%.4f (os:%.2f * of:%.2f)", supplyRate, s_s, s_f, demandRate, o_s, o_f)

	if demandRate <= 0 {
		if supplyRate <= 0 {
			DeltaRatio = 1.0 // No activity
		} else {
			DeltaRatio = math.Inf(1) // Infinite supply relative to demand
		}
	} else {
		DeltaRatio = supplyRate / demandRate
	}
	dlog("    DeltaRatio (SR/DR): %.4f", DeltaRatio)

	// 3) Calculate IF, RR, Adjustment, and Primary C10M based on DeltaRatio
	base := qty * sellP // Calculate base cost once (cost if sell order filled instantly)
	dlog("    Base Cost (qty * sellP): %.2f", base)

	if DeltaRatio > 1.0 {
		// Supply exceeds demand, order likely fills quickly. C10M is just the base cost.
		dlog("    DeltaRatio > 1.0: Simplified logic.")
		IF = math.Inf(1) // Effectively infinite fill rate from supply side
		RR = 1.0         // Only one 'round' needed
		adjustment = 0.0 // No adjustment needed
		c10mPrimary = base
		dlog("    Primary C10M = base = %.2f", c10mPrimary)
	} else {
		// Demand meets or exceeds supply. Use the more complex IF/RR logic.
		dlog("    DeltaRatio <= 1.0: Full IF/RR logic.")
		// Calculate IF (InstaFill equivalent based on relative frequencies)
		if o_f <= 0 { // Avoid division by zero if order frequency is zero
			IF = 0 // Cannot fill if no orders are placed/filled
			dlog("    IF Calc: OF <= 0. IF = 0.")
		} else {
			IF = s_s * (s_f / o_f) // Items supplied per order cycle
			dlog("    IF Calc: ss*(sf/of) = %.4f*(%.4f/%.4f) = %.4f", s_s, s_f, o_f, IF)
		}
		if IF < 0 { // Ensure IF is not negative
			IF = 0
		}
		dlog("    Final Calculated IF: %.4f", IF)

		// Calculate RR (Refill Rounds needed)
		if IF <= 0 { // Cannot fill if IF is zero
			if supplyRate <= 0 {
				RR = math.Inf(1) // Infinite rounds if no supply at all
				dlog("    RR Calc: IF=0, SR=0 -> RR=Inf.")
			} else {
				// Supply exists but orders don't match frequency (IF=0 because o_f=0)
				// Or supply size is zero? This case is ambiguous.
				// Let's assume if IF is 0 but supply rate > 0, it still takes infinite rounds
				// because the mechanism (orders) isn't there.
				RR = math.Inf(1)
				dlog("    RR Calc: IF <= 0 but SR > 0 -> RR=Inf (mechanism missing).")
				// Previous logic set RR=1.0 here, which seems incorrect. Infinite seems better.
			}
		} else {
			RR = math.Ceil(qty / IF) // Rounds needed = total qty / qty per round (IF)
			dlog("    RR Calc: IF > 0. RR = Ceil(%.2f/%.4f) = %.2f", qty, IF, RR)
		}
		// Validate RR
		if RR < 1 && !math.IsInf(RR, 1) { // RR must be at least 1 if not infinite
			RR = 1.0
		}
		if math.IsNaN(RR) { // Should not happen with checks above, but safeguard
			RR = math.Inf(1)
		}
		dlog("    Final RR: %.2f", RR)

		// Calculate Primary C10M using RR
		if math.IsInf(RR, 1) {
			// If infinite rounds are needed, cost is infinite
			dlog("    RR is Infinite, Primary C10M is Infinite.")
			c10mPrimary = math.Inf(1)
			adjustment = 0.0 // Adjustment doesn't apply
		} else {
			// Finite rounds needed, calculate adjustment factor
			if RR <= 1.0 { // Should only be RR=1.0 after validation
				adjustment = 0.0 // No adjustment for single round
				dlog("    Adj: RR <= 1.0 -> adj = 0.0")
			} else {
				adjustment = 1.0 - (1.0 / RR) // Adjustment factor based on rounds
				dlog("    Adj: 1.0 - 1.0/%.2f = %.4f", RR, adjustment)
			}

			// Calculate extra cost component (related to partial fills over rounds)
			var extra float64 = 0.0
			if adjustment > 0 { // Only calculate if adjustment applies (RR > 1)
				// This formula seems complex and possibly specific to Hypixel's model.
				// It involves sum of integers up to RR.
				RRint := int(RR)                       // Use integer RR for summation
				sumK := float64(RRint*(RRint+1)) / 2.0 // Sum of 1 to RRint
				extra = sellP * (qty*RR - IF*sumK)
				if extra < 0 { // Ensure extra cost isn't negative
					dlog("    WARN: Negative Extra Cost (%.2f). Clamping to 0.", extra)
					extra = 0
				}
				dlog("    Extra Cost: sellP*(qty*RR - IF*sumK(RR=%d)) = %.2f", RRint, extra)
			} else {
				dlog("    Extra Cost: Skipped (adj=0).")
			}
			// Final Primary C10M
			c10mPrimary = base + adjustment*extra
			if math.IsInf(c10mPrimary, 0) || math.IsNaN(c10mPrimary) {
				dlog("    Primary C10M calculation resulted in Inf/NaN.")
				c10mPrimary = math.Inf(1) // Ensure infinite if calculation fails
			} else if c10mPrimary < 0 {
				dlog("    WARN: Primary C10M calculation resulted in negative (%.2f). Setting to Inf.", c10mPrimary)
				c10mPrimary = math.Inf(1) // Cost cannot be negative
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
		c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment)

	// err is already set if input validation failed, otherwise it's nil
	return // Returns named variables including the calculated values and err
}

// getBestC10M calculates both C10M values and returns the better one AND its associated simple cost.
// Returns: bestCost, method ("Primary", "Secondary", "N/A"), associatedCost, calculated RR for Primary path, error
// Assumes safeGetProductData, safeGetMetricsData, BAZAAR_ID defined in utils.go
// Assumes calculateC10MInternal defined above
func getBestC10M(
	itemID string,
	quantity float64,
	apiResp *HypixelAPIResponse,
	metricsMap map[string]ProductMetrics,
) (bestCost float64, bestMethod string, associatedCost float64, rrValue float64, err error) {

	itemIDNorm := BAZAAR_ID(itemID) // Normalize ID
	dlog("Getting Best C10M for %.2f x %s", quantity, itemIDNorm)

	// Initialize return values
	bestCost = math.Inf(1)
	bestMethod = "N/A"
	associatedCost = math.NaN()
	rrValue = math.NaN() // RR specifically relates to the primary calculation path

	if quantity <= 0 {
		err = fmt.Errorf("quantity must be positive (got %.2f)", quantity)
		return 0, "N/A", 0, 0, err // Return 0s for cost/RR on invalid qty.
	}

	// Get data from caches using helper functions
	productData, apiOk := safeGetProductData(apiResp, itemIDNorm)        // Assumes defined in utils.go
	metricsData, metricsOk := safeGetMetricsData(metricsMap, itemIDNorm) // Assumes defined in utils.go

	var sellP, buyP float64 = math.NaN(), math.NaN() // Store prices for associated cost calc and C10MInternal

	// --- Validate API Data & Prices ---
	if !apiOk {
		dlog("  [%s] API data not found.", itemIDNorm)
		err = fmt.Errorf("API data not found for %s", itemIDNorm)
		// Cannot calculate either C10M without API prices
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
		if sellP <= 0 || buyP <= 0 || math.IsNaN(sellP) || math.IsNaN(buyP) {
			errMsg := fmt.Sprintf("invalid prices from API (sP: %.2f, bP: %.2f)", sellP, buyP)
			dlog("  [%s] %s", itemIDNorm, errMsg)
			err = fmt.Errorf(errMsg+" for %s", itemIDNorm)
			// Cannot calculate either C10M with invalid prices
			return math.Inf(1), "N/A", math.NaN(), math.NaN(), err
		}
		dlog("  [%s] Prices - SellP: %.2f, BuyP: %.2f", itemIDNorm, sellP, buyP)
	}

	// --- Validate Metrics Data ---
	if !metricsOk {
		// Metrics missing: We can *only* calculate Secondary C10M. Primary is impossible.
		dlog("  [%s] Metrics data not found. Primary C10M calculation skipped.", itemIDNorm)
		err = fmt.Errorf("metrics not found for %s", itemIDNorm) // Store the error

		// Calculate Secondary C10M
		c10mSec := quantity * buyP
		// Validate Secondary C10M
		if math.IsNaN(c10mSec) || c10mSec < 0 || math.IsInf(c10mSec, 0) {
			dlog("  [%s] Secondary C10M calculation failed (%.2f) even without metrics.", itemIDNorm, c10mSec)
			// If secondary also fails (e.g., price was somehow bad despite earlier check), return Inf/N/A
			return math.Inf(1), "N/A", math.NaN(), math.NaN(), fmt.Errorf("metrics missing and secondary C10M failed for %s", itemIDNorm)
		}

		// Secondary is the only valid option
		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = bestCost // For secondary, associated cost IS the C10M cost
		rrValue = math.NaN()      // RR is not applicable to secondary path
		dlog("  [%s] Using Secondary C10M (%.2f) due to missing metrics.", itemIDNorm, bestCost)
		// Return the calculated secondary cost and the error indicating metrics were missing
		return bestCost, bestMethod, associatedCost, rrValue, err
	}

	// --- Both API (with valid prices) and Metrics data available ---
	dlog("  [%s] Both API and Metrics data available. Calculating C10M...", itemIDNorm)
	var c10mPrim, c10mSec float64
	var calcErr error
	// Pass extracted prices and metrics data directly to the internal calculator
	c10mPrim, c10mSec, _, rrValue, _, _, calcErr = calculateC10MInternal(itemIDNorm, quantity, sellP, buyP, metricsData)

	if calcErr != nil {
		// If the internal calculation itself reported an error (e.g., bad inputs despite checks)
		dlog("  [%s] Error during C10M internal calculation: %v", itemIDNorm, calcErr)
		if err == nil { // Preserve original error (e.g., metrics missing) if it exists
			err = calcErr
		}
		// Costs might be NaN/Inf due to error, comparison below will handle it.
	}

	// --- Determine Best C10M and Associated Cost ---
	// Validate calculated costs again just in case internal logic had issues
	validPrim := !math.IsInf(c10mPrim, 0) && !math.IsNaN(c10mPrim) && c10mPrim >= 0
	validSec := !math.IsInf(c10mSec, 0) && !math.IsNaN(c10mSec) && c10mSec >= 0

	if validPrim && validSec {
		// Both primary and secondary are valid, finite, non-negative numbers
		if c10mPrim <= c10mSec {
			bestCost = c10mPrim
			bestMethod = "Primary"
			associatedCost = quantity * sellP // Simple sell price for primary path
			dlog("  [%s] Primary (%.2f) <= Secondary (%.2f). Using Primary. Assoc=%.2f", itemIDNorm, c10mPrim, c10mSec, associatedCost)
		} else {
			bestCost = c10mSec
			bestMethod = "Secondary"
			associatedCost = quantity * buyP // Simple buy price for secondary path
			rrValue = math.NaN()             // RR doesn't apply if Secondary is chosen
			dlog("  [%s] Secondary (%.2f) < Primary (%.2f). Using Secondary. Assoc=%.2f", itemIDNorm, c10mSec, c10mPrim, associatedCost)
		}
	} else if validPrim {
		// Only Primary is valid
		bestCost = c10mPrim
		bestMethod = "Primary"
		associatedCost = quantity * sellP
		dlog("  [%s] Secondary Invalid, using Primary (%.2f). Assoc=%.2f", itemIDNorm, bestCost, associatedCost)
	} else if validSec {
		// Only Secondary is valid
		bestCost = c10mSec
		bestMethod = "Secondary"
		associatedCost = quantity * buyP
		rrValue = math.NaN() // RR doesn't apply
		dlog("  [%s] Primary Invalid, using Secondary (%.2f). Assoc=%.2f", itemIDNorm, bestCost, associatedCost)
	} else {
		// Neither Primary nor Secondary is valid
		bestCost = math.Inf(1)
		bestMethod = "N/A"
		associatedCost = math.NaN()
		rrValue = math.NaN()
		dlog("  [%s] Both C10M results invalid.", itemIDNorm)
		if err == nil { // If no specific calc error, create a generic one
			err = fmt.Errorf("failed to determine valid C10M for %s (results invalid)", itemIDNorm)
		}
	}

	// Ensure associatedCost is NaN if it's invalid
	if math.IsNaN(associatedCost) || associatedCost < 0 {
		associatedCost = math.NaN()
	}
	// Ensure rrValue is NaN if it's invalid or not applicable
	if math.IsNaN(rrValue) || math.IsInf(rrValue, 0) {
		rrValue = math.NaN()
	}

	dlog("  [%s] Best C10M Result: Cost=%.2f, Method=%s, AssocCost=%.2f, RR=%.2f, Err=%v", itemIDNorm, bestCost, bestMethod, associatedCost, rrValue, err)
	return bestCost, bestMethod, associatedCost, rrValue, err // Return error status along with results
}
