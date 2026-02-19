package test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func fixtureScriptPath(tb testing.TB, name string) string {
	tb.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatalf("failed to resolve runtime caller")
	}
	return filepath.Join(filepath.Dir(current), "fixtures", "scripts", name)
}

func TestFixtureScriptsRespectTimeoutAndExitShape(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		args       []string
		expectZero bool
	}{
		{name: "infinite-looper", file: "infinite_looper.py", args: []string{"--timeout", "1"}, expectZero: false},
		{name: "memory-leaker", file: "memory_leaker.py", args: []string{"--timeout", "1"}, expectZero: false},
		{name: "healthy-spike", file: "healthy_spike.py", args: []string{"--timeout", "20", "--spike-seconds", "1"}, expectZero: true},
		{name: "zombie-spawner", file: "zombie_spawner.py", args: []string{"--timeout", "1"}, expectZero: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cmdArgs := append([]string{fixtureScriptPath(t, tt.file)}, tt.args...)
			cmd := exec.Command("python3", cmdArgs...)

			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()

			select {
			case err := <-done:
				if tt.expectZero && err != nil {
					t.Fatalf("expected zero exit, got err=%v", err)
				}
				if !tt.expectZero && err == nil {
					t.Fatalf("expected non-zero exit for %s", tt.file)
				}
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("script %s did not terminate within timeout budget", tt.file)
			}
		})
	}
}
