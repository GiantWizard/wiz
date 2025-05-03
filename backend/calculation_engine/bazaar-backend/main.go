package main

import (
	"log"
)

// These globals will be populated at startup and used by server.go
var (
	apiRespGlobal    *HypixelAPIResponse
	metricsMapGlobal map[string]ProductMetrics
)

func main() {
	// Load Hypixel API cache
	var err error
	apiRespGlobal, err = getApiResponse()
	if err != nil {
		log.Printf("WARNING: Could not fetch API data: %v", err)
	}

	// Load metrics cache
	metricsMapGlobal, err = getMetricsMap(metricsFilename)
	if err != nil {
		log.Fatalf("CRITICAL: Failed loading metrics: %v", err)
	}

	// Hand off to server runner in server.go
	runServer()
}
