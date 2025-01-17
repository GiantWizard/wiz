package main

import (
    "bufio"
    "compress/gzip"
    "encoding/json"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "sort"
    "strings"
    "sync"
    "time"
)

// Structures for Hypixel Bazaar API
type BazaarResponse struct {
    Success     bool                     `json:"success"`
    LastUpdated int64                    `json:"lastUpdated"`
    Products    map[string]BazaarProduct `json:"products"`
}

type BazaarProduct struct {
    ProductID   string      `json:"product_id"`
    QuickStatus QuickStatus `json:"quick_status"`
}

type QuickStatus struct {
    ProductID string  `json:"productId"`
    BuyPrice  float64 `json:"buyPrice"`
    SellPrice float64 `json:"sellPrice"`
}

// Recipe structures
type Recipe struct {
    A1    string      `json:"A1"`
    A2    string      `json:"A2"`
    A3    string      `json:"A3"`
    B1    string      `json:"B1"`
    B2    string      `json:"B2"`
    B3    string      `json:"B3"`
    C1    string      `json:"C1"`
    C2    string      `json:"C2"`
    C3    string      `json:"C3"`
    Count interface{} `json:"count"`
}

func (r Recipe) GetCount() int {
    switch v := r.Count.(type) {
    case float64:
        return int(v)
    case int:
        return v
    case string:
        var count int
        fmt.Sscanf(v, "%d", &count)
        return count
    default:
        return 1
    }
}

type Item struct {
    Name   string `json:"name"`
    Recipe Recipe `json:"recipe"`
    Wiki   string `json:"wiki"`
    Rarity string `json:"base_rarity"`
}

type RecipeTree struct {
    ItemID   string
    Quantity int
    Children map[string]*RecipeTree
}

// Structure for Moulberry's lowest bin data
type LowestBinData map[string]float64

// Cache structure
type PriceCache struct {
    bazaarData BazaarResponse
    lowestBins LowestBinData
    recipeTrees   map[string]*RecipeTree
    lastUpdate time.Time
    mu         sync.RWMutex
}

func NewCache() *PriceCache {
    return &PriceCache{
        bazaarData:  BazaarResponse{},
        lowestBins:  make(LowestBinData),
        recipeTrees: make(map[string]*RecipeTree),
        lastUpdate:  time.Now(),
    }
}

type ItemDatabase map[string]Item
type ItemTotals map[string]int

func getPriceFromCache(itemID string) (float64, string) {
    cache.mu.RLock()
    defer cache.mu.RUnlock()

    // Try Bazaar first
    if product, exists := cache.bazaarData.Products[itemID]; exists {
        return product.QuickStatus.BuyPrice, "Bazaar"
    }

    // Try Lowest Bin
    if price, exists := cache.lowestBins[itemID]; exists {
        return price, "Lowest Bin"
    }

    return 0, ""
}

func formatPrice(price float64) string {
	if price >= 1000000000 {
        return fmt.Sprintf("%.2fB", price/1000000000)
    } else if price >= 1000000 {
        return fmt.Sprintf("%.2fM", price/1000000)
    } else if price >= 1000 {
        return fmt.Sprintf("%.2fk", price/1000)
    }
    return fmt.Sprintf("%.2f", price)
}

// Add this function after the Recipe struct methods
func isBaseMaterial(itemID string) bool {
    item, exists := items[itemID]
    if !exists {
        return true // If item doesn't exist in recipes, consider it base material
    }

    // Check if this is a "9-from-1" recipe or has no recipe
    recipe := item.Recipe
    nonEmptySlots := 0
    for _, slot := range []string{recipe.A1, recipe.A2, recipe.A3, recipe.B1, recipe.B2, recipe.B3, recipe.C1, recipe.C2, recipe.C3} {
        if slot != "" {
            nonEmptySlots++
        }
    }
    
    // Consider it a base material if it has no recipe or is a 9-from-1 recipe
    return nonEmptySlots == 0 || (nonEmptySlots == 1 && recipe.GetCount() == 9)
}

