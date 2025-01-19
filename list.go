package main

import (
    "context"
    "bytes"
    "github.com/andybalholm/brotli"
    "crypto/tls"
    "net"
    "bufio"
    "compress/gzip"
    "encoding/json"
    "fmt"
    "io"
    "io/ioutil"
    "net/http"
    "os"
    "sort"
    "strings"
    "sync"
    "time"
    "math"
)

// Structures for Hypixel Bazaar API
type BazaarResponse struct {
    Success     bool                     `json:"success"`
    LastUpdated int64                    `json:"lastUpdated"`
    Products    map[string]BazaarProduct `json:"products"`
}

type BazaarProduct struct {
    ProductID    string         `json:"product_id"`
    SellSummary  []OrderSummary `json:"sell_summary"`
    BuySummary   []OrderSummary `json:"buy_summary"`
    QuickStatus  QuickStatus    `json:"quick_status"`
}

type QuickStatus struct {
    ProductID       string  `json:"productId"`
    SellPrice       float64 `json:"sellPrice"`
    SellVolume      int     `json:"sellVolume"`
    SellMovingWeek  int     `json:"sellMovingWeek"`
    SellOrders      int     `json:"sellOrders"`
    BuyPrice        float64 `json:"buyPrice"`
    BuyVolume       int     `json:"buyVolume"`
    BuyMovingWeek   int     `json:"buyMovingWeek"`
    BuyOrders       int     `json:"buyOrders"`
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

type PriceMethod struct {
    Price  float64
    Method string // "buy order" or "instabuy"
}

type ItemDatabase map[string]Item
type ItemTotals map[string]int


type OrderSummary struct {
    Amount       int     `json:"amount"`
    PricePerUnit float64 `json:"pricePerUnit"`
    Orders       int     `json:"orders"`
}

func getPriceFromCache(itemID string) (float64, string, string) {
    cache.mu.RLock()
    defer cache.mu.RUnlock()

    // Try Bazaar first
    if product, exists := cache.bazaarData.Products[itemID]; exists {
        priceMethod := determineBuyMethod(product.QuickStatus)
        return priceMethod.Price, priceMethod.Method, "Bazaar"
    }

    // Try Lowest Bin
    if price, exists := cache.lowestBins[itemID]; exists {
        return price, "buy", "Lowest Bin"
    }

    return 0, "", ""
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

type PerformanceMetrics struct {
    ItemLoadTime         time.Duration
    FirstAPICallTime     time.Duration
    SecondAPICallTime    time.Duration
    CacheInitTime        time.Duration
    RecipeTreeBuildTime  time.Duration
    TotalProcessingTime  time.Duration
    LastUpdate          time.Time
    mu                  sync.RWMutex
}

func NewPerformanceMetrics() *PerformanceMetrics {
    return &PerformanceMetrics{
        LastUpdate: time.Now(),
    }
}

func (pm *PerformanceMetrics) Track(operation string, duration time.Duration) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    switch operation {
    case "item_load":
        pm.ItemLoadTime = duration
    case "bazaar_api":
        pm.FirstAPICallTime = duration
    case "bins_api":
        pm.SecondAPICallTime = duration
    case "cache_init":
        pm.CacheInitTime = duration
    case "recipe_tree":
        pm.RecipeTreeBuildTime = duration
    }
    pm.TotalProcessingTime = pm.ItemLoadTime + pm.FirstAPICallTime + 
        pm.SecondAPICallTime + pm.CacheInitTime + pm.RecipeTreeBuildTime
}

func (pm *PerformanceMetrics) PrintMetrics() {
    pm.mu.RLock()
    defer pm.mu.RUnlock()

    fmt.Println("\n╔════════════════════ Performance Metrics ════════════════════")
    fmt.Printf("║ Items Load Time:        %8.2fms\n", float64(pm.ItemLoadTime.Microseconds())/1000.0)
    fmt.Printf("║ Bazaar API Call:        %8.2fms\n", float64(pm.FirstAPICallTime.Microseconds())/1000.0)
    fmt.Printf("║ Lowest Bins API Call:   %8.2fms\n", float64(pm.SecondAPICallTime.Microseconds())/1000.0)
    fmt.Printf("║ Cache Initialization:   %8.2fms\n", float64(pm.CacheInitTime.Microseconds())/1000.0)
    fmt.Printf("║ Recipe Tree Building:   %8.2fms\n", float64(pm.RecipeTreeBuildTime.Microseconds())/1000.0)
    fmt.Printf("║ Total Processing Time:  %8.2fms\n", float64(pm.TotalProcessingTime.Microseconds())/1000.0)
    fmt.Println("╚═══════════════════════════════════════════════════════════\n")
}

type MarketMetrics struct {
    SellPrice      float64
    BuyPrice       float64
    SellVolume     int
    BuyVolume      int
    SellMovingWeek int
    BuyMovingWeek  int
    SellOrders     int
    BuyOrders      int
}

func calculateMarketPressure(metrics MarketMetrics) float64 {
    // Calculate volume pressure (-1 to 1 range)
    // Positive means more selling pressure, negative means more buying pressure
    totalVolume := float64(metrics.SellMovingWeek + metrics.BuyMovingWeek)
    if totalVolume == 0 {
        return 0
    }
    
    volumePressure := (float64(metrics.SellMovingWeek) - float64(metrics.BuyMovingWeek)) / totalVolume
    
    // Calculate price spread pressure (0 to 1 range)
    // How far apart are buy/sell prices relative to the average price
    avgPrice := (metrics.BuyPrice + metrics.SellPrice) / 2
    if avgPrice == 0 {
        return 0
    }
    
    spreadPressure := math.Abs(metrics.BuyPrice - metrics.SellPrice) / avgPrice
    
    // Combine pressures - spreadPressure amplifies the effect of volumePressure
    return volumePressure * (1 + spreadPressure)
}

func calculateIdealPrice(metrics MarketMetrics) float64 {
    // Base price starts at weighted average
    totalVolume := float64(metrics.SellMovingWeek + metrics.BuyMovingWeek)
    if totalVolume == 0 {
        return (metrics.SellPrice + metrics.BuyPrice) / 2
    }

    // Calculate market pressure (-1 to 1 range)
    pressure := calculateMarketPressure(metrics)
    
    // Calculate dynamic spread threshold based on volume ratio
    volumeRatio := math.Abs(float64(metrics.SellMovingWeek-metrics.BuyMovingWeek)) / totalVolume
    
    // Price adjustment factor scales with pressure and volume
    adjustment := pressure * volumeRatio
    
    // Calculate base price
    basePrice := metrics.SellPrice
    if pressure < 0 {
        // Negative pressure (more buying) suggests using buyPrice as base
        basePrice = metrics.BuyPrice
    }
    
    // Apply dynamic adjustment
    adjustedPrice := basePrice * (1 - adjustment)
    
    // Ensure price stays within reasonable bounds
    minPrice := math.Min(metrics.SellPrice, metrics.BuyPrice)
    maxPrice := math.Max(metrics.SellPrice, metrics.BuyPrice)
    
    return math.Max(minPrice, math.Min(maxPrice, adjustedPrice))
}

func determineBuyMethod(qs QuickStatus) PriceMethod {
    metrics := MarketMetrics{
        SellPrice:      qs.SellPrice,
        BuyPrice:       qs.BuyPrice,
        SellMovingWeek: qs.SellMovingWeek,
        BuyMovingWeek:  qs.BuyMovingWeek,
    }
    
    pressure := calculateMarketPressure(metrics)
    idealPrice := calculateIdealPrice(metrics)
    
    // If pressure is strongly negative, suggest instabuy
    if pressure < 0 {
        return PriceMethod{
            Price:  idealPrice,
            Method: "instabuy",
        }
    }
    
    // If pressure is positive, suggest buy order
    return PriceMethod{
            Price:  idealPrice,
            Method: "buy order",
    }
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

var (
    cache PriceCache
    items ItemDatabase
    stats = &metrics{
        updateTimes: make([]time.Duration, 0, 100),
    }
    pool = &http.Client{
        Timeout: httpTimeout,
        Transport: &http.Transport{
            ForceAttemptHTTP2:     true,
            MaxIdleConns:          poolSize,
            MaxIdleConnsPerHost:   poolSize,
            MaxConnsPerHost:       poolSize,
            IdleConnTimeout:       90 * time.Second,
            TLSHandshakeTimeout:   5 * time.Second,
            ResponseHeaderTimeout: 5 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
            WriteBufferSize:       64 * 1024,
            ReadBufferSize:        64 * 1024,
            DisableKeepAlives:     false,
            TLSClientConfig: &tls.Config{
                MinVersion: tls.VersionTLS13,
            },
            DialContext: (&net.Dialer{
                Timeout:   5 * time.Second,
                KeepAlive: 30 * time.Second,
                DualStack: true,
            }).DialContext,
        },
    }
)

type brotliReadCloser struct {
    br *brotli.Reader
    rc io.ReadCloser
}

func (b *brotliReadCloser) Read(p []byte) (n int, err error) {
    return b.br.Read(p)
}

func (b *brotliReadCloser) Close() error {
    return b.rc.Close()
}

func newBrotliReadCloser(r io.ReadCloser) io.ReadCloser {
    return &brotliReadCloser{
        br: brotli.NewReader(r),
        rc: r,
    }
}

// Add these constants
const (
    bazaarURL    = "https://api.hypixel.net/v2/skyblock/bazaar"
    lowestBinURL = "https://moulberry.codes/lowestbin.json"
    cacheTimeout = 5 * time.Minute
    httpTimeout  = 10 * time.Second
    maxRetries   = 3
    backoffBase  = 500 * time.Millisecond
    poolSize     = 100
    maxCacheSize = 1000
)

type fetchResult struct {
    data []byte
    err  error
}

func fetchWithRetry(url string) ([]byte, error) {
    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        if attempt > 0 {
            backoff := backoffBase * time.Duration(1<<uint(attempt-1))
            time.Sleep(backoff)
        }

        ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            cancel()
            lastErr = err
            continue
        }

        req.Header.Set("Accept-Encoding", "gzip, br")
        req.Header.Set("Connection", "keep-alive")
        req.Header.Set("Accept", "application/json")

        // Use pool directly since it's an *http.Client
        resp, err := pool.Do(req)
        if err != nil {
            cancel()
            lastErr = err
            continue
        }

        data, err := handleResponse(resp)
        cancel()
        if err != nil {
            lastErr = err
            continue
        }

        return data, nil
    }
    return nil, fmt.Errorf("all retries failed: %v", lastErr)
}

