package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const (
	remoteDir    = "/remote_metrics"
	listInterval = 60 * time.Second
)

// listRemote uses the non-interactive `mega-ls` command to list files.
// It assumes the session is already active, managed by supervisord.
func listRemote() {
	log.Printf("[GO-APP] Listing remote directory: %s", remoteDir)

	// The command will inherit the MEGA_CMD_SOCKET from the environment.
	cmd := exec.Command("mega-ls", "-h", remoteDir) // -h for human-readable sizes

	// Execute the command and capture its combined stdout/stderr.
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Log the error and the output for easier debugging.
		log.Printf("[GO-APP] ERROR: mega-ls failed: %v\nOutput: %s", err, out)
		return
	}

	log.Printf("[GO-APP] SUCCESS: Remote listing for %s:\n%s", remoteDir, out)
}

func main() {
	// Goroutine to periodically list the remote directory.
	go func() {
		// Wait a few seconds on initial start to ensure the session is ready.
		// A more robust solution could be to retry on failure.
		log.Println("[GO-APP] Initial delay of 5s before starting periodic listing...")
		time.Sleep(5 * time.Second)

		// Create a ticker for periodic execution.
		ticker := time.NewTicker(listInterval)
		defer ticker.Stop()

		// Run once immediately, then on every tick.
		listRemote()
		for range ticker.C {
			listRemote()
		}
	}()

	// Health probe endpoint for Kubernetes, Docker Swarm, etc.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	log.Printf("[GO-APP] Health check server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("[GO-APP] FATAL: ListenAndServe failed: %v", err)
	}
}
