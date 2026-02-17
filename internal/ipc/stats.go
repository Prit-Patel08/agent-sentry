package ipc

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const LiveStatsFile = ".sentry_live"

type LiveStats struct {
	CPU       float64 `json:"cpu"`
	LastLine  string  `json:"last_line"`
	Status    string  `json:"status"` // RUNNING, STOPPED, LOOP_DETECTED, WATCHDOG_ALERT
	Command   string  `json:"command"`
	PID       int     `json:"pid"`
	Timestamp int64   `json:"timestamp"`
}

var (
	mu sync.Mutex
)

func WriteLiveStats(stats LiveStats) error {
	mu.Lock()
	defer mu.Unlock()

	stats.Timestamp = time.Now().UnixMilli()
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}

	return os.WriteFile(LiveStatsFile, data, 0644)
}

func ReadLiveStats() (*LiveStats, error) {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(LiveStatsFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default non-running stats
			return &LiveStats{Status: "STOPPED"}, nil
		}
		return nil, err
	}

	var stats LiveStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}

	// If stats are too old (e.g. > 5 seconds), assume stopped
	if time.Now().UnixMilli()-stats.Timestamp > 5000 {
		stats.Status = "STOPPED"
		stats.CPU = 0
	}

	return &stats, nil
}
