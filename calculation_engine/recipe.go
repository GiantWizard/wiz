// recipe.go
package main

// Item struct definitions remain here as they describe the recipe file format.

type Recipe struct {
	Type  string `json:"type"` // e.g., "minecraft:crafting_shaped" - may not be used by current logic but good for completeness
	A1    string `json:"A1"`
	A2    string `json:"A2"`
	A3    string `json:"A3"`
	B1    string `json:"B1"`
	B2    string `json:"B2"`
	B3    string `json:"B3"`
	C1    string `json:"C1"`
	C2    string `json:"C2"`
	C3    string `json:"C3"`
	Count int    `json:"count"` // How many items this recipe yields
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
	Count int    `json:"count"` // How many items this recipe yields
}

// Item represents the structure of a recipe JSON file.
// It can contain either a single 'recipe' object or an array of 'recipes'.
// The parsing logic in level_cost_expansion.go prioritizes 'recipes' if present.
type Item struct {
	ItemID  string       `json:"itemid"`         // The Hypixel/Minecraft ID of the item this recipe crafts
	Name    string       `json:"name,omitempty"` // Optional: a human-readable name
	Recipe  SingleRecipe `json:"recipe"`         // Used if 'recipes' array is not present or empty
	Recipes []Recipe     `json:"recipes"`        // Preferred: an array of possible recipes
}

// ItemStep is used for cycle detection path tracking during recursive expansion.
// It stores the item name (normalized) and the quantity being processed at that step.
type ItemStep struct {
	name     string  // Normalized item ID
	quantity float64 // Quantity of this item in the path
}

// Note: The old expandItemRecursive and ExpandItem functions were removed from here
// as their new counterparts (expandItemRecursiveTree, ExpandItemToTree) are in tree_builder.go.
