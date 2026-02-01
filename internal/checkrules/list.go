package checkrules

import (
	"context"
	"fmt"
	"os"

	dash0 "github.com/dash0hq/dash0-api-client-go"
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
		Short:   "List check rules",
		Long:    `List all check rules (alerting rules) in the specified dataset`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	iter := apiClient.ListCheckRulesIter(ctx, client.DatasetPtr(flags.Dataset))

	var rules []*dash0.PrometheusAlertRuleApiListItem
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

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(rules)
	default:
		return printCheckRuleTable(formatter, rules, format)
	}
}

func printCheckRuleTable(f *output.Formatter, rules []*dash0.PrometheusAlertRuleApiListItem, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			r := item.(*dash0.PrometheusAlertRuleApiListItem)
			if r.Name != nil {
				return *r.Name
			}
			return ""
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			r := item.(*dash0.PrometheusAlertRuleApiListItem)
			return r.Id
		}},
	}

	if format == output.FormatWide {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				r := item.(*dash0.PrometheusAlertRuleApiListItem)
				return string(r.Dataset)
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 30, Value: func(item interface{}) string {
				r := item.(*dash0.PrometheusAlertRuleApiListItem)
				if r.Origin != nil {
					return *r.Origin
				}
				return ""
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
