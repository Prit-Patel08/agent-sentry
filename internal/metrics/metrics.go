package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu              sync.Mutex
	startedAt       time.Time
	authFailures    uint64
	processKills    uint64
	processRestarts uint64
	httpRequests    map[string]uint64
}

func NewStore() *Store {
	return &Store{
		startedAt:    time.Now(),
		httpRequests: make(map[string]uint64),
	}
}

func (s *Store) IncRequest(path, method string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s|%s|%d", path, method, status)
	s.httpRequests[key]++
}

func (s *Store) IncAuthFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authFailures++
}

func (s *Store) IncProcessKill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processKills++
}

func (s *Store) IncProcessRestart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processRestarts++
}

func (s *Store) Prometheus(activeProcess bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var b strings.Builder
	b.WriteString("# HELP agent_sentry_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE agent_sentry_http_requests_total counter\n")

	keys := make([]string, 0, len(s.httpRequests))
	for k := range s.httpRequests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 {
			continue
		}
		fmt.Fprintf(&b, "agent_sentry_http_requests_total{path=%q,method=%q,status=%q} %d\n",
			parts[0], parts[1], parts[2], s.httpRequests[k])
	}

	b.WriteString("# HELP agent_sentry_auth_failures_total Failed auth attempts.\n")
	b.WriteString("# TYPE agent_sentry_auth_failures_total counter\n")
	fmt.Fprintf(&b, "agent_sentry_auth_failures_total %d\n", s.authFailures)

	b.WriteString("# HELP agent_sentry_process_kill_total Process kill actions.\n")
	b.WriteString("# TYPE agent_sentry_process_kill_total counter\n")
	fmt.Fprintf(&b, "agent_sentry_process_kill_total %d\n", s.processKills)

	b.WriteString("# HELP agent_sentry_process_restart_total Process restart actions.\n")
	b.WriteString("# TYPE agent_sentry_process_restart_total counter\n")
	fmt.Fprintf(&b, "agent_sentry_process_restart_total %d\n", s.processRestarts)

	b.WriteString("# HELP agent_sentry_uptime_seconds API uptime in seconds.\n")
	b.WriteString("# TYPE agent_sentry_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "agent_sentry_uptime_seconds %.0f\n", time.Since(s.startedAt).Seconds())

	b.WriteString("# HELP agent_sentry_active_process Whether a supervised process is active.\n")
	b.WriteString("# TYPE agent_sentry_active_process gauge\n")
	if activeProcess {
		b.WriteString("agent_sentry_active_process 1\n")
	} else {
		b.WriteString("agent_sentry_active_process 0\n")
	}

	return b.String()
}
