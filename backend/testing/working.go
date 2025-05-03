package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ////////////////// Global Variables /////////////////////
var (
	auctionPriceMap    map[string]float64
	productMetricsMap  map[string]ProductMetrics
	sellPriceBazaarMap map[string]float64
	buyPriceBazaarMap  map[string]float64 // stores buy_summary[0] prices
	sellMovingWeekMap  map[string]float64
	buyMovingWeekMap   map[string]float64
	itemCache          = make(map[string]Item)
	cacheMutex         sync.RWMutex
	sem                = make(chan struct{}, 50)
)

const (
	profitTimeout = 5 * time.Second
)

// ////////////////// Data Types /////////////////////
type QuickStatus struct {
	SellMovingWeek float64 `json:"sellMovingWeek"`
	BuyMovingWeek  float64 `json:"buyMovingWeek"`
}

type HypixelProduct struct {
	SellSummary    []OrderSummary `json:"sell_summary"`
	BuySummary     []OrderSummary `json:"buy_summary"`
	SellMovingWeek float64        `json:"sellmovingweek"`
	BuyMovingWeek  float64        `json:"buymovingweek"`
	QuickStatus    QuickStatus    `json:"quick_status"`
}

type OrderSummary struct {
	PricePerUnit float64 `json:"pricePerUnit"`
}

type HypixelAPIResponse struct {
	Products map[string]HypixelProduct `json:"products"`
}

type ProductMetrics struct {
	ProductID             string  `json:"product_id"`
	SellSize              float64 `json:"sell_size"`
	SellFrequency         float64 `json:"sell_frequency"`
	OrderFrequencyAverage float64 `json:"order_frequency_average"`
	OrderSizeAverage      float64 `json:"order_size_average"`
}

type Recipe struct {
	Type  string `json:"type"`
	A1    string `json:"A1"`
	A2    string `json:"A2"`
	A3    string `json:"A3"`
	B1    string `json:"B1"`
	B2    string `json:"B2"`
	B3    string `json:"B3"`
	C1    string `json:"C1"`
	C2    string `json:"C2"`
	C3    string `json:"C3"`
	Count int    `json:"count"`
}

type SingleRecipe struct {
	A1    string `json:"A1"`
	A2    string `json:"A2"`
	A3    string `json:"A3"`
	B1    string `json:"B1"`
	B2    string `json:"B2"`
	B3    string `json:"B3"`
	C1    string `json:"C1"`
	C2    string `json:"C2"`
	C3    string `json:"C3"`
	Count int    `json:"count"`
}

type Item struct {
	ItemID       string       `json:"itemid"`
	DisplayName  string       `json:"displayname"`
	NBTTAG       string       `json:"nbttag"`
	Damage       int          `json:"damage"`
	Lore         []string     `json:"lore"`
	Recipe       SingleRecipe `json:"recipe"`
	Recipes      []Recipe     `json:"recipes"`
	SlayerReq    string       `json:"slayer_req"`
	InternalName string       `json:"internalname"`
	ClickCommand string       `json:"clickcommand"`
	ModVer       string       `json:"modver"`
	InfoType     string       `json:"infoType"`
	Info         []string     `json:"info"`
	CraftText    string       `json:"crafttext"`
}

// ////////////////// Utility Functions /////////////////////
// loadBazaarPrices loads both sell and buy prices from the Hypixel API.
func loadBazaarPrices() map[string]float64 {
	start := time.Now()
	resp, err := http.Get("https://api.hypixel.net/v2/skyblock/bazaar")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var apiResp HypixelAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Fatal(err)
	}
	result := make(map[string]float64)
	buyPriceBazaarMap = make(map[string]float64)
	sellMovingWeekMap = make(map[string]float64)
	buyMovingWeekMap = make(map[string]float64)
	for productID, product := range apiResp.Products {
		if len(product.SellSummary) > 0 {
			result[productID] = product.SellSummary[0].PricePerUnit
		} else {
			result[productID] = math.Inf(1)
		}
		if len(product.BuySummary) > 0 {
			buyPriceBazaarMap[productID] = product.BuySummary[0].PricePerUnit
		} else {
			buyPriceBazaarMap[productID] = math.Inf(1)
		}
		sellMoving := product.SellMovingWeek
		if sellMoving == 0 {
			sellMoving = product.QuickStatus.SellMovingWeek
		}
		buyMoving := product.BuyMovingWeek
		if buyMoving == 0 {
			buyMoving = product.QuickStatus.BuyMovingWeek
		}
		sellMovingWeekMap[productID] = sellMoving
		buyMovingWeekMap[productID] = buyMoving
	}
	log.Printf("Loaded bazaar prices for %d products in %s", len(result), time.Since(start))
	return result
}

