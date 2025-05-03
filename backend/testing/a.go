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
	"sort"
	"strconv"
	"strings"
)

// --- Debug helper -----------------------------------------------------------

var debug = os.Getenv("DEBUG") == "1"

// dlog prints only when DEBUG=1 is set in the environment.
func dlog(format string, args ...interface{}) {
	if debug {
		fmt.Printf(format+"\n", args...)
	}
}

// --- Bazaar & Metrics Structs ----------------------------------------------

type OrderSummary struct {
	PricePerUnit float64 `json:"pricePerUnit"`
	Quantity     float64 `json:"amount"`
}

type QuickStatus struct {
	BuyPrice       float64 `json:"buyPrice"`
	BuyVolume      float64 `json:"buyVolume"`
	BuyMovingWeek  float64 `json:"buyMovingWeek"`
	SellPrice      float64 `json:"sellPrice"`
	SellVolume     float64 `json:"sellVolume"`
	SellMovingWeek float64 `json:"sellMovingWeek"`
}

type HypixelProduct struct {
	SellSummary []OrderSummary `json:"sell_summary"`
	BuySummary  []OrderSummary `json:"buy_summary"`
	QuickStatus QuickStatus    `json:"quick_status"`
}

type HypixelAPIResponse struct {
	Success  bool                      `json:"success"`
	Products map[string]HypixelProduct `json:"products"`
}

type ProductMetrics struct {
	ProductID      string  `json:"product_id"`
	SellSize       float64 `json:"sell_size"`
	SellFrequency  float64 `json:"sell_frequency"`
	OrderSize      float64 `json:"order_size_average"`
	OrderFrequency float64 `json:"order_frequency_average"`
}

// --- Item/Recipe Structs ---------------------------------------------------

type SingleRecipe struct {
	A1, A2, A3 string `json:"A1"`
	B1, B2, B3 string `json:"B1"`
	C1, C2, C3 string `json:"C1"`
	Count      int    `json:"count"`
}

type Recipe struct {
	A1, A2, A3 string `json:"A1"`
	B1, B2, B3 string `json:"B1"`
	C1, C2, C3 string `json:"C1"`
	Count      int    `json:"count"`
}

type Item struct {
	ItemID    string          `json:"itemid"`
	Recipe    SingleRecipe    `json:"-"`
	Recipes   []Recipe        `json:"recipes"`
	RawRecipe json.RawMessage `json:"recipe"`
}

// --- In‑memory cache for item JSONs ----------------------------------------

var itemCache = make(map[string]Item)

func loadItem(itemName string) (Item, error) {
	if itm, ok := itemCache[itemName]; ok {
		return itm, nil
	}

	fp := filepath.Join("dependencies", "items", itemName+".json")
	data, err := os.ReadFile(fp)
	if err != nil {
		return Item{}, err
	}

	var itm Item
	if err := json.Unmarshal(data, &itm); err != nil {
		return Item{}, err
	}

	// Manual recipe parsing
	if itm.RawRecipe != nil && len(itm.RawRecipe) > 0 && string(itm.RawRecipe) != "null" {
		var recipeMap map[string]interface{}
		if err := json.Unmarshal(itm.RawRecipe, &recipeMap); err == nil {
			if v, ok := recipeMap["A1"].(string); ok {
				itm.Recipe.A1 = v
			}
			if v, ok := recipeMap["A2"].(string); ok {
				itm.Recipe.A2 = v
			}
			if v, ok := recipeMap["A3"].(string); ok {
				itm.Recipe.A3 = v
			}
			if v, ok := recipeMap["B1"].(string); ok {
				itm.Recipe.B1 = v
			}
			if v, ok := recipeMap["B2"].(string); ok {
				itm.Recipe.B2 = v
			}
			if v, ok := recipeMap["B3"].(string); ok {
				itm.Recipe.B3 = v
			}
			if v, ok := recipeMap["C1"].(string); ok {
				itm.Recipe.C1 = v
			}
			if v, ok := recipeMap["C2"].(string); ok {
				itm.Recipe.C2 = v
			}
			if v, ok := recipeMap["C3"].(string); ok {
				itm.Recipe.C3 = v
			}
			if v, ok := recipeMap["count"].(float64); ok {
				itm.Recipe.Count = int(v)
			}
		}
	}

	itemCache[itemName] = itm
	return itm, nil
}

