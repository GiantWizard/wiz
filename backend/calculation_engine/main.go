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

// isMegaSessionActive checks if the MEGA session is logged in and active.
// It returns true if logged in, false otherwise. This is a more robust check.
func isMegaSessionActive() bool {
	log.Println("[CHECK] Verifying MEGA session status with 'mega-whoami'...")

	cmd := exec.Command("mega-whoami")
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		log.Printf("[CHECK] 'mega-whoami' command failed. Error: %v. Output: %s", err, outputStr)
		return false
	}

	// The most reliable check is to see if the output says we are NOT logged in.
	if strings.Contains(outputStr, "Not logged in") {
		log.Println("[CHECK] Session is not active yet ('Not logged in' detected). Waiting...")
		return false
	}

	// Any other output (like an email) means we are likely logged in.
	log.Printf("[CHECK] SUCCESS: Session appears to be active. Status: %s", strings.TrimSpace(outputStr))
	return true
}

// runAndLogMegaLs executes the 'megals' command and prints its output to the log.
func runAndLogMegaLs() {
	log.Println("[ACTION] Executing 'megals' to list files in /remote_metrics...")

	cmd := exec.Command("megals", "/remote_metrics")
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] Failed to run 'megals'. Error: %v", err)
		log.Printf("[ERROR_OUTPUT] %s", string(out))
		return
	}

	log.Printf("[SUCCESS] Remote file listing:\n--- Remote Files in /remote_metrics ---\n%s\n---------------------------------------", string(out))
}

func main() {
	// --- Background task to wait for session and then list files ---
	go func() {
		log.Println("[SETUP] Waiting for MEGA session to become active...")
		for !isMegaSessionActive() {
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
