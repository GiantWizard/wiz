package main

import (
	"fmt"
	"math"
)

// --- calculateInstasellFillTime ---
// Uses LIVE BuyMovingWeek data from the API cache.
func calculateInstasellFillTime(qty float64, productData HypixelProduct) (float64, error) {
	dlog("Calculating Instasell Fill Time for qty %.2f", qty)
	if qty <= 0 {
		return 0, nil // No time needed for zero quantity
	}

	buyMovingWeek := productData.QuickStatus.BuyMovingWeek
	dlog("  Using live BuyMovingWeek: %.2f", buyMovingWeek)

	if buyMovingWeek <= 0 {
		dlog("  Live BuyMovingWeek <= 0, instasell fill time is Infinite.")
		return math.Inf(1), fmt.Errorf("live BuyMovingWeek is <= 0")
	}

	secondsInWeek := 604800.0 // 7 * 24 * 60 * 60
	buyRatePerSecond := buyMovingWeek / secondsInWeek
	dlog("  Buy rate per second: %.5f", buyRatePerSecond)

	if buyRatePerSecond <= 0 {
		// This shouldn't happen if buyMovingWeek > 0, but check anyway
		dlog("  WARN: buyRatePerSecond <= 0 despite buyMovingWeek > 0. Fill time Infinite.")
		return math.Inf(1), fmt.Errorf("calculated buy rate per second is <= 0")
	}

	timeToFill := qty / buyRatePerSecond
	dlog("  Calculated Instasell Fill Time = qty / rate = %.2f / %.5f = %.4f seconds", qty, buyRatePerSecond, timeToFill)

	// Final validation
	if math.IsNaN(timeToFill) || math.IsInf(timeToFill, 0) || timeToFill < 0 {
		dlog("  WARN: Instasell time validation failed (%.4f). Setting to Inf.", timeToFill)
		return math.Inf(1), fmt.Errorf("instasell time calculation resulted in invalid value")
	}
	return timeToFill, nil
}

