// api.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// --- Struct Definitions (copied from your original) ---
type OrderSummary struct {
	PricePerUnit float64 `json:"pricePerUnit"`
	Amount       int     `json:"amount"`
	Orders       int     `json:"orders"`
}

type QuickStatus struct {
	BuyMovingWeek  float64 `json:"buyMovingWeek"`
	SellMovingWeek float64 `json:"sellMovingWeek"`
	BuyPrice       float64 `json:"buyPrice"`
	SellPrice      float64 `json:"sellPrice"`
}

type HypixelProduct struct {
	ProductID   string         `json:"product_id"`
	SellSummary []OrderSummary `json:"sell_summary"`
	BuySummary  []OrderSummary `json:"buy_summary"`
	QuickStatus QuickStatus    `json:"quick_status"`
}

type HypixelAPIResponse struct {
	Success     bool                      `json:"success"`
	LastUpdated int64                     `json:"lastUpdated"` // Unix timestamp in milliseconds
	Products    map[string]HypixelProduct `json:"products"`
}

// --- Global Variables for API Data Cache ---
var (
	apiResponseCache *HypixelAPIResponse // Holds the latest successfully fetched API response
	apiFetchErr      error               // Stores the last error encountered during an API fetch
	apiCacheMutex    sync.RWMutex        // Protects access to apiResponseCache and apiFetchErr
	lastAPIFetchTime time.Time           // Timestamp of the last successful API fetch
)

