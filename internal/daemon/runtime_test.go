package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveRuntimeDirUsesEnvOverride(t *testing.T) {
	t.Setenv(envRuntimeDir, filepath.Join(t.TempDir(), "daemon-dir"))
	got, err := ResolveRuntimeDir()
	if err != nil {
		t.Fatalf("ResolveRuntimeDir() error = %v", err)
	}
	if got != os.Getenv(envRuntimeDir) {
		t.Fatalf("ResolveRuntimeDir() = %q, want %q", got, os.Getenv(envRuntimeDir))
	}
}

func TestWriteReadPID(t *testing.T) {
	t.Setenv(envRuntimeDir, t.TempDir())
	paths, err := EnsureRuntimeDir()
	if err != nil {
		t.Fatalf("EnsureRuntimeDir() error = %v", err)
	}
	if err := WritePID(paths, 12345); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}
	got, err := ReadPID(paths)
	if err != nil {
		t.Fatalf("ReadPID() error = %v", err)
	}
	if got != 12345 {
		t.Fatalf("ReadPID() = %d, want 12345", got)
	}
}

func TestWriteReadState(t *testing.T) {
	t.Setenv(envRuntimeDir, t.TempDir())
	paths, err := EnsureRuntimeDir()
	if err != nil {
		t.Fatalf("EnsureRuntimeDir() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	want := State{
		PID:       22,
		Port:      "8080",
		Status:    "running",
		StartedAt: now,
	}
	if err := WriteState(paths, want); err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}
	got, err := ReadState(paths)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if got.PID != want.PID || got.Port != want.Port || got.Status != want.Status || !got.StartedAt.Equal(want.StartedAt) {
		t.Fatalf("ReadState() = %+v, want %+v", got, want)
	}
}

func TestProcessAliveCurrentProcess(t *testing.T) {
	if !ProcessAlive(os.Getpid()) {
		t.Fatal("ProcessAlive(current pid) = false, want true")
	}
}
