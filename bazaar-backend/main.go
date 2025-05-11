package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv" // Added for parsing query params in optimizer handler
	"strings"
	"time"
)

// --- Constants ---
const (
	metricsFilename = "latest_metrics.json"
	itemFilesDir    = "dependencies/items"
)

// --- Structs for /api/fill ---
type Ingredient struct {
	Name             string  `json:"name"`
	Qty              float64 `json:"qty"`
	CostPerUnit      float64 `json:"cost_per_unit"`
	TotalCost        float64 `json:"total_cost"`
	PriceSource      string  `json:"price_source"`
	BuyOrderFillTime float64 `json:"buy_order_fill_time"`
	RR               float64 `json:"rr"`
	IF               float64 `json:"if,omitempty"` // If you decide to add IF to /api/fill
}
type FillResponse struct {
	Recipe                []Ingredient `json:"recipe"`
	SlowestIngredient     string       `json:"slowest_ingredient"`
	SlowestIngredientQty  float64      `json:"slowest_ingredient_qty"`
	SlowestFillTime       float64      `json:"slowest_fill_time"`
	TopLevelInstasellTime float64      `json:"top_level_instasell_time"`
	TotalBaseCost         float64      `json:"total_base_cost"`
	TopSellPrice          float64      `json:"top_sell_price"` // Price user gets when instaselling crafted item
	TotalRevenue          float64      `json:"total_revenue"`
	ProfitPerUnit         float64      `json:"profit_per_unit"`
	TotalProfit           float64      `json:"total_profit"`
}

// --- Entry point ---
func main() {
	var err error
	_, err = getApiResponse() // Initial load
	if err != nil {
		log.Printf("WARNING: Initial API load failed: %v.", err)
	} else {
		log.Println("Initial API data loaded.")
	}
	_, err = getMetricsMap(metricsFilename) // Initial load
	if err != nil {
		log.Fatalf("CRITICAL: Cannot load metrics '%s': %v", metricsFilename, err)
	} else {
		log.Printf("Metrics data loaded from '%s'.", metricsFilename)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("public")))
	mux.Handle("/api/fill", withCORS(withRecovery(fillHandler)))
	mux.Handle("/api/expand-dual", withCORS(withRecovery(dualExpansionHandler)))
	mux.Handle("/api/optimize-all", withCORS(withRecovery(optimizerApiHandler))) // Added optimizer endpoint

	log.Println("Listening on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("CRITICAL: Server failed: %v", err)
	}
}

