package api

import (
	"agent-sentry/internal/database"
	"agent-sentry/internal/metrics"
	"agent-sentry/internal/state"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	apiMetrics  = metrics.NewStore()
	apiLimiter  = newRateLimiter(120, 10, 10*time.Minute)
	allowedCORS = []string{
		"http://localhost:3000",
		"http://localhost",
		"http://127.0.0.1:3000",
		"http://127.0.0.1",
	}
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func corsMiddleware(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))

	allowed := make(map[string]struct{}, len(allowedCORS)+1)
	for _, o := range allowedCORS {
		allowed[o] = struct{}{}
	}

	if envOrigin := strings.TrimSpace(os.Getenv("SENTRY_ALLOWED_ORIGIN")); envOrigin != "" && isLocalOrigin(envOrigin) {
		allowed[envOrigin] = struct{}{}
	}

	if _, ok := allowed[origin]; !ok {
		origin = "http://localhost:3000"
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func isLocalOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1"
}

func withSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		corsMiddleware(rec, r)

		if r.Method == "OPTIONS" {
			rec.WriteHeader(http.StatusOK)
			apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
			return
		}

		if !apiLimiter.allow(clientIP(r.RemoteAddr)) {
			http.Error(rec, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
			return
		}

		next(rec, r)
		apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
	}
}

// requireAuth checks the SENTRY_API_KEY env var.
// If no key is set, mutating endpoints are blocked.
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	ip := clientIP(r.RemoteAddr)
	apiKey := os.Getenv("SENTRY_API_KEY")

	if apiKey == "" {
		if r.Method == "POST" {
			http.Error(w, `{"error":"Security Alert: You must set SENTRY_API_KEY environment variable to perform mutations."}`, http.StatusForbidden)
			return false
		}
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		apiMetrics.IncAuthFailure()
		if apiLimiter.addAuthFailure(ip) {
			http.Error(w, `{"error":"Too many failed auth attempts. Retry later."}`, http.StatusTooManyRequests)
			return false
		}
		http.Error(w, `{"error":"Authorization required"}`, http.StatusUnauthorized)
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
		apiMetrics.IncAuthFailure()
		if apiLimiter.addAuthFailure(ip) {
			http.Error(w, `{"error":"Too many failed auth attempts. Retry later."}`, http.StatusTooManyRequests)
			return false
		}
		http.Error(w, `{"error":"Invalid API key"}`, http.StatusForbidden)
		return false
	}

	apiLimiter.clearAuthFailures(ip)
	return true
}

func StartServer(port string) {
	apiKey := os.Getenv("SENTRY_API_KEY")
	if apiKey != "" {
		fmt.Println("🔒 API Key authentication ENABLED for /process/* endpoints")
	} else {
		fmt.Println("⚠️  No SENTRY_API_KEY set - mutating endpoints are blocked")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", withSecurity(handleStream))
	mux.HandleFunc("/incidents", withSecurity(HandleIncidents))
	mux.HandleFunc("/process/kill", withSecurity(HandleProcessKill))
	mux.HandleFunc("/process/restart", withSecurity(HandleProcessRestart))
	mux.HandleFunc("/healthz", withSecurity(HandleHealth))
	mux.HandleFunc("/readyz", withSecurity(HandleReady))
	mux.HandleFunc("/metrics", withSecurity(HandleMetrics))

	addr := resolveBindAddr(port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	go func() {
		fmt.Printf("API listening on %s\n", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	<-stop
	fmt.Println("\n[API] Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	fmt.Println("[API] Server stopped")
}

func resolveBindAddr(port string) string {
	// Keep local-only binding unless explicitly asked for localhost alias.
	host := os.Getenv("SENTRY_BIND_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	if host != "127.0.0.1" && host != "localhost" {
		fmt.Printf("[API] Refusing non-local bind host %q. Falling back to 127.0.0.1.\n", host)
		host = "127.0.0.1"
	}
	return host + ":" + port
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			jsonData, err := state.JSON()
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				flusher.Flush()
			}
		}
	}
}

// HandleHealth returns process health for container liveness.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleReady checks DB readiness for startup probes.
func HandleReady(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			http.Error(w, `{"status":"not-ready"}`, http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// HandleMetrics emits Prometheus-style metrics.
func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := state.GetState()
	active := st.Status != "STOPPED" && st.PID > 0
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprint(w, apiMetrics.Prometheus(active))
}

// HandleIncidents is exported for testing.
func HandleIncidents(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			http.Error(w, "Database not initialized", http.StatusInternalServerError)
			return
		}
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
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !requireAuth(w, r) {
		return
	}

	stats := state.GetState()
	if stats.Status == "STOPPED" || stats.PID == 0 {
		http.Error(w, `{"error":"No active process to kill"}`, http.StatusBadRequest)
		return
	}

	if err := syscall.Kill(-stats.PID, syscall.SIGKILL); err != nil {
		if err2 := syscall.Kill(stats.PID, syscall.SIGKILL); err2 != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to kill process: %v"}`, err2), http.StatusInternalServerError)
			return
		}
	}

	state.UpdateState(0, "", "STOPPED", stats.Command, stats.Args, stats.Dir, stats.PID)
	apiMetrics.IncProcessKill()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"killed","pid":%d}`, stats.PID)
}

// HandleProcessRestart is exported for testing.
func HandleProcessRestart(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !requireAuth(w, r) {
		return
	}

	stats := state.GetState()
	if stats.Command == "" || len(stats.Args) == 0 {
		http.Error(w, `{"error":"No command available to restart"}`, http.StatusBadRequest)
		return
	}

	if stats.Status != "STOPPED" && stats.PID > 0 {
		_ = syscall.Kill(-stats.PID, syscall.SIGKILL)
		time.Sleep(500 * time.Millisecond)
	}

	go func() {
		cmd := exec.Command(stats.Args[0], stats.Args[1:]...)
		if stats.Dir != "" {
			cmd.Dir = stats.Dir
		}
		fmt.Printf("[API] Secure restart: %v\n", stats.Args)
		if err := cmd.Start(); err != nil {
			fmt.Printf("[API] Failed to restart command: %v\n", err)
			return
		}
		fmt.Printf("[API] Restarted process with PID: %d\n", cmd.Process.Pid)
	}()

	apiMetrics.IncProcessRestart()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"restarting","command":"%s"}`, stats.Command)
}
