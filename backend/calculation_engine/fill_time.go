// time_calculations.go
package main

import (
	"fmt"
	"math"
)

// calculateInstasellFillTime calculates the time to instasell a quantity of an item.
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
		// Return Inf(1) as this function's contract implies calculable time or an error state represented by Inf.
		// The caller (PerformDualExpansion) will convert this to NaN if needed for storage.
		return math.Inf(1), fmt.Errorf("live BuyMovingWeek is <= 0 for %s", productData.ProductID)
	}

	secondsInWeek := 604800.0
	buyRatePerSecond := buyMovingWeek / secondsInWeek
	dlog("  Buy rate per second: %.5f", buyRatePerSecond)

	if buyRatePerSecond <= 0 { // Should be caught by buyMovingWeek <=0, but defensive
		dlog("  WARN: buyRatePerSecond <= 0 despite buyMovingWeek > 0. Fill time Infinite.")
		return math.Inf(1), fmt.Errorf("calculated buy rate per second is <= 0 for %s", productData.ProductID)
	}

	timeToFill := qty / buyRatePerSecond
	dlog("  Calculated Instasell Fill Time = qty / rate = %.2f / %.5f = %.4f seconds", qty, buyRatePerSecond, timeToFill)

	// Validate the calculated time
	if math.IsNaN(timeToFill) || math.IsInf(timeToFill, 0) || timeToFill < 0 {
		dlog("  WARN: Instasell time validation failed (%.4f). Setting to Inf.", timeToFill)
		return math.Inf(1), fmt.Errorf("instasell time calculation resulted in invalid value (%.4f) for %s", timeToFill, productData.ProductID)
	}

	dlog("  Instasell Fill Time Result: %.4f seconds", timeToFill)
	return timeToFill, nil
}

