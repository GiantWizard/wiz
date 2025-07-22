package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	// Register the handler for the /latest_metrics/ endpoint.
	// The trailing slash is important for matching behavior.
	http.HandleFunc("/latest_metrics/", latestMetricsHandler)

	// A simple health check endpoint.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000" // Default port if not specified
	}

	log.Printf("Calculation engine starting. Server listening on :%s", port)
	log.Printf("Endpoints available at /healthz and /latest_metrics/")

	// Start the server.
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}