// --- Bazaar / Metrics loaders ----------------------------------------------

func loadMetricsMap(filename string) map[string]ProductMetrics {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("reading metrics file '%s': %v", filename, err)
	}
	var slice []ProductMetrics
	if err := json.Unmarshal(data, &slice); err != nil {
		log.Fatalf("parsing metrics JSON from '%s': %v", filename, err)
	}
	m := make(map[string]ProductMetrics, len(slice))
	for _, pm := range slice {
		m[pm.ProductID] = pm
	}
	return m
}

func getPricesFromBazaar(itemID string, bazaarData map[string]HypixelProduct) (sellP, buyP float64, found bool) {
	p, ok := bazaarData[itemID]
	if !ok || len(p.SellSummary) == 0 || len(p.BuySummary) == 0 {
		return 0, 0, false
	}
	if p.SellSummary[0].PricePerUnit <= 0 || p.BuySummary[0].PricePerUnit <= 0 {
		return 0, 0, false
	}
	return p.SellSummary[0].PricePerUnit, p.BuySummary[0].PricePerUnit, true
}

func getMetricsForItem(itemID string, metrics map[string]ProductMetrics) (ProductMetrics, bool) {
	pm, ok := metrics[itemID]
	return pm, ok
}

// --- calculateC10M with detailed dlog tracing -----------------------------

func calculateC10M(qty float64, sellP, buyP float64, pm ProductMetrics) (primary, secondary float64) {
	dlog("[C10M] Start: qty=%.2f, sellP=%.2f, buyP=%.2f, pm={ID:%s, s_s:%.2f, s_f:%.2f, o_s:%.2f, o_f:%.2f}",
		qty, sellP, buyP, pm.ProductID, pm.SellSize, pm.SellFrequency, pm.OrderSize, pm.OrderFrequency)

	if qty <= 0 {
		dlog("[C10M] Early return qty<=0 -> (0,0)")
		return 0, 0
	}
	if sellP <= 0 || buyP <= 0 {
		dlog("[C10M] Early return invalid prices sellP=%.2f, buyP=%.2f -> (Inf,Inf)", sellP, buyP)
		return math.Inf(1), math.Inf(1)
	}

	s_s := math.Max(0, pm.SellSize)
	s_f := math.Max(0, pm.SellFrequency)
	o_s := math.Max(0, pm.OrderSize)
	o_f := math.Max(0, pm.OrderFrequency)
	dlog("[C10M] Clamped metrics: s_s=%.2f, s_f=%.2f, o_s=%.2f, o_f=%.2f", s_s, s_f, o_s, o_f)

	supplyVol := s_s * s_f
	demandVol := o_s * o_f
	dlog("[C10M] supplyVol=%.2f, demandVol=%.2f", supplyVol, demandVol)

	IF := supplyVol / o_f
	dlog("[C10M] IF before clamp=%.2f (supplyVol=%.2f / o_f=%.2f)", IF, supplyVol, o_f)
	if IF < 0 {
		IF = 0
	}
	dlog("[C10M] IF after clamp=%.2f", IF)

	var RR float64
	switch {
	case demandVol <= supplyVol:
		RR = 1.0
		dlog("[C10M] RR branch: demandVol<=supplyVol -> RR=1.0")
	case IF <= 0:
		RR = 1.0
		dlog("[C10M] RR branch: IF<=0 -> RR=1.0")
	default:
		raw := qty / IF
		RR = math.Ceil(raw)
		if RR < 1 {
			RR = 1
		}
		dlog("[C10M] RR branch: else -> qty/IF=%.2f, ceil=%.2f, final RR=%.2f", raw, math.Ceil(raw), RR)
	}

	if math.IsInf(RR, 1) {
		dlog("[C10M] RR is infinite -> returning (Inf,Inf)")
		return math.Inf(1), math.Inf(1)
	}
	dlog("[C10M] Using finite RR=%.2f", RR)

	RRint := int(RR)
	base := qty * sellP
	sumK := float64(RRint*(RRint+1)) / 2.0
	extra := sellP * (qty*RR - IF*sumK)
	adj := 1.0 - 1.0/RR
	dlog("[C10M] Components: RRint=%d, base=%.2f, sumK=%.2f, extra=%.2f, adj=%.4f",
		RRint, base, sumK, extra, adj)

	primary = base + adj*extra
	secondary = qty * buyP
	dlog("[C10M] Raw costs: primary=%.2f, secondary=%.2f", primary, secondary)

	if math.IsNaN(primary) || math.IsInf(primary, -1) || primary < 0 {
		dlog("[C10M] primary invalid -> setting Inf")
		primary = math.Inf(1)
	}
	if math.IsNaN(secondary) || math.IsInf(secondary, -1) || secondary < 0 {
		dlog("[C10M] secondary invalid -> setting Inf")
		secondary = math.Inf(1)
	}

	dlog("[C10M] Returning: primary=%.2f, secondary=%.2f", primary, secondary)
	return primary, secondary
}

