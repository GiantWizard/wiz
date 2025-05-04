package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"runtime/debug" // Import for stack trace in recovery
	"sort"          // Import the sort package
	"strconv"
	"time" // Needed for handlerID
	// Include other necessary imports from your other files if you split them back later
	// e.g., "os", "path/filepath", "sync" etc.
)

// --- Constants REQUIRED by main.go ---
const (
	metricsFilename = "latest_metrics.json"
	itemFilesDir    = "dependencies/items"
)

// Note: Assumes global variables like apiResponseCache and metricsCache are defined elsewhere (e.g., api.go, metrics.go)
// Note: Assumes function definitions like getApiResponse, getMetricsMap, BAZAAR_ID, expandItem, getBestC10M, etc.
//       and associated types (HypixelAPIResponse, ProductMetrics, etc.) are defined elsewhere in package main.

// --- JSON response types (Specific to this handler's output) ---
// <<< MODIFICATION: Added PriceSource field >>>
type Ingredient struct {
	Name              string  `json:"name"`
	Qty               float64 `json:"qty"`
	CostPerUnit       float64 `json:"cost_per_unit"`       // Based on simple associated cost
	TotalCost         float64 `json:"total_cost"`          // Based on simple associated cost
	PriceSource       string  `json:"price_source"`        // "SellP" (Buy Order) or "BuyP" (Insta-Buy) or "N/A"
	InstasellFillTime float64 `json:"instasell_fill_time"` // Calculated
	BuyOrderFillTime  float64 `json:"buy_order_fill_time"` // Calculated
	RR                float64 `json:"rr"`                  // From C10M/fill time calc context
}

type FillResponse struct {
	Recipe               []Ingredient `json:"recipe"` // This will be sorted alphabetically
	SlowestIngredient    string       `json:"slowest_ingredient"`
	SlowestIngredientQty float64      `json:"slowest_ingredient_qty"`
	SlowestFillTime      float64      `json:"slowest_fill_time"`
	TotalBaseCost        float64      `json:"total_base_cost"` // Sum of simple associated costs
	TopSellPrice         float64      `json:"top_sell_price"`  // Instasell price of final item
	TotalRevenue         float64      `json:"total_revenue"`   // topSell * qty
	ProfitPerUnit        float64      `json:"profit_per_unit"` // Based on simple associated costs
	TotalProfit          float64      `json:"total_profit"`    // Based on simple associated costs
}

// --- Entry point ---
func main() {
	var err error
	_, err = getApiResponse() // Assumes defined in api.go
	if err != nil {
		log.Printf("WARNING: Initial API load failed: %v.", err)
	} else {
		log.Println("Initial API data loaded.")
	}
	_, err = getMetricsMap(metricsFilename) // Assumes defined in metrics.go
	if err != nil {
		log.Fatalf("CRITICAL: Cannot load metrics '%s': %v", metricsFilename, err)
	} else {
		log.Printf("Metrics data loaded from '%s'.", metricsFilename)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("public")))
	mux.Handle("/api/fill", withCORS(withRecovery(fillHandler)))
	log.Println("Listening on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("CRITICAL: Server failed: %v", err)
	}
}

// --- Helper Function to Sanitize Floats ---
// Replaces NaN or +/- Inf with 0.0 for JSON compatibility
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0.0
	}
	return f
}

