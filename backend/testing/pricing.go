package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Recipe represents a crafting recipe structure for items with multiple recipes.
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

// SingleRecipe represents a single recipe with a count field.
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

// Item represents the structure of the JSON file.
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

// ItemStep holds an item name and its associated factor at a recursion level.
type ItemStep struct {
	name   string
	factor int
}

// aggregateCells ignores cell positions and sums amounts per ingredient.
func aggregateCells(cells map[string]string) map[string]int {
	positions := []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"}
	ingredients := make(map[string]int)
	log.Printf("Aggregating cells: %v", cells)
	for _, pos := range positions {
		cell := cells[pos]
		if cell == "" {
			continue
		}
		// Expected format: "INGREDIENT:AMOUNT"
		parts := strings.Split(cell, ":")
		if len(parts) != 2 {
			log.Printf("Unexpected format in cell %s: %s", pos, cell)
			continue
		}
		ing := parts[0]
		amt, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Error parsing amount in cell %s: %s", pos, cell)
			continue
		}
		ingredients[ing] += amt
		log.Printf("Found ingredient '%s' with amount %d (accumulated: %d)", ing, amt, ingredients[ing])
	}
	log.Printf("Aggregated ingredients: %v", ingredients)
	return ingredients
}

// shouldExpand makes a decision before recursing into an ingredient.
// For example, here we decide to expand further only if a recipe file exists.
func shouldExpand(itemName string, factor int, path []ItemStep) bool {
	filePath := filepath.Join("dependencies", "items", itemName+".json")
	if _, err := os.Stat(filePath); err != nil {
		log.Printf("No recipe file for '%s'; will not expand further.", itemName)
		return false
	}
	return true
}

// expandItem recursively expands an item into its base ingredients.
// 'factor' is multiplied into each ingredient's quantity.
// 'path' holds the recursion chain to detect cycles.
// This version performs decision making before aggregating down a level.
func expandItem(itemName string, factor int, path []ItemStep) map[string]int {
	log.Printf("Expanding item '%s' with factor %d; current path: %v", itemName, factor, path)

	// Cycle detection: if the item already exists in the path, backtrack 2 steps.
	for _, step := range path {
		if step.name == itemName {
			if len(path) >= 2 {
				target := path[len(path)-2]
				log.Printf("Cycle detected for item '%s'. Backtracking 2 steps to '%s' with factor %d.",
					itemName, target.name, target.factor)
				return map[string]int{target.name: target.factor}
			}
			log.Printf("Cycle detected for item '%s' but insufficient history. Treating as base ingredient.", itemName)
			return map[string]int{itemName: factor}
		}
	}

	// Append current step to path.
	newPath := append(path, ItemStep{name: itemName, factor: factor})

	// Build file path and read the JSON file.
	filePath := filepath.Join("dependencies", "items", itemName+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("File not found for item '%s'. Assuming it is a base ingredient.", itemName)
		return map[string]int{itemName: factor}
	}

	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		log.Printf("Error unmarshaling JSON for item '%s'. Treating as base ingredient.", itemName)
		return map[string]int{itemName: factor}
	}

	var cells map[string]string
	if len(item.Recipes) > 0 {
		log.Printf("Item '%s' has multiple recipes. Selecting one based on criteria.", itemName)
		var chosen *Recipe
		for _, rec := range item.Recipes {
			if rec.Count == 1 {
				chosen = &rec
				break
			}
		}
		if chosen == nil {
			chosen = &item.Recipes[0]
		}
		cells = map[string]string{
			"A1": chosen.A1,
			"A2": chosen.A2,
			"A3": chosen.A3,
			"B1": chosen.B1,
			"B2": chosen.B2,
			"B3": chosen.B3,
			"C1": chosen.C1,
			"C2": chosen.C2,
			"C3": chosen.C3,
		}
		log.Printf("Using recipe: %+v", chosen)
	} else if item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" ||
		item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" ||
		item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "" {
		log.Printf("Item '%s' has a single recipe.", itemName)
		cells = map[string]string{
			"A1": item.Recipe.A1,
			"A2": item.Recipe.A2,
			"A3": item.Recipe.A3,
			"B1": item.Recipe.B1,
			"B2": item.Recipe.B2,
			"B3": item.Recipe.B3,
			"C1": item.Recipe.C1,
			"C2": item.Recipe.C2,
			"C3": item.Recipe.C3,
		}
	} else {
		log.Printf("Item '%s' has no recipe. Treating as base ingredient.", itemName)
		return map[string]int{itemName: factor}
	}

	aggregated := aggregateCells(cells)
	final := make(map[string]int)

	// Decision making: for each ingredient, decide whether to expand it or treat it as base.
	for ing, amt := range aggregated {
		newFactor := factor * amt
		log.Printf("Considering ingredient '%s' with new factor %d (base factor %d * ingredient amount %d)",
			ing, newFactor, factor, amt)
		if shouldExpand(ing, newFactor, newPath) {
			log.Printf("Deciding to expand ingredient '%s'", ing)
			subIngredients := expandItem(ing, newFactor, newPath)
			for sub, subAmt := range subIngredients {
				final[sub] += subAmt
				log.Printf("Merging ingredient '%s': added %d (total now %d)", sub, subAmt, final[sub])
			}
		} else {
			log.Printf("Not expanding ingredient '%s'; treating as base with factor %d", ing, newFactor)
			final[ing] += newFactor
		}
	}

	log.Printf("Finished expanding '%s'. Aggregated ingredients: %v", itemName, final)
	return final
}

func main() {
	fmt.Println("Enter item names (type 'exit' to quit):")
	for {
		var itemName string
		fmt.Print("Item name: ")
		_, err := fmt.Scanln(&itemName)
		if err != nil {
			log.Println("Error reading input:", err)
			continue
		}
		if itemName == "exit" {
			break
		}

		result := expandItem(itemName, 1, []ItemStep{})
		fmt.Printf("Aggregated base ingredients for %s:\n", itemName)
		for ing, amt := range result {
			fmt.Printf("%s: %d\n", ing, amt)
		}
		fmt.Println()
	}
}
