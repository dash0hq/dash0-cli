package main

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/apply"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/log"
	"github.com/dash0hq/dash0-cli/internal/metrics"
	"github.com/dash0hq/dash0-cli/internal/syntheticchecks"
	"github.com/dash0hq/dash0-cli/internal/views"
	"github.com/spf13/cobra"
)

// Version information (set by build)
var (
	version = "dev"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "dash0ctl",
	Short:   "Dash0 CLI",
	Long:    `Command line interface for interacting with Dash0 services`,
	Version: version,
}

func init() {
	// Setup logging
	log.SetupLogger()

	// Register subcommands
	rootCmd.AddCommand(apply.NewApplyCmd())
	rootCmd.AddCommand(checkrules.NewCheckRulesCmd())
	rootCmd.AddCommand(config.NewConfigCmd())
	rootCmd.AddCommand(dashboards.NewDashboardsCmd())
	rootCmd.AddCommand(metrics.NewMetricsCmd())
	rootCmd.AddCommand(syntheticchecks.NewSyntheticChecksCmd())
	rootCmd.AddCommand(views.NewViewsCmd())

	// Add version command
	rootCmd.AddCommand(newVersionCmd())
}

// newVersionCmd creates a new version command
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Long:  `Display the version and build information for dash0ctl`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("dash0ctl version %s (built on %s)\n", version, date)
		},
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Logger.Error().Err(err).Msg("Command execution failed")
		os.Exit(1)
	}
}
