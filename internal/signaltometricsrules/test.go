package signaltometricsrules

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

type testFlags struct {
	ApiUrl    string
	AuthToken string
	Dataset   string
	Output    string
	File      string
	From      string
	To        string
}

type testFormat string

const (
	testFormatTable testFormat = "table"
	testFormatJSON  testFormat = "json"
)

func parseTestFormat(s string) (testFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return testFormatJSON, nil
		}
		return testFormatTable, nil
	case "table":
		return testFormatTable, nil
	case "json":
		return testFormatJSON, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json)", s)
	}
}

func newTestCmd() *cobra.Command {
	var flags testFlags

	cmd := &cobra.Command{
		Use:   "test -f <file>",
		Short: "Validate a signal-to-metrics rule and count the signals it matches",
		Long: `Validate a signal-to-metrics rule definition and count the logs or spans its
match filter selects over a time window. Nothing is created or changed. Use
'-f -' to read from stdin.

A definition that fails validation returns an error listing every problem the
Dash0 API found, so use this command before 'create' or 'apply' to check a new
rule.` + internal.CONFIG_HINT,
		Example: `  # Count the signals a rule matches over the last 15 minutes
  dash0 s2m test -f rule.yaml

  # Count over the last hour
  dash0 s2m test -f rule.yaml --from now-1h

  # Test the definition of an existing rule
  dash0 s2m get <origin-or-id> -o yaml | dash0 s2m test -f -

  # Output as JSON
  dash0 s2m test -f rule.yaml -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTest(cmd.Context(), &flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset to test against")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table or json (default: table; json in agent mode)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().StringVar(&flags.From, "from", "now-15m", "Start of time range (e.g. now-1h, 2024-01-25T10:00:00.000Z)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (e.g. now, 2024-01-25T11:00:00.000Z)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runTest(ctx context.Context, flags *testFlags) error {
	format, err := parseTestFormat(flags.Output)
	if err != nil {
		return err
	}

	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read signal-to-metrics rule definition: %w", err)
	}

	rule, err := decode(raw)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// The server's own fields mean nothing to a simulation, and the dataset
	// label would conflict with --dataset when they differ.
	dash0api.StripSignalToMetricsServerFields(rule)

	request := &dash0api.SignalToMetricsTestRequest{
		Definition: *rule,
		TimeRange: &dash0api.TimeReferenceRange{
			From: dash0api.TimeReferenceFromString(query.NormalizeTimestamp(flags.From)),
			To:   dash0api.TimeReferenceFromString(query.NormalizeTimestamp(flags.To)),
		},
	}

	result, err := apiClient.TestSignalToMetrics(ctx, request, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if dash0api.IsBadRequest(err) {
			return fmt.Errorf("%s %q is not valid:\n  %w\nHint: fix the fields the error names, then run the test again",
				assetType, dash0api.GetSignalToMetricsName(rule), err)
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetName: dash0api.GetSignalToMetricsName(rule),
		})
	}

	if format == testFormatJSON {
		return output.NewFormatter(output.FormatJSON, os.Stdout).PrintJSON(result)
	}

	fmt.Printf("%s %q matched %d %s\n", displayKind, dash0api.GetSignalToMetricsName(rule), result.MatchedCount, signal(rule))
	fmt.Printf("From: %s\n", result.TimeRange.From.UTC().Format(time.RFC3339))
	fmt.Printf("To: %s\n", result.TimeRange.To.UTC().Format(time.RFC3339))
	return nil
}
