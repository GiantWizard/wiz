package main

import (
	"encoding/json"
	"fmt"
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

// SingleRecipe represents a single recipe.
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

// Item represents the JSON structure for an item.
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

// ItemStep holds an item name and its factor when it was expanded.
type ItemStep struct {
	name   string
	factor int
}

// aggregateCells reads the 3×3 grid of recipe cells and aggregates amounts.
// Each non-empty cell is expected to be in the form "INGREDIENT:amount".
func aggregateCells(cells map[string]string) map[string]int {
	positions := []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"}
	ingredients := make(map[string]int)
	for _, pos := range positions {
		cell := cells[pos]
		if cell == "" {
			continue
		}
		parts := strings.Split(cell, ":")
		if len(parts) != 2 {
			continue
		}
		ing := parts[0]
		amt, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		ingredients[ing] += amt
	}
	return ingredients
}

// isInPath returns true if the given item is already in the current expansion path.
func isInPath(itemName string, path []ItemStep) bool {
	for _, step := range path {
		if step.name == itemName {
			return true
		}
	}
	return false
}

// expandItem recursively expands an item into its base ingredients.
// If a cycle is detected for an ingredient, it finds the most recent occurrence in the path
// and "backs up" to that occurrence (i.e. uses its stored factor) instead of expanding further.
func expandItem(itemName string, factor int, path []ItemStep) map[string]int {
	// Top-level cycle check: if the current item already appears in the path,
	// backtrack by returning the factor stored at its most recent occurrence.
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].name == itemName {
			// Use the factor from the most recent occurrence.
			return map[string]int{path[i].name: path[i].factor}
		}
	}

	// Append the current item to the path.
	path = append(path, ItemStep{name: itemName, factor: factor})

	// Read the JSON file for the item.
	filePath := filepath.Join("dependencies", "items", itemName+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		// If file not found, treat the item as a base ingredient.
		return map[string]int{itemName: factor}
	}
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		return map[string]int{itemName: factor}
	}

	// Determine which recipe to use.
	var cells map[string]string
	if len(item.Recipes) > 0 {
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
			"A1": chosen.A1, "A2": chosen.A2, "A3": chosen.A3,
			"B1": chosen.B1, "B2": chosen.B2, "B3": chosen.B3,
			"C1": chosen.C1, "C2": chosen.C2, "C3": chosen.C3,
		}
	} else if item.Recipe.A1 != "" || item.Recipe.A2 != "" || item.Recipe.A3 != "" ||
		item.Recipe.B1 != "" || item.Recipe.B2 != "" || item.Recipe.B3 != "" ||
		item.Recipe.C1 != "" || item.Recipe.C2 != "" || item.Recipe.C3 != "" {
		cells = map[string]string{
			"A1": item.Recipe.A1, "A2": item.Recipe.A2, "A3": item.Recipe.A3,
			"B1": item.Recipe.B1, "B2": item.Recipe.B2, "B3": item.Recipe.B3,
			"C1": item.Recipe.C1, "C2": item.Recipe.C2, "C3": item.Recipe.C3,
		}
	} else {
		// No recipe found; treat as a base ingredient.
		return map[string]int{itemName: factor}
	}

	// Aggregate ingredients from the recipe cells.
	aggregated := aggregateCells(cells)
	final := make(map[string]int)

	// Process each ingredient.
	for ing, amt := range aggregated {
		newFactor := factor * amt
		ingFilePath := filepath.Join("dependencies", "items", ing+".json")
		if _, err := os.Stat(ingFilePath); err == nil {
			// If this ingredient is already in the path, find its most recent occurrence.
			if isInPath(ing, path) {
				var dupIndex int = -1
				for i := len(path) - 1; i >= 0; i-- {
					if path[i].name == ing {
						dupIndex = i
						break
					}
				}
				// Backtrack to the most recent occurrence (which should be GOLD_INGOT in your case).
				if dupIndex != -1 {
					target := path[dupIndex]
					fmt.Printf("Cycle detected for ingredient '%s'. Backtracking to its previous occurrence (using '%s' with factor %d).\n", ing, target.name, target.factor)
					final[target.name] += target.factor
					continue
				}
				// Fallback: add normally if no occurrence found (should not happen).
				final[ing] += newFactor
				continue
			}

			// Ask user if they want to expand the ingredient.
			var answer string
			fmt.Printf("Ingredient '%s' has a recipe file. Expand it? (y/n): ", ing)
			fmt.Scanln(&answer)
			if strings.ToLower(answer) == "y" {
				subIngredients := expandItem(ing, newFactor, path)
				for sub, subAmt := range subIngredients {
					final[sub] += subAmt
				}
			} else {
				final[ing] += newFactor
			}
		} else {
			// No recipe file exists; treat as a base ingredient.
			final[ing] += newFactor
		}
	}

	return final
}

func main() {
	fmt.Println("Enter item names (type 'exit' to quit):")
	for {
		fmt.Print("Item name: ")
		var itemName string
		if _, err := fmt.Scanln(&itemName); err != nil {
			fmt.Println("Error reading input:", err)
			continue
		}
		if itemName == "exit" {
			break
		}

		// Check whether the top-level item has a recipe file.
		topFile := filepath.Join("dependencies", "items", itemName+".json")
		expandTop := false
		if _, err := os.Stat(topFile); err == nil {
			var answer string
			fmt.Printf("Item '%s' has a recipe file. Expand it? (y/n): ", itemName)
			fmt.Scanln(&answer)
			expandTop = strings.ToLower(answer) == "y"
		}

		if !expandTop {
			fmt.Printf("Aggregated base ingredients for %s:\n%s: %d\n\n", itemName, itemName, 1)
			continue
		}

		result := expandItem(itemName, 1, []ItemStep{})
		fmt.Printf("Aggregated base ingredients for %s:\n", itemName)
		for ing, amt := range result {
			fmt.Printf("%s: %d\n", ing, amt)
		}
		fmt.Println()
	}
}
