package main

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestOptimizerAgainstLiveData hits the real Hypixel bazaar and checks results are internally consistent, since profit numbers move with prices.
func TestOptimizerAgainstLiveData(t *testing.T) {
	if os.Getenv("SKIP_NETWORK_TESTS") != "" {
		t.Skip("SKIP_NETWORK_TESTS set, skipping live Hypixel API test")
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.hypixel.net/v2/skyblock/bazaar")
	if err != nil {
		t.Skipf("live Hypixel API unreachable, skipping: %v", err)
	}
	resp.Body.Close()

	metricsBytes, err := os.ReadFile(metricsFilename)
	if err != nil {
		t.Fatalf("could not read committed fixture %s: %v", metricsFilename, err)
	}
	metricsMap, err := parseProductMetricsData(metricsBytes)
	if err != nil {
		t.Fatalf("failed to parse committed metrics fixture: %v", err)
	}
	if len(metricsMap) == 0 {
		t.Fatalf("committed metrics fixture parsed to zero entries")
	}

	if err := fetchBazaarData(); err != nil {
		t.Fatalf("live fetchBazaarData failed: %v", err)
	}
	apiResp, err := getApiResponse()
	if err != nil {
		t.Fatalf("getApiResponse failed: %v", err)
	}
	if len(apiResp.Products) == 0 {
		t.Fatalf("live API response had zero products")
	}

	// A handful of common, always-craftable items rather than the full ~7000 item catalog.
	sampleItems := []string{"ENCHANTED_DIAMOND", "ENCHANTED_IRON", "ENCHANTED_GOLD"}
	var present []string
	for _, id := range sampleItems {
		if _, ok := apiResp.Products[id]; ok {
			present = append(present, id)
		}
	}
	if len(present) == 0 {
		t.Skip("none of the sample items are present in the live API response right now")
	}

	results := RunFullOptimization(present, 3600.0, apiResp, metricsMap, itemFilesDir, 1000000.0)
	if len(results) != len(present) {
		t.Fatalf("expected %d results, got %d", len(present), len(results))
	}

	checked := 0
	for _, r := range results {
		if !r.CalculationPossible {
			continue // Infeasible items (no profitable path right now) are a valid outcome, not a bug.
		}
		checked++

		cost := float64(r.CostAtOptimalQty)
		revenue := float64(r.RevenueAtOptimalQty)
		profit := float64(r.MaxProfit)
		if math.Abs(profit-(revenue-cost)) > 0.01 {
			t.Errorf("%s: MaxProfit (%.2f) != Revenue (%.2f) - Cost (%.2f)", r.ItemName, profit, revenue, cost)
		}

		acqTime := float64(r.AcquisitionTimeAtOptimalQty)
		saleTime := float64(r.SaleTimeAtOptimalQty)
		totalTime := float64(r.TotalCycleTimeAtOptimalQty)
		if math.Abs(totalTime-(acqTime+saleTime)) > 0.01 {
			t.Errorf("%s: TotalCycleTime (%.2f) != AcquisitionTime (%.2f) + SaleTime (%.2f)", r.ItemName, totalTime, acqTime, saleTime)
		}

		if r.MaxFeasibleQuantity <= 0 {
			t.Errorf("%s: marked CalculationPossible but MaxFeasibleQuantity is %v", r.ItemName, r.MaxFeasibleQuantity)
		}
	}

	t.Logf("verified %d/%d sample items with internally-consistent, live-data-derived results", checked, len(present))
}

// Sanity check that the marshaled JSON shape matches what parseProductMetricsData expects,
// i.e. the struct tags actually line up with the committed fixture's field names.
func TestParseProductMetricsData_RealFixture(t *testing.T) {
	data, err := os.ReadFile(metricsFilename)
	if err != nil {
		t.Fatalf("could not read %s: %v", metricsFilename, err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("fixture is not a JSON array: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("fixture is an empty array")
	}
	for _, requiredField := range []string{"product_id", "sell_size", "sell_frequency", "order_size_average", "order_frequency_average"} {
		if _, ok := raw[0][requiredField]; !ok {
			t.Errorf("fixture entries are missing expected field %q used by ProductMetrics", requiredField)
		}
	}
}
