package recordingrulegroups

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

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	iter := apiClient.ListRecordingRuleGroupsIter(ctx, dataset)

	// The list endpoint returns full definitions — no second fetch needed.
	var groups []*dash0api.RecordingRuleGroupDefinition
	count := 0
	for iter.Next() {
		groups = append(groups, iter.Current())
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
	case output.FormatJSON, output.FormatYAML:
		definitions := make([]interface{}, len(groups))
		for i, g := range groups {
			definitions[i] = g
		}
		if format == output.FormatYAML {
			return formatter.PrintMultiDocYAML(definitions)
		}
		return formatter.PrintJSON(definitions)
	default:
		return printRecordingRuleGroupTable(formatter, groups, format)
	}
}

func printRecordingRuleGroupTable(f *output.Formatter, groups []*dash0api.RecordingRuleGroupDefinition, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			g := item.(*dash0api.RecordingRuleGroupDefinition)
			return asset.ExtractRecordingRuleGroupName(g)
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			g := item.(*dash0api.RecordingRuleGroupDefinition)
			return asset.ExtractRecordingRuleGroupID(g)
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: "ENABLED", Width: 7, Value: func(item interface{}) string {
				g := item.(*dash0api.RecordingRuleGroupDefinition)
				if g.Spec.Enabled {
					return "true"
				}
				return "false"
			}},
			output.Column{Header: "INTERVAL", Width: 10, Value: func(item interface{}) string {
				g := item.(*dash0api.RecordingRuleGroupDefinition)
				return string(g.Spec.Interval)
			}},
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				g := item.(*dash0api.RecordingRuleGroupDefinition)
				if g.Metadata.Labels != nil && g.Metadata.Labels.Dash0Comdataset != nil {
					return *g.Metadata.Labels.Dash0Comdataset
				}
				return ""
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				g := item.(*dash0api.RecordingRuleGroupDefinition)
				if g.Metadata.Labels != nil && g.Metadata.Labels.Dash0Comorigin != nil {
					return *g.Metadata.Labels.Dash0Comorigin
				}
				return ""
			}},
		)
	}

	if len(groups) == 0 {
		fmt.Println("No recording rules found.")
		return nil
	}

	data := make([]interface{}, len(groups))
	for i, g := range groups {
		data[i] = g
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