type metrics struct {
    mu            sync.RWMutex
    updateTimes   []time.Duration
    failedUpdates int
    lastError     error
}

func (m *metrics) recordUpdate(d time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.updateTimes = append(m.updateTimes, d)
    if len(m.updateTimes) > 100 {
        m.updateTimes = m.updateTimes[1:]
    }
}

func (m *metrics) getAverageTime() time.Duration {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    if len(m.updateTimes) == 0 {
        return 0
    }
    
    var total time.Duration
    for _, t := range m.updateTimes {
        total += t
    }
    return total / time.Duration(len(m.updateTimes))
}

func handleResponse(resp *http.Response) ([]byte, error) {
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    var reader io.ReadCloser
    switch resp.Header.Get("Content-Encoding") {
    case "gzip":
        var err error
        reader, err = gzip.NewReader(resp.Body)
        if err != nil {
            return nil, err
        }
        defer reader.Close()
    case "br":
        reader = newBrotliReadCloser(resp.Body)
        defer reader.Close()
    default:
        reader = resp.Body
    }

    // Use a buffered reader with a reasonable size
    return io.ReadAll(bufio.NewReaderSize(reader, 64*1024))
}

type apiResponse struct {
    data     []byte
    err      error
    name     string
    duration time.Duration
}

func (c *PriceCache) getIdealPrice(itemID string) float64 {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    if product, exists := c.bazaarData.Products[itemID]; exists {
        metrics := MarketMetrics{
            SellPrice:      product.QuickStatus.SellPrice,
            BuyPrice:       product.QuickStatus.BuyPrice,
            SellVolume:     product.QuickStatus.SellVolume,
            BuyVolume:      product.QuickStatus.BuyVolume,
            SellMovingWeek: product.QuickStatus.SellMovingWeek,
            BuyMovingWeek:  product.QuickStatus.BuyMovingWeek,
            SellOrders:     product.QuickStatus.SellOrders,
            BuyOrders:      product.QuickStatus.BuyOrders,
        }
        return calculateIdealPrice(metrics)
    }
    
    return 0
}

