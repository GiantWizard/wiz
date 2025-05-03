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
// Ensure these structs match your needs and JSON structure
type OrderSummary struct {
	PricePerUnit float64 `json:"pricePerUnit"`
	// Add other fields like Amount, Orders if needed
}

type QuickStatus struct {
	BuyMovingWeek float64 `json:"buyMovingWeek"`
	// Add other fields like SellMovingWeek, BuyPrice, SellPrice if needed
}

type HypixelProduct struct {
	ProductID   string         `json:"-"` // Often the map key, not in product JSON itself
	SellSummary []OrderSummary `json:"sell_summary"`
	BuySummary  []OrderSummary `json:"buy_summary"`
	QuickStatus QuickStatus    `json:"quick_status"`
}

type HypixelAPIResponse struct {
	Success     bool                      `json:"success"`
	LastUpdated int64                     `json:"lastUpdated"`
	Products    map[string]HypixelProduct `json:"products"` // Key is ProductID
}

// --- Global variables for API caching ---
var (
	apiResponseCache *HypixelAPIResponse
	fetchApiOnce     sync.Once
	apiFetchErr      error
	apiCacheMutex    sync.RWMutex // Use RWMutex for better concurrent reads
	lastAPIFetchTime time.Time    // Track last fetch time
)

// --- Core API Fetching Logic ---
// fetchBazaarData performs the actual HTTP request and parsing.
// It updates the global cache variables under lock. Returns error on failure.
func fetchBazaarData() error {
	var fetchErr error // Local error variable for this fetch attempt

	// Assumes dlog is defined in utils.go
	dlog("Fetching live Hypixel Bazaar data...")
	url := "https://api.hypixel.net/v2/skyblock/bazaar"

	// Consider adding a timeout to the HTTP client
	client := http.Client{
		Timeout: 15 * time.Second, // Example timeout
	}
	resp, err := client.Get(url)
	if err != nil {
		fetchErr = fmt.Errorf("fetch API GET (%s): %w", url, err)
		log.Printf("ERROR: %v", fetchErr)
		return fetchErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Limit body size in error message
		maxBody := 500
		if len(bodyBytes) > maxBody {
			bodyBytes = append(bodyBytes[:maxBody], []byte("...")...)
		}
		fetchErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
		log.Printf("ERROR: %v", fetchErr)
		return fetchErr
	}

	var apiResp HypixelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		fetchErr = fmt.Errorf("parse API JSON: %w", err)
		log.Printf("ERROR: %v", fetchErr)
		return fetchErr
	}

	if !apiResp.Success {
		fetchErr = fmt.Errorf("API success field reported: false")
		log.Printf("ERROR: %v", fetchErr)
		return fetchErr
	}

	// --- Update Cache under Write Lock ---
	apiCacheMutex.Lock()
	defer apiCacheMutex.Unlock()

	apiResponseCache = &apiResp   // Update cache
	apiFetchErr = nil             // Reset global error state on successful fetch
	lastAPIFetchTime = time.Now() // Update timestamp

	// Assumes dlog is defined in utils.go
	dlog("Hypixel Bazaar data fetched and cached successfully at %s.", lastAPIFetchTime.Format(time.RFC3339))
	return nil // Success
}

// --- Functions to Access/Refresh Cache ---

// getApiResponse ensures data is fetched ONCE (useful for initial load/single-item mode)
// and returns the cached response.
func getApiResponse() (*HypixelAPIResponse, error) {
	fetchApiOnce.Do(func() {
		// Initial fetch attempt
		err := fetchBazaarData()
		if err != nil {
			// Set the global error state ONLY on the very first fetch attempt failure
			apiCacheMutex.Lock()
			apiFetchErr = err
			apiCacheMutex.Unlock()
		}
	})

	apiCacheMutex.RLock()
	defer apiCacheMutex.RUnlock()

	// Check global error state AFTER sync.Once has run
	if apiFetchErr != nil {
		return nil, apiFetchErr
	}
	if apiResponseCache == nil {
		// This might happen if the initial fetch failed silently before setting apiFetchErr,
		// or if the API returned an empty but successful response initially.
		// Return an error indicating data is unavailable.
		if apiFetchErr == nil { // If no specific error was recorded, create one.
			return nil, fmt.Errorf("API data unavailable: cache is nil after initial load attempt")
		}
		// If apiFetchErr was set, return that.
		return nil, apiFetchErr

	}
	return apiResponseCache, nil
}

// forceRefreshAPIData ALWAYS attempts to fetch new data and update the cache.
// This is used by the optimizer loop.
// It returns the latest cache content (even if stale on error) AND the error status.
func forceRefreshAPIData() (*HypixelAPIResponse, error) {
	err := fetchBazaarData() // Attempt to fetch and update cache

	// Regardless of error, return the current state of the cache
	apiCacheMutex.RLock()
	currentCacheState := apiResponseCache
	apiCacheMutex.RUnlock()

	if err != nil {
		// Fetch failed, return the current (potentially stale) cache and the error
		log.Printf("WARN: Failed to refresh API data: %v. Using potentially stale data (last fetched: %s).", err, lastAPIFetchTime.Format(time.RFC3339))
		return currentCacheState, err // Return potentially stale data AND the error
	}

	// Fetch succeeded, cache was updated internally by fetchBazaarData
	return currentCacheState, nil // Return the up-to-date cache and nil error
}
