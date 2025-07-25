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

func isReady() bool {
	const readyFile = "/tmp/mega.ready"
	if _, err := os.Stat(readyFile); err == nil {
		log.Println("[CHECK] Ready file found.")
		return true
	}
	log.Println("[CHECK] Ready file not found; waiting…")
	return false
}

func listRemote() {
	log.Println("[ACTION] Running non-interactive mega-ls…")
	cmd := exec.Command("bash", "-lc", `mega-ls /remote_metrics`)
	cmd.Env = append(os.Environ(), "HOME=/home/appuser")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] mega-ls failed: %v\n%s", err, out)
		return
	}
	outStr := strings.TrimSpace(string(out))
	log.Printf("[SUCCESS] /remote_metrics:\n%s", outStr)
}

func main() {
	// Background listing
	go func() {
		log.Println("[SETUP] waiting for MEGA session…")
		for !isReady() {
			time.Sleep(10 * time.Second)
		}
		log.Println("[SETUP] session ready – starting periodic listing")
		listRemote()
		ticker := time.NewTicker(60 * time.Second)
		for range ticker.C {
			listRemote()
		}
	}()

	// Health check endpoint
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	log.Printf("[SETUP] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
