// server.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- Configuration (Server Specific) ---
const (
	serverAddress = "localhost:8080" // Address for the server
)

// --- Result Structure for JSON API ---
// Defines the data returned by the /calculate endpoint
type CalculationResult struct {
	InputProductID      string         `json:"inputProductId"`
	NormalizedProductID string         `json:"normalizedProductId"`
	Quantity            float64        `json:"quantity"`
	Error               string         `json:"error,omitempty"` // Include errors in the response
	DirectCost          CostInfo       `json:"directCost"`
	CraftingCost        CraftingInfo   `json:"craftingCost"`
	InstaSellFillTime   TimeInfo       `json:"instaSellFillTime"` // Time to sell the final product
	Comparison          ComparisonInfo `json:"comparison"`
	CalculationTimeMs   int64          `json:"calculationTimeMs"`
}

type CostInfo struct {
	Cost   float64 `json:"cost"`
	Method string  `json:"method"` // "Primary", "Secondary", "N/A"
	RR     float64 `json:"rr"`     // Refill Rate
	Error  string  `json:"error,omitempty"`
}

type CraftingInfo struct {
	TotalCost            float64            `json:"totalCost"`
	BaseIngredients      []IngredientDetail `json:"baseIngredients"`
	BottleneckFillTime   TimeInfo           `json:"bottleneckFillTime"` // Time to acquire slowest base ingredient
	ExpansionStatus      string             `json:"expansionStatus"`    // e.g., "Success", "Failed", "Not Expanded"
	IngredientCostErrors []string           `json:"ingredientCostErrors,omitempty"`
	IngredientFillErrors []string           `json:"ingredientFillErrors,omitempty"`
}

type IngredientDetail struct {
	ID       string  `json:"id"`
	Quantity float64 `json:"quantity"`
	Cost     float64 `json:"cost"`
	Method   string  `json:"method"`
}

type TimeInfo struct {
	Seconds      float64 `json:"seconds"`
	Formatted    string  `json:"formatted"`
	BottleneckID string  `json:"bottleneckId,omitempty"` // Only for bottleneck time
	Error        string  `json:"error,omitempty"`
}

type ComparisonInfo struct {
	CheaperOption string  `json:"cheaperOption"` // "Direct", "Crafting", "Equal", "N/A"
	Difference    float64 `json:"difference"`
	Message       string  `json:"message"`
}

// --- runServer function ---
// Sets up HTTP routes and starts listening.
func runServer() {
	// --- Setup HTTP Handlers ---
	// The handler function will access the global data caches (apiRespGlobal, metricsMapGlobal)
	http.HandleFunc("/calculate", enableCORS(handleCalculate))

	// --- Start Server ---
	fmt.Printf("Server starting and listening on http://%s\n", serverAddress)
	fmt.Println("Use endpoint: /calculate?id=<ITEM_ID>&qty=<QUANTITY>")
	log.Printf("Starting server on %s", serverAddress)

	// Use ListenAndServe and log fatal errors
	err := http.ListenAndServe(serverAddress, nil)
	if err != nil {
		log.Fatalf("CRITICAL: Server failed to start: %v", err)
	}
}

// --- CORS Middleware ---
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow any origin
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// --- HTTP Handler for /calculate ---
func handleCalculate(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	log.Printf("Received request for: %s", r.URL.String())

	// 1. Get Query Parameters
	query := r.URL.Query()
	productID := query.Get("id")
	quantityStr := query.Get("qty")

	// 2. Validate Input
	if productID == "" {
		http.Error(w, `{"error": "Missing 'id' query parameter"}`, http.StatusBadRequest)
		return
	}
	if quantityStr == "" {
		http.Error(w, `{"error": "Missing 'qty' query parameter"}`, http.StatusBadRequest)
		return
	}
	quantity, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil || quantity <= 0 {
		http.Error(w, fmt.Sprintf(`{"error": "Invalid 'qty' parameter: %s. Must be a positive number."}`, quantityStr), http.StatusBadRequest)
		return
	}

	// Check if global data is loaded (essential check)
	// Accessing global variables defined in main.go (possible because they are in the same package)
	if metricsMapGlobal == nil {
		log.Println("ERROR: handleCalculate called but metricsMapGlobal is nil")
		http.Error(w, `{"error": "Metrics data not available on server"}`, http.StatusInternalServerError)
		return
	}
	// apiRespGlobal might be nil if initial fetch failed, performCalculations should handle this state.

	// 3. Perform Calculations
	// Pass the global caches to the calculation function
	result := performCalculations(productID, quantity, apiRespGlobal, metricsMapGlobal)
	result.CalculationTimeMs = time.Since(startTime).Milliseconds()

	// 4. Respond with JSON
	w.Header().Set("Content-Type", "application/json")
	jsonData, err := json.Marshal(result)
	if err != nil {
		log.Printf("ERROR: Failed to marshal JSON response: %v", err)
		http.Error(w, `{"error": "Failed to generate JSON response"}`, http.StatusInternalServerError)
		return
	}
	_, writeErr := w.Write(jsonData)
	if writeErr != nil {
		log.Printf("ERROR: Failed to write HTTP response: %v", writeErr)
	}
}

