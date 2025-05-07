package main

import (
	"fmt"
	"math" // Needed for Inf, IsNaN, Max, Ceil
	// "log" // Only needed if adding extra logging
)

// --- calculateInstasellFillTime ---
// Uses LIVE BuyMovingWeek data from the API cache product data.
// Calculates estimated time for an insta-sell order (selling to buy orders) to fill.
func calculateInstasellFillTime(qty float64, productData HypixelProduct) (float64, error) {
	dlog("Calculating Instasell Fill Time for qty %.2f of %s", qty, productData.ProductID) // Assumes dlog defined
	if qty <= 0 {
		dlog("  Qty <= 0, instasell fill time is 0.")
		return 0, nil // No time needed for zero quantity
	}

	// Use the live BuyMovingWeek from the quick status
	buyMovingWeek := productData.QuickStatus.BuyMovingWeek
	dlog("  Using live BuyMovingWeek: %.2f", buyMovingWeek)

	if buyMovingWeek <= 0 {
		// If there's no buy volume over the past week, assume it takes infinite time
		dlog("  Live BuyMovingWeek <= 0, instasell fill time is Infinite.")
		// Return Inf and an error indicating the reason
		return math.Inf(1), fmt.Errorf("live BuyMovingWeek is <= 0 for %s", productData.ProductID)
	}

	// Calculate buy rate per second
	secondsInWeek := 604800.0 // 7 days * 24 hours * 60 minutes * 60 seconds
	buyRatePerSecond := buyMovingWeek / secondsInWeek
	dlog("  Buy rate per second: %.5f", buyRatePerSecond)

	// Safety check for rate calculation (shouldn't happen if buyMovingWeek > 0)
	if buyRatePerSecond <= 0 {
		dlog("  WARN: buyRatePerSecond <= 0 despite buyMovingWeek > 0. Fill time Infinite.")
		return math.Inf(1), fmt.Errorf("calculated buy rate per second is <= 0 for %s", productData.ProductID)
	}

	// Calculate time to fill
	timeToFill := qty / buyRatePerSecond
	dlog("  Calculated Instasell Fill Time = qty / rate = %.2f / %.5f = %.4f seconds", qty, buyRatePerSecond, timeToFill)

	// Final validation on the calculated time
	if math.IsNaN(timeToFill) || math.IsInf(timeToFill, 0) || timeToFill < 0 {
		dlog("  WARN: Instasell time validation failed (%.4f). Setting to Inf.", timeToFill)
		// Return Inf and an error indicating calculation failure
		return math.Inf(1), fmt.Errorf("instasell time calculation resulted in invalid value (%.4f) for %s", timeToFill, productData.ProductID)
	}

	dlog("  Instasell Fill Time Result: %.4f seconds", timeToFill)
	return timeToFill, nil // Return calculated time and nil error
}