// fetchBazaarData handles the actual HTTP request to the Hypixel API.
// It updates the global apiResponseCache, apiFetchErr, and lastAPIFetchTime.
func fetchBazaarData() error {
	dlog("Fetching live Hypixel Bazaar data...")        // Original dlog call
	url := "https://api.hypixel.net/v2/skyblock/bazaar" // Using v2 endpoint

	client := http.Client{Timeout: 15 * time.Second} // HTTP client with a timeout

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fetchErr := fmt.Errorf("creating API request for %s: %w", url, err)
		log.Printf("[fetchBazaarData] ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr // Store the error
		apiCacheMutex.Unlock()
		return fetchErr
	}
	// If you have an API key, set it as a header or query parameter as per Hypixel's docs
	// Example: req.Header.Set("API-Key", "YOUR_HYPIXEL_API_KEY")

	resp, err := client.Do(req)
	if err != nil {
		fetchErr := fmt.Errorf("executing API GET request to %s: %w", url, err)
		log.Printf("[fetchBazaarData] ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr // Store the error
		apiCacheMutex.Unlock()
		return fetchErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := ""
		if readErr == nil {
			maxBody := 500 // Limit error body logging
			if len(bodyBytes) > maxBody {
				bodyBytes = append(bodyBytes[:maxBody], []byte("... (truncated)")...)
			}
			bodyStr = string(bodyBytes)
		} else {
			bodyStr = fmt.Sprintf("(failed to read response body for error status: %v)", readErr)
		}
		fetchErr := fmt.Errorf("Hypixel API returned non-OK status %d from %s. Body: %s", resp.StatusCode, url, bodyStr)
		log.Printf("[fetchBazaarData] ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr // Store the error
		apiCacheMutex.Unlock()
		return fetchErr
	}

	var apiResp HypixelAPIResponse
	// It's generally safer to read the whole body first, then unmarshal, for better error reporting.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fetchErr := fmt.Errorf("reading API response body from %s: %w", url, err)
		log.Printf("[fetchBazaarData] ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		maxBodyForLog := 500
		bodySample := string(bodyBytes)
		if len(bodySample) > maxBodyForLog {
			bodySample = bodySample[:maxBodyForLog] + "... (truncated)"
		}
		fetchErr := fmt.Errorf("parsing API JSON from %s: %w. Response body sample: %s", url, err, bodySample)
		log.Printf("[fetchBazaarData] ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	if !apiResp.Success {
		// Even if !Success, Hypixel might still provide a LastUpdated timestamp.
		// The decision to treat this as a hard error depends on how you want to handle partial/failed API states.
		fetchErr := fmt.Errorf("Hypixel API response 'success' field was false. LastUpdated: %d", apiResp.LastUpdated)
		log.Printf("[fetchBazaarData] ERROR: %v", fetchErr)
		// We might still update the cache with this "unsuccessful" response if it contains usable data like LastUpdated.
		// Or, we might preserve the old cache. For now, let's treat it as an error that prevents updating the cache with this response.
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr // Store this specific error
		apiCacheMutex.Unlock()
		return fetchErr // Return the error
	}

	// Lock for writing to global cache variables
	apiCacheMutex.Lock()
	defer apiCacheMutex.Unlock()

	// Optional: Product ID consistency check (from your original code)
	for id, product := range apiResp.Products {
		if product.ProductID != "" && product.ProductID != id {
			dlog("API Cache: Product ID mismatch for key '%s': product_id field is '%s'", id, product.ProductID)
		}
	}

	apiResponseCache = &apiResp   // Update the cache with the new, successful response
	apiFetchErr = nil             // Clear any previous fetch error
	lastAPIFetchTime = time.Now() // Update the timestamp of this successful fetch

	dlog("Hypixel Bazaar data fetched and cached successfully at %s. New LastUpdated: %d",
		lastAPIFetchTime.Format(time.RFC3339), apiResp.LastUpdated)
	return nil // Success
}

// getApiResponse is called by the main application logic to get the latest API data.
// This version will trigger a fresh fetch on every call.
func getApiResponse() (*HypixelAPIResponse, error) {
	log.Println("[getApiResponse] Attempting to fetch/refresh Bazaar data by calling fetchBazaarData()...")

	// Attempt to fetch new data. This will update the global cache if successful,
	// or update apiFetchErr if it fails.
	fetchAttemptErr := fetchBazaarData()

	// Regardless of fetchAttemptErr, we will return the current state of the cache
	// and the most recent error state (which fetchBazaarData would have set).
	apiCacheMutex.RLock()
	currentCache := apiResponseCache
	// currentError := apiFetchErr // This line was previously here, but fetchAttemptErr is more direct
	apiCacheMutex.RUnlock()

	if fetchAttemptErr != nil {
		// The fetch attempt failed. currentError should be the same as fetchAttemptErr.
		log.Printf("[getApiResponse] fetchBazaarData() reported an error: %v. Returning current cache (if any) and this error.", fetchAttemptErr)
		return currentCache, fetchAttemptErr // Return potentially stale cache and the new error
	}

	// Fetch attempt was successful (fetchAttemptErr is nil).
	// apiResponseCache should have been updated by fetchBazaarData.
	if currentCache == nil { // This should ideally not happen if fetch was successful
		log.Println("[getApiResponse] fetchBazaarData() succeeded but apiResponseCache is still nil. This is unexpected.")
		// This implies fetchBazaarData succeeded but set apiResponseCache to nil, which it shouldn't.
		// Or, that another goroutine set it to nil between fetchBazaarData and here.
		// The most robust error to return here is that data is unavailable.
		return nil, fmt.Errorf("API data unavailable: cache is nil even after a successful fetch attempt by fetchBazaarData")
	}

	log.Printf("[getApiResponse] Successfully returned data from cache after fetchBazaarData(). LastUpdated in cache: %d", currentCache.LastUpdated)
	return currentCache, nil // apiFetchErr should be nil if fetchBazaarData succeeded
}

// forceRefreshAPIData provides an explicit way to trigger a data refresh.
// With the current getApiResponse always fetching, this might be redundant for the main loop,
// but could be useful for other purposes (e.g., an admin endpoint).
func forceRefreshAPIData() (*HypixelAPIResponse, error) {
	log.Println("[forceRefreshAPIData] Explicitly refreshing API data via fetchBazaarData()...")
	err := fetchBazaarData() // This will attempt to update the global cache and apiFetchErr

	apiCacheMutex.RLock()
	currentCacheToReturn := apiResponseCache
	// The error to return should be the one from the fetchBazaarData call we just made
	// The global apiFetchErr would have been set by fetchBazaarData.
	// So, 'err' from fetchBazaarData is the most direct error for this operation.
	apiCacheMutex.RUnlock()

	if err != nil {
		lastFetchStr := "never"
		if !lastAPIFetchTime.IsZero() { // Check if lastAPIFetchTime was ever set
			lastFetchStr = lastAPIFetchTime.Format(time.RFC3339)
		}
		log.Printf("[forceRefreshAPIData] WARN: Failed to refresh API data: %v. Returning current cache (if any, last successful fetch: %s) and this error.", err, lastFetchStr)
		return currentCacheToReturn, err
	}

	// If currentCacheToReturn is nil here, it means the successful fetchBazaarData somehow
	// resulted in a nil cache, or it was nil before and fetchBazaarData didn't populate it.
	// This would be an unexpected state.
	if currentCacheToReturn == nil {
		log.Printf("[forceRefreshAPIData] ERROR: fetchBazaarData() succeeded but apiResponseCache is still nil. This is highly unexpected.")
		return nil, fmt.Errorf("API data unavailable: cache is nil after explicit successful refresh by forceRefreshAPIData")
	}

	log.Printf("[forceRefreshAPIData] Successfully refreshed data. LastUpdated in cache: %d", currentCacheToReturn.LastUpdated)
	return currentCacheToReturn, nil // Error from fetchBazaarData is nil here
}
