package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
)

// --- Structs ---
type ProductMetrics struct {
	ProductID      string  `json:"product_id"`
	SellSize       float64 `json:"sell_size"`
	SellFrequency  float64 `json:"sell_frequency"`
	OrderSize      float64 `json:"order_size_average"`      // Corrected tag
	OrderFrequency float64 `json:"order_frequency_average"` // Corrected tag
}

type OrderSummary struct {
	PricePerUnit float64 `json:"pricePerUnit"`
}

type HypixelProduct struct {
	SellSummary []OrderSummary `json:"sell_summary"`
	BuySummary  []OrderSummary `json:"buy_summary"`
}

type HypixelAPIResponse struct {
	Success  bool                      `json:"success"`
	Products map[string]HypixelProduct `json:"products"`
}

// --- Debug helper (Optional) ---
var debug = os.Getenv("DEBUG") == "1"

func dlog(format string, args ...interface{}) {
	if debug {
		log.Printf("DEBUG: "+format, args...)
	}
}

// --- Load metrics ---
func loadMetrics(filename string) []ProductMetrics {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read metrics file '%s': %v", filename, err)
	}
	var metrics []ProductMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		log.Fatalf("Failed to parse metrics JSON: %v", err)
	}
	return metrics
}

// --- C10M Calculation ---

// calculateC10M computes primary and secondary C10M values.
// LATEST LOGIC:
// If DeltaRatio > 1: IF=Inf, RR=1, Adj=0, Prim=Base
// If DeltaRatio <= 1: Calculate IF = ss*(sf/of), then calculate RR, Adj, Extra, Prim normally using IF.
// Returns: c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment
func calculateC10M(
	prod string,
	qty float64,
	sellP float64,
	buyP float64,
	metrics []ProductMetrics,
) (c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment float64) {
	// 1) look up metrics record
	var pm ProductMetrics
	found := false
	for _, m := range metrics {
		if m.ProductID == prod {
			pm = m
			found = true
			break
		}
	}
	if !found {
		log.Printf("Warning: Metrics for product '%s' not found. Cannot calculate C10M.", prod)
		return math.Inf(1), math.Inf(1), math.NaN(), math.NaN(), math.NaN(), math.NaN()
	}

	// 2) Clamp Metrics & Calculate Rates / Delta Ratio
	s_s := pm.SellSize
	s_f := pm.SellFrequency
	o_f := pm.OrderFrequency
	o_s := pm.OrderSize

	if s_s < 0 {
		s_s = 0
	}
	if s_f < 0 {
		s_f = 0
	}
	if o_f < 0 {
		o_f = 0
	}
	if o_s < 0 {
		o_s = 0
	}

	supplyRate := s_s * s_f
	demandRate := o_s * o_f
	dlog("Calculated Rates: SupplyRate=%.4f, DemandRate=%.4f", supplyRate, demandRate)

	if demandRate <= 0 {
		if supplyRate <= 0 {
			DeltaRatio = 1.0
		} else {
			DeltaRatio = math.Inf(1)
		}
	} else {
		DeltaRatio = supplyRate / demandRate
	}
	dlog("DeltaRatio (SR/DR): %.4f", DeltaRatio)

	// 3) Calculate IF, RR, Adjustment, and Primary C10M based on DeltaRatio
	base := qty * sellP // Calculate base cost once
	dlog("Base Cost: %.2f", base)

	// ================= NEW LOGIC BLOCK START =================
	if DeltaRatio > 1.0 {
		// Supply > Demand: Simplified logic as requested
		dlog("DeltaRatio > 1.0: Using simplified logic.")
		IF = math.Inf(1)   // Set IF to Infinite
		RR = 1.0           // Set RR to 1
		adjustment = 0.0   // Set adjustment to 0
		c10mPrimary = base // Primary cost is just the base cost

		dlog("  Set IF=Inf, RR=1.0, Adjustment=0.0")
		dlog("  Primary C10M = base = %.2f", c10mPrimary)

	} else { // DeltaRatio <= 1.0: Demand >= Supply - Use full IF/RR logic for Primary Cost
		dlog("DeltaRatio <= 1.0: Using full IF/RR logic for Primary Cost.")

		// Calculate IF using ss * (sf / of)
		if o_f <= 0 {
			IF = 0
			dlog("  IF Calculation: Order Frequency (o_f) is zero. Setting IF to 0.")
		} else {
			IF = s_s * (s_f / o_f)
			dlog("  IF Calculation: ss * (sf / of) = %.4f * (%.4f / %.4f) = %.4f", s_s, s_f, o_f, IF)
		}
		if IF < 0 {
			IF = 0
		}
		dlog("  Final Calculated IF for this branch: %.4f", IF)

		// Calculate RR based on this calculated IF
		if IF <= 0 {
			RR = 1.0
			dlog("  RR Calculation: IF <= 0 -> RR = 1.0")
		} else {
			RR = math.Ceil(qty / IF)
			dlog("  RR Calculation: IF > 0. RR = Ceil(%.2f / %.4f) = %.2f", qty, IF, RR)
		}
		// Validate RR
		if RR < 1 {
			RR = 1.0
		}
		if math.IsInf(RR, 0) || math.IsNaN(RR) {
			RR = math.Inf(1)
		}
		dlog("  Final RR for this branch: %.2f", RR)

		// Handle Infinite RR case for cost calculation
		if math.IsInf(RR, 1) {
			dlog("  RR is Infinite, Primary C10M is Infinite.")
			c10mPrimary = math.Inf(1)
			adjustment = 0.0 // Adjustment is undefined/irrelevant
		} else {
			// Calculate Adjustment Factor only if RR is finite
			if RR == 1.0 {
				adjustment = 0.0
				dlog("  Adjustment Factor: RR is 1.0 -> adj = 0.0")
			} else {
				adjustment = 1.0 - 1.0/RR
				dlog("  Adjustment Factor: 1.0 - 1.0/%.2f = %.4f", RR, adjustment)
			}

			// Calculate Extra Cost only if adjustment > 0 (i.e., RR > 1)
			var extra float64 = 0.0
			if adjustment > 0 {
				RRint := int(RR)
				sumK := float64(RRint*(RRint+1)) / 2.0
				extra = sellP * (qty*RR - IF*sumK)
				dlog("  Extra Cost: sellP * (qty*RR - IF*sumK) = %.2f", extra)
			} else {
				dlog("  Extra Cost: Skipped calculation as adjustment is 0.")
			}

			// Calculate Primary C10M using full formula
			c10mPrimary = base + adjustment*extra
			dlog("  Primary C10M: base + adj*extra = %.2f + %.4f*%.2f = %.2f", base, adjustment, extra, c10mPrimary)
		}
	}
	// ================= NEW LOGIC BLOCK END =================

	// --- 4) Secondary C10M (Remains the same) ---
	c10mSecondary = qty * buyP
	dlog("Secondary C10M = qty * buyP = %.2f", c10mSecondary)

	// --- 5) Final Validation ---
	if math.IsNaN(c10mPrimary) || math.IsInf(c10mPrimary, -1) || c10mPrimary < 0 {
		dlog("Primary C10M validation failed (value: %.2f), setting to Inf.", c10mPrimary)
		c10mPrimary = math.Inf(1)
	}
	if math.IsNaN(c10mSecondary) || math.IsInf(c10mSecondary, -1) || c10mSecondary < 0 {
		dlog("Secondary C10M validation failed (value: %.2f), setting to Inf.", c10mSecondary)
		c10mSecondary = math.Inf(1)
	}

	dlog("Returning: Prim=%.2f, Sec=%.2f, IF=%.4f, RR=%.2f, DeltaRatio=%.4f, Adj=%.4f",
		c10mPrimary, c10mSecondary, IF, RR, DeltaRatio, adjustment)
	return
}