// --- calculateBuyOrderFillTime ---
// Calculates the estimated time to fill a buy order (buying from sell orders) using HISTORICAL metrics data.
// Returns: fillTime (seconds), calculated RR (for context, used in C10M), error
func calculateBuyOrderFillTime(itemID string, quantity float64, metricsData ProductMetrics) (float64, float64, error) {
	// Uses normalized ID internally via metrics lookup
	normItemID := BAZAAR_ID(itemID) // Assumes BAZAAR_ID in utils
	dlog("Calculating Buy Order Fill Time for %.0f x %s using provided metrics", quantity, normItemID)

	// Initialize return values
	rrValue := math.NaN()  // Default RR (Refill Rounds)
	fillTime := math.NaN() // Default fill time
	var calcErr error      // To store errors during calculation

	if quantity <= 0 {
		dlog("  Quantity <= 0, returning 0 time, 0 RR, nil error")
		return 0, 0, nil // Zero quantity takes zero time
	}

	// Use provided metrics data (already fetched by caller)
	pm := metricsData
	dlog("  Using Metrics: SS=%.2f, SF=%.2f, OS=%.2f, OF=%.2f", pm.SellSize, pm.SellFrequency, pm.OrderSize, pm.OrderFrequency)

	// --- Calculations (Mirroring C10M logic where applicable for consistency) ---
	// Clamp metrics to be non-negative
	ss := math.Max(0, pm.SellSize)       // Avg size of items listed for sale (Sell Offers)
	sf := math.Max(0, pm.SellFrequency)  // Rate at which Sell Offers appear/are filled
	osz := math.Max(0, pm.OrderSize)     // Avg size of Buy Orders being placed
	of := math.Max(0, pm.OrderFrequency) // Rate at which Buy Orders appear/are filled

	dlog("  Clamped Metrics: ss=%.4f, sf=%.4f, osz=%.4f, of=%.4f", ss, sf, osz, of)

	// Calculate Supply Rate (items becoming available on sell orders per unit time)
	supplyRate := ss * sf // Avg items per sell offer * sell offer frequency
	// Calculate Demand Rate (items being sought via buy orders per unit time)
	demandRate := osz * of // Avg items per buy order * buy order frequency
	dlog("  Rates: Supply=%.4f, Demand=%.4f", supplyRate, demandRate)

	// Calculate Delta Ratio (Supply Rate / Demand Rate) - consistent with C10M
	var deltaRatio float64
	if demandRate <= 0 {
		if supplyRate <= 0 {
			deltaRatio = 1.0 // No activity
		} else {
			deltaRatio = math.Inf(1) // Supply exists, no demand
		}
	} else {
		deltaRatio = supplyRate / demandRate
	}
	dlog("  DeltaRatio (SR/DR): %.4f", deltaRatio)

	// --- Logic Switch based on DeltaRatio (consistent with C10M approach) ---
	if deltaRatio > 1.0 {
		// Supply > Demand: Buy order should fill relatively quickly, limited by Demand Rate.
		// How quickly? Time = Quantity / Rate. The rate limiting factor is how fast people are buying (Demand Rate).
		dlog("  DeltaRatio > 1.0: Supply > Demand.")
		rrValue = 1.0 // Consistent with C10M, only one 'round' needed as supply is abundant
		if demandRate <= 0 {
			// If supply > 0 but demand = 0, it takes infinite time
			dlog("  WARN: Demand rate is <= 0 despite DeltaRatio > 1. Fill time Infinite.")
			fillTime = math.Inf(1)
			calcErr = fmt.Errorf("demand rate is zero, cannot fill buy order for %s", normItemID)
		} else {
			fillTime = quantity / demandRate // Time = Quantity / Rate of consumption
			dlog("  Fill Time = quantity / DemandRate = %.1f / %.4f = %.4f seconds", quantity, demandRate, fillTime)
		}

	} else { // DeltaRatio <= 1.0: Demand >= Supply - Buy order fill time is limited by Supply Rate.
		dlog("  DeltaRatio <= 1.0: Demand >= Supply.")

		// Calculate RR value (Refill Rounds) - needed for C10M context, even if not directly used for time here.
		// Calculate IF = ss * (sf / of) - Instafill equivalent amount per order cycle
		var calculatedIF float64
		if of <= 0 {
			calculatedIF = 0 // Cannot calculate if order frequency is zero
			dlog("  IF Calc: Order Frequency (of) <= 0. Setting IF to 0.")
		} else {
			calculatedIF = ss * (sf / of) // Note: This is IF = SupplyRate / OrderFrequency
			if calculatedIF < 0 {         // Should not happen with Max(0, ...) but check
				calculatedIF = 0
			}
			dlog("  IF Calc: (ss*sf)/of = (%.4f*%.4f)/%.4f = %.4f", ss, sf, of, calculatedIF)
		}
		dlog("  Final Calculated IF: %.4f", calculatedIF)

		// Calculate RR based on this IF (consistent with C10M)
		if calculatedIF <= 0 {
			if supplyRate <= 0 {
				rrValue = math.Inf(1) // Infinite rounds if no supply
				dlog("  RR Calc: IF=0 due to Supply Rate <= 0. Setting RR=Inf.")
			} else { // Supply exists, but IF=0 (likely of=0)
				rrValue = math.Inf(1) // Also infinite rounds if orders aren't happening to consume supply
				dlog("  RR Calc: IF=0 but Supply Rate > 0 (likely OF=0). Setting RR=Inf.")
			}
		} else {
			rrValue = math.Ceil(quantity / calculatedIF)
			if rrValue < 1 { // Ensure RR is at least 1 if not infinite
				rrValue = 1
			}
			dlog("  RR Calc: IF > 0. RR = Ceil(%.1f / %.4f) = %.1f", quantity, calculatedIF, rrValue)
		}
		// Validate RR
		if math.IsNaN(rrValue) { // Should not happen with checks above
			dlog("  WARN: RR resulted in NaN. Setting to Inf.")
			rrValue = math.Inf(1)
		}

		// Calculate Fill Time: Limited by Supply Rate
		// Time = Quantity / Rate. The limiting rate is how fast items become available (Supply Rate).
		if supplyRate <= 0 {
			dlog("  Fill Time Calc: Supply Rate is 0. Fill time Infinite.")
			fillTime = math.Inf(1)
			if calcErr == nil { // Set error if not already set
				calcErr = fmt.Errorf("supply rate is zero, cannot fill buy order for %s", normItemID)
			}
		} else {
			fillTime = quantity / supplyRate
			dlog("  Fill Time = quantity / SupplyRate = %.1f / %.4f = %.4f seconds", quantity, supplyRate, fillTime)
		}
	}

	// --- Final Validation and Return ---
	// Validate calculated fillTime
	if math.IsNaN(fillTime) || math.IsInf(fillTime, -1) || fillTime < 0 {
		dlog("  WARN: Final buy order fill time validation failed (%.4f). Setting to Inf.", fillTime)
		fillTime = math.Inf(1)
		if calcErr == nil {
			calcErr = fmt.Errorf("buy order fill time calculation resulted in invalid value (%.4f) for %s", fillTime, normItemID)
		}
	}
	// Validate rrValue for return consistency (NaN indicates impossibility)
	if math.IsNaN(rrValue) || math.IsInf(rrValue, 0) {
		// Don't overwrite fillTime if it was already set to Inf
		if !math.IsInf(fillTime, 1) && calcErr == nil {
			// If fill time calculation didn't already determine Inf, but RR is Inf/NaN, set time to Inf
			// This handles cases where RR calc fails but time calc might have seemed okay initially.
			dlog("  WARN: RR is Inf/NaN (%.2f), forcing fill time to Inf.", rrValue)
			fillTime = math.Inf(1)
			calcErr = fmt.Errorf("buy order fill time depends on RR which is invalid (%.2f) for %s", rrValue, normItemID)
		}
		// Ensure returned RR is NaN if it was Inf/NaN
		rrValue = math.NaN()
	}

	dlog("  Returning Buy Order Fill Time: %.4f seconds, RR: %.2f", fillTime, rrValue)
	return fillTime, rrValue, calcErr // Return time, RR value, and any error encountered
}
