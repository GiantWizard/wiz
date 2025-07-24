// file: calculation_engine/main.go
package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// runAndLogMegaLs executes the 'megals' command and prints its output and any errors to the standard log.
func runAndLogMegaLs() {
	log.Println("ACTION: Executing 'megals' to list remote files in /remote_metrics...")

	// Define the command to be executed.
	// The '-q' flag can be used for less verbose output from megals, but we'll remove it
	// for now to get as much information as possible during debugging.
	cmd := exec.Command("megals", "/remote_metrics")

	// Crucially, set the HOME environment variable so that mega-cmd can find its
	// configuration and session files located in /home/appuser/.megaCmd
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	// Run the command and capture its combined standard output and standard error.
	out, err := cmd.CombinedOutput()

	// Check if the command execution resulted in an error.
	if err != nil {
		// If there's an error, log the error message itself and include any
		// output the command produced, which often contains valuable diagnostic info.
		log.Printf("ERROR: Failed to run 'megals'. Error: %v", err)
		log.Printf("ERROR_OUTPUT: %s", string(out))
		return
	}

	// If the command was successful, log the list of files.
	log.Println("SUCCESS: 'megals' executed correctly. Remote file listing:")
	log.Printf("--- Remote Files in /remote_metrics ---\n%s\n---------------------------------------", string(out))
}

func main() {
	// --- Background task to periodically list files ---

	// Create a ticker that will fire at a set interval.
	// We'll set it to 60 seconds.
	const listingInterval = 60 * time.Second
	ticker := time.NewTicker(listingInterval)
	log.Printf("SETUP: Background task configured to list MEGA files every %v.", listingInterval)

	// Start a new goroutine (a concurrent background process).
	// This allows our periodic task to run without blocking the HTTP server.
	go func() {
		// It's important to wait for a moment on the first run.
		// This gives the 'session-keeper' service enough time to start and
		// successfully log in to the MEGA service. Without this, the first
		// command will likely fail because the session isn't ready.
		const initialDelay = 20 * time.Second
		log.Printf("SETUP: Waiting %v for MEGA session to be established before the first run...", initialDelay)
		time.Sleep(initialDelay)

		// Perform the first run immediately after the initial delay.
		runAndLogMegaLs()

		// This loop runs forever in the background.
		for range ticker.C {
			// Every time the ticker fires (e.g., every 60 seconds),
			// it will call our function to list the files.
			runAndLogMegaLs()
		}
	}()

	// --- HTTP server for health checks ---

	// The web server runs in the main goroutine. Its primary purpose here
	// is to provide a health check endpoint, which is standard practice for
	// containerized services.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Determine the port to listen on.
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	log.Printf("SETUP: Calculation Engine is running. Starting health check server on port :%s", port)

	// Start the HTTP server. This is a blocking call and will keep the main
	// function from exiting, allowing the background goroutine to continue its work.
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("FATAL: Failed to start HTTP server: %v", err)
	}
}
