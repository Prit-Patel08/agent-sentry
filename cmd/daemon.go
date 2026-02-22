package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"flowforge/internal/api"
	"flowforge/internal/daemon"

	"github.com/spf13/cobra"
)

const (
	defaultDaemonPort = "8080"
)

var (
	daemonPort          string
	daemonWaitSeconds   int
	daemonStopTimeout   int
	daemonStatusJSON    bool
	daemonLogsLines     int
	daemonLogsFollow    bool
	daemonRunHiddenPort string
)

type daemonStartResult struct {
	PID            int
	AlreadyRunning bool
}

type daemonStatusReport struct {
	Status       string `json:"status"`
	PID          int    `json:"pid"`
	APIHealthy   bool   `json:"api_healthy"`
	Port         string `json:"port"`
	RuntimeDir   string `json:"runtime_dir"`
	PIDFile      string `json:"pid_file"`
	LogFile      string `json:"log_file"`
	StatePresent bool   `json:"state_present"`
	StartedAt    string `json:"started_at,omitempty"`
	StoppedAt    string `json:"stopped_at,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the local FlowForge background daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the local FlowForge daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		wait := time.Duration(daemonWaitSeconds) * time.Second
		result, err := ensureDaemonRunning(daemonPort, wait)
		if err != nil {
			return err
		}
		if result.AlreadyRunning {
			fmt.Printf("FlowForge daemon already running (pid=%d) on http://127.0.0.1:%s\n", result.PID, daemonPort)
			return nil
		}
		fmt.Printf("FlowForge daemon started (pid=%d) on http://127.0.0.1:%s\n", result.PID, daemonPort)
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the local FlowForge daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopDaemon(daemonPort, time.Duration(daemonStopTimeout)*time.Second)
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := collectDaemonStatus(daemonPort)
		if err != nil {
			return err
		}
		if daemonStatusJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}

		fmt.Printf("Status: %s\n", strings.ToUpper(report.Status))
		fmt.Printf("API: http://127.0.0.1:%s (healthy=%v)\n", report.Port, report.APIHealthy)
		if report.PID > 0 {
			fmt.Printf("PID: %d\n", report.PID)
		}
		fmt.Printf("Runtime dir: %s\n", report.RuntimeDir)
		fmt.Printf("PID file: %s\n", report.PIDFile)
		fmt.Printf("Log file: %s\n", report.LogFile)
		if report.StatePresent && report.StartedAt != "" {
			fmt.Printf("Started at: %s\n", report.StartedAt)
		}
		if report.StatePresent && report.StoppedAt != "" {
			fmt.Printf("Last stopped at: %s\n", report.StoppedAt)
		}
		if report.LastError != "" {
			fmt.Printf("Last error: %s\n", report.LastError)
		}
		return nil
	},
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show daemon logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := daemon.Paths()
		if err != nil {
			return err
		}
		lines, err := tailFileLines(paths.LogFile, daemonLogsLines)
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		if !daemonLogsFollow {
			return nil
		}
		return followFile(paths.LogFile)
	},
}

var daemonRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run daemon process (internal use)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon(daemonRunHiddenPort)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
	daemonCmd.AddCommand(daemonRunCmd)

	daemonStartCmd.Flags().StringVar(&daemonPort, "port", defaultDaemonPort, "API port to serve from daemon")
	daemonStartCmd.Flags().IntVar(&daemonWaitSeconds, "wait-seconds", 10, "seconds to wait for health after start")

	daemonStopCmd.Flags().StringVar(&daemonPort, "port", defaultDaemonPort, "API port expected for daemon")
	daemonStopCmd.Flags().IntVar(&daemonStopTimeout, "timeout-seconds", 10, "seconds to wait for graceful stop before force kill")

	daemonStatusCmd.Flags().StringVar(&daemonPort, "port", defaultDaemonPort, "API port expected for daemon")
	daemonStatusCmd.Flags().BoolVar(&daemonStatusJSON, "json", false, "output daemon status as JSON")

	daemonLogsCmd.Flags().IntVar(&daemonLogsLines, "lines", 120, "number of log lines to show")
	daemonLogsCmd.Flags().BoolVar(&daemonLogsFollow, "follow", false, "follow log output")

	daemonRunCmd.Flags().StringVar(&daemonRunHiddenPort, "port", defaultDaemonPort, "API port to serve from daemon")
	_ = daemonRunCmd.Flags().MarkHidden("port")
}

func ensureDaemonRunning(port string, wait time.Duration) (daemonStartResult, error) {
	if wait <= 0 {
		wait = 5 * time.Second
	}
	paths, err := daemon.EnsureRuntimeDir()
	if err != nil {
		return daemonStartResult{}, err
	}

	if pid, err := daemon.ReadPID(paths); err == nil {
		if daemon.ProcessAlive(pid) {
			if err := waitForDaemonReady(port, pid, wait); err != nil {
				return daemonStartResult{}, fmt.Errorf("daemon pid=%d present but not healthy: %w", pid, err)
			}
			return daemonStartResult{PID: pid, AlreadyRunning: true}, nil
		}
		_ = daemon.RemovePID(paths)
	}

	exe, err := os.Executable()
	if err != nil {
		return daemonStartResult{}, fmt.Errorf("resolve executable path: %w", err)
	}

	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return daemonStartResult{}, fmt.Errorf("open daemon log: %w", err)
	}
	defer logFile.Close()

	child := exec.Command(exe, "daemon", "run", "--port", port)
	child.Stdout = logFile
	child.Stderr = logFile
	child.Stdin = nil
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := child.Start(); err != nil {
		return daemonStartResult{}, fmt.Errorf("start daemon: %w", err)
	}
	childPID := child.Process.Pid
	_ = child.Process.Release()

	if err := waitForDaemonReady(port, childPID, wait); err != nil {
		return daemonStartResult{}, err
	}

	if pid, err := daemon.ReadPID(paths); err == nil && pid > 0 {
		return daemonStartResult{PID: pid, AlreadyRunning: false}, nil
	}
	return daemonStartResult{PID: childPID, AlreadyRunning: false}, nil
}

func runDaemon(port string) error {
	paths, err := daemon.EnsureRuntimeDir()
	if err != nil {
		return err
	}

	pid := os.Getpid()
	if existingPID, err := daemon.ReadPID(paths); err == nil && existingPID != pid && daemon.ProcessAlive(existingPID) {
		return fmt.Errorf("daemon already running with pid=%d", existingPID)
	}

	startedAt := time.Now().UTC()
	if err := daemon.WritePID(paths, pid); err != nil {
		return fmt.Errorf("write daemon pid file: %w", err)
	}
	if err := daemon.WriteState(paths, daemon.State{
		PID:       pid,
		Port:      port,
		Status:    "running",
		StartedAt: startedAt,
	}); err != nil {
		return fmt.Errorf("write daemon state: %w", err)
	}

	finalStatus := "stopped"
	finalErr := ""
	defer func() {
		_ = daemon.RemovePID(paths)
		_ = daemon.WriteState(paths, daemon.State{
			PID:       0,
			Port:      port,
			Status:    finalStatus,
			StartedAt: startedAt,
			StoppedAt: time.Now().UTC(),
			LastError: finalErr,
		})
	}()

	stop := api.Start(port)
	if err := waitForDaemonReady(port, pid, 5*time.Second); err != nil {
		finalStatus = "failed"
		finalErr = err.Error()
		stop()
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	<-sigCh
	stop()
	return nil
}

func waitForDaemonReady(port string, pid int, wait time.Duration) error {
	if wait <= 0 {
		wait = 5 * time.Second
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if probeDaemonHealth(port, 900*time.Millisecond) {
			return nil
		}
		if pid > 0 && !daemon.ProcessAlive(pid) {
			return fmt.Errorf("daemon process pid=%d exited before readiness", pid)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("daemon health check timed out after %s", wait)
}

func probeDaemonHealth(port string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func collectDaemonStatus(port string) (daemonStatusReport, error) {
	paths, err := daemon.Paths()
	if err != nil {
		return daemonStatusReport{}, err
	}

	report := daemonStatusReport{
		Status:     "stopped",
		Port:       port,
		RuntimeDir: paths.Dir,
		PIDFile:    paths.PIDFile,
		LogFile:    paths.LogFile,
	}

	pid, err := daemon.ReadPID(paths)
	if err == nil {
		report.PID = pid
		if daemon.ProcessAlive(pid) {
			report.Status = "degraded"
		}
	}
	report.APIHealthy = probeDaemonHealth(port, 900*time.Millisecond)

	if report.PID > 0 && report.APIHealthy {
		report.Status = "running"
	}
	if report.PID == 0 && report.APIHealthy {
		report.Status = "external"
	}
	if report.PID > 0 && !daemon.ProcessAlive(report.PID) {
		report.Status = "stale_pid"
	}

	if st, err := daemon.ReadState(paths); err == nil {
		report.StatePresent = true
		if !st.StartedAt.IsZero() {
			report.StartedAt = st.StartedAt.Format(time.RFC3339)
		}
		if !st.StoppedAt.IsZero() {
			report.StoppedAt = st.StoppedAt.Format(time.RFC3339)
		}
		report.LastError = st.LastError
	}
	return report, nil
}

func stopDaemon(port string, timeout time.Duration) error {
	paths, err := daemon.Paths()
	if err != nil {
		return err
	}

	pid, err := daemon.ReadPID(paths)
	if err != nil {
		if os.IsNotExist(err) {
			if probeDaemonHealth(port, 900*time.Millisecond) {
				fmt.Printf("No daemon pid file found, but API on port %s is healthy (likely foreground mode).\n", port)
				return nil
			}
			fmt.Println("FlowForge daemon is not running.")
			return nil
		}
		return err
	}

	if !daemon.ProcessAlive(pid) {
		_ = daemon.RemovePID(paths)
		fmt.Printf("Removed stale daemon pid file for pid=%d\n", pid)
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find daemon process: %w", err)
	}

	fmt.Printf("Stopping FlowForge daemon (pid=%d)...\n", pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !daemon.ProcessAlive(pid) {
			_ = daemon.RemovePID(paths)
			_ = daemon.WriteState(paths, daemon.State{
				PID:       0,
				Port:      port,
				Status:    "stopped",
				StoppedAt: time.Now().UTC(),
			})
			fmt.Println("FlowForge daemon stopped.")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("Graceful stop timed out after %s, forcing kill...\n", timeout)
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("send SIGKILL: %w", err)
	}

	for i := 0; i < 20; i++ {
		if !daemon.ProcessAlive(pid) {
			_ = daemon.RemovePID(paths)
			_ = daemon.WriteState(paths, daemon.State{
				PID:       0,
				Port:      port,
				Status:    "stopped",
				StoppedAt: time.Now().UTC(),
			})
			fmt.Println("FlowForge daemon force-stopped.")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("daemon process is still alive after SIGKILL")
}

func tailFileLines(path string, n int) ([]string, error) {
	if n <= 0 {
		n = 1
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make([]string, 0, n)
	for scanner.Scan() {
		line := scanner.Text()
		if len(lines) < n {
			lines = append(lines, line)
			continue
		}
		copy(lines, lines[1:])
		lines[len(lines)-1] = line
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func followFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			return nil
		case <-ticker.C:
			info, err := f.Stat()
			if err != nil {
				return err
			}
			size := info.Size()
			if size < offset {
				offset = 0
			}
			if size == offset {
				continue
			}
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return err
			}
			if _, err := io.Copy(os.Stdout, io.LimitReader(f, size-offset)); err != nil {
				return err
			}
			offset = size
		}
	}
}
