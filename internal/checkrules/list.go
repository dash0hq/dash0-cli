package checkrules

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags asset.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short: "List check rules",
		Long: `List all check rules (alerting rules) in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List check rules (default: up to 50)
  dash0 check-rules list

  # Output as YAML for backup or version control
  dash0 check-rules list -o yaml > check-rules.yaml

  # Output as JSON for scripting
  dash0 check-rules list -o json

  # List without the header row (pipe-friendly)
  dash0 check-rules list --skip-header`,
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

	iter := apiClient.ListCheckRulesIter(ctx, client.ResolveDataset(ctx, flags.Dataset))

	var rules []*dash0api.PrometheusAlertRuleApiListItem
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
			AssetType: "check rule",
		})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(rules)
	default:
		return printCheckRuleTable(formatter, rules, format, apiUrl)
	}
}

func printCheckRuleTable(f *output.Formatter, rules []*dash0api.PrometheusAlertRuleApiListItem, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			r := item.(*dash0api.PrometheusAlertRuleApiListItem)
			if r.Name != nil {
				return *r.Name
			}
			return ""
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			r := item.(*dash0api.PrometheusAlertRuleApiListItem)
			return r.Id
		}},
	}

	if format == output.FormatWide {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				r := item.(*dash0api.PrometheusAlertRuleApiListItem)
				return string(r.Dataset)
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				r := item.(*dash0api.PrometheusAlertRuleApiListItem)
				if r.Origin != nil {
					return *r.Origin
				}
				return ""
			}},
			output.Column{Header: internal.HEADER_URL, Width: 70, Value: func(item interface{}) string {
				r := item.(*dash0api.PrometheusAlertRuleApiListItem)
				return asset.DeeplinkURL(apiUrl, "check rule", r.Id)
			}},
		)
	}

	if len(rules) == 0 {
		fmt.Println("No check rules found.")
		return nil
	}

	data := make([]interface{}, len(rules))
	for i, r := range rules {
		data[i] = r
	}

	return f.PrintTable(columns, data)
}
