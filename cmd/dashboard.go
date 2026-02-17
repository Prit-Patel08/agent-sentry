package cmd

import (
	"agent-sentry/internal/api"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

// dashboardCmd represents the dashboard command
var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Start the Agent-Sentry dashboard API",
	Long: `Starts the HTTP API server on port 8080.
This allows the Next.js dashboard to query incident history.
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting Dashboard API...")
		fmt.Println("Frontend initialized in ./dashboard. Run 'npm run dev' there to start the UI.")
		fmt.Println("Opening API endpoint http://localhost:8080/incidents in browser...")

		go func() {
			time.Sleep(500 * time.Millisecond)
			exec.Command("open", "http://localhost:8080/incidents").Start()
		}()

		api.StartServer("8080")
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}