func getBuyPrice(productID string) float64 {
	if price, ok := buyPriceBazaarMap[productID]; ok {
		return price
	}
	return math.Inf(1)
}

func getSellPriceBazaar(productID string) float64 {
	if price, ok := sellPriceBazaarMap[productID]; ok {
		return price
	}
	return math.Inf(1)
}

func getInstasellPrice(productID string) float64 {
	if price, ok := sellPriceBazaarMap[productID]; ok {
		return price
	}
	return math.Inf(1)
}

func loadAuctionPrices() map[string]float64 {
	start := time.Now()
	resp, err := http.Get("https://moulberry.codes/lowestbin.json")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var prices map[string]float64
	if err := json.Unmarshal(body, &prices); err != nil {
		log.Fatal(err)
	}
	log.Printf("Loaded auction prices in %s", time.Since(start))
	return prices
}

func getAuctionPrice(productID string) float64 {
	if price, ok := auctionPriceMap[productID]; ok {
		return price
	}
	return math.Inf(1)
}

func inProductMetrics(productID string) bool {
	_, exists := productMetricsMap[productID]
	return exists
}

func getPrice(productID string) float64 {
	if inProductMetrics(productID) {
		return getSellPriceBazaar(productID)
	}
	return getAuctionPrice(productID)
}

func getTopLevelProductPrice(productID string) float64 {
	return getInstasellPrice(productID)
}

func loadMetrics(filename string) []ProductMetrics {
	start := time.Now()
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	var metrics []ProductMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		log.Fatal(err)
	}
	productMetricsMap = make(map[string]ProductMetrics)
	for _, m := range metrics {
		productMetricsMap[m.ProductID] = m
	}
	log.Printf("Loaded metrics in %s", time.Since(start))
	return metrics
}

// calcC10M calculates the primary c10m value.
func calcC10M(productID string, quantity float64, unitPrice float64) float64 {
	if !inProductMetrics(productID) {
		return quantity * unitPrice
	}
	metrics := productMetricsMap[productID]
	idealFill := metrics.SellSize * (metrics.SellFrequency / metrics.OrderFrequencyAverage)
	rounds := math.Ceil(quantity / idealFill)
	sum := 0.0
	for i := 0.0; i < rounds; i++ {
		fill := quantity - i*idealFill
		if fill < 0 {
			fill = 0
		}
		sum += fill
	}
	return sum * unitPrice
}

// ////////////////// Recipe & Item Helpers /////////////////////
func aggregateCells(cells map[string]string) map[string]int {
	ingredients := make(map[string]int)
	for _, pos := range []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"} {
		if cell := cells[pos]; cell != "" {
			parts := strings.Split(cell, ":")
			if len(parts) == 2 {
				if amt, err := strconv.Atoi(parts[1]); err == nil {
					ingredients[parts[0]] += amt
				}
			}
		}
	}
	return ingredients
}

