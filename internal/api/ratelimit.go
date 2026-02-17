package api

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type limiterEntry struct {
	windowStart  time.Time
	requestCount int
	authFailures int
	blockedUntil time.Time
}

type rateLimiter struct {
	mu            sync.Mutex
	requestLimit  int
	authFailLimit int
	blockDuration time.Duration
	entries       map[string]*limiterEntry
}

func newRateLimiter(requestLimit, authFailLimit int, blockDuration time.Duration) *rateLimiter {
	if requestLimit <= 0 {
		requestLimit = 120
	}
	if authFailLimit <= 0 {
		authFailLimit = 10
	}
	if blockDuration <= 0 {
		blockDuration = 10 * time.Minute
	}
	return &rateLimiter{
		requestLimit:  requestLimit,
		authFailLimit: authFailLimit,
		blockDuration: blockDuration,
		entries:       make(map[string]*limiterEntry),
	}
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getEntry(ip)
	now := time.Now()
	if now.Before(e.blockedUntil) {
		return false
	}
	if now.Sub(e.windowStart) >= time.Minute {
		e.windowStart = now
		e.requestCount = 0
		e.authFailures = 0
	}
	e.requestCount++
	return e.requestCount <= r.requestLimit
}

func (r *rateLimiter) addAuthFailure(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getEntry(ip)
	now := time.Now()
	if now.Sub(e.windowStart) >= time.Minute {
		e.windowStart = now
		e.requestCount = 0
		e.authFailures = 0
	}
	e.authFailures++
	if e.authFailures >= r.authFailLimit {
		e.blockedUntil = now.Add(r.blockDuration)
		return true
	}
	return false
}

func (r *rateLimiter) clearAuthFailures(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getEntry(ip)
	e.authFailures = 0
}

func (r *rateLimiter) getEntry(ip string) *limiterEntry {
	e, ok := r.entries[ip]
	if !ok {
		e = &limiterEntry{windowStart: time.Now()}
		r.entries[ip] = e
	}
	return e
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	if strings.Contains(remoteAddr, ":") && strings.Count(remoteAddr, ":") == 1 {
		if _, pErr := strconv.Atoi(strings.Split(remoteAddr, ":")[1]); pErr == nil {
			return strings.Split(remoteAddr, ":")[0]
		}
	}
	return remoteAddr
}
