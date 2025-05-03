package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// runServer sets up static‐file serving and JSON endpoints.
func runServer() {
	// Serve your Svelte build from ./public
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	// JSON API routes
	http.HandleFunc("/api/instasell", instasellHandler)
	http.HandleFunc("/api/buyorder", buyOrderHandler)

	log.Println("▶ Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func instasellHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	item := q.Get("item")
	qty, err := strconv.ParseFloat(q.Get("qty"), 64)
	if err != nil {
		http.Error(w, "invalid qty", http.StatusBadRequest)
		return
	}
	prod, ok := safeGetProductData(apiRespGlobal, item)
	if !ok {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	secs, ferr := calculateInstasellFillTime(qty, prod)
	respondJSON(w, map[string]interface{}{
		"item":      item,
		"quantity":  qty,
		"fill_time": secs,
		"error":     errStr(ferr),
	})
}

func buyOrderHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	item := q.Get("item")
	qty, err := strconv.ParseFloat(q.Get("qty"), 64)
	if err != nil {
		http.Error(w, "invalid qty", http.StatusBadRequest)
		return
	}
	metrics, ok := safeGetMetricsData(metricsMapGlobal, item)
	if !ok {
		http.Error(w, "metrics not found", http.StatusNotFound)
		return
	}
	secs, rr, ferr := calculateBuyOrderFillTime(item, qty, metrics)
	respondJSON(w, map[string]interface{}{
		"item":      item,
		"quantity":  qty,
		"fill_time": secs,
		"rr":        rr,
		"error":     errStr(ferr),
	})
}

func respondJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}

func errStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