// --- calculateFillTime -----------------------------------------------------

func calculateFillTime(qty, ss, sf, osz, of float64) float64 {
	if qty <= 0 {
		return 0
	}
	ss = math.Max(0, ss)
	sf = math.Max(0, sf)
	osz = math.Max(0, osz)
	of = math.Max(0, of)

	delta := ss*sf - osz*of
	if delta > 0 {
		return (20.0 * qty) / delta
	}
	if of <= 0 {
		return math.Inf(1)
	}
	calculatedIF := (ss * sf) / of
	if calculatedIF < 0 {
		calculatedIF = 0
	}

	var RR float64
	if calculatedIF <= 0 {
		RR = 1.0
	} else {
		RR = math.Ceil(qty / calculatedIF)
		if RR < 1 {
			RR = 1
		}
		if math.IsInf(RR, 0) || math.IsNaN(RR) {
			return math.Inf(1)
		}
	}

	fillTime := (20.0 * RR * qty) / of
	if math.IsNaN(fillTime) || math.IsInf(fillTime, -1) || fillTime < 0 {
		return math.Inf(1)
	}
	return fillTime
}

// --- calculateInstasellFillTime --------------------------------------------

func calculateInstasellFillTime(qty, buyMovingWeek float64) float64 {
	if qty <= 0 {
		return 0
	}
	if buyMovingWeek <= 0 {
		return math.Inf(1)
	}
	secondsInWeek := 604800.0
	buyRatePerSecond := buyMovingWeek / secondsInWeek
	if buyRatePerSecond <= 0 {
		return math.Inf(1)
	}
	return qty / buyRatePerSecond
}

// --- IngredientInfo & calculateBestCostInfo (Model V4) --------------------

type IngredientInfo struct {
	Quantity          int
	UnitCost          float64
	TotalCost         float64
	BuyOrderPrice     float64
	InstabuyPrice     float64
	BestMethod        string
	FillTime          float64
	InstasellFillTime float64
}

func calculateBestCostInfo(
	item string,
	qty int,
	bazaarData map[string]HypixelProduct,
	metricsData map[string]ProductMetrics,
) IngredientInfo {
	info := IngredientInfo{
		Quantity:          qty,
		UnitCost:          math.Inf(1),
		TotalCost:         math.Inf(1),
		BuyOrderPrice:     math.Inf(1),
		InstabuyPrice:     math.Inf(1),
		BestMethod:        "N/A",
		FillTime:          0,
		InstasellFillTime: math.Inf(1),
	}

	if qty <= 0 {
		info.UnitCost = 0
		info.TotalCost = 0
		info.InstasellFillTime = 0
		return info
	}

	sP, bP, okP := getPricesFromBazaar(item, bazaarData)
	if okP {
		info.BuyOrderPrice = sP
		info.InstabuyPrice = bP
	}

	m, okM := getMetricsForItem(item, metricsData)

	if okP && okM {
		primCost, _ := calculateC10M(float64(qty), sP, bP, m)
		secCost := float64(qty) * bP

		methodName := "Instabuy"
		methodCost := secCost

		if !math.IsInf(primCost, 1) && primCost <= secCost {
			methodName = "BuyOrder"
			if sP > 0 {
				methodCost = float64(qty) * sP
			} else {
				methodCost = math.Inf(1)
			}
		}

		info.BestMethod = methodName
		info.TotalCost = methodCost
		info.UnitCost = methodCost / float64(qty)

		if methodName == "BuyOrder" {
			info.FillTime = calculateFillTime(
				float64(qty),
				m.SellSize, m.SellFrequency,
				m.OrderSize, m.OrderFrequency,
			)
		}

		info.InstasellFillTime = calculateInstasellFillTime(
			float64(qty),
			bazaarData[item].QuickStatus.BuyMovingWeek,
		)
		return info
	}

	if okP {
		dlog(" [CostInfo V4] Metrics missing for %s, defaulting to Instabuy", item)
		info.BestMethod = "Instabuy (Default)"
		if bP > 0 {
			info.TotalCost = float64(qty) * bP
			info.UnitCost = bP
			info.FillTime = 0
		} else {
			info.TotalCost = math.Inf(1)
			info.UnitCost = math.Inf(1)
			info.BestMethod = "N/A"
		}
		if prod, okBaz := bazaarData[item]; okBaz {
			info.InstasellFillTime = calculateInstasellFillTime(
				float64(qty),
				prod.QuickStatus.BuyMovingWeek,
			)
		}
		return info
	}

	dlog(" [CostInfo V4] No bazaar data for %s", item)
	return info
}