// --- calculateBuyOrderFillTime ---
// Calculates the estimated time to fill a buy order using metrics data.
// Returns: fillTime (seconds), calculated RR, error
func calculateBuyOrderFillTime(itemID string, quantity float64, metricsData ProductMetrics) (float64, float64, error) {
	dlog("Calculating Buy Order Fill Time for %.0f x %s using provided metrics", quantity, itemID)
	rrValue := math.NaN()  // Default RR
	fillTime := math.NaN() // Default fill time

	if quantity <= 0 {
		dlog("  Quantity <= 0, returning 0 time, 0 RR, nil error")
		return 0, 0, nil
	}

	// Use provided metrics data
	pm := metricsData
	dlog("  Using Metrics: SS=%.2f, SF=%.2f, OS=%.2f, OF=%.2f", pm.SellSize, pm.SellFrequency, pm.OrderSize, pm.OrderFrequency)

	// --- Calculations (mirroring C10M logic for consistency) ---
	ss := math.Max(0, pm.SellSize)
	sf := math.Max(0, pm.SellFrequency)
	osz := math.Max(0, pm.OrderSize) // Use 'osz' for order size to avoid conflict
	of := math.Max(0, pm.OrderFrequency)
	dlog("  Clamped Metrics: ss=%.4f, sf=%.4f, osz=%.4f, of=%.4f", ss, sf, osz, of)

	supplyRate := ss * sf
	demandRate := osz * of
	dlog("  Rates: Supply=%.4f, Demand=%.4f", supplyRate, demandRate)

	var deltaRatio float64
	if demandRate <= 0 {
		if supplyRate <= 0 {
			deltaRatio = 1.0
		} else {
			deltaRatio = math.Inf(1)
		}
	} else {
		deltaRatio = supplyRate / demandRate
	}
	dlog("  DeltaRatio (SR/DR): %.4f", deltaRatio)

	// --- Logic Switch based on DeltaRatio (consistent with C10M) ---
	if deltaRatio > 1.0 {
		// Supply > Demand: Use simpler formula based on difference (delta)
		// This case suggests orders fill relatively quickly as supply outpaces demand.
		// The formula (20 * qty) / (SR - DR) is heuristic.
		// Let's also keep RR=1 consistent with C10M.
		dlog("  DeltaRatio > 1.0: Using heuristic formula based on rate difference.")
		rrValue = 1.0 // Consistent with C10M logic for this case
		deltaDifference := supplyRate - demandRate
		if deltaDifference <= 0 { // Safety check, should not happen if ratio > 1
			dlog("  WARN: DeltaDifference <= 0 despite DeltaRatio > 1. Fill time Infinite.")
			fillTime = math.Inf(1)
		} else {
			// Heuristic: Assume fill rate is proportional to the *excess* supply rate.
			// Factor 20 is arbitrary, maybe related to game ticks? Let's use it as given.
			// fillTime = (20.0 * quantity) / deltaDifference // Original formula from filltime.go
			// Alternative interpretation: Time = Quantity / FillRate. FillRate might be demandRate?
			// Let's stick to the original formula provided for now.
			fillTime = (20.0 * quantity) / deltaDifference
			dlog("  Fill Time = (20 * %.1f) / %.4f = %.4f seconds", quantity, deltaDifference, fillTime)
		}

	} else { // DeltaRatio <= 1.0: Demand >= Supply - Use formula based on IF and RR
		// This case suggests orders fill slower, limited by supply refills.
		dlog("  DeltaRatio <= 1.0: Using formula based on IF and RR.")

		// Calculate IF = ss * (sf / of)
		var calculatedIF float64
		if of <= 0 {
			calculatedIF = 0
			dlog("  IF Calc: Order Frequency (of) <= 0. Setting IF to 0.")
		} else {
			calculatedIF = ss * (sf / of) // Note: This is IF = SupplyRate / OrderFrequency
			if calculatedIF < 0 {
				calculatedIF = 0
			}
			dlog("  IF Calc: (ss*sf)/of = (%.4f*%.4f)/%.4f = %.4f", ss, sf, of, calculatedIF)
		}
		dlog("  Final Calculated IF: %.4f", calculatedIF)

		// Calculate RR based on this IF
		if calculatedIF <= 0 {
			// If IF is 0 because supply rate is 0, RR should be Inf.
			// If IF is 0 because o_f is 0, RR calculation would div-by-zero.
			if supplyRate <= 0 {
				rrValue = math.Inf(1)
				dlog("  RR Calc: IF is 0 due to Supply Rate <= 0. Setting RR=Inf.")
			} else if of <= 0 {
				rrValue = math.NaN() // Indicate calculation impossibility
				dlog("  RR Calc: IF is 0 due to Order Frequency <= 0. Setting RR=NaN.")
			} else {
				// Should not happen if IF=0 and SR>0, OF>0, but fallback
				rrValue = 1.0
				dlog("  RR Calc: IF <= 0 (unexpected case). Setting RR=1.0")
			}
		} else {
			rrValue = math.Ceil(quantity / calculatedIF)
			if rrValue < 1 {
				rrValue = 1
			} // RR must be at least 1
			dlog("  RR Calc: IF > 0. RR = Ceil(%.1f / %.4f) = %.1f", quantity, calculatedIF, rrValue)
		}
		// Validate RR
		if math.IsNaN(rrValue) {
			dlog("  RR resulted in NaN (likely OF=0). Fill time Infinite.")
			fillTime = math.Inf(1)
		} else if math.IsInf(rrValue, 0) { // Check positive/negative Inf
			dlog("  RR resulted in Inf (likely IF=0 due to SR=0). Fill time Infinite.")
			fillTime = math.Inf(1)
		} else {
			// RR is a valid finite number (>= 1)
			// Calculate fill time using RR
			// Formula: fillTime = (20 * RR * quantity) / of
			// This formula seems dimensionally strange (time = time * unitless * qty / (qty/time) = time^2 ??)
			// Let's reconsider the fill time logic.
			// If RR refills are needed, and each refill cycle is related to order frequency 'of',
			// maybe the time per cycle is 1/of? Or related to sell frequency 'sf'?
			// Let's assume the provided formula intended something like:
			// Time = Number of cycles (RR) * Time per cycle
			// Time per cycle might be related to how fast orders appear (1/of)?
			// Or how fast items are supplied (Quantity / SupplyRate)?

			// Let's try a simpler logic: Time = Total Quantity / Effective Fill Rate
			// Effective Fill Rate might be limited by the Demand Rate (osz * of) when SR > DR?
			// Or by the Supply Rate (ss * sf) when DR > SR?
			// When DR >= SR (DeltaRatio <= 1), the bottleneck is SupplyRate.
			// Time = quantity / supplyRate (where supplyRate = ss * sf)
			if supplyRate <= 0 {
				dlog("  Fill Time Calc: Supply Rate is 0. Fill time Infinite.")
				fillTime = math.Inf(1)
			} else {
				fillTime = quantity / supplyRate
				dlog("  Fill Time = quantity / SupplyRate = %.1f / %.4f = %.4f seconds", quantity, supplyRate, fillTime)
			}
			// We still return the calculated rrValue, even if not used in this fill time formula.
			// The RR value from C10M is important for understanding cost structure.
		}
	}

	// Final validation for calculated fillTime
	var finalErr error
	if math.IsNaN(fillTime) || math.IsInf(fillTime, -1) || fillTime < 0 {
		dlog("  WARN: Final buy order fill time validation failed (%.4f). Setting to Inf.", fillTime)
		fillTime = math.Inf(1)
		finalErr = fmt.Errorf("buy order fill time calculation resulted in invalid value")
	}
	if math.IsNaN(rrValue) {
		dlog("  WARN: Final RR is NaN (likely OrderFrequency was 0).")
		// Don't overwrite fillTime if it was already set to Inf
		if !math.IsInf(fillTime, 1) {
			fillTime = math.Inf(1) // RR NaN implies infinite time
			finalErr = fmt.Errorf("buy order fill time depends on RR which is NaN")
		}
	}

	dlog("  Returning Buy Order Fill Time: %.4f seconds, RR: %.2f", fillTime, rrValue)
	return fillTime, rrValue, finalErr
}
