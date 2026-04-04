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

  # Output as CSV (pipe-friendly)
  dash0 check-rules list -o csv

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

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	iter := apiClient.ListCheckRulesIter(ctx, dataset)

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
		definitions, err := fetchFullCheckRules(ctx, apiClient, rules, dataset)
		if err != nil {
			return err
		}
		if format == output.FormatYAML {
			return formatter.PrintMultiDocYAML(definitions)
		}
		return formatter.PrintJSON(definitions)
	default:
		return printCheckRuleTable(formatter, rules, format, apiUrl)
	}
}

func fetchFullCheckRules(
	ctx context.Context,
	apiClient dash0api.Client,
	rules []*dash0api.PrometheusAlertRuleApiListItem,
	dataset *string,
) ([]interface{}, error) {
	progress := output.NewProgress("check rules", len(rules))
	defer progress.Done()
	definitions := make([]interface{}, 0, len(rules))
	for i, item := range rules {
		progress.Update(i + 1)
		rule, err := apiClient.GetCheckRule(ctx, item.Id, dataset)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch check rule %q: %w", item.Id, err)
		}
		dash0api.SetCheckRuleIDIfAbsent(rule, item.Id)
		definitions = append(definitions, rule)
	}
	return definitions, nil
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

	if format == output.FormatWide || format == output.FormatCSV {
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
				return asset.DeeplinkURL(apiUrl, "checkrule", r.Id)
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

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
