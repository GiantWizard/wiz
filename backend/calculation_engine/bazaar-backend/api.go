package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time" // Import time
)

// --- API Structs ---
// Ensure these structs match your needs and JSON structure from Hypixel API
type OrderSummary struct {
	PricePerUnit float64 `json:"pricePerUnit"`
	Amount       int     `json:"amount"` // Example: Add other fields if needed
	Orders       int     `json:"orders"` // Example: Add other fields if needed
}

type QuickStatus struct {
	BuyMovingWeek  float64 `json:"buyMovingWeek"`
	SellMovingWeek float64 `json:"sellMovingWeek"` // Example: Add other fields if needed
	BuyPrice       float64 `json:"buyPrice"`       // Example: Add other fields if needed
	SellPrice      float64 `json:"sellPrice"`      // Example: Add other fields if needed
	// Add other fields like SellVolume, BuyVolume, SellOrders, BuyOrders if present and needed
}

type HypixelProduct struct {
	ProductID   string         `json:"product_id"` // Explicitly map if present in JSON, otherwise handled by map key
	SellSummary []OrderSummary `json:"sell_summary"`
	BuySummary  []OrderSummary `json:"buy_summary"`
	QuickStatus QuickStatus    `json:"quick_status"`
}

type HypixelAPIResponse struct {
	Success     bool                      `json:"success"`
	LastUpdated int64                     `json:"lastUpdated"` // Unix timestamp (milliseconds)
	Products    map[string]HypixelProduct `json:"products"`    // Key is ProductID (e.g., "ENCHANTED_DIAMOND")
}

// --- Global variables for API caching ---
var (
	apiResponseCache *HypixelAPIResponse // Holds the latest successful response
	fetchApiOnce     sync.Once           // Ensures initial fetch runs only once
	apiFetchErr      error               // Stores the error from the last fetch attempt
	apiCacheMutex    sync.RWMutex        // Read/Write mutex for thread-safe access to cache and error
	lastAPIFetchTime time.Time           // Track last successful fetch time
)

// --- Core API Fetching Logic ---

