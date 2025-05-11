package main

import (
	"fmt"
	"math"
)

// --- calculateInstasellFillTime ---
// (No changes to this function based on your request for calculateBuyOrderFillTime)
func calculateInstasellFillTime(qty float64, productData HypixelProduct) (float64, error) {
	dlog("Calculating Instasell Fill Time for qty %.2f of %s", qty, productData.ProductID)
	if qty <= 0 {
		dlog("  Qty <= 0, instasell fill time is 0.")
		return 0, nil
	}

	buyMovingWeek := productData.QuickStatus.BuyMovingWeek
	dlog("  Using live BuyMovingWeek: %.2f", buyMovingWeek)

	if buyMovingWeek <= 0 {
		dlog("  Live BuyMovingWeek <= 0, instasell fill time is Infinite.")
		return math.Inf(1), fmt.Errorf("live BuyMovingWeek is <= 0 for %s", productData.ProductID)
	}

	secondsInWeek := 604800.0
	buyRatePerSecond := buyMovingWeek / secondsInWeek
	dlog("  Buy rate per second: %.5f", buyRatePerSecond)

	if buyRatePerSecond <= 0 {
		dlog("  WARN: buyRatePerSecond <= 0 despite buyMovingWeek > 0. Fill time Infinite.")
		return math.Inf(1), fmt.Errorf("calculated buy rate per second is <= 0 for %s", productData.ProductID)
	}

	timeToFill := qty / buyRatePerSecond
	dlog("  Calculated Instasell Fill Time = qty / rate = %.2f / %.5f = %.4f seconds", qty, buyRatePerSecond, timeToFill)

	if math.IsNaN(timeToFill) || math.IsInf(timeToFill, 0) || timeToFill < 0 {
		dlog("  WARN: Instasell time validation failed (%.4f). Setting to Inf.", timeToFill)
		return math.Inf(1), fmt.Errorf("instasell time calculation resulted in invalid value (%.4f) for %s", timeToFill, productData.ProductID)
	}

	dlog("  Instasell Fill Time Result: %.4f seconds", timeToFill)
	return timeToFill, nil
}

