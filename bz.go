package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	dataURL      = "https://raw.githubusercontent.com/GiantWizard/Wiz/main/Wiz/data.json"
	bazaarURL    = "https://api.hypixel.net/skyblock/bazaar"
	lowestBinURL = "http://moulberry.codes/lowestbin.json"
)

type ItemData struct {
	Name   string                   `json:"name"`
	Recipe map[string]json.RawMessage `json:"recipe"`
}

type PriceData struct {
	Price  float64 `json:"price"`
	Method string  `json:"method"`
}

type BazaarResponse struct {
	Products map[string]struct {
		QuickStatus struct {
			BuyPrice       float64 `json:"buyPrice"`
			SellPrice      float64 `json:"sellPrice"`
			SellMovingWeek int     `json:"sellMovingWeek"`
			BuyMovingWeek  int     `json:"buyMovingWeek"`
		} `json:"quick_status"`
	} `json:"products"`
}

type ProfitData struct {
	ItemID        string
	Profit        float64
	ProfitPercent int
	CraftingCost  float64
	SellPrice     float64
}

// Fetches the JSON data from the provided URL
func fetchData(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

// Helper function to parse recipe values (handles both strings and numbers)
func parseRecipeValue(raw json.RawMessage) (string, error) {
	var strValue string
	var numValue int

	// Try to unmarshal as a string
	if err := json.Unmarshal(raw, &strValue); err == nil {
		return strValue, nil
	}

	// If unmarshaling as a string fails, try as a number
	if err := json.Unmarshal(raw, &numValue); err == nil {
		return strconv.Itoa(numValue), nil
	}

	return "", fmt.Errorf("unknown recipe value format")
}

// Fetches Bazaar prices and returns a map of prices
func fetchBazaarPrices() (map[string]PriceData, error) {
	var response BazaarResponse
	err := fetchData(bazaarURL, &response)
	if err != nil {
		return nil, err
	}

	prices := make(map[string]PriceData)
	for itemID, details := range response.Products {
		quickStatus := details.QuickStatus
		buyPrice := quickStatus.BuyPrice
		sellPrice := quickStatus.SellPrice

		if buyPrice > 0 && sellPrice > 0 {
			method := "Instabuy"
			if buyPrice/sellPrice >= 1.07 {
				method = "Buy Order"
			}
			prices[itemID] = PriceData{
				Price:  buyPrice,
				Method: method,
			}
		}
	}
	return prices, nil
}

// Fetches lowest BIN prices and returns a map
func fetchLowestBINPrices() (map[string]float64, error) {
	var lbinData map[string]float64
	err := fetchData(lowestBinURL, &lbinData)
	return lbinData, err
}

// Builds a recipe tree recursively
func buildRecipeTree(data map[string]ItemData, itemID string, prices map[string]PriceData, lbinData map[string]float64, visited map[string]bool) (map[string]interface{}, error) {
	if visited[itemID] {
		return map[string]interface{}{"name": itemID, "note": "cycle detected"}, nil
	}

	item, exists := data[itemID]
	if !exists {
		price := prices[itemID].Price
		if price == 0 {
			price = lbinData[itemID]
		}
		return map[string]interface{}{"name": itemID, "note": "base item", "cost": price}, nil
	}

	visited[itemID] = true
	tree := map[string]interface{}{"name": itemID, "children": []map[string]interface{}{}, "count": 1}
	var totalCost float64

	for ing, rawCount := range item.Recipe {
		countStr, err := parseRecipeValue(rawCount)
		if err != nil {
			return nil, fmt.Errorf("failed to parse recipe value for %s: %v", ing, err)
		}
		count, _ := strconv.Atoi(countStr)
		subTree, err := buildRecipeTree(data, ing, prices, lbinData, visited)
		if err != nil {
			return nil, err
		}
		subTree["count"] = count
		tree["children"] = append(tree["children"].([]map[string]interface{}), subTree)

		subPrice := prices[ing].Price
		if subPrice == 0 {
			subPrice = lbinData[ing]
		}
		totalCost += subPrice * float64(count)
	}

	tree["cost"] = totalCost
	visited[itemID] = false
	return tree, nil
}

// Prints the recipe tree recursively with formatting
func printRecipeTree(tree map[string]interface{}, level int, multiplier int) {
	indent := strings.Repeat("  ", level)
	note := ""
	if n, ok := tree["note"].(string); ok {
		note = fmt.Sprintf(" (%s)", n)
	}

	count := tree["count"].(int) * multiplier
	cost := tree["cost"].(float64)

	fmt.Printf("%s- %s x%d, Cost: %.2f%s\n", indent, tree["name"], count, cost, note)

	if children, ok := tree["children"].([]map[string]interface{}); ok {
		for _, child := range children {
			printRecipeTree(child, level+1, count)
		}
	}
}

// Function to separate lines and format output
func printFormattedTree(tree map[string]interface{}) {
	fmt.Println("\n--- Recipe Tree ---")
	printRecipeTree(tree, 0, 1)
	fmt.Println("--- End of Tree ---\n")
}

// Calculates profit and returns the top 20 most profitable crafts
func calculateProfit(data map[string]ItemData, prices map[string]PriceData, lbinData map[string]float64) []ProfitData {
	var profits []ProfitData
	for itemID := range data {
		tree, _ := buildRecipeTree(data, itemID, prices, lbinData, map[string]bool{})
		craftingCost := tree["cost"].(float64)
		bazaarPrice := prices[itemID].Price

		if bazaarPrice > 50000 && craftingCost < bazaarPrice {
			profit := bazaarPrice - craftingCost
			profitPercent := int((bazaarPrice - craftingCost) / craftingCost * 100)
			profits = append(profits, ProfitData{
				ItemID:        itemID,
				Profit:        profit,
				ProfitPercent: profitPercent,
				CraftingCost:  craftingCost,
				SellPrice:     bazaarPrice,
			})
		}
	}
	sort.Slice(profits, func(i, j int) bool {
		return profits[i].ProfitPercent > profits[j].ProfitPercent
	})
	return profits[:20]
}

func main() {
	var data map[string]ItemData
	err := fetchData(dataURL, &data)
	if err != nil {
		fmt.Println("Failed to load data:", err)
		return
	}

	prices, err := fetchBazaarPrices()
	if err != nil {
		fmt.Println("Failed to fetch Bazaar prices:", err)
		return
	}

	lbinData, err := fetchLowestBINPrices()
	if err != nil {
		fmt.Println("Failed to fetch Lowest BIN prices:", err)
		return
	}

	topCrafts := calculateProfit(data, prices, lbinData)
	fmt.Println("Top 20 Most Profitable Crafts:")
	for _, craft := range topCrafts {
		fmt.Printf("- %s: Profit = %.2f, Profit Percent = %d%%, Crafting Cost = %.2f, Sell Price = %.2f\n",
			craft.ItemID, craft.Profit, craft.ProfitPercent, craft.CraftingCost, craft.SellPrice)
	}

	itemID := "SOME_ITEM_ID" // Replace with desired item ID or take user input
	recipeTree, err := buildRecipeTree(data, itemID, prices, lbinData, map[string]bool{})
	if err != nil {
		fmt.Println("Failed to build recipe tree:", err)
		return
	}

	printFormattedTree(recipeTree)
}