// Global variables
var (
    cache      PriceCache
    items      ItemDatabase
    httpClient = &http.Client{
        Timeout: httpTimeout,
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 100,
            IdleConnTimeout:     90 * time.Second,
            DisableCompression:  false,
        },
    }
)

const (
    bazaarURL    = "https://api.hypixel.net/v2/skyblock/bazaar"
    lowestBinURL = "https://moulberry.codes/lowestbin.json"
    cacheTimeout = 5 * time.Minute
    httpTimeout  = 10 * time.Second
)

type fetchResult struct {
    data []byte
    err  error
}

func (c *PriceCache) getOrBuildRecipeTree(itemID string) *RecipeTree {
    c.mu.RLock()
    if tree, exists := c.recipeTrees[itemID]; exists {
        c.mu.RUnlock()
        return cloneRecipeTree(tree, 1)
    }
    c.mu.RUnlock()

    // Build new tree
    visited := make(map[string]bool)
    tree := buildRecipeTree(itemID, 1, visited)

    // Cache it
    c.mu.Lock()
    c.recipeTrees[itemID] = tree
    c.mu.Unlock()

    return cloneRecipeTree(tree, 1)
}

func (c *PriceCache) update() error {
    c.mu.Lock()
    defer c.mu.Unlock()

    startTime := time.Now()

    // Create channels for parallel fetching
    bazaarChan := make(chan fetchResult)
    binsChan := make(chan fetchResult)

    // Fetch Bazaar data concurrently
    go func() {
        req, err := http.NewRequest("GET", bazaarURL, nil)
        if err != nil {
            bazaarChan <- fetchResult{nil, err}
            return
        }
        req.Header.Set("Accept-Encoding", "gzip, deflate")
        
        resp, err := httpClient.Do(req)
        if err != nil {
            bazaarChan <- fetchResult{nil, err}
            return
        }
        defer resp.Body.Close()

        var reader io.ReadCloser
        switch resp.Header.Get("Content-Encoding") {
        case "gzip":
            reader, err = gzip.NewReader(resp.Body)
            if err != nil {
                bazaarChan <- fetchResult{nil, err}
                return
            }
            defer reader.Close()
        default:
            reader = resp.Body
        }

        data, err := io.ReadAll(reader)
        bazaarChan <- fetchResult{data, err}
    }()

    // Fetch Lowest Bin data concurrently
    go func() {
        req, err := http.NewRequest("GET", lowestBinURL, nil)
        if err != nil {
            binsChan <- fetchResult{nil, err}
            return
        }
        req.Header.Set("Accept-Encoding", "gzip, deflate")
        
        resp, err := httpClient.Do(req)
        if err != nil {
            binsChan <- fetchResult{nil, err}
            return
        }
        defer resp.Body.Close()

        var reader io.ReadCloser
        switch resp.Header.Get("Content-Encoding") {
        case "gzip":
            reader, err = gzip.NewReader(resp.Body)
            if err != nil {
                binsChan <- fetchResult{nil, err}
                return
            }
            defer reader.Close()
        default:
            reader = resp.Body
        }

        data, err := io.ReadAll(reader)
        binsChan <- fetchResult{data, err}
    }()

    // Wait for both results
    bazaarResult := <-bazaarChan
    if bazaarResult.err != nil {
        return fmt.Errorf("failed to fetch bazaar data: %v", bazaarResult.err)
    }

    binsResult := <-binsChan
    if binsResult.err != nil {
        return fmt.Errorf("failed to fetch lowest bin data: %v", binsResult.err)
    }

    // Decode data
    if err := json.Unmarshal(bazaarResult.data, &c.bazaarData); err != nil {
        return fmt.Errorf("failed to decode bazaar data: %v", err)
    }

    if err := json.Unmarshal(binsResult.data, &c.lowestBins); err != nil {
        return fmt.Errorf("failed to decode lowest bin data: %v", err)
    }

    // Initialize recipe trees if not already done
    if c.recipeTrees == nil {
        c.recipeTrees = make(map[string]*RecipeTree)
        currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
        log.Printf("[%s] Precomputing recipe trees...", currentTime)
        for itemID := range items {
            visited := make(map[string]bool)
            c.recipeTrees[itemID] = buildRecipeTree(itemID, 1, visited)
        }
        log.Printf("[%s] Recipe tree precomputation complete", currentTime)
    }

    totalDuration := time.Since(startTime)
    currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
    log.Printf("[%s] Cache update timing - Total: %dms", currentTime, totalDuration.Milliseconds())

    c.lastUpdate = time.Now()
    return nil
}

