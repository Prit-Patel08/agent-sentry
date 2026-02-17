package cmd

import (
	"agent-sentry/internal/database"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cleanDays int
var forceClean bool

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Prune old logs and optimize database",
	Long: `Removes incidents older than the specified number of days and runs a VACUUM to reclaim disk space.
This prevents the SQLite database from growing indefinitely.

Example:
  sentry clean --days 30
  sentry clean --days 0 --force  # Dangerous: Wipes everything
`,
	Run: func(cmd *cobra.Command, args []string) {
		if cleanDays < 0 {
			fmt.Println("Error: --days cannot be negative.")
			os.Exit(1)
		}

		if cleanDays == 0 && !forceClean {
			fmt.Println("Error: To delete ALL logs (--days 0), you must use --force.")
			os.Exit(1)
		}

		fmt.Printf("🧹 Cleaning logs older than %d days...\n", cleanDays)

		if err := database.InitDB(); err != nil {
			fmt.Printf("Error: Failed to connect to database: %v\n", err)
			os.Exit(1)
		}
		defer database.CloseDB()

		deleted, err := database.PruneIncidents(cleanDays)
		if err != nil {
			fmt.Printf("Error during pruning: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Cleanup complete. Deleted %d records.\n", deleted)
		fmt.Println("database vacuumed and optimized.")
	},
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().IntVar(&cleanDays, "days", 30, "Delete logs older than N days")
	cleanCmd.Flags().BoolVar(&forceClean, "force", false, "Force deletion without confirmation (required for --days 0)")
}
