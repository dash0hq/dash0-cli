package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/apply"
	"github.com/dash0hq/dash0-cli/internal/help"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/logging"
	"github.com/dash0hq/dash0-cli/internal/members"
	"github.com/dash0hq/dash0-cli/internal/metrics"
	"github.com/dash0hq/dash0-cli/internal/notificationchannels"
	"github.com/dash0hq/dash0-cli/internal/syntheticchecks"
	"github.com/dash0hq/dash0-cli/internal/teams"
	"github.com/dash0hq/dash0-cli/internal/tracing"
	versionpkg "github.com/dash0hq/dash0-cli/internal/version"
	"github.com/dash0hq/dash0-cli/internal/views"
	"github.com/spf13/cobra"
)

// Version information (set by build)
var (
	version = "dev"
	date    = "unknown"
)

// colorMode represents the supported color output modes.
type colorMode string

const (
	colorModeSemantic colorMode = "semantic"
	colorModeNone     colorMode = "none"
)

var validColorModes = []colorMode{
	colorModeSemantic,
	colorModeNone,
}

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
	rootCmd.AddCommand(logging.NewLogsCmd())
	rootCmd.AddCommand(members.NewMembersCmd())
	rootCmd.AddCommand(metrics.NewMetricsCmd())
	rootCmd.AddCommand(notificationchannels.NewNotificationChannelsCmd())
	rootCmd.AddCommand(syntheticchecks.NewSyntheticChecksCmd())
	rootCmd.AddCommand(teams.NewTeamsCmd())
	rootCmd.AddCommand(tracing.NewSpansCmd())
	rootCmd.AddCommand(tracing.NewTracesCmd())
	rootCmd.AddCommand(views.NewViewsCmd())

	// Add version command
	rootCmd.AddCommand(newVersionCmd())

	// Global flags
	rootCmd.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	rootCmd.PersistentFlags().String("color", "", `Color mode for output: "semantic" or "none" (env: DASH0_COLOR)`)
	rootCmd.PersistentFlags().Bool("agent-mode", false, "Enable agent mode for AI coding agents (env: DASH0_AGENT_MODE)")
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
	if agentmode.Enabled {
		agentmode.PrintJSONError(os.Stderr, err)
		return
	}

	errStr := err.Error()

	o := dashcolor.StderrOutput()

	errorPrefix := o.String("Error: ").Foreground(o.Color("1")).Bold().String()
	hintPrefix := o.String("Hint:").Foreground(o.Color("6")).String()

	// Check if error contains a hint
	if idx := strings.Index(errStr, "\nHint:"); idx != -1 {
		mainError := errStr[:idx]
		hint := errStr[idx+1:] // Skip the newline

		fmt.Fprint(os.Stderr, errorPrefix)
		fmt.Fprintln(os.Stderr, mainError)
		fmt.Fprint(os.Stderr, hintPrefix)
		fmt.Fprintln(os.Stderr, hint[5:]) // Skip "Hint:" prefix
	} else {
		fmt.Fprint(os.Stderr, errorPrefix)
		fmt.Fprintln(os.Stderr, errStr)
	}
}

// loadConfig attempts to resolve the CLI configuration (active profile +
// environment variable overrides). It returns the resolved configuration on
// success or nil if resolution fails. Errors are intentionally swallowed:
// commands that actually need configuration will fail with a clear error
// when they try to create a client and the required values are missing.
//
// This approach avoids the need to predict which command will run — something
// that cobra's Traverse cannot reliably do when persistent flags like -X
// precede the subcommand name.
func loadConfig() *profiles.Configuration {
	cfg, err := profiles.ResolveConfiguration("", "")
	if err != nil {
		return nil
	}
	return cfg
}

func resolveColorMode() (colorMode, error) {
	flagVal, _ := rootCmd.PersistentFlags().GetString("color")
	raw := flagVal
	if raw == "" {
		raw = os.Getenv("DASH0_COLOR")
	}
	if raw == "" {
		return colorModeSemantic, nil
	}
	mode := colorMode(raw)
	for _, valid := range validColorModes {
		if mode == valid {
			return mode, nil
		}
	}
	names := make([]string, len(validColorModes))
	for i, m := range validColorModes {
		names[i] = fmt.Sprintf("%q", m)
	}
	return "", fmt.Errorf("unknown color mode: %q (valid values: %s)", raw, strings.Join(names, ", "))
}

func main() {
	ctx := context.Background()

	// Determine which command will be executed (best-effort; Traverse may
	// return the root command when persistent flags like -X come first).
	targetCmd, _, _ := rootCmd.Traverse(os.Args[1:])

	// Resolve agent mode before any output.
	// Flags are not yet parsed at this point, so scan os.Args directly.
	agentModeFlag := hasFlag(os.Args[1:], "--agent-mode")
	agentmode.Init(agentModeFlag)

	// In agent mode, force colors off and install a JSON help function.
	if agentmode.Enabled {
		dashcolor.NoColor = true
		installJSONHelp(rootCmd)
	}

	// Resolve and apply the color mode before any output
	colorMode, colorErr := resolveColorMode()
	if colorErr != nil {
		printError(colorErr)
		os.Exit(1)
	}
	if colorMode == colorModeNone {
		dashcolor.NoColor = true
	}

	// Always attempt to load configuration. Commands that don't need it
	// (help, version, config) simply ignore it. Commands that do need it
	// will fail with a clear error if the required values are missing.
	if cfg := loadConfig(); cfg != nil {
		ctx = profiles.WithConfiguration(ctx, cfg)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		printError(err)
		// Show usage only for flag/argument errors, not for runtime errors.
		// Commands set SilenceUsage = true once past flag validation.
		if !agentmode.Enabled && targetCmd != nil && targetCmd.Name() != "dash0" && !targetCmd.SilenceUsage {
			fmt.Fprintln(os.Stderr)
			_ = targetCmd.Usage()
		}
		os.Exit(1)
	}
}

// installJSONHelp replaces the default help function on cmd and all
// subcommands so that --help produces JSON output.
func installJSONHelp(cmd *cobra.Command) {
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = help.PrintJSONHelp(os.Stdout, cmd)
	})
}

// hasFlag checks whether a boolean flag (e.g. "--agent-mode") appears in args.
// This is used before cobra has parsed flags, so we scan manually.
func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
		// Stop scanning after "--" (end of flags).
		if arg == "--" {
			return false
		}
	}
	return false
}