func loadItems() error {
    data, err := ioutil.ReadFile("data.json")
    if err != nil {
        return fmt.Errorf("error reading file: %v", err)
    }

    if err := json.Unmarshal(data, &items); err != nil {
        return fmt.Errorf("error parsing JSON: %v", err)
    }

    // Initialize cache.recipeTrees after loading items
    if cache.recipeTrees == nil {
        cache.mu.Lock()
        cache.recipeTrees = make(map[string]*RecipeTree)
        currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
        log.Printf("[%s] Building initial recipe tree cache for user %s", currentTime, os.Getenv("USER"))
        for itemID := range items {
            visited := make(map[string]bool)
            cache.recipeTrees[itemID] = buildRecipeTree(itemID, 1, visited)
        }
        log.Printf("[%s] Recipe tree cache initialization complete", currentTime)
        cache.mu.Unlock()
    }

    return nil
}

func parseQuantity(input string) (string, int) {
    parts := strings.Split(input, ":")
    if len(parts) != 2 {
        // Check if it contains a hyphen instead
        parts = strings.Split(input, "-")
        if len(parts) != 2 {
            return input, 1
        }
    }
    
    // If we found a hyphen in the ID, convert it back to colon
    if strings.Contains(parts[0], "-") {
        parts[0] = strings.Replace(parts[0], "-", ":", 1)
    }
    
    quantity := 1
    fmt.Sscanf(parts[1], "%d", &quantity)
    return parts[0], quantity
}

func buildRecipeTree(itemID string, quantity int, visited map[string]bool) *RecipeTree {
    // Check if we've already visited this item to prevent infinite recursion
    if visited[itemID] {
        return &RecipeTree{ItemID: itemID, Quantity: quantity}
    }
    visited[itemID] = true

    item, exists := items[itemID]
    if !exists {
        return &RecipeTree{ItemID: itemID, Quantity: quantity}
    }

    recipe := item.Recipe
    nonEmptySlots := 0
    for _, slot := range []string{recipe.A1, recipe.A2, recipe.A3, recipe.B1, recipe.B2, recipe.B3, recipe.C1, recipe.C2, recipe.C3} {
        if slot != "" {
            nonEmptySlots++
        }
    }

    recipeCount := recipe.GetCount()
    if nonEmptySlots == 1 && recipeCount == 9 {
        return &RecipeTree{ItemID: itemID, Quantity: quantity}
    }

    tree := &RecipeTree{
        ItemID:   itemID,
        Quantity: quantity,
        Children: make(map[string]*RecipeTree),
    }

    if recipeCount == 0 {
        recipeCount = 1
    }

    // Calculate how many recipe iterations we need
    // Use ceiling division to round up
    recipesNeeded := (quantity + recipeCount - 1) / recipeCount

    // Allow aggregation at the same level by using itemID as key
    for _, slot := range []string{recipe.A1, recipe.A2, recipe.A3, recipe.B1, recipe.B2, recipe.B3, recipe.C1, recipe.C2, recipe.C3} {
        if slot == "" {
            continue
        }
        ingredientID, ingredientQuantity := parseQuantity(slot)
        // Multiply by recipesNeeded to get total ingredients needed
        totalQuantity := ingredientQuantity * recipesNeeded

        if existing, exists := tree.Children[ingredientID]; exists {
            existing.Quantity += totalQuantity
        } else {
            branchVisited := make(map[string]bool)
            for k, v := range visited {
                branchVisited[k] = v
            }
            child := buildRecipeTree(ingredientID, totalQuantity, branchVisited)
            tree.Children[ingredientID] = child
        }
    }

    return tree
}