// --- Handler for original /api/fill ---
func fillHandler(w http.ResponseWriter, r *http.Request) {
	handlerID := fmt.Sprintf("[%d-fill]", time.Now().UnixNano())
	log.Printf("%s Handler Start: %s %s", handlerID, r.Method, r.URL.String())
	defer log.Printf("%s Handler End", handlerID)

	itemQuery := r.URL.Query().Get("item")
	qtyStr := r.URL.Query().Get("qty")
	if itemQuery == "" {
		http.Error(w, "missing item parameter", http.StatusBadRequest)
		return
	}
	item := BAZAAR_ID(itemQuery)
	qty, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil || qty <= 0 {
		http.Error(w, "invalid qty parameter", http.StatusBadRequest)
		return
	}
	log.Printf("%s Validated Request: item=%s, qty=%.2f", handlerID, item, qty)

	apiCacheMutex.RLock()
	currentApiResp := apiResponseCache
	currentApiErr := apiFetchErr
	apiCacheMutex.RUnlock()
	if currentApiResp == nil {
		errMsg := "API data unavailable"
		if currentApiErr != nil {
			errMsg += fmt.Sprintf(" (%v)", currentApiErr)
		}
		log.Printf("%s Error: %s", handlerID, errMsg)
		http.Error(w, errMsg, http.StatusServiceUnavailable)
		return
	}

	currentMetricsMap, metricsErr := getMetricsMap(metricsFilename)
	if metricsErr != nil || currentMetricsMap == nil {
		log.Printf("%s Error: Metrics unavailable (%v)", handlerID, metricsErr)
		http.Error(w, "Metrics unavailable", http.StatusInternalServerError)
		return
	}
	log.Printf("%s Global data OK", handlerID)

	log.Printf("%s Determining Primary-Based expansion for %s...", handlerID, item)
	shouldExpandFill := false
	sellP := getSellPrice(currentApiResp, item)
	buyP := getBuyPrice(currentApiResp, item)
	metricsP := getMetrics(currentMetricsMap, item)

	topC10mPrim, topC10mSec, _, _, _, _, errTopC10M := calculateC10MInternal(
		item, qty, sellP, buyP, metricsP,
	)
	isApiNotFoundErrorFill := errTopC10M != nil && strings.Contains(errTopC10M.Error(), "API data not found")
	topLevelRecipeExistsFill := false
	if errTopC10M == nil || isApiNotFoundErrorFill {
		filePathFill := filepath.Join(itemFilesDir, item+".json")
		if _, statErr := os.Stat(filePathFill); statErr == nil {
			topLevelRecipeExistsFill = true
		} else if !os.IsNotExist(statErr) {
			log.Printf("%s CRITICAL Error checking recipe file: %v", handlerID, statErr)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	if isApiNotFoundErrorFill {
		shouldExpandFill = topLevelRecipeExistsFill
	} else if errTopC10M != nil {
		shouldExpandFill = false
		log.Printf("%s WARN: Top-level C10M failed for %s: %v. Treating as base.", handlerID, item, errTopC10M)
	} else {
		validPrim := !math.IsInf(topC10mPrim, 0) && !math.IsNaN(topC10mPrim) && topC10mPrim >= 0
		validSec := !math.IsInf(topC10mSec, 0) && !math.IsNaN(topC10mSec) && topC10mSec >= 0
		if validPrim && validSec {
			shouldExpandFill = topC10mPrim <= topC10mSec
		} else if validPrim {
			shouldExpandFill = true
		} else {
			shouldExpandFill = false
		}
	}
	log.Printf("%s Primary-Based Decision: Expand = %v", handlerID, shouldExpandFill)

	var baseMap map[string]float64
	// var errExpand error // Declared errExpand here to avoid shadowing
	if shouldExpandFill {
		var expandErr error // Use a different name to avoid conflict if errExpand is used later
		baseMap, expandErr = ExpandItem(item, qty, currentApiResp, currentMetricsMap, itemFilesDir)
		if expandErr != nil {
			log.Printf("%s CRITICAL Error during ExpandItem: %v", handlerID, expandErr)
			http.Error(w, fmt.Sprintf("Error expanding recipe: %v", expandErr), http.StatusInternalServerError)
			return
		}
		if len(baseMap) == 0 {
			log.Printf("%s WARN: Expansion resulted in empty map (likely cycles). Treating %s as base.", handlerID, item)
			baseMap = map[string]float64{item: qty}
		}
	} else {
		baseMap = map[string]float64{item: qty}
		// errExpand = nil // Not strictly needed if not used later
	}

	if baseMap == nil {
		log.Printf("%s CRITICAL Error: baseMap is nil after expansion logic", handlerID)
		http.Error(w, "Internal server error during recipe processing", http.StatusInternalServerError)
		return
	}
	log.Printf("%s Expansion processing complete. Found %d base types.", handlerID, len(baseMap))

	ingredientResults := make([]Ingredient, 0, len(baseMap))
	var slowestTime float64 = 0.0
	var slowestIngredientName string = ""
	var slowestIngredientQty float64 = 0.0
	var sumSimpleCost float64 = 0.0
	processingErrorOccurred := false
	log.Printf("%s Processing %d base ingredients...", handlerID, len(baseMap))

	for name, amt := range baseMap {
		// Updated to accept 6 return values from getBestC10M
		_, method, assocCost, rr, ifVal, errC10M := getBestC10M(name, amt, currentApiResp, currentMetricsMap)
		priceSource := "N/A"
		costPerUnitSimple := math.NaN()
		currentTotalCost := math.NaN()
		currentIF := math.NaN()

		if errC10M != nil || method == "N/A" || math.IsNaN(assocCost) || math.IsInf(assocCost, 0) || assocCost < 0 {
			rr = math.NaN()
			currentIF = math.NaN()
			processingErrorOccurred = true
		} else {
			currentTotalCost = assocCost
			if amt > 0 {
				costPerUnitSimple = assocCost / amt
			}
			if method == "Primary" {
				priceSource = "SellP"
				currentIF = ifVal
			} else if method == "Secondary" {
				priceSource = "BuyP"
				rr = math.NaN()
				currentIF = math.NaN()
			}
		}

		var buyTime float64 = math.NaN()
		metricsData, metricsOk := safeGetMetricsData(currentMetricsMap, name)
		if !metricsOk {
			processingErrorOccurred = true
			rr = math.NaN() // Ensure RR is NaN if metrics fail, as it depends on them
			currentIF = math.NaN()
		} else {
			var buyErr error
			buyTime, _, buyErr = calculateBuyOrderFillTime(name, amt, metricsData)
			if buyErr != nil || math.IsNaN(buyTime) || math.IsInf(buyTime, 0) || buyTime < 0 {
				buyTime = math.NaN()
				processingErrorOccurred = true
			}
		}
		ingredientResults = append(ingredientResults, Ingredient{
			Name:             name,
			Qty:              amt,
			CostPerUnit:      sanitizeFloat(costPerUnitSimple),
			TotalCost:        sanitizeFloat(currentTotalCost),
			PriceSource:      priceSource,
			BuyOrderFillTime: sanitizeFloat(buyTime),
			RR:               sanitizeFloat(rr),
			IF:               sanitizeFloat(currentIF),
		})
		if !math.IsNaN(buyTime) && !math.IsInf(buyTime, 0) && buyTime > slowestTime {
			slowestTime = buyTime
			slowestIngredientName = name
			slowestIngredientQty = amt
		}
		if !math.IsNaN(assocCost) && !math.IsInf(assocCost, 0) && assocCost >= 0 {
			sumSimpleCost += assocCost
		} else {
			if !math.IsNaN(sumSimpleCost) { // Avoid NaN propagation if already NaN
				sumSimpleCost = math.NaN()
			}
			processingErrorOccurred = true
		}
	}
	log.Printf("%s Finished ingredient loop.", handlerID)

	resp := FillResponse{}
	sort.Slice(ingredientResults, func(i, j int) bool { return ingredientResults[i].Name < ingredientResults[j].Name })
	resp.Recipe = ingredientResults
	log.Printf("%s Sorted results.", handlerID)

	log.Printf("%s Calculating top-level profit & instasell time for %s...", handlerID, item)
	topProd, topApiOk := safeGetProductData(currentApiResp, item)
	var topInstaSellPrice float64 = math.NaN() // Price user gets when instaselling
	var topLevelInstaSellTime float64 = math.NaN()

	if !topApiOk {
		processingErrorOccurred = true
	} else {
		// For user instaSelling the crafted item, they sell to existing BUY orders
		if len(topProd.BuySummary) > 0 {
			price := topProd.BuySummary[0].PricePerUnit
			if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
				processingErrorOccurred = true
			} else {
				topInstaSellPrice = price
			}
		} else {
			processingErrorOccurred = true // No buy orders to instasell to
		}

		var instaErr error
		topLevelInstaSellTime, instaErr = calculateInstasellFillTime(qty, topProd)
		if instaErr != nil || math.IsNaN(topLevelInstaSellTime) || math.IsInf(topLevelInstaSellTime, 0) || topLevelInstaSellTime < 0 {
			topLevelInstaSellTime = math.NaN()
			processingErrorOccurred = true
		}
	}
	log.Printf("%s Top Instasell Price for crafted item: %.2f | Top Instasell Time: %.2f", handlerID, topInstaSellPrice, topLevelInstaSellTime)

	totalRevCalc, profitUnitSimpleCalc, totalProfitSimpleCalc := math.NaN(), math.NaN(), math.NaN()
	if !math.IsNaN(sumSimpleCost) && !math.IsNaN(topInstaSellPrice) && qty > 0 {
		totalRevCalc = topInstaSellPrice * qty
		profitUnitSimpleCalc = topInstaSellPrice - (sumSimpleCost / qty)
		totalProfitSimpleCalc = totalRevCalc - sumSimpleCost
	} else {
		processingErrorOccurred = true
	}
	resp.SlowestFillTime = sanitizeFloat(slowestTime)
	resp.SlowestIngredient = slowestIngredientName
	resp.SlowestIngredientQty = slowestIngredientQty
	resp.TopLevelInstasellTime = sanitizeFloat(topLevelInstaSellTime)
	resp.TotalBaseCost = sanitizeFloat(sumSimpleCost)
	resp.TopSellPrice = sanitizeFloat(topInstaSellPrice) // This is the price the user gets for the final item
	resp.TotalRevenue = sanitizeFloat(totalRevCalc)
	resp.ProfitPerUnit = sanitizeFloat(profitUnitSimpleCalc)
	resp.TotalProfit = sanitizeFloat(totalProfitSimpleCalc)

	if processingErrorOccurred {
		w.Header().Set("X-Calculation-Warnings", "true")
		log.Printf("%s Fill handler completed WITH warnings.", handlerID)
	}

	log.Printf("%s Setting headers & encoding JSON...", handlerID)
	w.Header().Set("Content-Type", "application/json")
	jsonBytes, errMarshal := json.MarshalIndent(resp, "", "  ")
	if errMarshal != nil {
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

// --- Handler for New /api/expand-dual (No changes needed here from previous version) ---
func dualExpansionHandler(w http.ResponseWriter, r *http.Request) {
	handlerID := fmt.Sprintf("[%d-dual]", time.Now().UnixNano())
	log.Printf("%s Handler Start: %s %s", handlerID, r.Method, r.URL.String())
	defer log.Printf("%s Handler End", handlerID)

	itemQuery := r.URL.Query().Get("item")
	qtyStr := r.URL.Query().Get("qty")
	if itemQuery == "" {
		http.Error(w, "missing item parameter", http.StatusBadRequest)
		return
	}
	item := BAZAAR_ID(itemQuery)
	qty, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil || qty <= 0 {
		http.Error(w, "invalid qty parameter", http.StatusBadRequest)
		return
	}
	log.Printf("%s Validated Request: item=%s, qty=%.2f", handlerID, item, qty)

	apiCacheMutex.RLock()
	currentApiResp := apiResponseCache
	currentApiErr := apiFetchErr
	apiCacheMutex.RUnlock()
	if currentApiResp == nil {
		errMsg := "API data unavailable"
		if currentApiErr != nil {
			errMsg += fmt.Sprintf(" (%v)", currentApiErr)
		}
		log.Printf("%s Error: %s", handlerID, errMsg)
		http.Error(w, errMsg, http.StatusServiceUnavailable)
		return
	}

	currentMetricsMap, metricsErr := getMetricsMap(metricsFilename)
	if metricsErr != nil {
		log.Printf("%s Error getting metrics: %v", handlerID, metricsErr)
		http.Error(w, "Metrics unavailable", http.StatusInternalServerError)
		return
	}
	if currentMetricsMap == nil {
		log.Printf("%s CRITICAL ERROR: Metrics map is nil after load.", handlerID)
		http.Error(w, "Internal server error: Metrics unavailable", http.StatusInternalServerError)
		return
	}
	log.Printf("%s Global data OK", handlerID)

	log.Printf("%s Calling PerformDualExpansion from expansion.go for %s...", handlerID, item)
	dualResult, errPerform := PerformDualExpansion(item, qty, currentApiResp, currentMetricsMap, itemFilesDir)
	if errPerform != nil {
		log.Printf("%s CRITICAL Error PerformDualExpansion: %v", handlerID, errPerform)
		http.Error(w, fmt.Sprintf("Error setting up expansion: %v", errPerform), http.StatusInternalServerError)
		return
	}
	if dualResult == nil {
		log.Printf("%s CRITICAL Error: PerformDualExpansion returned nil result without error", handlerID)
		http.Error(w, "Internal server error during expansion calculation", http.StatusInternalServerError)
		return
	}
	log.Printf("%s PerformDualExpansion completed.", handlerID)

	if !dualResult.PrimaryBased.CalculationPossible || !dualResult.SecondaryBased.CalculationPossible {
		w.Header().Set("X-Calculation-Warnings", "true")
		log.Printf("%s Completed WITH warnings (Primary Possible: %v [%s], Secondary Possible: %v [%s])", handlerID, dualResult.PrimaryBased.CalculationPossible, dualResult.PrimaryBased.ErrorMessage, dualResult.SecondaryBased.CalculationPossible, dualResult.SecondaryBased.ErrorMessage)
	} else {
		log.Printf("%s Completed successfully.", handlerID)
	}

	log.Printf("%s Setting headers & encoding JSON...", handlerID)
	w.Header().Set("Content-Type", "application/json")
	jsonBytes, errMarshal := json.MarshalIndent(dualResult, "", "  ")
	if errMarshal != nil {
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

// --- Handler for New /api/optimize-all ---
func optimizerApiHandler(w http.ResponseWriter, r *http.Request) {
	handlerID := fmt.Sprintf("[%d-optimize]", time.Now().UnixNano())
	log.Printf("%s Handler Start: %s %s", handlerID, r.Method, r.URL.String())
	defer log.Printf("%s Handler End", handlerID)

	apiCacheMutex.RLock()
	currentApiResp := apiResponseCache
	currentApiErr := apiFetchErr
	apiCacheMutex.RUnlock()

	if currentApiResp == nil || currentApiResp.Products == nil {
		errMsg := "API data unavailable for optimizer"
		if currentApiErr != nil {
			errMsg += fmt.Sprintf(" (%v)", currentApiErr)
		}
		log.Printf("%s Error: %s", handlerID, errMsg)
		http.Error(w, errMsg, http.StatusServiceUnavailable)
		return
	}

	currentMetricsMap, metricsErr := getMetricsMap(metricsFilename)
	if metricsErr != nil || currentMetricsMap == nil {
		log.Printf("%s Error: Metrics unavailable for optimizer (%v)", handlerID, metricsErr)
		http.Error(w, "Metrics unavailable for optimizer", http.StatusInternalServerError)
		return
	}

	var itemIDs []string
	for id := range currentApiResp.Products {
		itemIDs = append(itemIDs, id)
	}

	maxAllowedFillTime := 3600.0     // Default: 1 hour in seconds
	maxInitialSearchQty := 1000000.0 // Default: 1 million for initial high bound in binary search

	if r.URL.Query().Get("time_limit_secs") != "" {
		if t, err := strconv.ParseFloat(r.URL.Query().Get("time_limit_secs"), 64); err == nil && t > 0 {
			maxAllowedFillTime = t
		} else {
			log.Printf("%s WARN: Invalid time_limit_secs query param '%s', using default %.1fs", handlerID, r.URL.Query().Get("time_limit_secs"), maxAllowedFillTime)
		}
	}
	if r.URL.Query().Get("max_qty_search") != "" {
		if q, err := strconv.ParseFloat(r.URL.Query().Get("max_qty_search"), 64); err == nil && q > 0 {
			maxInitialSearchQty = q
		} else {
			log.Printf("%s WARN: Invalid max_qty_search query param '%s', using default %.1f", handlerID, r.URL.Query().Get("max_qty_search"), maxInitialSearchQty)
		}
	}
	log.Printf("%s Optimizer Params: TimeLimit=%.1fs, MaxInitialQtySearch=%.1f", handlerID, maxAllowedFillTime, maxInitialSearchQty)

	optimizedResults := RunFullOptimization(itemIDs, maxAllowedFillTime, currentApiResp, currentMetricsMap, itemFilesDir, maxInitialSearchQty)

	log.Printf("%s Optimizer run complete. Returning %d results.", handlerID, len(optimizedResults))

	w.Header().Set("Content-Type", "application/json")
	jsonBytes, err := json.MarshalIndent(optimizedResults, "", "  ")
	if err != nil {
		log.Printf("%s ERROR: Failed to marshal optimization results: %v", handlerID, err)
		http.Error(w, "Failed to marshal optimization results", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(jsonBytes)))
	_, writeErr := w.Write(jsonBytes)
	if writeErr != nil {
		log.Printf("%s ERROR: Failed to write optimizer JSON response: %v", handlerID, writeErr)
	}
	log.Printf("%s Optimizer JSON response sent.", handlerID)
}

// --- Middleware ---
func withRecovery(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC recovered: %v\n%s", rec, string(debug.Stack()))
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