// fetchBazaarData performs the actual HTTP request and parsing.
// It updates the global cache variables under lock. Returns error on failure.
// It should only be called internally, ideally via getApiResponse (initial) or forceRefreshAPIData (manual).
func fetchBazaarData() error {
	var fetchErr error // Local error variable for this specific fetch attempt

	dlog("Fetching live Hypixel Bazaar data...") // Assumes dlog is defined in utils.go
	url := "https://api.hypixel.net/v2/skyblock/bazaar"

	// Use a shared client or configure one here with timeout
	client := http.Client{
		Timeout: 15 * time.Second, // Example: 15-second timeout
	}
	resp, err := client.Get(url)
	if err != nil {
		fetchErr = fmt.Errorf("fetch API GET (%s): %w", url, err)
		log.Printf("ERROR: %v", fetchErr)
		// Update global error state under lock immediately
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr // Store the error globally
		apiCacheMutex.Unlock()
		return fetchErr // Return the specific error for this attempt
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := ""
		if readErr == nil {
			// Limit body size in error message for readability
			maxBody := 500
			if len(bodyBytes) > maxBody {
				bodyBytes = append(bodyBytes[:maxBody], []byte("... (truncated)")...)
			}
			bodyStr = string(bodyBytes)
		} else {
			bodyStr = fmt.Sprintf("(failed to read response body: %v)", readErr)
		}
		fetchErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, bodyStr)
		log.Printf("ERROR: %v", fetchErr)
		// Update global error state under lock
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	// Decode the JSON response
	var apiResp HypixelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		fetchErr = fmt.Errorf("parse API JSON: %w", err)
		log.Printf("ERROR: %v", fetchErr)
		// Update global error state under lock
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	// Check the 'success' field in the response
	if !apiResp.Success {
		fetchErr = fmt.Errorf("API success field reported: false")
		log.Printf("ERROR: %v", fetchErr)
		// Update global error state under lock
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	// --- Update Cache under Write Lock ---
	// If we reached here, the fetch was successful
	apiCacheMutex.Lock()
	defer apiCacheMutex.Unlock()

	// Pre-process product IDs if needed (e.g., ensure they match normalized format)
	// For now, assume keys are usable as is or normalization happens during lookup
	for id, product := range apiResp.Products {
		// If product_id field exists and is different from map key, prioritize map key?
		// Or ensure consistency? For now, trust map key. Add logging if needed.
		if product.ProductID != "" && product.ProductID != id {
			dlog("API Cache: Product ID mismatch for key '%s': product_id field is '%s'", id, product.ProductID)
		}
		// If product_id is empty, maybe fill it from the key?
		// tempProduct := product // Avoid modifying loop variable directly
		// tempProduct.ProductID = id
		// apiResp.Products[id] = tempProduct
	}

	apiResponseCache = &apiResp   // Update cache with the new data
	apiFetchErr = nil             // Reset global error state on successful fetch
	lastAPIFetchTime = time.Now() // Update timestamp of successful fetch

	dlog("Hypixel Bazaar data fetched and cached successfully at %s.", lastAPIFetchTime.Format(time.RFC3339))
	return nil // Success for this fetch attempt
}

// --- Functions to Access/Refresh Cache ---

// getApiResponse ensures data is fetched ONCE on the first call (e.g., for initial load)
// and returns the cached response. Subsequent calls return the cache without re-fetching.
// It checks the global error state set by fetch attempts.
func getApiResponse() (*HypixelAPIResponse, error) {
	fetchApiOnce.Do(func() {
		// This block runs only once for the lifetime of the application.
		// Perform the initial fetch attempt. fetchBazaarData handles setting apiFetchErr internally.
		_ = fetchBazaarData() // We don't need the error here, check global apiFetchErr below
	})

	// Acquire read lock to check cache and global error state
	apiCacheMutex.RLock()
	defer apiCacheMutex.RUnlock()

	// Check global error state AFTER sync.Once has potentially run
	if apiFetchErr != nil {
		// Return the stored error from the last failed fetch attempt (could be initial or later)
		return nil, apiFetchErr
	}

	// Check if cache is populated (it might be nil if initial fetch failed silently before error was set,
	// or if API returned success:true but empty products, though fetchBazaarData should prevent nil cache on success)
	if apiResponseCache == nil {
		// This indicates a problem - fetch ran, no error stored, but cache is nil.
		// Should not happen if fetchBazaarData works correctly.
		// Return a generic error indicating data is unavailable.
		return nil, fmt.Errorf("API data unavailable: cache is nil after initialization attempt")
	}

	// Cache exists and no error is stored, return the cached data
	return apiResponseCache, nil
}

// forceRefreshAPIData ALWAYS attempts to fetch new data and update the cache.
// This is typically used by background refresh loops or manual triggers.
// It returns the latest cache content (even if stale if the refresh fails) AND the error status of the refresh attempt.
func forceRefreshAPIData() (*HypixelAPIResponse, error) {
	err := fetchBazaarData() // Attempt to fetch and update cache (handles global state internally)

	// Regardless of the refresh attempt's success or failure, return the current state of the cache.
	// Acquire read lock to safely access the potentially updated cache.
	apiCacheMutex.RLock()
	currentCacheState := apiResponseCache // Get the pointer to the current cache
	currentErrorState := apiFetchErr      // Get the current global error state
	apiCacheMutex.RUnlock()

	if err != nil {
		// The refresh attempt failed (err is the specific error from this attempt).
		// Log a warning indicating potentially stale data is being returned.
		lastFetchStr := "never"
		if !lastAPIFetchTime.IsZero() {
			lastFetchStr = lastAPIFetchTime.Format(time.RFC3339)
		}
		log.Printf("WARN: Failed to refresh API data: %v. Using potentially stale data (last successful fetch: %s).", err, lastFetchStr)
		// Return the current cache state (which might be old or nil) and the error from THIS attempt.
		return currentCacheState, err
	}

	// Fetch succeeded, cache was updated by fetchBazaarData.
	// The global apiFetchErr should be nil now.
	// Return the up-to-date cache and nil error for this attempt.
	// Note: currentCacheState pointer now points to the newly fetched data.
	// We check currentErrorState just in case something went wrong between fetch and RLock, though unlikely.
	if currentErrorState != nil {
		log.Printf("WARN: API fetch succeeded, but global error state was non-nil briefly: %v", currentErrorState)
		// Still return success based on the fetch attempt's outcome
		return currentCacheState, nil
	}

	return currentCacheState, nil // Return updated cache and nil error
}