func (c *PriceCache) getOrBuildRecipeTree(itemID string) *RecipeTree {
    c.mu.RLock()
    if tree, exists := c.recipeTrees[itemID]; exists {
        c.mu.RUnlock()
        return cloneRecipeTree(tree, 1)
    }
    c.mu.RUnlock()

    // Build new tree without holding the lock
    visited := make(map[string]bool)
    tree := buildRecipeTree(itemID, 1, visited)

    // Try to store in cache, but don't block if another routine beat us to it
    c.mu.Lock()
    if existing, exists := c.recipeTrees[itemID]; exists {
        c.mu.Unlock()
        return cloneRecipeTree(existing, 1)
    }
    c.recipeTrees[itemID] = tree
    c.mu.Unlock()

    return cloneRecipeTree(tree, 1)
}

type decodedResponse struct {
    bazaarData *BazaarResponse
    binsData   LowestBinData
    err        error
    duration   time.Duration
}

func (c *PriceCache) update() error {
    startTime := time.Now()

    // Create channels for both API and decode responses
    apiChan := make(chan apiResponse, 2)
    decodeChan := make(chan decodedResponse, 2)
    var wg sync.WaitGroup
    wg.Add(2)

    // Launch parallel API fetches with immediate decoding
    go func() {
        defer wg.Done()
        fetchStart := time.Now()
        data, err := fetchWithRetry(bazaarURL)
        fetchDuration := time.Since(fetchStart)

        apiChan <- apiResponse{
            data:     data,
            err:     err,
            name:    "Bazaar",
            duration: fetchDuration,
        }

        if err == nil {
            decodeStart := time.Now()
            bazaarResp := &BazaarResponse{
                Products: make(map[string]BazaarProduct, 1500),
            }
            
            decoder := json.NewDecoder(bytes.NewReader(data))
            decoder.UseNumber()
            decodeErr := decoder.Decode(bazaarResp)
            
            decodeChan <- decodedResponse{
                bazaarData: bazaarResp,
                err:       decodeErr,
                duration:  time.Since(decodeStart),
            }
        }
    }()

    go func() {
        defer wg.Done()
        fetchStart := time.Now()
        data, err := fetchWithRetry(lowestBinURL)
        fetchDuration := time.Since(fetchStart)

        apiChan <- apiResponse{
            data:     data,
            err:     err,
            name:    "Lowest Bins",
            duration: fetchDuration,
        }

        if err == nil {
            decodeStart := time.Now()
            lowestBins := make(LowestBinData, 10000)
            
            decoder := json.NewDecoder(bytes.NewReader(data))
            decoder.UseNumber()
            decodeErr := decoder.Decode(&lowestBins)
            
            decodeChan <- decodedResponse{
                binsData: lowestBins,
                err:      decodeErr,
                duration: time.Since(decodeStart),
            }
        }
    }()

    // Process API responses
    var (
        bazaarFetchDuration, binsFetchDuration time.Duration
        bazaarDecodeDuration, binsDecodeDuration time.Duration
        bazaarResp *BazaarResponse
        lowestBins LowestBinData
        fetchErr error
    )

    // Collect API responses
    for i := 0; i < 2; i++ {
        resp := <-apiChan
        if resp.err != nil {
            fetchErr = resp.err
            continue
        }
        switch resp.name {
        case "Bazaar":
            bazaarFetchDuration = resp.duration
        case "Lowest Bins":
            binsFetchDuration = resp.duration
        }
    }

    if fetchErr != nil {
        return fmt.Errorf("API fetch failed: %v", fetchErr)
    }

    // Collect decoded responses
    for i := 0; i < 2; i++ {
        resp := <-decodeChan
        if resp.err != nil {
            return fmt.Errorf("decode failed: %v", resp.err)
        }
        if resp.bazaarData != nil {
            bazaarResp = resp.bazaarData
            bazaarDecodeDuration = resp.duration
        } else {
            lowestBins = resp.binsData
            binsDecodeDuration = resp.duration
        }
    }

    // Update cache
    updateStart := time.Now()
    c.mu.Lock()
    c.bazaarData = *bazaarResp
    c.lowestBins = lowestBins
    c.lastUpdate = time.Now()
    c.mu.Unlock()
    updateDuration := time.Since(updateStart)

    totalDuration := time.Since(startTime)

    // Print timing summary
    fmt.Println("\n╔════════════════════ Cache Update Summary ═══════════════════════")
    fmt.Printf("║ User:                  %s\n", os.Getenv("USER"))
    fmt.Printf("║ Bazaar Fetch:          %8dms\n", bazaarFetchDuration.Milliseconds())
    fmt.Printf("║ Bazaar Decode:         %8dms\n", bazaarDecodeDuration.Milliseconds())
    fmt.Printf("║ Bins Fetch:            %8dms\n", binsFetchDuration.Milliseconds())
    fmt.Printf("║ Bins Decode:           %8dms\n", binsDecodeDuration.Milliseconds())
    fmt.Printf("║ Cache Update:          %8dms\n", updateDuration.Milliseconds())
    fmt.Printf("║ Total Time:            %8dms\n", totalDuration.Milliseconds())
    fmt.Printf("║ Items Loaded:          %8d Bazaar, %d Bins\n", 
        len(bazaarResp.Products), len(lowestBins))
    fmt.Println("╚═══════════════════════════════════════════════════════════════\n")

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
        for itemID := range items {
            visited := make(map[string]bool)
            cache.recipeTrees[itemID] = buildRecipeTree(itemID, 1, visited)
        }
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
        price, method, source := getPriceFromCache(tree.ItemID)
        totalCost := price * float64(tree.Quantity)
        costs[tree.ItemID] = totalCost
        
        fmt.Printf("%-40s │x%-7d", 
            displayName,
            tree.Quantity)
        if price > 0 {
            fmt.Printf(" │ Cost: %-12s (%.2f ea - %s) from %s", 
                formatPrice(totalCost),
                price,
                method,
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
            price, method, source := getPriceFromCache(itemID)
            totalItemCost := price * float64(totals[itemID])
            costPerUnit := price
            totalCost += totalItemCost
            
            if price > 0 {
                fmt.Printf("╠═ %-30s x%-10d │ Cost: %-10s (%.2f ea - %s) from %s\n", 
                    name, totals[itemID], formatPrice(totalItemCost), costPerUnit, method, source)
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

var perfMetrics = NewPerformanceMetrics()

func initialize() error {
    
    // Initialize cache and database
    cache = *NewCache()
    items = make(ItemDatabase)

    // Load items with retry
    itemLoadStart := time.Now()
    var loadErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        if err := loadItems(); err != nil {
            loadErr = err
            time.Sleep(backoffBase * time.Duration(1<<uint(attempt)))
            continue
        }
        loadErr = nil
        break
    }
    perfMetrics.Track("item_load", time.Since(itemLoadStart))
    
    if loadErr != nil {
        return fmt.Errorf("failed to load items after %d attempts: %v", maxRetries, loadErr)
    }

    // Initial cache update with retry - Track API calls separately
    bazaarStart := time.Now()
    bazaarData, err := fetchWithRetry(bazaarURL)
    perfMetrics.Track("bazaar_api", time.Since(bazaarStart))
    if err != nil {
        return fmt.Errorf("failed to fetch bazaar data: %v", err)
    }

    binsStart := time.Now()
    binsData, err := fetchWithRetry(lowestBinURL)
    perfMetrics.Track("bins_api", time.Since(binsStart))
    if err != nil {
        return fmt.Errorf("failed to fetch lowest bin data: %v", err)
    }

    // Cache initialization
    cacheStart := time.Now()
    if err := initializeCache(bazaarData, binsData); err != nil {
        return fmt.Errorf("failed to initialize cache: %v", err)
    }
    perfMetrics.Track("cache_init", time.Since(cacheStart))

    perfMetrics.PrintMetrics()
    return nil
}

func initializeCache(bazaarData, binsData []byte) error {
    // Parse the data
    var bazaarResp BazaarResponse
    if err := json.NewDecoder(bytes.NewReader(bazaarData)).Decode(&bazaarResp); err != nil {
        return fmt.Errorf("failed to decode bazaar data: %v", err)
    }

    var lowestBins LowestBinData
    if err := json.NewDecoder(bytes.NewReader(binsData)).Decode(&lowestBins); err != nil {
        return fmt.Errorf("failed to decode lowest bin data: %v", err)
    }

    // Update cache with the parsed data
    cache.mu.Lock()
    defer cache.mu.Unlock()
    
    cache.bazaarData = bazaarResp
    cache.lowestBins = lowestBins
    cache.lastUpdate = time.Now()

    return nil
}

// Add this to the main function where you process recipe trees
func processRecipeTree(itemID string) {
    startTime := time.Now()
    fmt.Printf("\n╔════════════════════ Process Started ════════════════════")
    fmt.Printf("\n║ Time (UTC):           %s", time.Now().UTC().Format("2006-01-02 15:04:05"))
    fmt.Printf("\n║ User:                 %s", os.Getenv("USER"))
    fmt.Printf("\n║ Item:                 %s", items[itemID].Name)
    fmt.Println("\n╚════════════════════════════════════════════════════════")
    // Recipe Tree Processing Phase
    treeStart := time.Now()
    tree := cache.getOrBuildRecipeTree(itemID)
    treeBuildTime := time.Since(treeStart)

    // Cost Calculation Phase
    totalsStart := time.Now()
    totals := make(ItemTotals)
    costs := make(map[string]float64)

    fmt.Println("\n╔════════════════════ Recipe Tree ════════════════════")
    printRecipeTree(tree, 0, totals, costs)
    treeProcessTime := time.Since(totalsStart)

    // Results Processing Phase
    resultsStart := time.Now()
    printTotals(itemID, totals, costs)
    resultsTime := time.Since(resultsStart)

    // Final Timing Summary
    totalTime := time.Since(startTime)
    fmt.Println("\n╔════════════════════ Processing Times ════════════════════")
    fmt.Printf("║ Tree Building:         %8.2fms\n", float64(treeBuildTime.Microseconds())/1000.0)
    fmt.Printf("║ Tree Processing:       %8.2fms\n", float64(treeProcessTime.Microseconds())/1000.0)
    fmt.Printf("║ Results Processing:    %8.2fms\n", float64(resultsTime.Microseconds())/1000.0)
    fmt.Printf("║ Total Time:           %8.2fms\n", float64(totalTime.Microseconds())/1000.0)
    fmt.Println("╚═════════════════════════════════════════════════════════")
}

// Helper function to find all base materials in a recipe tree
func findBaseMaterials(tree *RecipeTree, materials map[string]bool) {
    if tree == nil {
        return
    }

    if isBaseMaterial(tree.ItemID) {
        materials[tree.ItemID] = true
        return
    }

    for _, child := range tree.Children {
        findBaseMaterials(child, materials)
    }
}

// Helper function to format price differences
func formatPriceDiff(price float64) string {
    if price > 0 {
        return fmt.Sprintf("+%s", formatPrice(price))
    }
    return formatPrice(price)
}

func main() {
    // Initialize global variables
    cache = *NewCache()
    items = make(ItemDatabase)

    if err := loadItems(); err != nil {
        fmt.Printf("Failed to load items: %v\n", err)
        os.Exit(1)
    }

    // Initial cache update
    if err := cache.update(); err != nil {
        fmt.Printf("Failed to initialize cache: %v\n", err)
        os.Exit(1)
    }

    // Start periodic cache updates
    go func() {
        ticker := time.NewTicker(cacheTimeout)
        defer ticker.Stop()
        for range ticker.C {
            cache.update()
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

        processRecipeTree(itemID)
    }
}