// File: calculation_engine/main.go
package main

import (
	"log"
	"os/exec"
	"time"
)

// This function shows how to safely run a MEGA command.
// It assumes the session is managed externally by supervisord.
func performMegaOperation() {
	remoteDir := "/some/remote/path"
	log.Printf("[CALC-ENGINE] Performing operation on: %s", remoteDir)

	// Use a non-interactive command like 'mega-ls', 'mega-put', etc.
	cmd := exec.Command("mega-ls", remoteDir)

	// The command inherits the environment (including MEGA_CMD_SOCKET) from supervisord.
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[CALC-ENGINE] ERROR: mega command failed: %v\nOutput: %s", err, out)
		return
	}

	log.Printf("[CALC-ENGINE] SUCCESS: Command output:\n%s", out)
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
