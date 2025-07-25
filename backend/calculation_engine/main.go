// file: calculation_engine/main.go
package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// isMegaSessionReady now checks for the existence of a "ready file"
// created by the session-keeper. This is 100% reliable.
func isMegaSessionReady() bool {
	readyFile := "/tmp/mega.ready"
	log.Printf("[CHECK] Looking for ready file: %s", readyFile)

	// os.Stat returns an error if the file does not exist.
	_, err := os.Stat(readyFile)

	if err == nil {
		log.Println("[CHECK] SUCCESS: Ready file found. MEGA session is active.")
		return true // File exists
	}

	log.Println("[CHECK] Ready file not found. Waiting for session-keeper...")
	return false // File does not exist
}

// runAndLogMegaLs executes the 'mega-ls' command and prints its output to the log.
func runAndLogMegaLs() {
	log.Println("[ACTION] Executing 'mega-ls /remote_metrics' to list files…")

	// Invoke the client; it will connect over the MEGA_CMD_SOCKET you exported
	cmd := exec.Command("mega-ls", "/remote_metrics")

	// Inherit the current environment, which should include:
	//   MEGA_CMD_SOCKET=/home/appuser/.megaCmd/megacmd.sock
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] mega-ls failed: %v\n%s", err, string(out))
		return
	}

	listing := strings.TrimSpace(string(out))
	log.Printf("[SUCCESS] Remote file listing:\n--- Remote Files in /remote_metrics ---\n%s\n---------------------------------------", listing)
}

func main() {
	// --- Background task to wait for session and then list files ---
	go func() {
		log.Println("[SETUP] Waiting for MEGA session to become fully active...")

		// This loop will now reliably wait for the ready file.
		for !isMegaSessionReady() {
			time.Sleep(10 * time.Second)
		}

		log.Println("[SETUP] MEGA session is now active. Starting periodic file listing.")
		runAndLogMegaLs()

		ticker := time.NewTicker(60 * time.Second)
		for range ticker.C {
			runAndLogMegaLs()
		}
	}()

	// --- HTTP server for health checks ---
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	log.Printf("[SETUP] Calculation Engine is running. Starting health check server on port :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("[FATAL] Failed to start HTTP server: %v", err)
	}
}