// calculateBuyOrderFillTime calculates the buy order fill time based on metrics.
func calculateBuyOrderFillTime(itemID string, quantity float64, metricsData ProductMetrics) (float64, float64, error) {
	normItemID := BAZAAR_ID(itemID) // Assuming BAZAAR_ID is available
	dlog("Calculating Buy Order Fill Time for %.0f x %s using LaTeX formula logic", quantity, normItemID)

	var calculatedRR float64 = math.NaN() // This is the RR for the formula, not necessarily the final RR for the item
	fillTime := math.NaN()                // Default to NaN, will be Inf or a value
	var calcErr error

	if quantity <= 0 {
		dlog("  Quantity <= 0, returning 0 time, NaN RR, nil error")
		return 0, math.NaN(), nil // 0 time, RR is not applicable (NaN)
	}

	pm := metricsData
	dlog("  Using Metrics: SS=%.2f, SF=%.2f, OS=%.2f, OF=%.2f", pm.SellSize, pm.SellFrequency, pm.OrderSize, pm.OrderFrequency)

	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_s_metric := math.Max(0, pm.OrderSize)
	o_f_metric := math.Max(0, pm.OrderFrequency)

	dlog("  Clamped Metrics: s_s=%.4f, s_f=%.4f, o_s_metric=%.4f, o_f_metric=%.4f", s_s, s_f, o_s_metric, o_f_metric)

	deltaNetFlow := (s_s * s_f) - (o_s_metric * o_f_metric)
	dlog("  Net Flow Rate (Δ) = (s_s * s_f) - (o_s_metric * o_f_metric) = (%.4f * %.4f) - (%.4f * %.4f) = %.4f",
		s_s, s_f, o_s_metric, o_f_metric, deltaNetFlow)

	if deltaNetFlow > 0 {
		dlog("  Δ > 0 (%.4f), using Fill Time = (20 * qty) / Δ", deltaNetFlow)
		if deltaNetFlow == 0 { // Should not happen if deltaNetFlow > 0, but defensive
			fillTime = math.Inf(1)
			if calcErr == nil {
				calcErr = fmt.Errorf("deltaNetFlow is zero in positive delta branch for %s", normItemID)
			}
		} else {
			fillTime = (20.0 * quantity) / deltaNetFlow
		}
		dlog("    Fill Time = (20 * %.2f) / %.4f = %.4f", quantity, deltaNetFlow, fillTime)

		// Calculate contextual RR for this branch
		var localIF float64
		if o_f_metric <= 0 || s_f <= 0 { // if o_f_metric is 0, or s_f is 0 (no supply to meet demand)
			localIF = 0
		} else {
			localIF = s_s * (s_f / o_f_metric) // InstaFills per order cycle
		}

		if localIF <= 0 { // if IF is zero or negative, RR is effectively infinite for positive quantity
			calculatedRR = math.Inf(1)
		} else {
			calculatedRR = math.Ceil(quantity / localIF)
			if calculatedRR < 1 && !math.IsInf(calculatedRR, 0) { // Ensure RR is at least 1 unless it's already Inf
				calculatedRR = 1
			}
		}

	} else { // deltaNetFlow <= 0
		dlog("  Δ <= 0 (%.4f), using Fill Time = (20 * RR * qty) / o_f_metric", deltaNetFlow)
		// Calculate contextual RR for this branch
		var localIF float64
		if o_f_metric <= 0 || s_f <= 0 { // if o_f_metric is 0, or s_f is 0
			localIF = 0
			dlog("    o_f_metric or s_f is 0 or less, so localIF is 0 for RR calculation.")
		} else {
			localIF = s_s * (s_f / o_f_metric)
			if localIF < 0 { // Ensure IF is not negative
				localIF = 0
			}
			dlog("    Calculated localIF for RR: %.4f", localIF)
		}

		if localIF <= 0 { // if IF is zero or negative, RR is effectively infinite for positive quantity
			calculatedRR = math.Inf(1)
			dlog("    localIF <= 0, so calculatedRR for formula is Infinite.")
		} else {
			calculatedRR = math.Ceil(quantity / localIF)
			if calculatedRR < 1 && !math.IsInf(calculatedRR, 0) { // Ensure RR is at least 1
				calculatedRR = 1.0
			}
			dlog("    Calculated RR for formula: %.2f", calculatedRR)
		}

		// If calculatedRR became NaN somehow (e.g. 0/0 in IF calc leading to NaN), treat as Inf for the formula
		if math.IsNaN(calculatedRR) {
			calculatedRR = math.Inf(1) // For formula purposes, NaN RR means infinite time effectively
		}

		if o_f_metric <= 0 {
			dlog("    o_f_metric is 0, cannot divide. Fill time is Infinite.")
			fillTime = math.Inf(1)
			if calcErr == nil {
				calcErr = fmt.Errorf("order frequency (o_f_metric) is zero and Δ <= 0, cannot calculate fill time for %s", normItemID)
			}
		} else if math.IsInf(calculatedRR, 1) { // If RR needed for formula is Inf
			dlog("    CalculatedRR for formula is Infinite, fill time is Infinite.")
			fillTime = math.Inf(1)
			if calcErr == nil {
				calcErr = fmt.Errorf("calculated RR for formula is infinite and Δ <= 0 for %s", normItemID)
			}
		} else {
			fillTime = (20.0 * calculatedRR * quantity) / o_f_metric
			dlog("    Fill Time = (20 * %.2f * %.2f) / %.4f = %.4f", calculatedRR, quantity, o_f_metric, fillTime)
		}
	}

	// Final validation of fillTime
	if math.IsNaN(fillTime) || math.IsInf(fillTime, 0) || fillTime < 0 { // Consolidate checks, ensure positive Inf
		dlog("  WARN: Final fill time validation failed (NaN, Inf or negative: %.4f). Setting to Inf.", fillTime)
		fillTime = math.Inf(1) // Ensure it's positive Inf for error state
		if calcErr == nil {    // Set error if not already set by more specific condition
			calcErr = fmt.Errorf("fill time calculation resulted in invalid value for %s", normItemID)
		}
	}

	// For the returned calculatedRR (which is for formula context, not the item's final RR):
	// If it was Inf for the formula, keep it Inf. If it was NaN for the formula, it means something went very wrong, return NaN.
	// This specific `calculatedRR` is more of a diagnostic/intermediate value for this function.
	// The RR stored in BaseIngredientDetail will be handled by valueOrNaN by the caller.
	if math.IsInf(calculatedRR, 0) {
		// Keep Inf as is.
	} else if math.IsNaN(calculatedRR) {
		// Keep NaN as is.
	}

	dlog("  Returning Buy Order Fill Time (LaTeX logic): %.4f seconds, CalculatedRR (for formula context): %.2f, Err: %v", fillTime, calculatedRR, calcErr)
	// fillTime can be Inf. calculatedRR (for formula context) can be Inf or NaN.
	return fillTime, calculatedRR, calcErr
}
