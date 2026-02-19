package supervisor

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStopTerminatesProcessTree(t *testing.T) {
	script := strings.Join([]string{
		"import subprocess, time, sys",
		`child = subprocess.Popen(["python3", "-c", "import time; time.sleep(120)"])`,
		"print(child.pid, flush=True)",
		"time.sleep(120)",
	}, "\n")

	cmd := exec.Command("python3", "-c", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	s := New(cmd)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}

	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", line, err)
	}

	if err := s.Stop(2 * time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if processExists(childPID) {
		t.Fatalf("child process %d is still running after stop", childPID)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	cmd := exec.Command("python3", "-c", "import time; time.sleep(120)")
	s := New(cmd)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := s.Stop(500 * time.Millisecond); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := s.Stop(500 * time.Millisecond); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

func TestTrapSignalsStopsProcess(t *testing.T) {
	cmd := exec.Command("python3", "-c", "import time; time.sleep(120)")
	s := New(cmd)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := s.PID()

	untrap := s.TrapSignals(500*time.Millisecond, nil, syscall.SIGUSR1)
	defer untrap()

	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("send signal: %v", err)
	}

	time.Sleep(400 * time.Millisecond)
	if processExists(pid) {
		t.Fatalf("process %d still running after trapped signal cleanup", pid)
	}
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}
