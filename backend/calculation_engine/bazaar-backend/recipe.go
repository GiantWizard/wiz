package main

// Item struct definitions remain here as they describe the recipe file format.

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
	ItemID  string       `json:"itemid"`         // Make sure this matches the JSON field name, e.g., "internalname" or "itemid"
	Name    string       `json:"name,omitempty"` // Optional: if your JSON has a display name
	Recipe  SingleRecipe `json:"recipe"`
	Recipes []Recipe     `json:"recipes"`
}

// ItemStep is used for cycle detection path tracking.
type ItemStep struct {
	name     string
	quantity float64
}

// The old expandItemRecursive and ExpandItem are removed from here.
// Their new counterparts are in tree_builder.go (expandItemRecursiveTree, ExpandItemToTree).