func getRecipeCells(item Item) map[string]string {
	for _, rec := range item.Recipes {
		if strings.ToLower(rec.Type) == "forge" {
			return nil
		}
	}
	if len(item.Recipes) > 0 {
		chosen := &item.Recipes[0]
		for _, rec := range item.Recipes {
			if rec.Count == 1 {
				chosen = &rec
				break
			}
		}
		return map[string]string{
			"A1": chosen.A1, "A2": chosen.A2, "A3": chosen.A3,
			"B1": chosen.B1, "B2": chosen.B2, "B3": chosen.B3,
			"C1": chosen.C1, "C2": chosen.C2, "C3": chosen.C3,
		}
	} else if item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" ||
		item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" ||
		item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "" {
		return map[string]string{
			"A1": item.Recipe.A1, "A2": item.Recipe.A2, "A3": item.Recipe.A3,
			"B1": item.Recipe.B1, "B2": item.Recipe.B2, "B3": item.Recipe.B3,
			"C1": item.Recipe.C1, "C2": item.Recipe.C2, "C3": item.Recipe.C3,
		}
	}
	return nil
}

func loadItem(itemName string) (Item, error) {
	cacheMutex.RLock()
	if item, ok := itemCache[itemName]; ok {
		cacheMutex.RUnlock()
		return item, nil
	}
	cacheMutex.RUnlock()
	filePath := filepath.Join("dependencies", "items", itemName+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Item{}, err
	}
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		return Item{}, err
	}
	cacheMutex.Lock()
	itemCache[itemName] = item
	cacheMutex.Unlock()
	return item, nil
}

func cloneVisited(visited map[string]int) map[string]int {
	newVisited := make(map[string]int)
	for k, v := range visited {
		newVisited[k] = v
	}
	return newVisited
}

// ////////////////// Expansion /////////////////////
// expandItemConcurrent recursively expands an item.
// If an item has no recipe cells, it returns a map with the item itself.
// (Items without a recipe are treated as base items with infinite fill time.)
func expandItemConcurrent(itemName string, multiplier int, parentC10M float64, visited map[string]int, forcedSecondary bool) map[string]int {
	if prev, exists := visited[itemName]; exists {
		return map[string]int{itemName: prev}
	}
	visited[itemName] = multiplier
	item, err := loadItem(itemName)
	if err != nil {
		return map[string]int{itemName: multiplier}
	}

	cells := getRecipeCells(item)
	if cells == nil {
		// No recipe exists; treat this as a base item.
		return map[string]int{itemName: multiplier}
	}

	aggregated := aggregateCells(cells)

	// Compute aggregated cost for all ingredients.
	totalCost := 0.0
	for ingredient, amt := range aggregated {
		ingredientPath := filepath.Join("dependencies", "items", ingredient+".json")
		if _, err := os.Stat(ingredientPath); err == nil {
			newMultiplier := multiplier * amt
			var priceParent float64
			// Only apply forced secondary pricing for sub-items (i.e. when not at the top level).
			if forcedSecondary && len(visited) > 1 {
				priceParent = getBuyPrice(ingredient)
			} else {
				priceParent = getPrice(ingredient)
			}
			primary := calcC10M(ingredient, float64(newMultiplier), priceParent)
			secondary := getBuyPrice(ingredient) * float64(newMultiplier) * math.Pow(float64(newMultiplier)/2240, (float64(newMultiplier)/2240)/math.Sqrt(2240))
			var chosenSubMetric float64
			if forcedSecondary && len(visited) > 1 {
				chosenSubMetric = secondary
			} else {
				chosenSubMetric = primary
			}
			totalCost += chosenSubMetric
		}
	}
	if totalCost > parentC10M {
		log.Printf("Aggregated cost (%.0f) exceeds parent's metric (%.0f) for %s. Not expanding further.", totalCost, parentC10M, itemName)
		return map[string]int{itemName: multiplier}
	}

	final := make(map[string]int)
	var wg sync.WaitGroup
	resultChan := make(chan map[string]int, len(aggregated))

	for ingredient, amt := range aggregated {
		newMultiplier := multiplier * amt
		ingredientPath := filepath.Join("dependencies", "items", ingredient+".json")
		if _, err := os.Stat(ingredientPath); err == nil {
			var priceParent float64
			// Again, apply forced secondary pricing only for sub-items.
			if forcedSecondary && len(visited) > 1 {
				priceParent = getBuyPrice(ingredient)
			} else {
				priceParent = getPrice(ingredient)
			}
			forceExpand := math.IsInf(priceParent, 1)
			primary := calcC10M(ingredient, float64(newMultiplier), priceParent)
			secondary := getBuyPrice(ingredient) * float64(newMultiplier) * math.Pow(float64(newMultiplier)/2240, (float64(newMultiplier)/2240)/math.Sqrt(2240))
			var chosenSubMetric float64
			if forcedSecondary && len(visited) > 1 {
				chosenSubMetric = secondary
			} else {
				chosenSubMetric = primary
			}
			if !forceExpand && parentC10M <= chosenSubMetric {
				log.Printf("Pre-check: For ingredient %s, parent's metric (%.0f) <= chosen sub-metric (%.0f). Not expanding further.", ingredient, parentC10M, chosenSubMetric)
				final[ingredient] += newMultiplier
				continue
			}

			log.Printf("{%d %s, parent: %.0f, chosen metric: %.0f}", newMultiplier, ingredient, parentC10M, chosenSubMetric)
			wg.Add(1)
			sem <- struct{}{}
			newVisited := cloneVisited(visited)
			go func(ingredient string, newMultiplier int, chosenMetric float64) {
				defer wg.Done()
				subRes := expandItemConcurrent(ingredient, newMultiplier, chosenMetric, newVisited, forcedSecondary)
				resultChan <- subRes
				<-sem
			}(ingredient, newMultiplier, chosenSubMetric)
		} else {
			final[ingredient] += newMultiplier
		}
	}
	wg.Wait()
	close(resultChan)
	for subRes := range resultChan {
		for k, v := range subRes {
			final[k] += v
		}
	}
	return final
}