// --- expandItem -------------------------------------------------------------

type ItemStep struct {
	name   string
	factor int
}

func isInPath(item string, path []ItemStep) bool {
	for _, step := range path {
		if step.name == item {
			return true
		}
	}
	return false
}

func recipeHasIngredients(r SingleRecipe) bool {
	return r.A1 != "" || r.A2 != "" || r.A3 != "" ||
		r.B1 != "" || r.B2 != "" || r.B3 != "" ||
		r.C1 != "" || r.C2 != "" || r.C3 != ""
}

func aggregateCells(cells map[string]string) map[string]int {
	out := make(map[string]int)
	for _, pos := range []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"} {
		cell := cells[pos]
		if cell == "" {
			continue
		}
		parts := strings.Split(cell, ":")
		if len(parts) != 2 {
			continue
		}
		amt, err := strconv.Atoi(parts[1])
		if err != nil || amt <= 0 {
			continue
		}
		out[parts[0]] += amt
	}
	return out
}

func expandItem(
	itemName string,
	factor int,
	path []ItemStep,
	bazaarData map[string]HypixelProduct,
	metricsData map[string]ProductMetrics,
	depth int,
	levelCosts *map[int]float64,
	topLevelBuyStrategy string,
) map[string]IngredientInfo {

	addCostAtLevel := func(cost float64, level int) {
		if !math.IsInf(cost, 0) && !math.IsNaN(cost) && cost > 0 {
			(*levelCosts)[level] += cost
		}
	}

	// 1) Cycle detection
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].name == itemName {
			dlog("  [CYCLE] depth=%d item=%s", depth, itemName)
			baseInfo := calculateBestCostInfo(itemName, path[i].factor, bazaarData, metricsData)
			addCostAtLevel(baseInfo.TotalCost, depth)
			return map[string]IngredientInfo{itemName: baseInfo}
		}
	}
	path = append(path, ItemStep{itemName, factor})

	// 2) Load item
	itm, err := loadItem(itemName)
	if err != nil {
		baseInfo := calculateBestCostInfo(itemName, factor, bazaarData, metricsData)
		addCostAtLevel(baseInfo.TotalCost, depth)
		return map[string]IngredientInfo{itemName: baseInfo}
	}

	// 3) Choose recipe
	var cells map[string]string
	recipeUsed := false
	if len(itm.Recipes) > 0 {
		recipeUsed = true
		chosen := itm.Recipes[0]
		for _, r := range itm.Recipes {
			if r.Count == 1 {
				chosen = r
				break
			}
		}
		cells = map[string]string{
			"A1": chosen.A1, "A2": chosen.A2, "A3": chosen.A3,
			"B1": chosen.B1, "B2": chosen.B2, "B3": chosen.B3,
			"C1": chosen.C1, "C2": chosen.C2, "C3": chosen.C3,
		}
	} else if recipeHasIngredients(itm.Recipe) {
		recipeUsed = true
		r := itm.Recipe
		cells = map[string]string{
			"A1": r.A1, "A2": r.A2, "A3": r.A3,
			"B1": r.B1, "B2": r.B2, "B3": r.B3,
			"C1": r.C1, "C2": r.C2, "C3": r.C3,
		}
	}
	if !recipeUsed {
		baseInfo := calculateBestCostInfo(itemName, factor, bazaarData, metricsData)
		addCostAtLevel(baseInfo.TotalCost, depth)
		return map[string]IngredientInfo{itemName: baseInfo}
	}

	// 4) Decide craft vs buy
	primCost, secCost := math.Inf(1), math.Inf(1)
	if sP, bP, okP := getPricesFromBazaar(itemName, bazaarData); okP {
		if m, okM := getMetricsForItem(itemName, metricsData); okM {
			primCost, secCost = calculateC10M(float64(factor), sP, bP, m)
		}
	}
	if primCost <= 0 || math.IsNaN(primCost) || math.IsInf(primCost, -1) {
		primCost = math.Inf(1)
	}
	if secCost <= 0 || math.IsNaN(secCost) || math.IsInf(secCost, -1) {
		secCost = math.Inf(1)
	}

	var costBuy float64
	if depth == 0 && topLevelBuyStrategy == "FORCE_PRIMARY" {
		costBuy = primCost
	} else if depth == 0 && topLevelBuyStrategy == "FORCE_SECONDARY" {
		costBuy = secCost
	} else {
		costBuy = math.Min(primCost, secCost)
	}
	if costBuy <= 0 || math.IsNaN(costBuy) || math.IsInf(costBuy, -1) {
		costBuy = math.Inf(1)
	}

	// Record top-level cost
	if depth == 0 {
		addCostAtLevel(costBuy, 0)
	}

	ingMap := aggregateCells(cells)
	if len(ingMap) == 0 {
		dlog("  [WARN] depth=%d item=%s | no ingredients", depth, itemName)
		baseInfo := calculateBestCostInfo(itemName, factor, bazaarData, metricsData)
		addCostAtLevel(baseInfo.TotalCost, depth)
		return map[string]IngredientInfo{itemName: baseInfo}
	}

	totalCraft := 0.0
	for ing, amt := range ingMap {
		ii := calculateBestCostInfo(ing, factor*amt, bazaarData, metricsData)
		if math.IsInf(ii.TotalCost, 1) {
			totalCraft = math.Inf(1)
			dlog("  [INFO] depth=%d item=%s | cannot craft %s", depth, itemName, ing)
			break
		}
		totalCraft += ii.TotalCost
	}

	epsilon := 0.01
	shouldCraft := !math.IsInf(totalCraft, 1) && (totalCraft < costBuy-epsilon)

	dlog("  [DECISION] depth=%d item=%s | buy=%.2f craft=%.2f -> craft? %v",
		depth, itemName, costBuy, totalCraft, shouldCraft)

	if !shouldCraft {
		if depth > 0 {
			addCostAtLevel(costBuy, depth)
		}
		return map[string]IngredientInfo{
			itemName: calculateBestCostInfo(itemName, factor, bazaarData, metricsData),
		}
	}

	final := make(map[string]IngredientInfo)
	for ing, amt := range ingMap {
		sub := expandItem(ing, factor*amt, path, bazaarData, metricsData, depth+1, levelCosts, "DEFAULT")
		for k, v := range sub {
			if ex, ok := final[k]; ok {
				ex.Quantity += v.Quantity
				final[k] = ex
			} else {
				final[k] = v
			}
		}
	}
	return final
}

