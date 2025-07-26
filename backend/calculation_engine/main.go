package main

import (
	"log"
	"os/exec"
	"strings"
	"time"
)

// This function shows how to safely run a MEGA command.
// It assumes the session is managed externally by supervisord.
func performMegaOperation() {
	// --- MODIFIED LINE ---
	// The remote path has been corrected based on your interactive session.
	remoteDir := "/remote_metrics"
	log.Printf("[CALC-ENGINE] Performing operation on: %s", remoteDir)

	const maxRetries = 5
	var backoff = time.Second * 2

	for i := 0; i < maxRetries; i++ {
		// Use a non-interactive command like 'mega-ls', 'mega-put', etc.
		cmd := exec.Command("mega-ls", remoteDir)

		// The command inherits the environment (including MEGA_CMD_SOCKET) from supervisord.
		out, err := cmd.CombinedOutput()

		// If the command is successful, log the output and exit the function.
		if err == nil {
			log.Printf("[CALC-ENGINE] SUCCESS: Command output:\n%s", out)
			return
		}

		// If the command fails, check if it's a "path not found" error and if we have retries left.
		if strings.Contains(string(out), "Couldn't find") && i < maxRetries-1 {
			log.Printf("[CALC-ENGINE] WARN: mega command failed, retrying in %v... (Attempt %d/%d)", backoff, i+1, maxRetries)
			time.Sleep(backoff)
			backoff *= 2 // Exponentially increase the backoff time for the next retry.
			continue
		}

		// If it's a different error or we've run out of retries, log the final error and exit.
		log.Printf("[CALC-ENGINE] ERROR: mega command failed after retries: %v\nOutput: %s", err, out)
		return
	}
}

func main() {
	log.Println("[CALC-ENGINE] Application starting up...")

	// A small initial delay to ensure the mega-session-manager has time to initialize.
	time.Sleep(5 * time.Second)

	// --- Your Application Logic ---
	// For this example, we run the operation periodically.
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Run once immediately, then on every tick.
	performMegaOperation()
	for range ticker.C {
		performMegaOperation()
	}
}
