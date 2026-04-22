package recordingrules

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags asset.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List recording rules",
		Long:    `List all recording rules in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List recording rules (default: up to 50)
  dash0 recording-rules list

  # Output as YAML for backup or version control
  dash0 recording-rules list -o yaml > recording-rules.yaml

  # Output as JSON for scripting
  dash0 recording-rules list -o json

  # Output as CSV (pipe-friendly)
  dash0 recording-rules list -o csv

  # List without the header row (pipe-friendly)
  dash0 recording-rules list --skip-header`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	iter := apiClient.ListRecordingRulesIter(ctx, dataset)

	var rules []*dash0api.RecordingRule
	count := 0
	for iter.Next() {
		rules = append(rules, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule",
		})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(toInterfaceSlice(rules))
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(toInterfaceSlice(rules))
	default:
		return printRecordingRuleTable(formatter, rules, format, apiUrl)
	}
}

func toInterfaceSlice(rules []*dash0api.RecordingRule) []interface{} {
	result := make([]interface{}, len(rules))
	for i, r := range rules {
		result[i] = r
	}
	return result
}

func printRecordingRuleTable(f *output.Formatter, rules []*dash0api.RecordingRule, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			r := item.(*dash0api.RecordingRule)
			return dash0api.GetRecordingRuleName(r)
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			r := item.(*dash0api.RecordingRule)
			return dash0api.GetRecordingRuleID(r)
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				r := item.(*dash0api.RecordingRule)
				return dash0api.GetRecordingRuleDataset(r)
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				r := item.(*dash0api.RecordingRule)
				if r.Metadata.Labels != nil {
					return (*r.Metadata.Labels)["dash0.com/origin"]
				}
				return ""
			}},
		)
	}

	if len(rules) == 0 {
		fmt.Println("No recording rules found.")
		return nil
	}

	data := make([]interface{}, len(rules))
	for i, r := range rules {
		data[i] = r
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