// --- Formatting & Printing Helpers -----------------------------------------

func formatFloatOrNA(val float64) string {
	if math.IsInf(val, 0) || math.IsNaN(val) || val < 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", val)
}

func calculateAndPrintProfit(
	scenarioLabel string,
	item string,
	startQuantity int,
	scenarioCraftCost float64,
	bazaarData map[string]HypixelProduct,
) {
	profitLabel := "Profit (N/A Scenario)"
	profitValStr := "N/A"
	var targetPrice float64
	priceFound := false
	calculationPossible := true

	if math.IsInf(scenarioCraftCost, 0) || math.IsNaN(scenarioCraftCost) {
		calculationPossible = false
		profitValStr = "N/A (Invalid Craft Cost)"
	}

	topLevelProductData, ok := bazaarData[item]
	if !ok {
		calculationPossible = false
		profitValStr = "N/A (Item not in Bazaar)"
	}

	if calculationPossible {
		if strings.Contains(scenarioLabel, "Instabuy Start") {
			profitLabel = "Est. Instasell Profit (vs Craft Cost):"
			if len(topLevelProductData.BuySummary) > 0 && topLevelProductData.BuySummary[0].PricePerUnit > 0 {
				targetPrice = topLevelProductData.BuySummary[0].PricePerUnit
				priceFound = true
			} else {
				profitValStr = "N/A (No Buy Orders)"
				calculationPossible = false
			}
		} else if strings.Contains(scenarioLabel, "Buy Order Start") {
			profitLabel = "Est. Sell Order Profit (vs Craft Cost):"
			if len(topLevelProductData.SellSummary) > 0 && topLevelProductData.SellSummary[0].PricePerUnit > 0 {
				targetPrice = topLevelProductData.SellSummary[0].PricePerUnit
				priceFound = true
			} else {
				profitValStr = "N/A (No Sell Orders)"
				calculationPossible = false
			}
		} else {
			calculationPossible = false
			profitValStr = "N/A (Unknown Scenario)"
		}
	}

	if calculationPossible && priceFound {
		totalRevenue := targetPrice * float64(startQuantity)
		profit := totalRevenue - scenarioCraftCost
		profitValStr = formatFloatOrNA(profit)
	}

	separatorWidth := 136
	fmt.Printf("%-*s %s\n", separatorWidth-len(profitValStr)-1, profitLabel, profitValStr)
}