func printRecipeTree(tree *RecipeTree, level int, totals ItemTotals, costs map[string]float64) {
    itemName := items[tree.ItemID].Name
    if itemName == "" {
        itemName = tree.ItemID
    }
    
    // Calculate indentation with tree structure
    baseIndent := strings.Repeat("  ", level)
    treePrefix := ""
    if level > 0 {
        treePrefix = "└─"
    }
    displayName := baseIndent + treePrefix + itemName
    
    // Print current item
    if isBaseMaterial(tree.ItemID) {
        totals[tree.ItemID] += tree.Quantity
        price, source := getPriceFromCache(tree.ItemID)
        totalCost := price * float64(tree.Quantity)
        costs[tree.ItemID] = totalCost
        
        fmt.Printf("%-40s │x%-7d", 
            displayName,
            tree.Quantity)
        if price > 0 {
            fmt.Printf(" │ Cost: %-12s (%s ea) from %s", 
                formatPrice(totalCost),
                formatPrice(price),
                source)
        }
        fmt.Println()
        return
    }

    // Print non-base item
    fmt.Printf("%-40s │x%-7d\n", 
        displayName,
        tree.Quantity)

    // Process recipe ingredients
    if item, exists := items[tree.ItemID]; exists {
        recipe := item.Recipe
        recipeCount := recipe.GetCount()
        if recipeCount == 0 {
            recipeCount = 1
        }

        recipesNeeded := (tree.Quantity + recipeCount - 1) / recipeCount

        // Collect ingredients with their quantities
        ingredientMap := make(map[string]int)
        for _, slot := range []string{recipe.A1, recipe.A2, recipe.A3, recipe.B1, recipe.B2, recipe.B3, recipe.C1, recipe.C2, recipe.C3} {
            if slot == "" {
                continue
            }
            ingredientID, qty := parseQuantity(slot)
            ingredientMap[ingredientID] += qty * recipesNeeded
        }

        // Sort ingredients by name
        var ingredients []string
        for id := range ingredientMap {
            ingredients = append(ingredients, id)
        }
        sort.Slice(ingredients, func(i, j int) bool {
            iName := items[ingredients[i]].Name
            jName := items[ingredients[j]].Name
            if iName == "" {
                iName = ingredients[i]
            }
            if jName == "" {
                jName = ingredients[j]
            }
            return iName < jName
        })

        // Print ingredients with proper tree structure
        lastIdx := len(ingredients) - 1
        for idx, id := range ingredients {
            subtree := &RecipeTree{
                ItemID:   id,
                Quantity: ingredientMap[id],
                Children: make(map[string]*RecipeTree),
            }
            
            // Handle last item differently for tree visualization
            if idx == lastIdx {
                printRecipeTree(subtree, level+1, totals, costs)
            } else {
                printRecipeTree(subtree, level+1, totals, costs)
            }
        }
    }
}

func cloneRecipeTree(original *RecipeTree, quantity int) *RecipeTree {
    if original == nil {
        return nil
    }

    clone := &RecipeTree{
        ItemID:   original.ItemID,
        Quantity: original.Quantity * quantity,
        Children: make(map[string]*RecipeTree),
    }

    for childID, child := range original.Children {
        clone.Children[childID] = cloneRecipeTree(child, quantity)
    }

    return clone
}

