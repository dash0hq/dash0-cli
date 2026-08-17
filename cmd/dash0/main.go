package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/apply"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	"github.com/dash0hq/dash0-cli/internal/failedchecks"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/diff"
	"github.com/dash0hq/dash0-cli/internal/help"
	"github.com/dash0hq/dash0-cli/internal/logging"
	"github.com/dash0hq/dash0-cli/internal/login"
	"github.com/dash0hq/dash0-cli/internal/members"
	"github.com/dash0hq/dash0-cli/internal/metrics"
	"github.com/dash0hq/dash0-cli/internal/notificationchannels"
	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/dash0hq/dash0-cli/internal/rawapi"
	"github.com/dash0hq/dash0-cli/internal/recordingrules"
	"github.com/dash0hq/dash0-cli/internal/skill"
	"github.com/dash0hq/dash0-cli/internal/spamfilters"
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
	rootCmd.AddCommand(rawapi.NewAPICmd())
	rootCmd.AddCommand(checkrules.NewCheckRulesCmd())
	rootCmd.AddCommand(failedchecks.NewFailedChecksCmd())
	rootCmd.AddCommand(config.NewConfigCmd())
	rootCmd.AddCommand(dashboards.NewDashboardsCmd())
	rootCmd.AddCommand(diff.NewDiffCmd())
	rootCmd.AddCommand(logging.NewLogsCmd())
	rootCmd.AddCommand(login.NewLoginCmd())
	rootCmd.AddCommand(login.NewLogoutCmd())
	rootCmd.AddCommand(members.NewMembersCmd())
	rootCmd.AddCommand(metrics.NewMetricsCmd())
	rootCmd.AddCommand(notificationchannels.NewNotificationChannelsCmd())
	rootCmd.AddCommand(otlp.NewOtlpCmd())
	rootCmd.AddCommand(recordingrules.NewRecordingRulesCmd())
	rootCmd.AddCommand(skill.NewSkillCmd())
	rootCmd.AddCommand(spamfilters.NewSpamFiltersCmd())
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
	rootCmd.PersistentFlags().String("profile", "", "Profile to use for this invocation; overrides the active profile on disk (env: DASH0_PROFILE)")
	rootCmd.PersistentFlags().String("max-retries", "", "Maximum number of retries for failed API requests (0-5; default: 3; env: DASH0_MAX_RETRIES)")
	rootCmd.PersistentFlags().Bool("no-skill-hint", false, "Suppress the agent-mode error hint pointing at dash0 skill install / dash0 skill show (env: DASH0_NO_SKILL_HINT)")
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
	err = withSkillHint(err)

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

// withSkillHint appends a follow-up hint to err so an AI agent (or a human
// setting up their AI coding session) has a next-step pointer even when
// the underlying error message is bare. Two shapes, both leading with
// `dash0 skill show` (no arg) — the invocation that prints the entry point
// with the full topic index, which is what an agent needs to discover what
// topics even exist before it can pass one:
//
//   - When the skill is NOT installed in the current directory: nudge at
//     `dash0 skill show` (disk-free, immediate — the discovery workflow
//     for ephemeral or read-only agent sessions) and, secondarily, at
//     `dash0 skill install` (persistent, one-time setup). Fires in both
//     agent and human mode since the setup benefits humans too.
//   - When the skill IS installed and agent mode is active: nudge at
//     `dash0 skill show` for the topic index, plus `dash0 skill show
//     <topic>` for a focused reference, and `dash0 --agent-mode --help`
//     as a fallback for the current flag surface. Fires only in agent
//     mode because a human seeing this on every error is noise (they'd
//     read `--help`).
//
// No-op when the hint has been suppressed (`--no-skill-hint` /
// `DASH0_NO_SKILL_HINT`), when err already carries its own `\nHint:`
// (e.g. the OAuth-empty-profile hint) rather than stacking a second
// less-specific one, and when the detected agent host is not a supported
// install target — running `dash0 skill install` under aider/cline/
// windsurf/mcp would only surface a second, more specific error.
func withSkillHint(err error) error {
	if skillHintSuppressed() {
		return err
	}
	if strings.Contains(err.Error(), "\nHint:") {
		return err
	}
	if slug := agentmode.DetectAgentSlug(); slug != "" && !skill.HostSupported(slug) {
		return err
	}
	wd, wdErr := os.Getwd()
	if wdErr != nil {
		return err
	}
	if skill.IsInstalled(wd) {
		if !agentmode.Enabled {
			return err
		}
		return fmt.Errorf("%w\nHint: consult the installed dash0-cli Agent Skill — start with `dash0 skill show` (no arg) to reprint the entry point, which lists every available topic, then `dash0 skill show <topic>` (e.g. `dash0 skill show dashboards`) for a focused reference; or `dash0 --agent-mode --help` for the current flag surface", err)
	}
	return fmt.Errorf("%w\nHint: start with `dash0 skill show` (no arg) to print the entry point, which lists every available topic, then `dash0 skill show <topic>` (e.g. `dash0 skill show dashboards`) for a focused reference — no files are written; or run `dash0 skill install` to add a persistent Agent Skills bundle to this project", err)
}

