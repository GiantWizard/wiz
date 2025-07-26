// File: calculation_engine/main.go
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
	remoteDir := "/some/remote/path"
	log.Printf("[CALC-ENGINE] Performing operation on: %s", remoteDir)

	const maxRetries = 5
	var backoff = time.Second * 2

	for i := 0; i < maxRetries; i++ {
		cmd := exec.Command("mega-ls", remoteDir)
		out, err := cmd.CombinedOutput()

		if err == nil {
			log.Printf("[CALC-ENGINE] SUCCESS: Command output:\n%s", out)
			return // Success, exit the function
		}

		// Check if the error is the one we're expecting during startup
		if strings.Contains(string(out), "Couldn't find") && i < maxRetries-1 {
			log.Printf("[CALC-ENGINE] WARN: mega command failed, retrying in %v... (Attempt %d/%d)", backoff, i+1, maxRetries)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
			continue
		}

		log.Printf("[CALC-ENGINE] ERROR: mega command failed after retries: %v\nOutput: %s", err, out)
		return // Permanent error or max retries reached
	}
}
func main() {
	log.Println("[CALC-ENGINE] Application starting up...")

	// --- Your Application Logic Goes Here ---
	// For example, you might want to run an operation periodically.

	// A small initial delay to ensure the mega-session-manager is fully initialized.
	time.Sleep(5 * time.Second)

	// Example: Run the operation every 2 minutes.
	// You should replace this with your actual application logic.
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Run once immediately, then on every tick.
	performMegaOperation()
	for range ticker.C {
		performMegaOperation()
	}

	// If your service is a long-running daemon, you might need
	// a way to keep the main goroutine alive.
	// select {} // block forever
}
