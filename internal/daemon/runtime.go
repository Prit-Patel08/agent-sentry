package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	envRuntimeDir = "FLOWFORGE_DAEMON_DIR"
	defaultSubdir = ".flowforge/daemon"
)

type RuntimePaths struct {
	Dir       string
	PIDFile   string
	LogFile   string
	StateFile string
}

type State struct {
	PID       int       `json:"pid"`
	Port      string    `json:"port"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

func ResolveRuntimeDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(envRuntimeDir)); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve daemon runtime dir: %w", err)
	}
	return filepath.Join(home, defaultSubdir), nil
}

func Paths() (RuntimePaths, error) {
	dir, err := ResolveRuntimeDir()
	if err != nil {
		return RuntimePaths{}, err
	}
	return RuntimePaths{
		Dir:       dir,
		PIDFile:   filepath.Join(dir, "flowforge-daemon.pid"),
		LogFile:   filepath.Join(dir, "flowforge-daemon.log"),
		StateFile: filepath.Join(dir, "flowforge-daemon.state.json"),
	}, nil
}

func EnsureRuntimeDir() (RuntimePaths, error) {
	paths, err := Paths()
	if err != nil {
		return RuntimePaths{}, err
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return RuntimePaths{}, fmt.Errorf("create daemon runtime dir: %w", err)
	}
	return paths, nil
}

func ReadPID(paths RuntimePaths) (int, error) {
	raw, err := os.ReadFile(paths.PIDFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %s", paths.PIDFile)
	}
	return pid, nil
}

func WritePID(paths RuntimePaths, pid int) error {
	if pid <= 0 {
		return errors.New("pid must be > 0")
	}
	return writeAtomic(paths.PIDFile, []byte(fmt.Sprintf("%d\n", pid)), 0o600)
}

func RemovePID(paths RuntimePaths) error {
	if err := os.Remove(paths.PIDFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ReadState(paths RuntimePaths) (State, error) {
	raw, err := os.ReadFile(paths.StateFile)
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

func WriteState(paths RuntimePaths, st State) error {
	blob, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	blob = append(blob, '\n')
	return writeAtomic(paths.StateFile, blob, 0o600)
}

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