// skillHintSuppressed reports whether the --no-skill-hint flag or
// DASH0_NO_SKILL_HINT env var is set. Flags are not yet parsed when
// printError may be called, so this scans os.Args directly.
//
// Precedence, highest to lowest:
//  1. Explicit CLI value (--no-skill-hint=false or =0) — never suppressed.
//  2. Bareword CLI flag (--no-skill-hint) or --no-skill-hint=true|1 — suppressed.
//  3. DASH0_NO_SKILL_HINT=0|false — never suppressed (explicit env disable).
//  4. DASH0_NO_SKILL_HINT=1|true — suppressed.
//  5. Otherwise — not suppressed.
func skillHintSuppressed() bool {
	if v, ok := flagBoolValue(os.Args[1:], "--no-skill-hint"); ok {
		return v
	}
	envVal := strings.ToLower(os.Getenv("DASH0_NO_SKILL_HINT"))
	if envVal == "0" || envVal == "false" {
		return false
	}
	return envVal == "1" || envVal == "true"
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

// flagValue returns the value of the named long-form flag in args, or empty
// string if it is not present. It supports both `--name value` and
// `--name=value` forms. It stops scanning after "--" (end of flags).
// This is used before cobra has parsed flags, so we scan manually.
func flagValue(args []string, name string) string {
	prefix := "--" + name
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return ""
		}
		if arg == prefix {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(arg, prefix+"=") {
			return strings.TrimPrefix(arg, prefix+"=")
		}
	}
	return ""
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
	// Wire SIGINT/SIGTERM cancellation into the root context. Commands that
	// pass the context to network calls (OAuth login, API requests) will see
	// `ctx.Err() == context.Canceled` on Ctrl-C, return cleanly, and run
	// their deferred cleanups (listener close, file handles, partial-state
	// compensation). Without this, the runtime kills the process on first
	// SIGINT and deferred cleanups are skipped — most visibly for `dash0
	// login`, which would leave the callback HTTP server bound to a port
	// until the OS reclaims the socket. The second SIGINT (or one after
	// `stop()`) gets the default behavior so a wedged command is still
	// killable.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Command.Traverse decides whether a "--name value"-shaped argument is a
	// flag (consuming the next token) or a bareword by looking up the flag
	// in c.Flags() — which does NOT include persistent flags registered via
	// PersistentFlags() until cobra's own (unexported) mergePersistentFlags
	// runs, which normally happens lazily during Execute, i.e. after this
	// pre-flight Traverse call. Without this merge, a boolean persistent
	// flag preceding the subcommand (e.g. `dash0 --experimental diff ...`,
	// the form used throughout this CLI's own docs and examples) is
	// wrongly treated as expecting a value, which swallows the subcommand
	// name as that value and makes Traverse return the root command
	// instead of the real target. Replicating the merge here first (a
	// public, idempotent equivalent of cobra's own step) fixes that. See
	// TestTraverseTargetCommand in main_test.go for a regression test
	// against an isolated command tree (rootCmd itself is a package-level
	// singleton that Execute() mutates as a side effect, which would mask
	// this bug in a test that reused rootCmd after any earlier Execute call).
	rootCmd.Flags().AddFlagSet(rootCmd.PersistentFlags())

	// Determine which command will be executed (best-effort; Traverse can
	// still fall back to the root command for shapes it doesn't model,
	// e.g. an unrecognized flag).
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

	// Propagate the resolved color state into internal/otlp. The proxy's
	// `--tail` renderer reuses the same semantic palette as `logs query`
	// but cannot import internal/color directly (cycle: color → otlp for
	// the severity range type), so main bridges the value here.
	otlp.SetTailColorEnabled(!dashcolor.NoColor)

	// Resolve the per-invocation profile selector (--profile flag or
	// DASH0_PROFILE env var) before loading config so the selection flows
	// into both the loaded configuration and the context consumed by
	// `config show`.
	selector := config.ResolveProfileSelector(flagValue(os.Args[1:], "profile"))
	// login/logout resolve the target profile themselves (login may create a
	// missing profile; logout produces its own profile-not-found message).
	// Skip the pre-resolve so `dash0 login --profile <new>` can reach runLogin.
	skipProfileResolve := targetCmd != nil && (targetCmd.Name() == "login" || targetCmd.Name() == "logout")
	if selector.IsSet() && !skipProfileResolve {
		cfg, err := config.ResolveConfigurationForProfile(selector.Name)
		if err != nil {
			printError(err)
			os.Exit(1)
		}
		ctx = profiles.WithConfiguration(ctx, cfg)
	} else if !selector.IsSet() {
		if cfg := loadConfig(); cfg != nil {
			// Always attempt to load configuration. Commands that don't need it
			// (help, version, config) simply ignore it. Commands that do need it
			// will fail with a clear error if the required values are missing.
			ctx = profiles.WithConfiguration(ctx, cfg)
		}
	}
	ctx = config.WithProfileSelector(ctx, selector)

	// Resolve --max-retries before cobra parses flags (same approach as --profile).
	if raw := flagValue(os.Args[1:], "max-retries"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			printError(fmt.Errorf("invalid --max-retries value %q: must be an integer", raw))
			os.Exit(1)
		}
		ctx = client.WithMaxRetries(ctx, &v)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		cmdName := ""
		if targetCmd != nil {
			cmdName = targetCmd.Name()
		}

		// A *diff.PendingDifferencesError is not a failure -- the diff
		// report was already printed to stdout/stderr by the time it's
		// returned, so it must never be rendered through the
		// "Error:"-prefixed path (and usage must not be printed either).
		if !errors.As(err, new(*diff.PendingDifferencesError)) {
			printError(err)
			// Show usage only for flag/argument errors, not for runtime errors.
			// Commands set SilenceUsage = true once past flag validation.
			if !agentmode.Enabled && targetCmd != nil && targetCmd.Name() != "dash0" && !targetCmd.SilenceUsage {
				fmt.Fprintln(os.Stderr)
				_ = targetCmd.Usage()
			}
		}
		os.Exit(exitCodeForError(cmdName, err))
	}
}

