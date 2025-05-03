package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// ── Configuration ──────────────────────────────────────────────────────────────

const (
	metricsFilename = "latest_metrics.json"
	itemFilesDir    = "dependencies/items"
)

// ── Globals (populated at startup) ─────────────────────────────────────────────

var (
	apiRespGlobal    *HypixelAPIResponse
	metricsMapGlobal map[string]ProductMetrics
)

// ── JSON response types ─────────────────────────────────────────────────────────

type Ingredient struct {
	Name              string  `json:"name"`
	Qty               float64 `json:"qty"`
	InstasellFillTime float64 `json:"instasell_fill_time"`
	BuyOrderFillTime  float64 `json:"buy_order_fill_time"`
	RR                float64 `json:"rr"`
}

type FillResponse struct {
	Recipe               []Ingredient `json:"recipe"`
	SlowestIngredient    string       `json:"slowest_ingredient"`
	SlowestIngredientQty float64      `json:"slowest_ingredient_qty"`
	SlowestFillTime      float64      `json:"slowest_fill_time"`
}

// ── Entry point ─────────────────────────────────────────────────────────────────

func main() {
	var err error

	// 1) load Bazaar API cache (api.go)
	apiRespGlobal, err = getApiResponse()
	if err != nil {
		log.Printf("WARNING: Bazaar API load failed: %v", err)
	}

	// 2) load metrics map (metrics.go)
	metricsMapGlobal, err = getMetricsMap(metricsFilename)
	if err != nil {
		log.Fatalf("CRITICAL: cannot load metrics: %v", err)
	}

	// 3) HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("public")))
	mux.Handle("/api/fill", withCORS(withRecovery(fillHandler)))

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// ── Handler ────────────────────────────────────────────────────────────────────

func fillHandler(w http.ResponseWriter, r *http.Request) {
	// parse item & qty
	item := r.URL.Query().Get("item")
	qty, err := strconv.ParseFloat(r.URL.Query().Get("qty"), 64)
	if err != nil || qty < 0 {
		http.Error(w, "invalid qty", http.StatusBadRequest)
		return
	}

	// expand into base ingredients (c10m.go → expandItem)
	baseMap, err := expandItem(item, qty, nil, apiRespGlobal, metricsMapGlobal, itemFilesDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var resp FillResponse
	var slowestTime float64

	for name, amt := range baseMap {
		// instasell fill time (c10m.go)
		prod := getProductData(apiRespGlobal, name)
		insta, _ := calculateInstasellFillTime(amt, prod)

		// buy‑order fill time & RR (c10m.go)
		buyTime, rr, _ := calculateBuyOrderFillTime(name, amt, metricsMapGlobal[name])

		resp.Recipe = append(resp.Recipe, Ingredient{
			Name:              name,
			Qty:               amt,
			InstasellFillTime: insta,
			BuyOrderFillTime:  buyTime,
			RR:                rr,
		})

		// track the overall slowest buy‑order fill
		if buyTime > slowestTime {
			slowestTime = buyTime
			resp.SlowestIngredient = name
			resp.SlowestIngredientQty = amt
		}
	}
	resp.SlowestFillTime = slowestTime

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func withRecovery(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		h(w, r)
	}
}

func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

// getProductData safely retrieves a HypixelProduct (c10m.go / api.go)
func getProductData(api *HypixelAPIResponse, id string) HypixelProduct {
	if api == nil || api.Products == nil {
		return HypixelProduct{}
	}
	if p, ok := api.Products[id]; ok {
		return p
	}
	return HypixelProduct{}
}
