package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/apply"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/logs"
	"github.com/dash0hq/dash0-cli/internal/metrics"
	"github.com/dash0hq/dash0-cli/internal/syntheticchecks"
	versionpkg "github.com/dash0hq/dash0-cli/internal/version"
	"github.com/dash0hq/dash0-cli/internal/views"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Version information (set by build)
var (
	version = "dev"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "dash0",
	Short:   "Dash0 CLI",
	Long:    `Command line interface for interacting with Dash0 services`,
	Version: version,
	// Customize the printing of error and usage information
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	// Propagate build version to shared package
	versionpkg.Version = version

	// Register subcommands
	rootCmd.AddCommand(apply.NewApplyCmd())
	rootCmd.AddCommand(checkrules.NewCheckRulesCmd())
	rootCmd.AddCommand(config.NewConfigCmd())
	rootCmd.AddCommand(dashboards.NewDashboardsCmd())
	rootCmd.AddCommand(logs.NewLogsCmd())
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
		Long:  `Display the version and build information for dash0`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("dash0 version %s (built on %s)\n", version, date)
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
}

// printError prints an error message with colored prefixes.
// "Error:" is printed in red, "Hint:" is printed in cyan.
// Colors are only used when stderr is a TTY (not piped).
func printError(err error) {
	errStr := err.Error()

	red := color.New(color.FgRed, color.Bold)
	cyan := color.New(color.FgCyan)

	// Check if error contains a hint
	if idx := strings.Index(errStr, "\nHint:"); idx != -1 {
		mainError := errStr[:idx]
		hint := errStr[idx+1:] // Skip the newline

		red.Fprint(os.Stderr, "Error: ")
		fmt.Fprintln(os.Stderr, mainError)
		cyan.Fprint(os.Stderr, "Hint:")
		fmt.Fprintln(os.Stderr, hint[5:]) // Skip "Hint:" prefix
	} else {
		red.Fprint(os.Stderr, "Error: ")
		fmt.Fprintln(os.Stderr, errStr)
	}
}

// needsConfig returns true if the command requires configuration to run
func needsConfig(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	name := cmd.Name()
	if name == "help" || name == "version" || name == "completion" || name == "dash0" {
		return false
	}
	// Walk the command chain to check if "config" is an ancestor that is
	// a direct child of the root command (e.g. "config show", "config profiles create")
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "config" && c.Parent() != nil && c.Parent().Parent() == nil {
			return false
		}
	}
	return true
}

func main() {
	ctx := context.Background()

	// Determine which command will be executed
	targetCmd, _, _ := rootCmd.Traverse(os.Args[1:])

	// Load configuration only for commands that need it
	if needsConfig(targetCmd) {
		cfg, cfgErr := config.ResolveConfiguration("", "")
		if cfgErr != nil {
			// Config errors: print error only, no usage
			printError(cfgErr)
			os.Exit(1)
		}
		if cfg != nil {
			ctx = config.WithConfiguration(ctx, cfg)
		}
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		printError(err)
		// Show usage only for flag/argument errors, not for runtime errors.
		// Commands set SilenceUsage = true once past flag validation.
		if targetCmd != nil && targetCmd.Name() != "dash0" && !targetCmd.SilenceUsage {
			fmt.Fprintln(os.Stderr)
			targetCmd.Usage()
		}
		os.Exit(1)
	}
}
