package notificationchannels

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
)

type listFlags struct {
	ApiUrl     string
	AuthToken  string
	Output     string
	SkipHeader bool
	Column     []string
}

// listFormat represents the output format for notification channel list.
type listFormat string

const (
	listFormatTable listFormat = "table"
	listFormatJSON  listFormat = "json"
	listFormatYAML  listFormat = "yaml"
	listFormatCSV   listFormat = "csv"
)

var notificationChannelListDefaultColumns = []query.ColumnDef{
	{Key: "name", Aliases: []string{"channel name"}, Header: internal.HEADER_NAME, Width: 30},
	{Key: "type", Aliases: []string{"channel type"}, Header: internal.HEADER_TYPE, Width: 15},
	{Key: "id", Aliases: []string{"channel id"}, Header: internal.HEADER_ID, Width: 36},
	{Key: "origin", Header: internal.HEADER_ORIGIN, Width: 20},
}

func parseListFormat(s string) (listFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return listFormatJSON, nil
		}
		return listFormatTable, nil
	case "table":
		return listFormatTable, nil
	case "json":
		return listFormatJSON, nil
	case "yaml", "yml":
		return listFormatYAML, nil
	case "csv":
		return listFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml, csv)", s)
	}
}

func newListCmd() *cobra.Command {
	flags := &listFlags{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "[experimental] List notification channels",
		Long:    `List all notification channels in your Dash0 organization.` + internal.CONFIG_HINT,
		Example: `  # List all notification channels
  dash0 --experimental notification-channels list

  # Output as JSON
  dash0 --experimental notification-channels list -o json

  # Output as YAML
  dash0 --experimental notification-channels list -o yaml

  # Output as CSV
  dash0 --experimental notification-channels list -o csv

  # Show only specific columns
  dash0 --experimental notification-channels list --column name --column type`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runList(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, yaml, csv (default: table)")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table and CSV output")
	cmd.Flags().StringArrayVar(&flags.Column, "column", nil, "Column to display (alias or attribute key; repeatable; table and CSV only)")

	return cmd
}

func runList(cmd *cobra.Command, flags *listFlags) error {
	ctx := cmd.Context()

	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	if err := query.ValidateColumnFormat(flags.Column, flags.Output); err != nil {
		return err
	}

	format, err := parseListFormat(flags.Output)
	if err != nil {
		return err
	}

	cols, err := resolveListColumns(flags.Column)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	iter := apiClient.ListNotificationChannelsIter(ctx)

	var items []*dash0api.NotificationChannelDefinition
	for iter.Next() {
		items = append(items, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: "notification channel"})
	}

	switch format {
	case listFormatJSON:
		return renderNotificationChannelsJSON(items)
	case listFormatYAML:
		return renderNotificationChannelsYAML(items)
	case listFormatTable:
		return renderNotificationChannelsTable(items, cols, flags.SkipHeader)
	case listFormatCSV:
		return renderNotificationChannelsCSV(items, cols, flags.SkipHeader)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func resolveListColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return notificationChannelListDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, notificationChannelListDefaultColumns), nil
}

func channelValues(item *dash0api.NotificationChannelDefinition) map[string]string {
	origin := dash0api.GetNotificationChannelOrigin(item)
	if origin == "" {
		origin = "-"
	}
	return map[string]string{
		"name":   dash0api.GetNotificationChannelName(item),
		"type":   string(item.Spec.Type),
		"id":     dash0api.GetNotificationChannelID(item),
		"origin": origin,
	}
}

func renderNotificationChannelsJSON(items []*dash0api.NotificationChannelDefinition) error {
	if items == nil {
		items = []*dash0api.NotificationChannelDefinition{}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(items)
}

func renderNotificationChannelsYAML(items []*dash0api.NotificationChannelDefinition) error {
	if len(items) == 0 {
		fmt.Println("No notification channels found.")
		return nil
	}
	for i, item := range items {
		if i > 0 {
			fmt.Println("---")
		}
		data, err := sigsyaml.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal notification channel as YAML: %w", err)
		}
		fmt.Print(string(data))
	}
	return nil
}

func renderNotificationChannelsTable(items []*dash0api.NotificationChannelDefinition, cols []query.ColumnDef, skipHeader bool) error {
	if len(items) == 0 {
		fmt.Println("No notification channels found.")
		return nil
	}
	var rows []map[string]string
	for _, item := range items {
		rows = append(rows, channelValues(item))
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func renderNotificationChannelsCSV(items []*dash0api.NotificationChannelDefinition, cols []query.ColumnDef, skipHeader bool) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
	}
	for _, item := range items {
		values := channelValues(item)
		if err := query.WriteCSVRow(w, cols, values); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
