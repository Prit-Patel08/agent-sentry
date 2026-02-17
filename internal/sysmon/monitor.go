package sysmon

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SysStats holds system monitoring data
type SysStats struct {
	OpenFDs     int
	SocketCount int
}

// GetStats returns current file descriptor and socket counts for the PID.
// Uses lsof, so it might be slow. Use with timeout.
func GetStats(pid int) (SysStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// lsof -p PID -n -P
	// -n: no host names
	// -P: no port names
	// faster
	cmd := exec.CommandContext(ctx, "lsof", "-p", strconv.Itoa(pid), "-n", "-P")
	out, err := cmd.Output()
	if err != nil {
		return SysStats{}, err
	}

	lines := strings.Split(string(out), "\n")
	fdCount := 0
	socketCount := 0

	// Header is line 0
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			if line == "" {
				continue
			}
			fdCount++
			if strings.Contains(line, "TCP") || strings.Contains(line, "UDP") || strings.Contains(line, "IPv4") || strings.Contains(line, "IPv6") {
				socketCount++
			}
		}
	}

	return SysStats{
		OpenFDs:     fdCount,
		SocketCount: socketCount,
	}, nil
}

// State for baselines
var baselines = make(map[int]SysStats)

func IsMonitoring(pid int) bool {
	_, ok := baselines[pid]
	return ok
}

func DetectProbing(pid int, current SysStats) (bool, string) {
	base, ok := baselines[pid]
	if !ok {
		// First time seeing this PID, set baseline
		baselines[pid] = current
		return false, ""
	}

	// Update baseline slowly? Or keep initial?
	// For "probing", we look for sudden spikes from initial state usually.
	// Let's stick to initial baseline for now, or maybe update max observed?
	// Ideally we want to detect "sudden" change.

	// Logic: If sockets double AND > 10, or FDs double AND > 20
	isProbing := false
	var details strings.Builder

	if current.SocketCount > 50 && current.SocketCount > base.SocketCount*2 {
		isProbing = true
		if base.SocketCount > 0 {
			details.WriteString(fmt.Sprintf("Sockets: %d -> %d (+%d%%)", base.SocketCount, current.SocketCount, (current.SocketCount-base.SocketCount)*100/base.SocketCount))
		} else {
			details.WriteString(fmt.Sprintf("Sockets: %d -> %d (New)", base.SocketCount, current.SocketCount))
		}
	}

	if current.OpenFDs > base.OpenFDs*3 && current.OpenFDs > 20 {
		if isProbing {
			details.WriteString(" | ")
		}
		isProbing = true
		details.WriteString(fmt.Sprintf("FDs: %d -> %d", base.OpenFDs, current.OpenFDs))
	}

	return isProbing, details.String()
}
