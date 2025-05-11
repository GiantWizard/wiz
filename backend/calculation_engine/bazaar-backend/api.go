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
	LastUpdated int64                     `json:"lastUpdated"`
	Products    map[string]HypixelProduct `json:"products"`
}

var (
	apiResponseCache *HypixelAPIResponse
	fetchApiOnce     sync.Once
	apiFetchErr      error
	apiCacheMutex    sync.RWMutex
	lastAPIFetchTime time.Time
)

func fetchBazaarData() error {
	var fetchErr error
	dlog("Fetching live Hypixel Bazaar data...")
	url := "https://api.hypixel.net/v2/skyblock/bazaar"
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fetchErr = fmt.Errorf("fetch API GET (%s): %w", url, err)
		log.Printf("ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := ""
		if readErr == nil {
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
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	var apiResp HypixelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		fetchErr = fmt.Errorf("parse API JSON: %w", err)
		log.Printf("ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	if !apiResp.Success {
		fetchErr = fmt.Errorf("API success field reported: false")
		log.Printf("ERROR: %v", fetchErr)
		apiCacheMutex.Lock()
		apiFetchErr = fetchErr
		apiCacheMutex.Unlock()
		return fetchErr
	}

	apiCacheMutex.Lock()
	defer apiCacheMutex.Unlock()
	for id, product := range apiResp.Products {
		if product.ProductID != "" && product.ProductID != id {
			dlog("API Cache: Product ID mismatch for key '%s': product_id field is '%s'", id, product.ProductID)
		}
	}
	apiResponseCache = &apiResp
	apiFetchErr = nil
	lastAPIFetchTime = time.Now()
	dlog("Hypixel Bazaar data fetched and cached successfully at %s.", lastAPIFetchTime.Format(time.RFC3339))
	return nil
}

func getApiResponse() (*HypixelAPIResponse, error) {
	fetchApiOnce.Do(func() {
		_ = fetchBazaarData()
	})
	apiCacheMutex.RLock()
	defer apiCacheMutex.RUnlock()
	if apiFetchErr != nil {
		return nil, apiFetchErr
	}
	if apiResponseCache == nil {
		return nil, fmt.Errorf("API data unavailable: cache is nil after initialization attempt")
	}
	return apiResponseCache, nil
}

func forceRefreshAPIData() (*HypixelAPIResponse, error) {
	err := fetchBazaarData()
	apiCacheMutex.RLock()
	currentCacheState := apiResponseCache
	currentErrorState := apiFetchErr
	apiCacheMutex.RUnlock()

	if err != nil {
		lastFetchStr := "never"
		if !lastAPIFetchTime.IsZero() {
			lastFetchStr = lastAPIFetchTime.Format(time.RFC3339)
		}
		log.Printf("WARN: Failed to refresh API data: %v. Using potentially stale data (last successful fetch: %s).", err, lastFetchStr)
		return currentCacheState, err
	}
	if currentErrorState != nil {
		log.Printf("WARN: API fetch succeeded, but global error state was non-nil briefly: %v", currentErrorState)
		return currentCacheState, nil
	}
	return currentCacheState, nil
}
