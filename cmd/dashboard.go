package cmd

import (
	"flowforge/internal/api"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	dashboardPort        string
	dashboardForeground  bool
	dashboardDaemonWaitS int
)

// dashboardCmd represents the dashboard command
var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Ensure the FlowForge dashboard API is running",
	Long: `Ensures the HTTP API server is running for the dashboard.
By default this command starts (or reuses) the local daemon.
Use --foreground to keep the API server attached to the current terminal.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if dashboardForeground {
			fmt.Println("Starting Dashboard API in foreground mode...")
			fmt.Println("Frontend initialized in ./dashboard. Run 'npm run build && npm run start -- -p 3001' for production UI.")
			fmt.Printf("API Status: http://localhost:%s/incidents\n", dashboardPort)
			fmt.Printf("Health: http://localhost:%s/healthz | Metrics: http://localhost:%s/metrics\n", dashboardPort, dashboardPort)
			api.StartServer(dashboardPort)
			return nil
		}

		fmt.Println("Starting Dashboard API...")
		result, err := ensureDaemonRunning(dashboardPort, time.Duration(dashboardDaemonWaitS)*time.Second)
		if err != nil {
			return err
		}
		if result.AlreadyRunning {
			fmt.Printf("FlowForge daemon already running (pid=%d)\n", result.PID)
		} else {
			fmt.Printf("FlowForge daemon started (pid=%d)\n", result.PID)
		}
		fmt.Println("Frontend initialized in ./dashboard. Run 'npm run build && npm run start -- -p 3001' for production UI.")
		fmt.Printf("API Status: http://localhost:%s/incidents\n", dashboardPort)
		fmt.Printf("Health: http://localhost:%s/healthz | Metrics: http://localhost:%s/metrics\n", dashboardPort, dashboardPort)
		fmt.Println("Use 'flowforge daemon status' for runtime status and 'flowforge daemon logs --follow' for live logs.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
	dashboardCmd.Flags().StringVar(&dashboardPort, "port", defaultDaemonPort, "API port for the dashboard backend")
	dashboardCmd.Flags().BoolVar(&dashboardForeground, "foreground", false, "run API in foreground (for scripts and managed process lifecycle)")
	dashboardCmd.Flags().IntVar(&dashboardDaemonWaitS, "wait-seconds", 10, "seconds to wait for daemon health")
}