// ////////////////// Analysis /////////////////////
// computeFillTimeMain computes fill time for the main product.
// (If no base metric is available, it returns infinity.)
func computeFillTimeMain(metrics ProductMetrics, quantity float64) float64 {
	if metrics.SellSize*metrics.SellFrequency > metrics.OrderSizeAverage*metrics.OrderFrequencyAverage {
		return 20 * quantity / (metrics.SellSize*metrics.SellFrequency - metrics.OrderSizeAverage*metrics.OrderFrequencyAverage)
	}
	return 20 * quantity / (metrics.SellSize * metrics.SellFrequency)
}

// computeFillTimeSub computes fill time for a subproduct.
func computeFillTimeSub(metrics ProductMetrics, factor, quantity float64) float64 {
	rof := metrics.SellSize * metrics.SellFrequency
	iino := metrics.OrderSizeAverage * metrics.OrderFrequencyAverage
	if rof > iino {
		return factor * quantity * 20 / (rof - iino)
	}
	return factor * quantity * 20 / (metrics.SellSize * metrics.SellFrequency)
}

// printRevampedAnalysisDual computes and prints analysis based on both expansions.
func printRevampedAnalysisDual(productID string, quantity float64, finalAggPrimary, finalAggSecondary map[string]int) {
	mainBuyPrice := getBuyPrice(productID)
	mainSellPrice := getInstasellPrice(productID)

	// Fix: Use getPrice() for both calculations to ensure consistency
	totalBaseCostPrimary := 0.0
	for sub, amt := range finalAggPrimary {
		totalBaseCostPrimary += getPrice(sub) * float64(amt)
	}

	// Fix: Use getPrice() here instead of getBuyPrice() for consistent comparison
	totalBaseCostSecondary := 0.0
	for sub, amt := range finalAggSecondary {
		totalBaseCostSecondary += getPrice(sub) * float64(amt)
	}

	baseCostPerUnitPrimary := totalBaseCostPrimary / quantity
	baseCostPerUnitSecondary := totalBaseCostSecondary / quantity

	sellRatio := 0.0
	buyRatio := 0.0
	if baseCostPerUnitPrimary > 0 {
		sellRatio = mainSellPrice / baseCostPerUnitPrimary
	}
	if baseCostPerUnitSecondary > 0 {
		buyRatio = mainBuyPrice / baseCostPerUnitSecondary
	}

	profitSell := mainSellPrice - baseCostPerUnitPrimary
	profitBuy := mainBuyPrice - baseCostPerUnitSecondary

	var fillTime float64
	if inProductMetrics(productID) {
		metrics := productMetricsMap[productID]
		fillTime = computeFillTimeMain(metrics, quantity)
	} else {
		fillTime = math.Inf(1)
	}

	fmt.Printf("Main Item Analysis for %s (quantity = %.2f):\n", productID, quantity)
	fmt.Printf("  Sell Summary Price: %.2f\n", mainSellPrice)
	fmt.Printf("  Buy Summary Price:  %.2f\n", mainBuyPrice)
	fmt.Printf("  Fill Time:          %.2f sec\n\n", fillTime)
	fmt.Println("=== Primary Expansion (Sell method) ===")
	fmt.Printf("  Total Base Cost:    %.2f (per unit: %.2f)\n", totalBaseCostPrimary, baseCostPerUnitPrimary)
	fmt.Printf("  Sell Price Ratio:   %.4f\n", sellRatio)
	fmt.Printf("  Profit (Sell):      %.2f\n\n", profitSell)
	fmt.Println("=== Secondary Expansion (Buy method) ===")
	fmt.Printf("  Total Base Cost:    %.2f (per unit: %.2f)\n", totalBaseCostSecondary, baseCostPerUnitSecondary)
	fmt.Printf("  Buy Price Ratio:    %.4f\n", buyRatio)
	fmt.Printf("  Profit (Buy):       %.2f\n\n", profitBuy)

	// Primary expansion sub-product analysis.
	var slowestSubPrimary string
	maxSubFillTimePrimary := -1.0
	for subID, qty := range finalAggPrimary {
		if subID == productID {
			continue
		}
		var subFillTime float64
		if inProductMetrics(subID) {
			metrics := productMetricsMap[subID]
			subFillTime = computeFillTimeSub(metrics, 1, float64(qty))
		} else {
			subFillTime = math.Inf(1)
		}
		if subFillTime > maxSubFillTimePrimary {
			maxSubFillTimePrimary = subFillTime
			slowestSubPrimary = subID
		}
	}

	if slowestSubPrimary != "" {
		fmt.Printf("Sub-Product with the Longest Fill Time (from primary expansion): %s\n", slowestSubPrimary)
		fmt.Printf("  Fill Time:          %.2f sec\n", maxSubFillTimePrimary)
		fmt.Printf("  Buy Summary Price:  %.2f\n", getBuyPrice(slowestSubPrimary))
		fmt.Printf("  Sell Summary Price: %.2f\n\n", getInstasellPrice(slowestSubPrimary))
	} else {
		fmt.Printf("Sub-Product with the Longest Fill Time (from primary expansion): N/A\n")
		fmt.Printf("  Fill Time:          +Inf sec\n\n")
	}

	// Secondary expansion sub-product analysis.
	var slowestSubSecondary string
	maxSubFillTimeSecondary := -1.0
	for subID, qty := range finalAggSecondary {
		if subID == productID {
			continue
		}
		var subFillTime float64
		if inProductMetrics(subID) {
			metrics := productMetricsMap[subID]
			subFillTime = computeFillTimeSub(metrics, 1, float64(qty))
		} else {
			subFillTime = math.Inf(1)
		}
		if subFillTime > maxSubFillTimeSecondary {
			maxSubFillTimeSecondary = subFillTime
			slowestSubSecondary = subID
		}
	}

	if slowestSubSecondary != "" {
		fmt.Printf("Sub-Product with the Longest Fill Time (from secondary expansion): %s\n", slowestSubSecondary)
		fmt.Printf("  Fill Time:          %.2f sec\n", maxSubFillTimeSecondary)
		fmt.Printf("  Buy Summary Price:  %.2f\n", getBuyPrice(slowestSubSecondary))
		fmt.Printf("  Sell Summary Price: %.2f\n\n", getInstasellPrice(slowestSubSecondary))
	} else {
		fmt.Printf("Sub-Product with the Longest Fill Time (from secondary expansion): N/A\n")
		fmt.Printf("  Fill Time:          +Inf sec\n\n")
	}
}

