package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	http.HandleFunc("/latest_metrics/", latestMetricsHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	log.Printf("Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func latestMetricsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for /latest_metrics/")

	// Simply list remote directory and return filenames
	cmd := exec.Command("megals", "-q", "/remote_metrics")
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error listing remote_metrics: %v", err)
		http.Error(w, "Failed to list metrics directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
}