func printResults(
	scenarioLabel string,
	item string,
	startQuantity int,
	result map[string]IngredientInfo,
	levelCosts map[int]float64,
	bazaarData map[string]HypixelProduct,
) {
	fmt.Printf("\n--- [%s] Base Ingredients for %d x %s ---\n", scenarioLabel, startQuantity, item)

	titleFormat := "%-28s | %-7s | %-10s | %-10s | %-12s | %-8s | %-12s | %-12s | %s\n"
	lineFormat := "%-28s | x %-7d | @ %-10s | BO: %-10s | IB: %-12s | %-8s | %-12s | %-12s | %s\n"
	separatorWidth := 136
	separator := strings.Repeat("-", separatorWidth)
	fmt.Printf(titleFormat, "Item", "Qty", "Unit Cost", "BO Price", "Instabuy Pr", "Method", "Total Cost", "Buy Fill", "Instasell")
	fmt.Println(separator)

	var maxFillTimeBO float64
	hasBuyOrderItems := false

	var topLevelInstasellFillTime float64 = math.NaN()
	isScenarioA := strings.Contains(scenarioLabel, "Instabuy Start")
	if isScenarioA {
		topLevelInstasellFillTime = math.Inf(1)
		if topLevelProductData, ok := bazaarData[item]; ok {
			topLevelInstasellFillTime = calculateInstasellFillTime(float64(startQuantity), topLevelProductData.QuickStatus.BuyMovingWeek)
		}
	}

	if len(result) == 0 {
		fmt.Printf(lineFormat, "(No ingredients listed/needed)", 0, "-", "-", "-", "N/A", "0.00", "-", "-")
	} else {
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, name := range keys {
			info := result[name]
			unitCostStr := formatFloatOrNA(info.UnitCost)
			boPriceStr := formatFloatOrNA(info.BuyOrderPrice)
			ibPriceStr := formatFloatOrNA(info.InstabuyPrice)
			itemTotalCostStr := formatFloatOrNA(info.TotalCost)

			fillTimeStr := "-"
			if info.BestMethod == "BuyOrder" {
				hasBuyOrderItems = true
				if math.IsInf(info.FillTime, 1) {
					fillTimeStr = "Inf"
					maxFillTimeBO = math.Inf(1)
				} else if !math.IsNaN(info.FillTime) && info.FillTime >= 0 {
					fillTimeStr = fmt.Sprintf("%.2fs", info.FillTime)
					if !math.IsInf(maxFillTimeBO, 1) && info.FillTime > maxFillTimeBO {
						maxFillTimeBO = info.FillTime
					}
				}
			}
			fmt.Printf(lineFormat, name, info.Quantity, unitCostStr, boPriceStr, ibPriceStr, info.BestMethod, itemTotalCostStr, fillTimeStr, "-")
		}
	}
	fmt.Println(separator)

	var sumOfLevelCosts float64
	for _, cost := range levelCosts {
		if !math.IsInf(cost, 0) && !math.IsNaN(cost) && cost > 0 {
			sumOfLevelCosts += cost
		}
	}
	totalCostStr := formatFloatOrNA(sumOfLevelCosts)
	fmt.Printf("%-*s Total: %s\n", separatorWidth-len(" Total: ")-len(totalCostStr)+1, "Estimated Total Cost (Sum of Decision Costs):", totalCostStr)

	maxFillTimeBOStr := "N/A"
	if len(result) > 0 {
		if hasBuyOrderItems {
			if math.IsInf(maxFillTimeBO, 1) {
				maxFillTimeBOStr = "Infinite"
			} else {
				maxFillTimeBOStr = fmt.Sprintf("%.2f seconds", maxFillTimeBO)
			}
		} else {
			maxFillTimeBOStr = "N/A (No BO items)"
		}
	}
	fmt.Printf("%-*s Max Buy Order Fill Time: %s\n", separatorWidth-len(" Max Buy Order Fill Time: ")-len(maxFillTimeBOStr)+1, "", maxFillTimeBOStr)

	instasellSummaryTimeStr := "N/A"
	if isScenarioA {
		if math.IsInf(topLevelInstasellFillTime, 1) {
			instasellSummaryTimeStr = "Infinite"
		} else if !math.IsNaN(topLevelInstasellFillTime) && topLevelInstasellFillTime >= 0 {
			instasellSummaryTimeStr = fmt.Sprintf("%.2f seconds", topLevelInstasellFillTime)
		}
	}
	fmt.Printf("%-*s Top-Level Instasell Fill Time: %s\n", separatorWidth-len(" Top-Level Instasell Fill Time: ")-len(instasellSummaryTimeStr)+1, "", instasellSummaryTimeStr)

	calculateAndPrintProfit(scenarioLabel, item, startQuantity, sumOfLevelCosts, bazaarData)

	fmt.Println(strings.Repeat("=", separatorWidth))
}
func main() {
	fmt.Println("Loading market data...")
	resp, err := http.Get("https://api.hypixel.net/v2/skyblock/bazaar")
	if err != nil {
		log.Fatalf("fetch bazaar: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("bazaar returned %d: %s", resp.StatusCode, b)
	}
	var bazResp HypixelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&bazResp); err != nil {
		log.Fatalf("parse bazaar JSON: %v", err)
	}
	if !bazResp.Success {
		log.Fatal("bazaar API reported failure")
	}
	bazaarData := bazResp.Products

	metricsData := loadMetricsMap("latest_metrics.json")
	fmt.Println("Market data loaded.")

	for {
		fmt.Print("\nEnter item to calculate ingredients (or 'exit'): ")
		var item string
		if _, err := fmt.Scanln(&item); err != nil {
			if err == io.EOF {
				fmt.Println("\nExiting.")
				break
			}
			fmt.Println("Invalid input.")
			var discard string
			fmt.Scanln(&discard)
			continue
		}
		item = strings.TrimSpace(item)
		if item == "exit" {
			break
		}
		if item == "" {
			continue
		}

		var startQuantity int
		for {
			fmt.Printf("Enter starting quantity for %s: ", item)
			var qtyStr string
			if _, err := fmt.Scanln(&qtyStr); err != nil {
				if err == io.EOF {
					fmt.Println("\nExiting.")
					return
				}
				fmt.Println("Invalid input.")
				var discard string
				fmt.Scanln(&discard)
				continue
			}
			qtyStr = strings.TrimSpace(qtyStr)
			qty, err := strconv.Atoi(qtyStr)
			if err != nil || qty <= 0 {
				fmt.Println("Quantity must be a positive integer.")
				continue
			}
			startQuantity = qty
			break
		}

		// Scenario A
		fmt.Printf("\n--- Scenario A: Top Level uses INSTABUY cost for initial decision ---\n")
		initialPathA := []ItemStep{}
		levelCostsA := make(map[int]float64)
		resultA := expandItem(item, startQuantity, initialPathA, bazaarData, metricsData, 0, &levelCostsA, "FORCE_SECONDARY")
		printResults("Instabuy Start", item, startQuantity, resultA, levelCostsA, bazaarData)

		// Scenario B
		fmt.Printf("\n--- Scenario B: Top Level uses BUY ORDER cost for initial decision ---\n")
		initialPathB := []ItemStep{}
		levelCostsB := make(map[int]float64)
		resultB := expandItem(item, startQuantity, initialPathB, bazaarData, metricsData, 0, &levelCostsB, "FORCE_PRIMARY")
		printResults("Buy Order Start", item, startQuantity, resultB, levelCostsB, bazaarData)
	}
}
