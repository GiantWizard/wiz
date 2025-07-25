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
// It returns true if logged in, false otherwise.
func isMegaSessionActive() bool {
	log.Println("[CHECK] Checking MEGA session status with 'mega-whoami'...")

	cmd := exec.Command("mega-whoami")
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("[CHECK] 'mega-whoami' failed, session is not active yet. Waiting...")
		return false
	}

	// A successful login will contain an email address.
	if strings.Contains(string(out), "@") {
		log.Printf("[CHECK] SUCCESS: Session is active. Logged in as: %s", strings.TrimSpace(string(out)))
		return true
	}

	log.Println("[CHECK] 'mega-whoami' ran but output was unexpected. Session likely not active.")
	return false
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
		// This loop will continue until the MEGA session is active.
		// This is far more reliable than a fixed sleep timer.
		for !isMegaSessionActive() {
			// Wait for 10 seconds before checking again to avoid spamming logs.
			time.Sleep(10 * time.Second)
		}

		// Once the session is confirmed active, we can proceed.
		log.Println("[SETUP] MEGA session is now active. Starting periodic file listing.")

		// Run once immediately.
		runAndLogMegaLs()

		// Now, start the ticker for subsequent runs.
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
