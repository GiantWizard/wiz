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
// It returns true only if the output of 'mega-whoami' contains an email address ('@').
func isMegaSessionActive() bool {
	log.Println("[CHECK] Verifying MEGA session status with 'mega-whoami'...")

	// Use the --non-interactive flag to prevent the command from hanging.
	cmd := exec.Command("mega-whoami", "--non-interactive")
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		// This is normal if the server isn't ready. Log it and continue.
		log.Printf("[CHECK] 'mega-whoami' command failed, which is expected if the session is not ready. Error: %v", err)
		return false
	}

	// THIS IS THE CRITICAL LOGIC THAT IS MISSING IN YOUR CURRENT DEPLOYMENT:
	// A successful login will always contain an email. The banner does not.
	if strings.Contains(outputStr, "@") {
		log.Printf("[CHECK] SUCCESS: Session is confirmed active for user: %s", strings.TrimSpace(outputStr))
		return true
	}

	// If the command ran but did not output an email, the session is not ready.
	log.Println("[CHECK] Session is not ready yet (no '@' in output). Waiting...")
	return false
}

// runAndLogMegaLs executes the 'megals' command and prints its output to the log.
func runAndLogMegaLs() {
	log.Println("[ACTION] Executing 'megals -q' to list files in /remote_metrics...")

	// The -q flag will successfully quiet the banner once the command can connect.
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

		// This loop will now correctly wait because isMegaSessionActive is fixed.
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
