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

// isMegaSessionActive provides the most reliable check for an active session.
// It returns true only if the output of 'mega-whoami' contains an email address.
func isMegaSessionActive() bool {
	log.Println("[CHECK] Verifying MEGA session status with 'mega-whoami'...")

	// Use the --non-interactive flag to prevent hangs
	cmd := exec.Command("mega-whoami", "--non-interactive")
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		// Log the error but don't exit; the server might just not be ready.
		log.Printf("[CHECK] 'mega-whoami' command failed. This is expected if the session is not ready. Error: %v", err)
		return false
	}

	// THIS IS THE CRITICAL CHANGE.
	// We look for a positive confirmation (an email address) instead of a weak negative one.
	// The banner does not contain an '@', but a successful login status does.
	if strings.Contains(outputStr, "@") {
		log.Printf("[CHECK] SUCCESS: Session is confirmed active for user: %s", strings.TrimSpace(outputStr))
		return true
	}

	// If we are here, it means the command ran but didn't show an email. The session is not ready.
	log.Println("[CHECK] Session is not ready yet. Waiting...")
	return false
}

// runAndLogMegaLs executes the 'megals' command and prints its output to the log.
func runAndLogMegaLs() {
	log.Println("[ACTION] Executing 'megals -q' to list files in /remote_metrics...")

	// Use -q flag to quiet the banner output on success, giving a clean file list.
	cmd := exec.Command("megals", "-q", "/remote_metrics")
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
		log.Println("[SETUP] Waiting for MEGA session to become fully active...")

		// This loop will now reliably wait until the session-keeper has logged in.
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