// --- calculateBuyOrderFillTime ---
// MODIFIED to follow the LaTeX formula.
// Returns: fillTime (seconds), calculated RR (local to this function for the formula), error
func calculateBuyOrderFillTime(itemID string, quantity float64, metricsData ProductMetrics) (float64, float64, error) {
	normItemID := BAZAAR_ID(itemID)
	dlog("Calculating Buy Order Fill Time for %.0f x %s using LaTeX formula logic", quantity, normItemID)

	var calculatedRR float64 = math.NaN() // RR used in the LaTeX formula if Δ <= 0
	fillTime := math.NaN()
	var calcErr error

	if quantity <= 0 {
		dlog("  Quantity <= 0, returning 0 time, NaN RR, nil error")
		return 0, math.NaN(), nil // Zero quantity takes zero time, RR not applicable
	}

	pm := metricsData
	dlog("  Using Metrics: SS=%.2f, SF=%.2f, OS=%.2f, OF=%.2f", pm.SellSize, pm.SellFrequency, pm.OrderSize, pm.OrderFrequency)

	// Clamp metrics to be non-negative
	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_s := math.Max(0, pm.OrderSize)      // Corresponds to o_s in LaTeX
	o_f := math.Max(0, pm.OrderFrequency) // Corresponds to o_f in LaTeX

	dlog("  Clamped Metrics: s_s=%.4f, s_f=%.4f, o_s=%.4f, o_f=%.4f", s_s, s_f, o_s, o_f)

	// Calculate Net flow rate (Δ)
	deltaNetFlow := (s_s * s_f) - (o_s * o_f)
	dlog("  Net Flow Rate (Δ) = (s_s * s_f) - (o_s * o_f) = (%.4f * %.4f) - (%.4f * %.4f) = %.4f",
		s_s, s_f, o_s, o_f, deltaNetFlow)

	if deltaNetFlow > 0 {
		dlog("  Δ > 0 (%.4f), using Fill Time = (20 * qty) / Δ", deltaNetFlow)
		if deltaNetFlow == 0 { // Should be caught by > 0, but defensive
			fillTime = math.Inf(1)
			calcErr = fmt.Errorf("net flow rate (Δ) is zero, cannot calculate fill time for %s", normItemID)
			dlog("    Δ is zero, fill time is Infinite.")
		} else {
			fillTime = (20.0 * quantity) / deltaNetFlow
			dlog("    Fill Time = (20 * %.2f) / %.4f = %.4f", quantity, deltaNetFlow, fillTime)
		}
		// RR is not directly used in this branch of the formula, return NaN or 1?
		// Let's calculate it for context anyway, as it was part of the original return.
		// This IF/RR is based on the simpler model, not the complex LaTeX IF.
		var localIF float64
		if o_f <= 0 {
			localIF = 0
		} else {
			localIF = s_s * (s_f / o_f) // IF = SupplyRate / OrderFrequency (for one type of order)
		}
		if localIF <= 0 {
			calculatedRR = math.Inf(1) // Or 1 if qty is 0, but qty > 0 here
		} else {
			calculatedRR = math.Ceil(quantity / localIF)
			if calculatedRR < 1 {
				calculatedRR = 1
			}
		}

	} else { // Δ <= 0
		dlog("  Δ <= 0 (%.4f), using Fill Time = (20 * RR * qty) / o_f", deltaNetFlow)
		// Calculate RR for this formula. This IF/RR is based on the simpler model.
		// The LaTeX formula's RR likely depends on its own IF definition.
		// For now, using the simpler IF -> RR derivation as before for this branch.
		var localIF float64
		if o_f <= 0 { // If order frequency is 0, localIF is 0
			localIF = 0
			dlog("    o_f is 0, so localIF is 0 for RR calculation.")
		} else {
			localIF = s_s * (s_f / o_f) // IF = SupplyRate / OrderFrequency for one type of order
			if localIF < 0 {
				localIF = 0
			}
			dlog("    Calculated localIF for RR: %.4f", localIF)
		}

		if localIF <= 0 {
			// If IF is 0, it implies infinite rounds are needed to fill any positive quantity
			// unless quantity itself is 0 (which is handled at the start).
			calculatedRR = math.Inf(1)
			dlog("    localIF <= 0, so calculatedRR is Infinite.")
		} else {
			calculatedRR = math.Ceil(quantity / localIF)
			if calculatedRR < 1 && !math.IsInf(calculatedRR, 0) { // Ensure RR is at least 1 if not infinite
				calculatedRR = 1.0
			}
			dlog("    Calculated RR for formula: %.2f", calculatedRR)
		}
		if math.IsNaN(calculatedRR) {
			calculatedRR = math.Inf(1)
		} // Safeguard

		if o_f <= 0 {
			dlog("    o_f is 0, cannot divide. Fill time is Infinite.")
			fillTime = math.Inf(1)
			if calcErr == nil {
				calcErr = fmt.Errorf("order frequency (o_f) is zero and Δ <= 0, cannot calculate fill time for %s", normItemID)
			}
		} else if math.IsInf(calculatedRR, 1) {
			dlog("    CalculatedRR is Infinite, fill time is Infinite.")
			fillTime = math.Inf(1)
			if calcErr == nil {
				calcErr = fmt.Errorf("calculated RR is infinite and Δ <= 0 for %s", normItemID)
			}
		} else {
			fillTime = (20.0 * calculatedRR * quantity) / o_f
			dlog("    Fill Time = (20 * %.2f * %.2f) / %.4f = %.4f", calculatedRR, quantity, o_f, fillTime)
		}
	}

	// Final Validation
	if math.IsNaN(fillTime) || fillTime < 0 { // Allow Inf
		dlog("  WARN: Final fill time validation failed (NaN or negative: %.4f). Setting to Inf.", fillTime)
		fillTime = math.Inf(1)
		if calcErr == nil {
			calcErr = fmt.Errorf("fill time calculation resulted in invalid value (%.4f) for %s", fillTime, normItemID)
		}
	}
	if math.IsNaN(calculatedRR) || math.IsInf(calculatedRR, 0) { // Ensure returned RR is NaN if it was Inf or NaN
		calculatedRR = math.NaN()
	}

	dlog("  Returning Buy Order Fill Time (LaTeX logic): %.4f seconds, CalculatedRR (for formula context): %.2f", fillTime, calculatedRR)
	return fillTime, calculatedRR, calcErr
}
