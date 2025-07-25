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

func isMegaSessionReady() bool {
	readyFile := "/tmp/mega.ready"
	log.Printf("[CHECK] Looking for ready file: %s", readyFile)
	if _, err := os.Stat(readyFile); err == nil {
		log.Println("[CHECK] SUCCESS: Ready file found. MEGA session is active.")
		return true
	}
	log.Println("[CHECK] Ready file not found. Waiting for session-keeper…")
	return false
}

func runAndLogMegaLs() {
	log.Println("[ACTION] Spawning interactive mega-cmd to list /remote_metrics…")

	// Use absolute path and non-interactive mode to run the client commands
	script := `
set -e
/usr/bin/mega-cmd --non-interactive << 'EOF'
cd /remote_metrics
ls
exit
EOF
`
	cmd := exec.Command("bash", "-lc", script)
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] interactive mega-cmd failed: %v", err)
		log.Printf("[ERROR_OUTPUT]\n%s", string(out))
		return
	}

	output := strings.TrimSpace(string(out))
	log.Printf("[SUCCESS] Remote file listing:\n%s", output)
}

func main() {
	// Background task: wait for ready-file, then list files every minute.
	go func() {
		log.Println("[SETUP] Waiting for MEGA session to become fully active…")
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

	// Expose health check endpoint
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