// ////////////////// Main /////////////////////
func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
	startGlobal := time.Now()
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		loadMetrics("latest_metrics.json")
		wg.Done()
	}()
	go func() {
		auctionPriceMap = loadAuctionPrices()
		wg.Done()
	}()
	go func() {
		sellPriceBazaarMap = loadBazaarPrices()
		wg.Done()
	}()
	wg.Wait()
	log.Printf("Global initialization took %s", time.Since(startGlobal))

	// Check for required command-line arguments.
	if len(os.Args) < 3 {
		fmt.Println("Usage: <executable> <product_name> <starting_quantity>")
		os.Exit(1)
	}
	productName := os.Args[1]
	quantity, err := strconv.ParseFloat(os.Args[2], 64)
	if err != nil || quantity <= 0 {
		fmt.Println("Invalid quantity. Please provide a positive number.")
		os.Exit(1)
	}

	topFile := filepath.Join("dependencies", "items", productName+".json")
	data, err := os.ReadFile(topFile)
	if err != nil {
		fmt.Printf("No recipe file for '%s'. Using '%s' as base.\n\n", productName, productName)
		os.Exit(1)
	}
	var topItem Item
	if err := json.Unmarshal(data, &topItem); err != nil {
		fmt.Printf("Error parsing '%s': %v\n", productName, err)
		os.Exit(1)
	}
	cells := getRecipeCells(topItem)
	// For the top-level item, use the primary metric (instasell price)
	primaryParent := calcC10M(productName, quantity, getInstasellPrice(productName))
	// For the secondary expansion we force secondary pricing on sub-items only.
	secondaryParent := calcC10M(productName, quantity, getInstasellPrice(productName))
	log.Printf("{%d %s, top level item primary c10m: %.0f}", int(quantity), productName, primaryParent)

	var finalAggPrimary, finalAggSecondary map[string]int
	if cells != nil {
		if primaryParent > 0 {
			finalAggPrimary = expandItemConcurrent(productName, int(quantity), primaryParent, make(map[string]int), false)
		} else {
			finalAggPrimary = map[string]int{productName: int(quantity)}
		}
		if secondaryParent > 0 {
			// Pass forcedSecondary=true so that sub-items use getBuyPrice,
			// but note that at the top-level (visited length==1) we ignore forced pricing.
			finalAggSecondary = expandItemConcurrent(productName, int(quantity), secondaryParent, make(map[string]int), true)
		} else {
			finalAggSecondary = map[string]int{productName: int(quantity)}
		}
	} else {
		finalAggPrimary = map[string]int{productName: int(quantity)}
		finalAggSecondary = map[string]int{productName: int(quantity)}
	}

	for sub, amt := range finalAggPrimary {
		if sub == productName {
			continue
		}
		log.Printf("{%d %s, primary parent: %.0f}", amt, sub, primaryParent)
	}

	fmt.Printf("Expansion for %s:\n", productName)
	totalBaseCost := 0.0
	for sub, amt := range finalAggPrimary {
		fmt.Printf("%s: %d\n", sub, amt)
		totalBaseCost += getPrice(sub) * float64(amt)
	}
	mainSell := getInstasellPrice(productName)
	fmt.Printf("\nMain product sell price: %.2f\n", mainSell)
	fmt.Printf("Total base cost (primary): %.2f\n", totalBaseCost)
	if totalBaseCost > 0 {
		fmt.Printf("Price Ratio (sell/base): %.4f\n\n", mainSell/(totalBaseCost/float64(int(quantity))))
	} else {
		fmt.Println("Total base cost is zero; cannot compute price ratio.\n")
	}

	// Fix: Changed productID to productName here
	printRevampedAnalysisDual(productName, quantity, finalAggPrimary, finalAggSecondary)
}