// exitCodeForError determines the process exit code for a command whose
// RunE returned a non-nil err. Every command exits 1 on error except dash0
// diff, which uses a three-way exit code (0 clean, 1 differences pending, 2
// genuine error) instead of this CLI's uniform 0/1 convention -- modeled on
// `kubectl diff`, so a naive CI step doesn't fail on the routine "changes
// pending" case, but still fails hard on a genuine error (bad --since ref,
// API unreachable, and so on).
func exitCodeForError(cmdName string, err error) int {
	if errors.As(err, new(*diff.PendingDifferencesError)) {
		return 1
	}
	if cmdName == "diff" {
		return 2
	}
	return 1
}

// installJSONHelp replaces the default help function on cmd and all
// subcommands so that --help produces JSON output.
func installJSONHelp(cmd *cobra.Command) {
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = help.PrintJSONHelp(os.Stdout, cmd)
	})
}

// hasFlag reports whether a boolean flag (e.g. "--agent-mode") is set to a
// truthy value in args. Recognizes the bareword form and every value form
// cobra itself accepts: --name, --name=true, --name=1 are truthy;
// --name=false, --name=0 are false. This is used before cobra has parsed
// flags, so we scan manually.
func hasFlag(args []string, name string) bool {
	v, ok := flagBoolValue(args, name)
	return ok && v
}

// flagBoolValue returns (value, explicit) for a boolean flag. explicit is
// true when the flag was passed in any form (bareword, =true, =false, =1,
// =0); value carries the resolved truthiness. Callers that need to
// distinguish "flag not passed" from "flag passed explicitly with false"
// (e.g. to let an explicit --flag=false trump an env-var setting) can
// branch on explicit. Scanning stops after "--" (end of flags).
func flagBoolValue(args []string, name string) (bool, bool) {
	prefix := name + "="
	for _, arg := range args {
		if arg == "--" {
			return false, false
		}
		if arg == name {
			return true, true
		}
		if strings.HasPrefix(arg, prefix) {
			v := strings.ToLower(arg[len(prefix):])
			switch v {
			case "", "true", "1":
				return true, true
			case "false", "0":
				return false, true
			default:
				// Unknown value: treat as not-set so the caller falls
				// through to lower-precedence sources (env var, default).
				return false, false
			}
		}
	}
	return false, false
}
