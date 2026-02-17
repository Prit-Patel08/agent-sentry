package api

import (
	"agent-sentry/internal/database"
	"agent-sentry/internal/ipc"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func corsMiddleware(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// requireAuth checks the SENTRY_API_KEY env var. Returns true if auth passes.
// If the env var is not set, auth is skipped (open access for local dev).
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	apiKey := os.Getenv("SENTRY_API_KEY")
	if apiKey == "" {
		return true // No key set = open access (dev mode)
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, `{"error":"Authorization required. Set Authorization: Bearer <SENTRY_API_KEY>"}`, http.StatusUnauthorized)
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != apiKey {
		http.Error(w, `{"error":"Invalid API key"}`, http.StatusUnauthorized)
		return false
	}

	return true
}

func StartServer(port string) {
	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/incidents", HandleIncidents)
	http.HandleFunc("/process/kill", HandleProcessKill)
	http.HandleFunc("/process/restart", HandleProcessRestart)
	fmt.Printf("Starting Dashboard API on port %s...\n", port)

	apiKey := os.Getenv("SENTRY_API_KEY")
	if apiKey != "" {
		fmt.Println("🔒 API Key authentication ENABLED for /process/* endpoints")
	} else {
		fmt.Println("⚠️  No SENTRY_API_KEY set — /process/* endpoints are OPEN")
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			stats, err := ipc.ReadLiveStats()
			if err == nil {
				jsonData, _ := json.Marshal(stats)
				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				flusher.Flush()
			}
		}
	}
}

// HandleIncidents is exported for testing.
func HandleIncidents(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w)

	if r.Method == "OPTIONS" {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	db := database.GetDB()
	if db == nil {
		if err := database.InitDB(); err != nil {
			http.Error(w, "Database not initialized", http.StatusInternalServerError)
			return
		}
		db = database.GetDB()
	}

	incidents, err := database.GetAllIncidents()
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(incidents); err != nil {
		http.Error(w, fmt.Sprintf("Encode error: %v", err), http.StatusInternalServerError)
	}
}

// HandleProcessKill is exported for testing.
func HandleProcessKill(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	if !requireAuth(w, r) {
		return
	}

	stats, err := ipc.ReadLiveStats()
	if err != nil || stats.Status == "STOPPED" || stats.PID == 0 {
		http.Error(w, `{"error":"No active process to kill"}`, http.StatusBadRequest)
		return
	}

	// Kill the process group (negative PID targets the group)
	if err := syscall.Kill(-stats.PID, syscall.SIGKILL); err != nil {
		// Try killing just the process
		if err2 := syscall.Kill(stats.PID, syscall.SIGKILL); err2 != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to kill process: %v"}`, err2), http.StatusInternalServerError)
			return
		}
	}

	// Update IPC status
	ipc.WriteLiveStats(ipc.LiveStats{
		Status:  "STOPPED",
		Command: stats.Command,
		PID:     stats.PID,
	})

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"killed","pid":%d}`, stats.PID)
}

// HandleProcessRestart is exported for testing.
func HandleProcessRestart(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	if !requireAuth(w, r) {
		return
	}

	stats, err := ipc.ReadLiveStats()
	if err != nil || stats.Command == "" {
		http.Error(w, `{"error":"No command available to restart"}`, http.StatusBadRequest)
		return
	}

	// Kill existing process if running
	if stats.Status != "STOPPED" && stats.PID > 0 {
		syscall.Kill(-stats.PID, syscall.SIGKILL)
		time.Sleep(500 * time.Millisecond) // Brief wait for cleanup
	}

	// Restart in background
	go func() {
		cmd := exec.Command("sh", "-c", stats.Command)
		if err := cmd.Start(); err != nil {
			fmt.Printf("[API] Failed to restart command: %v\n", err)
			return
		}
		fmt.Printf("[API] Restarted process with PID: %d\n", cmd.Process.Pid)
	}()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"restarting","command":"%s"}`, stats.Command)
}