// --- Calculation Logic ---
// (performCalculations function remains the same as in the previous version)
func performCalculations(inputProductID string, quantity float64, apiResp *HypixelAPIResponse, metricsMap map[string]ProductMetrics) CalculationResult {

	// Initialize result struct
	result := CalculationResult{
		InputProductID:      inputProductID,
		NormalizedProductID: BAZAAR_ID(inputProductID), // Normalize input ID
		Quantity:            quantity,
		DirectCost:          CostInfo{Cost: math.NaN(), Method: "N/A", RR: math.NaN()},
		CraftingCost: CraftingInfo{
			TotalCost:          math.NaN(),
			BaseIngredients:    make([]IngredientDetail, 0),
			BottleneckFillTime: TimeInfo{Seconds: math.NaN(), Formatted: "N/A"},
			ExpansionStatus:    "Not Run",
		},
		InstaSellFillTime: TimeInfo{Seconds: math.NaN(), Formatted: "N/A"},
		Comparison:        ComparisonInfo{CheaperOption: "N/A", Message: "Calculation pending"},
	}
	normalizedProductID := result.NormalizedProductID // Use normalized ID internally

	// --- Start Calculations ---
	dlog("Performing calculations for %.0f x %s", quantity, normalizedProductID)

	// --- Check if essential data is missing ---
	if metricsMap == nil {
		result.Error = "Metrics data is unavailable."
		result.Comparison.Message = "Cannot perform calculations without metrics data."
		result.Comparison.CheaperOption = "N/A"
		return result
	}
	if apiResp == nil {
		log.Println("Warning: performCalculations running with nil apiResp.")
		// Some calculations below might fail gracefully (e.g., direct cost, instasell)
	}

	// 1. Get DIRECT Best C10M Cost
	directBuyCost, directBuyMethod, _, rrValueDirect, directBuyErr := getBestC10M(normalizedProductID, quantity, apiResp, metricsMap)
	result.DirectCost.Cost = directBuyCost
	result.DirectCost.Method = directBuyMethod
	result.DirectCost.RR = rrValueDirect
	if directBuyErr != nil {
		dlog("Note: Error calculating direct buy cost for %s: %v", normalizedProductID, directBuyErr)
		result.DirectCost.Error = directBuyErr.Error()
	}

	// 2. Expand the item into base ingredients
	dlog("Expanding item into base ingredients (cost-based)...")
	baseIngredients, expansionErr := expandItem(normalizedProductID, quantity, []ItemStep{}, apiResp, metricsMap, itemFilesDir)

	totalCraftCost := math.NaN()
	var baseIngredientDetails []IngredientDetail
	var fillTimeErrors []string
	var costTimeErrors []string

	var bottleneckFillTime float64 = -math.Inf(1)
	var bottleneckItemID string = "N/A"
	expansionSuccessful := false

	if expansionErr != nil {
		result.CraftingCost.ExpansionStatus = fmt.Sprintf("Failed: %v", expansionErr)
		result.Error = "Expansion failed: " + expansionErr.Error()
	} else if len(baseIngredients) == 0 {
		result.CraftingCost.ExpansionStatus = "Yielded no ingredients"
	} else if len(baseIngredients) == 1 {
		if _, isSelf := baseIngredients[normalizedProductID]; isSelf {
			result.CraftingCost.ExpansionStatus = "Not Expanded (Treated as Base)"
			totalCraftCost = directBuyCost
			if !math.IsNaN(directBuyCost) && !math.IsInf(directBuyCost, 0) {
				baseIngredientDetails = append(baseIngredientDetails, IngredientDetail{ID: normalizedProductID, Quantity: quantity, Cost: directBuyCost, Method: directBuyMethod})
			} else {
				baseIngredientDetails = append(baseIngredientDetails, IngredientDetail{ID: normalizedProductID, Quantity: quantity, Cost: math.NaN(), Method: "N/A (Direct Failed)"})
			}
			expansionSuccessful = true

			metricsData, metricsOk := safeGetMetricsData(metricsMap, normalizedProductID)
			if metricsOk {
				boFillTimeSelf, _, boFillErrSelf := calculateBuyOrderFillTime(normalizedProductID, quantity, metricsData)
				if boFillErrSelf == nil && !math.IsNaN(boFillTimeSelf) && !math.IsInf(boFillTimeSelf, 0) && boFillTimeSelf >= 0 {
					bottleneckFillTime = boFillTimeSelf
					bottleneckItemID = normalizedProductID
				} else {
					fillTimeErrors = append(fillTimeErrors, fmt.Sprintf("Fill Time ERR: Invalid calc for %s (Self): %v", normalizedProductID, boFillErrSelf))
					bottleneckFillTime = math.NaN()
				}
			} else {
				fillTimeErrors = append(fillTimeErrors, fmt.Sprintf("Fill Time ERR: Metrics not found for %s (Self)", normalizedProductID))
				bottleneckFillTime = math.NaN()
			}
		} else {
			result.CraftingCost.ExpansionStatus = "Success (1 Base Ingredient)"
			expansionSuccessful = true
		}
	} else {
		result.CraftingCost.ExpansionStatus = fmt.Sprintf("Success (%d Base Ingredients)", len(baseIngredients))
		expansionSuccessful = true
	}

	// 3. Calculate TOTAL CRAFT COST and BOTTLENECK FILL TIME from base ingredients
	if expansionSuccessful && totalCraftCost != directBuyCost {
		dlog("Calculating total craft cost and bottleneck fill time from base ingredients...")
		currentTotalCost := 0.0
		possibleToCalcCraftCost := true

		ingredientKeys := make([]string, 0, len(baseIngredients))
		for k := range baseIngredients {
			ingredientKeys = append(ingredientKeys, k)
		}
		sort.Strings(ingredientKeys)

		for _, ingID := range ingredientKeys {
			ingQty := baseIngredients[ingID]
			if ingQty <= 0 {
				continue
			}

			// --- Calculate Cost ---
			cost, method, _, _, costErr := getBestC10M(ingID, ingQty, apiResp, metricsMap)
			if costErr != nil || math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
				dlog("  WARN: Cannot get valid cost for base ingredient %.2f x %s (Cost: %s, Method: %s, Err: %v). Total craft cost will be invalid.", ingQty, ingID, formatCost(cost), method, costErr)
				possibleToCalcCraftCost = false
				costErrStr := "Invalid cost value"
				if costErr != nil {
					costErrStr = costErr.Error()
				}
				costTimeErrors = append(costTimeErrors, fmt.Sprintf("Costing ERR: Failed for %s: %s", ingID, costErrStr))
				baseIngredientDetails = append(baseIngredientDetails, IngredientDetail{ID: ingID, Quantity: ingQty, Cost: math.NaN(), Method: "Error"})
			} else {
				currentTotalCost += cost
				baseIngredientDetails = append(baseIngredientDetails, IngredientDetail{ID: ingID, Quantity: ingQty, Cost: cost, Method: method})
			}

			// --- Calculate Bottleneck Fill Time ---
			metricsData, metricsOk := safeGetMetricsData(metricsMap, ingID)
			if !metricsOk {
				errMsg := fmt.Sprintf("Fill Time ERR: Metrics not found for %s", ingID)
				fillTimeErrors = append(fillTimeErrors, errMsg)
				if bottleneckFillTime != math.Inf(1) {
					bottleneckFillTime = math.Inf(1)
					bottleneckItemID = ingID + " (Metrics Missing)"
				}
			} else {
				ingFillTime, _, ingFillErr := calculateBuyOrderFillTime(ingID, ingQty, metricsData)
				if ingFillErr != nil || math.IsNaN(ingFillTime) || math.IsInf(ingFillTime, 0) || ingFillTime < 0 {
					errMsg := fmt.Sprintf("Fill Time ERR: Invalid calc for %s: %v (Time: %s)", ingID, ingFillErr, formatSeconds(ingFillTime))
					fillTimeErrors = append(fillTimeErrors, errMsg)
					if bottleneckFillTime != math.Inf(1) {
						bottleneckFillTime = math.Inf(1)
						bottleneckItemID = ingID + " (Fill Time Calc Failed)"
					}
				} else if ingFillTime > bottleneckFillTime {
					// If current max is infinite due to error, only update if we find a *finite* time
					// If current max is finite, update if new finite time is larger
					if bottleneckFillTime == math.Inf(1) || ingFillTime > bottleneckFillTime {
						bottleneckFillTime = ingFillTime
						bottleneckItemID = ingID
					}
				}
			}
		} // End loop

		if possibleToCalcCraftCost {
			totalCraftCost = currentTotalCost
		} else {
			totalCraftCost = math.NaN()
		}
		if bottleneckFillTime == -math.Inf(1) {
			bottleneckFillTime = math.NaN()
			bottleneckItemID = "N/A (No Valid Times)"
		}
	} // End if expansionSuccessful

	result.CraftingCost.TotalCost = totalCraftCost
	result.CraftingCost.BaseIngredients = baseIngredientDetails
	result.CraftingCost.BottleneckFillTime.Seconds = bottleneckFillTime
	result.CraftingCost.BottleneckFillTime.Formatted = formatSeconds(bottleneckFillTime)
	result.CraftingCost.BottleneckFillTime.BottleneckID = bottleneckItemID
	result.CraftingCost.IngredientCostErrors = costTimeErrors
	result.CraftingCost.IngredientFillErrors = fillTimeErrors

	// 4. Get InstaSell Fill Time for the TOP-LEVEL item
	isFillTime := math.NaN()
	var isFillErr error
	productData, apiOk := safeGetProductData(apiResp, normalizedProductID)
	if apiOk {
		isFillTime, isFillErr = calculateInstasellFillTime(quantity, productData)
	} else {
		isFillErr = fmt.Errorf("API data not found for %s", normalizedProductID)
	}
	result.InstaSellFillTime.Seconds = isFillTime
	result.InstaSellFillTime.Formatted = formatSeconds(isFillTime)
	if isFillErr != nil {
		result.InstaSellFillTime.Error = isFillErr.Error()
	}

	// 5. Perform Comparison
	directIsValid := !math.IsNaN(directBuyCost) && !math.IsInf(directBuyCost, 0) && directBuyCost >= 0
	craftIsValid := !math.IsNaN(totalCraftCost) && !math.IsInf(totalCraftCost, 0) && totalCraftCost >= 0
	isApiNotFoundForDirect := strings.Contains(result.DirectCost.Error, "API data not found")

	if craftIsValid && isApiNotFoundForDirect {
		result.Comparison.CheaperOption = "Crafting"
		result.Comparison.Difference = math.Inf(1)
		result.Comparison.Message = "Crafting is the only option (item not on Bazaar)."
	} else if directIsValid && craftIsValid {
		diff := directBuyCost - totalCraftCost
		result.Comparison.Difference = math.Abs(diff)
		if diff > 1e-2 {
			result.Comparison.CheaperOption = "Crafting"
			result.Comparison.Message = fmt.Sprintf("Crafting appears cheaper by %s", formatCost(diff))
		} else if diff < -1e-2 {
			result.Comparison.CheaperOption = "Direct"
			result.Comparison.Message = fmt.Sprintf("Buying directly appears cheaper by %s", formatCost(-diff))
		} else {
			result.Comparison.CheaperOption = "Equal"
			result.Comparison.Message = "Crafting and Buying costs are effectively equal."
		}
	} else if directIsValid {
		result.Comparison.CheaperOption = "Direct"
		result.Comparison.Difference = math.NaN()
		result.Comparison.Message = "Buying directly is the only valid cost calculated."
	} else if craftIsValid {
		result.Comparison.CheaperOption = "Crafting"
		result.Comparison.Difference = math.NaN()
		result.Comparison.Message = "Crafting is the only valid cost calculated."
	} else {
		result.Comparison.CheaperOption = "N/A"
		result.Comparison.Difference = math.NaN()
		result.Comparison.Message = "Cannot compare costs due to calculation errors or missing data."
		if result.Error == "" {
			result.Error = "Failed to calculate both direct and crafting costs."
		}
	}

	return result
}