// ── Handler (with NaN Sanitization) ──────────────────────────────────────────
func fillHandler(w http.ResponseWriter, r *http.Request) {
	handlerID := fmt.Sprintf("[%d]", time.Now().UnixNano())
	log.Printf("%s Handler Start: %s %s", handlerID, r.Method, r.URL.String())
	defer log.Printf("%s Handler End", handlerID)

	// --- Input Validation ---
	itemQuery := r.URL.Query().Get("item")
	qtyStr := r.URL.Query().Get("qty")
	if itemQuery == "" {
		http.Error(w, "missing item parameter", http.StatusBadRequest)
		log.Printf("%s Error: Missing item", handlerID)
		return
	}
	item := BAZAAR_ID(itemQuery) // Assumes BAZAAR_ID defined in utils.go
	qty, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil || qty <= 0 {
		http.Error(w, "invalid qty parameter", http.StatusBadRequest)
		log.Printf("%s Error: Invalid qty '%s'", handlerID, qtyStr)
		return
	}
	log.Printf("%s Validated Request: item=%s, qty=%.2f", handlerID, item, qty)

	// --- Check Global Data Availability ---
	log.Printf("%s Checking global data", handlerID)
	apiCacheMutex.RLock()
	currentApiResp := apiResponseCache
	currentApiErr := apiFetchErr
	apiCacheMutex.RUnlock() // Assumes globals from api.go
	if currentApiResp == nil {
		errMsg := "API data unavailable"
		if currentApiErr != nil {
			errMsg += fmt.Sprintf(" (%v)", currentApiErr)
		}
		log.Printf("%s Error: %s", handlerID, errMsg)
		http.Error(w, errMsg, http.StatusServiceUnavailable)
		return
	}
	if metricsCache == nil {
		log.Printf("%s CRITICAL ERROR: Metrics cache is nil.", handlerID)
		http.Error(w, "Internal server error: Metrics unavailable", http.StatusInternalServerError)
		return
	} // Assumes metricsCache from metrics.go
	log.Printf("%s Global data OK", handlerID)

	// --- Recipe Expansion ---
	log.Printf("%s Calling expandItem for %s...", handlerID, item)
	baseMap, err := expandItem(item, qty, nil, currentApiResp, metricsCache, itemFilesDir) // Assumes expandItem from recipe_expansion.go
	if err != nil {
		log.Printf("%s CRITICAL Error expandItem: %v", handlerID, err)
		http.Error(w, fmt.Sprintf("Error expanding recipe: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("%s expandItem completed. Found %d base types.", handlerID, len(baseMap))

	// --- Process Base Ingredients ---
	ingredientResults := make([]Ingredient, 0, len(baseMap))
	var slowestTime float64 = 0.0
	var slowestIngredientName string = ""
	var slowestIngredientQty float64 = 0.0
	var sumSimpleCost float64 = 0.0
	processingErrorOccurred := false
	log.Printf("%s Processing base ingredients...", handlerID)
	idx := 0
	for name, amt := range baseMap {
		idx++
		log.Printf("%s --- Ingredient %d: %.2f x %s ---", handlerID, idx, amt, name)
		if amt <= 0 {
			log.Printf("%s     Skipping non-positive amt", handlerID)
			continue
		}

		// Get Cost Info
		log.Printf("%s     Calling getBestC10M...", handlerID)
		_, method, assocCost, rr, errC10M := getBestC10M(name, amt, currentApiResp, metricsCache) // Assumes getBestC10M from c10m.go
		log.Printf("%s     getBestC10M: M=%s, Cost=%.2f, RR=%.2f, Err=%v", handlerID, method, assocCost, rr, errC10M)

		priceSource := "N/A"
		// Handle NaN/Inf originating from getBestC10M
		if errC10M != nil || math.IsNaN(assocCost) || math.IsInf(assocCost, 0) || assocCost < 0 {
			log.Printf("%s     WARN: Invalid cost/error for %s. Cost/RR/Source=N/A.", handlerID, name)
			assocCost = math.NaN()
			rr = math.NaN()
			priceSource = "N/A"
			processingErrorOccurred = true
		} else {
			if method == "Primary" {
				priceSource = "SellP"
			} else if method == "Secondary" {
				priceSource = "BuyP"
				rr = math.NaN()
			}
			log.Printf("%s     Price Source determined: %s", handlerID, priceSource)
		}

		// Calculate Fill Times
		var buyTime, instaTime float64 = math.NaN(), math.NaN()
		metricsData, metricsOk := safeGetMetricsData(metricsCache, name) // Assumes safeGetMetricsData from utils.go
		if !metricsOk {
			log.Printf("%s     WARN: Metrics missing for %s. Cannot calc buy time. RR=NaN.", handlerID, name)
			processingErrorOccurred = true
			rr = math.NaN()
		} else {
			var buyErr error
			buyTime, _, buyErr = calculateBuyOrderFillTime(name, amt, metricsData) // Assumes calculateBuyOrderFillTime from fill_time.go
			if buyErr != nil || math.IsNaN(buyTime) || math.IsInf(buyTime, 0) || buyTime < 0 {
				log.Printf("%s     WARN: Invalid buy time for %s (T=%.2f, E=%v). Set NaN.", handlerID, name, buyTime, buyErr)
				buyTime = math.NaN()
				processingErrorOccurred = true
			}
		}
		prod, apiOk := getProductData(currentApiResp, name) // Assumes getProductData defined below or utils.go
		if !apiOk {
			log.Printf("%s     WARN: API data missing for %s. Cannot calc insta time.", handlerID, name)
			processingErrorOccurred = true
		} else {
			var instaErr error
			instaTime, instaErr = calculateInstasellFillTime(amt, prod) // Assumes calculateInstasellFillTime from fill_time.go
			if instaErr != nil || math.IsNaN(instaTime) || math.IsInf(instaTime, 0) || instaTime < 0 {
				log.Printf("%s     WARN: Invalid insta time for %s (T=%.2f, E=%v). Set NaN.", handlerID, name, instaTime, instaErr)
				instaTime = math.NaN()
				processingErrorOccurred = true
			}
		}
		log.Printf("%s     Fill Times: Buy=%.2f, Insta=%.2f", handlerID, buyTime, instaTime)

		// Prepare Ingredient for Slice, SANITIZING float fields
		costPerUnitSimple := math.NaN()
		if amt > 0 && !math.IsNaN(assocCost) {
			costPerUnitSimple = assocCost / amt
		}
		ingredientResults = append(ingredientResults, Ingredient{
			Name:              name,
			Qty:               amt,                              // Qty is usually safe
			CostPerUnit:       sanitizeFloat(costPerUnitSimple), // <<< Sanitize >>>
			TotalCost:         sanitizeFloat(assocCost),         // <<< Sanitize >>>
			PriceSource:       priceSource,
			InstasellFillTime: sanitizeFloat(instaTime), // <<< Sanitize >>>
			BuyOrderFillTime:  sanitizeFloat(buyTime),   // <<< Sanitize >>>
			RR:                sanitizeFloat(rr),        // <<< Sanitize >>>
		})

		// Update Slowest Time (use original float64 buyTime for comparison)
		if !math.IsNaN(buyTime) && !math.IsInf(buyTime, 0) && buyTime > slowestTime {
			slowestTime = buyTime
			slowestIngredientName = name
			slowestIngredientQty = amt
			log.Printf("%s     New slowest time: %.2f (%s)", handlerID, slowestTime, name)
		}

		// Accumulate Total Simple Cost (use original float64 assocCost)
		if !math.IsNaN(assocCost) {
			sumSimpleCost += assocCost
		} else {
			if !math.IsNaN(sumSimpleCost) {
				log.Printf("%s     WARN: Total cost set NaN due to %s.", handlerID, name)
				sumSimpleCost = math.NaN()
			}
			processingErrorOccurred = true
		}
		log.Printf("%s     Current Sum Simple Cost: %.2f", handlerID, sumSimpleCost)
	} // End ingredient loop
	log.Printf("%s Finished ingredient loop.", handlerID)

	// --- Initialize response struct ---
	resp := FillResponse{}

	// --- Sort Results ---
	sort.Slice(ingredientResults, func(i, j int) bool { return ingredientResults[i].Name < ingredientResults[j].Name })
	resp.Recipe = ingredientResults
	log.Printf("%s Sorted results.", handlerID)

	// --- Calculate Top-Level Profit ---
	log.Printf("%s Calculating profit...", handlerID)
	topProd, topApiOk := getProductData(currentApiResp, item)
	var topSell float64 = math.NaN()
	if !topApiOk {
		log.Printf("%s WARN: API data missing for top item %s.", handlerID, item)
		processingErrorOccurred = true
	} else if len(topProd.SellSummary) == 0 {
		log.Printf("%s WARN: Top item %s has no sell summary.", handlerID, item)
		processingErrorOccurred = true
	} else {
		price := topProd.SellSummary[0].PricePerUnit
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			log.Printf("%s WARN: Invalid top sell price (%.2f).", handlerID, price)
			processingErrorOccurred = true
		} else {
			topSell = price
		}
	}
	log.Printf("%s Top Sell Price: %.2f", handlerID, topSell)
	totalRevCalc, profitUnitSimpleCalc, totalProfitSimpleCalc := math.NaN(), math.NaN(), math.NaN()
	if !math.IsNaN(sumSimpleCost) && !math.IsNaN(topSell) && qty > 0 {
		totalRevCalc = topSell * qty
		profitUnitSimpleCalc = topSell - (sumSimpleCost / qty)
		totalProfitSimpleCalc = totalRevCalc - sumSimpleCost
		log.Printf("%s Profit: Rev=%.2f, UnitP=%.2f, TotalP=%.2f", handlerID, totalRevCalc, profitUnitSimpleCalc, totalProfitSimpleCalc)
	} else {
		log.Printf("%s WARN: Cannot calc profit (CostNaN:%v, SellNaN:%v, Qty<=0:%v)", handlerID, math.IsNaN(sumSimpleCost), math.IsNaN(topSell), qty <= 0)
		processingErrorOccurred = true
	}

	// --- Populate Final Response (SANITIZING float fields) ---
	resp.SlowestFillTime = sanitizeFloat(slowestTime) // <<< Sanitize >>>
	resp.SlowestIngredient = slowestIngredientName
	resp.SlowestIngredientQty = slowestIngredientQty         // Qty is usually safe
	resp.TotalBaseCost = sanitizeFloat(sumSimpleCost)        // <<< Sanitize >>>
	resp.TopSellPrice = sanitizeFloat(topSell)               // <<< Sanitize >>>
	resp.TotalRevenue = sanitizeFloat(totalRevCalc)          // <<< Sanitize >>>
	resp.ProfitPerUnit = sanitizeFloat(profitUnitSimpleCalc) // <<< Sanitize >>>
	resp.TotalProfit = sanitizeFloat(totalProfitSimpleCalc)  // <<< Sanitize >>>
	if processingErrorOccurred {
		w.Header().Set("X-Calculation-Warnings", "true")
		log.Printf("%s Completed WITH warnings.", handlerID)
	}

	// --- Send Response ---
	log.Printf("%s Setting headers & encoding JSON...", handlerID)
	w.Header().Set("Content-Type", "application/json")
	// Use MarshalIndent + Write (should now work with sanitized floats)
	jsonBytes, errMarshal := json.MarshalIndent(resp, "", "  ")
	if errMarshal != nil {
		log.Printf("%s CRITICAL: Error marshalling JSON response: %v", handlerID, errMarshal)
		http.Error(w, "Internal server error during JSON creation", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(jsonBytes)))
	_, errWrite := w.Write(jsonBytes)
	if errWrite != nil {
		log.Printf("%s ERROR: Failed to write JSON response: %v", handlerID, errWrite)
		return
	}
	log.Printf("%s JSON sent.", handlerID)
}

// ── Middleware ────────────────────────────────────────────────────────────────
func withRecovery(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC recovered in handler for %s: %v\n%s", r.URL.Path, rec, string(debug.Stack()))
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		h(w, r)
	}
}
func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

// ── Local Helpers (if needed, or defined elsewhere) ---------------------------
// Example: getProductData (must be defined in package main)
// Assumes HypixelProduct, HypixelAPIResponse, BAZAAR_ID are defined elsewhere
func getProductData(api *HypixelAPIResponse, id string) (HypixelProduct, bool) {
	if api == nil || api.Products == nil {
		return HypixelProduct{}, false
	}
	lookupID := BAZAAR_ID(id)
	p, ok := api.Products[lookupID]
	return p, ok
}

// Note: This file still assumes other functions and types are defined elsewhere in 'package main'.