func printTotals(rootItemID string, totals ItemTotals, costs map[string]float64) {
    startTime := time.Now()
    
    fmt.Println("\n╔════════════════════════════════════════════════════════════════")
    fmt.Println("║ Raw materials needed:")
    fmt.Println("╠════════════════════════════════════════════════════════════════")
    
    baseMatsByName := make(map[string][]string)
    totalCost := 0.0
    
    for itemID := range totals {
        if isBaseMaterial(itemID) {
            itemName := items[itemID].Name
            if itemName == "" {
                itemName = itemID
            }
            baseMatsByName[itemName] = append(baseMatsByName[itemName], itemID)
        }
    }
    
    var sortedNames []string
    for name := range baseMatsByName {
        sortedNames = append(sortedNames, name)
    }
    sort.Strings(sortedNames)

    for _, name := range sortedNames {
        for _, itemID := range baseMatsByName[name] {
            price, source := getPriceFromCache(itemID)
            totalItemCost := price * float64(totals[itemID])
            costPerUnit := price
            totalCost += totalItemCost
            
            if price > 0 {
                fmt.Printf("╠═ %-30s x%-10d │ Cost: %-10s (%.2f ea) from %s\n", 
                    name, totals[itemID], formatPrice(totalItemCost), costPerUnit, source)
            } else {
                fmt.Printf("╠═ %-30s x%-10d │ No price data available\n", 
                    name, totals[itemID])
            }
        }
    }
    
    fetchDuration := time.Since(startTime)

    // Get the recipe count for the final item
    if finalItem, exists := items[rootItemID]; exists {
        recipeCount := finalItem.Recipe.GetCount()
        if recipeCount > 1 {
            totalCost = totalCost / float64(recipeCount)
        }
    }
    
    fmt.Println("╔════════════════════════════════════════════════════════════════")
    fmt.Printf("║ Total crafting cost: %s coins\n", formatPrice(totalCost))
    fmt.Printf("║ Price fetch time: %.2fms\n", float64(fetchDuration.Microseconds())/1000.0)
    fmt.Println("╚════════════════════════════════════════════════════════════════")
}

func main() {
    currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
    log.Printf("[%s] Starting Skyblock Recipe Checker", currentTime)
    log.Printf("User: %s", os.Getenv("USER"))

    if err := loadItems(); err != nil {
        log.Fatalf("Failed to load items: %v", err)
    }

    // Initial cache update
    if err := cache.update(); err != nil {
        log.Fatalf("Failed to initialize cache: %v", err)
    }

    // Precompute all recipe trees
    log.Printf("Precomputing recipe trees...")
    for itemID := range items {
        cache.getOrBuildRecipeTree(itemID)
    }
    log.Printf("Recipe tree precomputation complete")

    // Start periodic cache updates
    go func() {
        ticker := time.NewTicker(cacheTimeout)
        for range ticker.C {
            if err := cache.update(); err != nil {
                currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
                log.Printf("[%s] Cache update failed: %v", currentTime, err)
            }
        }
    }()

    reader := bufio.NewReader(os.Stdin)
    fmt.Println("\nEnter item ID to look up (or 'quit' to exit):")

    for {
        fmt.Print("> ")
        itemID, _ := reader.ReadString('\n')
        itemID = strings.TrimSpace(itemID)

        if itemID == "quit" {
            break
        }

        if _, exists := items[itemID]; !exists {
            fmt.Println("Item not found!")
            continue
        }

        startTime := time.Now()
        tree := cache.getOrBuildRecipeTree(itemID)
        totals := make(ItemTotals)
        costs := make(map[string]float64)
        
        currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
        fmt.Printf("\n[%s] Recipe tree for %s:\n", currentTime, items[itemID].Name)
        printRecipeTree(tree, 0, totals, costs)
        printTotals(itemID, totals, costs)  // This is the updated line
        fmt.Printf("Total processing time: %.2fms\n\n", float64(time.Since(startTime).Microseconds())/1000.0)
    }
}