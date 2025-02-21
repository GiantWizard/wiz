package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// HistoryEntry represents one entry in the historical data.
type HistoryEntry struct {
	MaxBuy         float64 `json:"maxBuy"`
	MaxSell        float64 `json:"maxSell"`
	MinBuy         float64 `json:"minBuy"`
	MinSell        float64 `json:"minSell"`
	Buy            float64 `json:"buy"`
	Sell           float64 `json:"sell"`
	SellVolume     int     `json:"sellVolume"`
	BuyVolume      int     `json:"buyVolume"`
	Timestamp      string  `json:"timestamp"`
	BuyMovingWeek  int     `json:"buyMovingWeek"`
	SellMovingWeek int     `json:"sellMovingWeek"`
}

// ItemHistory wraps the product key and its history data.
type ItemHistory struct {
	Item    string         `json:"item"`
	History []HistoryEntry `json:"history"`
}

// HypixelProduct holds basic info for a product.
type HypixelProduct struct {
	ProductID string `json:"product_id"`
	// Other fields omitted.
}

// HypixelResponse represents the JSON response from Hypixel.
type HypixelResponse struct {
	Success     bool                      `json:"success"`
	LastUpdated int64                     `json:"lastUpdated"`
	Products    map[string]HypixelProduct `json:"products"`
}

func main() {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// === Step 1: Fetch product list from Hypixel API ===
	hypURL := "https://api.hypixel.net/v2/skyblock/bazaar"
	log.Printf("Fetching product list from %s", hypURL)
	resp, err := client.Get(hypURL)
	if err != nil {
		log.Fatalf("Error fetching Hypixel API: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Non-OK HTTP status from Hypixel API: %s", resp.Status)
	}
	hypBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatalf("Error reading Hypixel API response: %v", err)
	}

	var hypResp HypixelResponse
	if err := json.Unmarshal(hypBody, &hypResp); err != nil {
		log.Fatalf("Error unmarshalling Hypixel API response: %v", err)
	}
	if !hypResp.Success {
		log.Fatalf("Hypixel API returned an unsuccessful response")
	}

	var productKeys []string
	for key := range hypResp.Products {
		productKeys = append(productKeys, key)
	}
	totalProducts := len(productKeys)
	log.Printf("Fetched %d products from Hypixel API", totalProducts)

	// === Step 2: Open JSON file for writing the array ===
	outputFile := "avgPriceEngine_output.json"
	// Truncate any existing file.
	f, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer f.Close()

	// Write the opening bracket for the JSON array.
	if _, err := f.Write([]byte("[\n")); err != nil {
		log.Fatalf("Error writing opening bracket: %v", err)
	}

	// Mutex to protect file writes and the "first" flag.
	var fileMutex sync.Mutex
	first := true

	// === Step 3: Concurrently fetch coflnet API history with retry logic ===
	// Limit concurrency to 100.
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 100)

	// Rate limiter: allow up to 100 requests per minute.
	limiter := rate.NewLimiter(rate.Every(time.Minute/100), 100)

	// Counter for successful fetches.
	var successCount int
	var successMutex sync.Mutex

	for _, productKey := range productKeys {
		wg.Add(1)
		go func(productKey string) {
			defer wg.Done()

			// Limit concurrency.
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Retry until the fetch is successful.
			for {
				// Respect the rate limiter on every attempt.
				if err := limiter.Wait(context.Background()); err != nil {
					log.Printf("Rate limiter error for %s: %v", productKey, err)
					continue
				}

				// Build the coflnet API URL (URL-encoding the product key).
				encodedKey := url.PathEscape(productKey)
				coflnetURL := fmt.Sprintf("https://sky.coflnet.com/api/bazaar/%s/history/week", encodedKey)
				log.Printf("Fetching data for %s from %s", productKey, coflnetURL)

				coflnetResp, err := client.Get(coflnetURL)
				if err != nil {
					log.Printf("Error fetching %s: %v", coflnetURL, err)
					continue
				}
				if coflnetResp.StatusCode != http.StatusOK {
					log.Printf("Non-OK HTTP status for %s: %s", coflnetURL, coflnetResp.Status)
					coflnetResp.Body.Close()
					continue
				}

				body, err := ioutil.ReadAll(coflnetResp.Body)
				coflnetResp.Body.Close()
				if err != nil {
					log.Printf("Error reading response body for %s: %v", coflnetURL, err)
					continue
				}
				if len(body) == 0 {
					log.Printf("Empty response for %s", coflnetURL)
					continue
				}

				var history []HistoryEntry
				if err := json.Unmarshal(body, &history); err != nil {
					log.Printf("Invalid JSON response for %s: %v", coflnetURL, err)
					continue
				}

				// Prepare the item history structure.
				itemHistory := ItemHistory{
					Item:    productKey,
					History: history,
				}

				// Marshal the item into JSON.
				data, err := json.MarshalIndent(itemHistory, "  ", "  ")
				if err != nil {
					log.Printf("Error marshalling item %s: %v", productKey, err)
					continue
				}

				// Append the JSON object to the file as part of the array.
				fileMutex.Lock()
				// If not the first item, add a comma separator.
				if !first {
					if _, err := f.Write([]byte(",\n")); err != nil {
						log.Printf("Error writing comma for item %s: %v", productKey, err)
						fileMutex.Unlock()
						continue
					}
				} else {
					first = false
				}
				// Write the JSON data.
				if _, err := f.Write(data); err != nil {
					log.Printf("Error writing item %s to output file: %v", productKey, err)
					fileMutex.Unlock()
					continue
				}
				// Flush to disk.
				f.Sync()
				fileMutex.Unlock()

				// Increase the success counter.
				successMutex.Lock()
				successCount++
				successMutex.Unlock()

				log.Printf("Successfully fetched and appended history for item: %s", productKey)
				break // exit retry loop on success
			}
		}(productKey)
	}

	// Wait for all fetch goroutines to finish.
	wg.Wait()

	// === Step 4: Write the closing bracket to complete the JSON array ===
	if _, err := f.Write([]byte("\n]")); err != nil {
		log.Fatalf("Error writing closing bracket: %v", err)
	}

	// Final check: Compare successful fetches with the total number of bazaar items.
	if successCount != totalProducts {
		log.Printf("Warning: Only %d successful fetches out of %d bazaar items", successCount, totalProducts)
	} else {
		log.Printf("Successfully fetched all %d bazaar items", totalProducts)
	}
}
