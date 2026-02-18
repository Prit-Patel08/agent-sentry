package patterns

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

const (
	BlacklistFile = "pattern_blacklist.json"
)

var mu sync.Mutex

// SyncPatterns appends a detected pattern to the local blacklist.
func SyncPatterns(pattern string) error {
	mu.Lock()
	defer mu.Unlock()

	blacklist, err := loadBlacklist()
	if err != nil {
		blacklist = []string{}
	}

	// Check if already exists (avoid duplicates)
	lev := metrics.NewLevenshtein()
	for _, existing := range blacklist {
		if strutil.Similarity(pattern, existing, lev) >= 0.95 {
			return nil // Already known
		}
	}

	blacklist = append(blacklist, pattern)

	// Save locally
	if err := saveBlacklist(blacklist); err != nil {
		return fmt.Errorf("failed to save blacklist: %v", err)
	}

	fmt.Printf("[FlowForge] 📋 Pattern registry updated (%d patterns)\n", len(blacklist))

	return nil
}

// PullPatterns loads patterns from the local blacklist.
func PullPatterns() []string {
	mu.Lock()
	defer mu.Unlock()

	blacklist, err := loadBlacklist()
	if err != nil {
		return []string{}
	}

	fmt.Printf("[FlowForge] 📋 Loaded %d known bad patterns\n", len(blacklist))
	return blacklist
}

// IsBlacklisted checks if a normalized pattern matches any entry
// in the blacklist at ≥90% Levenshtein similarity.
func IsBlacklisted(normalized string, blacklist []string) bool {
	if len(blacklist) == 0 {
		return false
	}

	lev := metrics.NewLevenshtein()
	for _, known := range blacklist {
		if strutil.Similarity(normalized, known, lev) >= 0.9 {
			return true
		}
	}
	return false
}

func loadBlacklist() ([]string, error) {
	data, err := os.ReadFile(BlacklistFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var blacklist []string
	if err := json.Unmarshal(data, &blacklist); err != nil {
		return nil, err
	}
	return blacklist, nil
}

func saveBlacklist(blacklist []string) error {
	data, err := json.MarshalIndent(blacklist, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(BlacklistFile, data, 0644)
}