func main() {
	// 1) User inputs
	var prod string
	var qty float64

	fmt.Print("Product ID (e.g., ENCHANTED_LAPIS_BLOCK): ")
	if _, err := fmt.Scanln(&prod); err != nil {
		log.Fatalf("Invalid input for Product ID: %v", err)
	}
	fmt.Print("Quantity: ")
	if _, err := fmt.Scanln(&qty); err != nil || qty <= 0 {
		log.Fatalf("Invalid input for Quantity: %v", err)
	}

	// 2) Fetch Bazaar data
	fmt.Println("Fetching Bazaar data...")
	resp, err := http.Get("https://api.hypixel.net/v2/skyblock/bazaar")
	if err != nil {
		log.Fatalf("Failed to fetch Hypixel API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("API returned %d: %s", resp.StatusCode, string(body))
	}
	var apiResp HypixelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		log.Fatalf("Failed to parse API response: %v", err)
	}
	if !apiResp.Success {
		log.Fatal("Hypixel API reported failure")
	}
	fmt.Println("Bazaar data fetched.")

	// 3) Extract product data
	prodData, ok := apiResp.Products[prod]
	if !ok {
		log.Fatalf("Product '%s' not found in API response", prod)
	}
	if len(prodData.SellSummary) == 0 || len(prodData.BuySummary) == 0 {
		log.Fatalf("sell_summary or buy_summary is empty for product '%s'", prod)
	}
	sellP := prodData.SellSummary[0].PricePerUnit
	buyP := prodData.BuySummary[0].PricePerUnit
	if sellP <= 0 || buyP <= 0 {
		log.Fatalf("Invalid (non-positive) price found for product '%s'", prod)
	}

	// 4) Load metrics
	fmt.Println("Loading metrics from latest_metrics.json...")
	metrics := loadMetrics("latest_metrics.json")
	fmt.Println("Metrics loaded.")

	// 5) Compute C10M
	fmt.Println("Calculating C10M...")
	c10mPrim, c10mSec, IF, RR, DeltaRatio, adj := calculateC10M(prod, qty, sellP, buyP, metrics)

	// --- Determine Best C10M ---
	var bestC10m float64
	var bestMethod string
	isPrimInf := math.IsInf(c10mPrim, 0)
	isSecInf := math.IsInf(c10mSec, 0)
	if isPrimInf && isSecInf {
		bestC10m = math.Inf(1)
		bestMethod = "N/A (Both Infinite)"
	} else if isPrimInf {
		bestC10m = c10mSec
		bestMethod = "Secondary"
	} else if isSecInf {
		bestC10m = c10mPrim
		bestMethod = "Primary"
	} else {
		if c10mPrim <= c10mSec {
			bestC10m = c10mPrim
			bestMethod = "Primary"
		} else {
			bestC10m = c10mSec
			bestMethod = "Secondary"
		}
	}
	// --- End Determine Best C10M ---

	// --- Calculate Associated Simple Cost ---
	var associatedCost float64 = math.NaN()
	if bestMethod == "Primary" {
		associatedCost = qty * sellP
	} else if bestMethod == "Secondary" {
		associatedCost = qty * buyP
	}
	// --- End Calculate Associated Simple Cost ---

	// 6) Print results
	fmt.Println("\n--- Results ---")
	fmt.Printf("Product:                        %s\n", prod)
	fmt.Printf("Quantity:                       %.0f\n", qty)
	fmt.Printf("Buy‑Order Price (sP):           %.2f\n", sellP)
	fmt.Printf("Instabuy Price (bP):            %.2f\n", buyP)
	// Print Delta Ratio
	if math.IsNaN(DeltaRatio) {
		fmt.Printf("Delta (Ratio SR/DR):            N/A (Metrics Error or Rates=0)\n")
	} else if math.IsInf(DeltaRatio, 1) {
		fmt.Printf("Delta (Ratio SR/DR):            Infinite (DR=0, SR>0)\n")
	} else {
		fmt.Printf("Delta (Ratio SR/DR):            %.4f\n", DeltaRatio)
	}
	// Print IF
	if math.IsNaN(IF) {
		fmt.Printf("Initial Fill (IF):              N/A (Metrics Error)\n")
	} else if math.IsInf(IF, 1) {
		fmt.Printf("Initial Fill (IF):              Infinite\n") // Correctly prints Inf now if DeltaRatio > 1
	} else {
		fmt.Printf("Initial Fill (IF):              %.4f\n", IF)
	}
	// Print RR
	if math.IsNaN(RR) {
		fmt.Printf("Refill Rate (RR):               N/A (Metrics Error)\n")
	} else if math.IsInf(RR, 1) {
		fmt.Printf("Refill Rate (RR):               Infinite\n")
	} else {
		fmt.Printf("Refill Rate (RR):               %.2f\n", RR)
	}
	// Print Adjustment Factor
	if math.IsNaN(adj) {
		fmt.Printf("Adjustment Factor:              N/A (Metrics Error)\n")
	} else {
		fmt.Printf("Adjustment Factor:              %.4f\n", adj)
	}

	// Format C10M costs
	primStr := "N/A"
	if !math.IsInf(c10mPrim, 0) {
		primStr = fmt.Sprintf("%.2f", c10mPrim)
	}
	secStr := "N/A"
	if !math.IsInf(c10mSec, 0) {
		secStr = fmt.Sprintf("%.2f", c10mSec)
	}
	fmt.Printf("C10M (Primary):                 %s\n", primStr)
	fmt.Printf("C10M (Secondary):               %s\n", secStr)

	// Format Best C10M
	bestStr := "N/A"
	if !math.IsInf(bestC10m, 0) {
		bestStr = fmt.Sprintf("%.2f", bestC10m)
	}
	fmt.Printf("Best C10M Estimate:             %s (%s)\n", bestStr, bestMethod)

	// Format and Print Associated Simple Cost
	assocCostStr := "N/A"
	if !math.IsNaN(associatedCost) && !math.IsInf(associatedCost, 0) {
		assocCostStr = fmt.Sprintf("%.2f", associatedCost)
	}
	fmt.Printf("Associated Simple Cost:         %s\n", assocCostStr)
}
