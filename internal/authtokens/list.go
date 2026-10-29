package authtokens

import (
	"encoding/csv"
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
)

type listFlags struct {
	ApiUrl     string
	AuthToken  string
	Output     string
	SkipHeader bool
	Column     []string
}

type listFormat string

const (
	listFormatTable listFormat = "table"
	listFormatWide  listFormat = "wide"
	listFormatJSON  listFormat = "json"
	listFormatYAML  listFormat = "yaml"
	listFormatCSV   listFormat = "csv"
)

var (
	nameColumn    = query.ColumnDef{Key: "name", Aliases: []string{"token name"}, Header: internal.HEADER_NAME, Width: 30}
	idColumn      = query.ColumnDef{Key: "id", Aliases: []string{"token id"}, Header: internal.HEADER_ID, Width: 36}
	actionsColumn = query.ColumnDef{Key: "actions", Header: "ACTIONS", Width: 24}
	originColumn  = query.ColumnDef{Key: "origin", Header: internal.HEADER_ORIGIN, Width: 30}

	tableColumns = []query.ColumnDef{nameColumn, idColumn, actionsColumn}
	wideColumns  = []query.ColumnDef{nameColumn, idColumn, actionsColumn, originColumn}
)

func parseListFormat(s string) (listFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return listFormatJSON, nil
		}
		return listFormatTable, nil
	case "table":
		return listFormatTable, nil
	case "wide":
		return listFormatWide, nil
	case "json":
		return listFormatJSON, nil
	case "yaml", "yml":
		return listFormatYAML, nil
	case "csv":
		return listFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, wide, json, yaml, csv)", s)
	}
}

func newListCmd() *cobra.Command {
	flags := &listFlags{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "[experimental] List auth tokens",
		Long:    `List all auth tokens in your Dash0 organization. The output never contains a secret.` + internal.CONFIG_HINT,
		Example: `  # List all auth tokens
  dash0 --experimental auth-tokens list

  # Include the origin column
  dash0 --experimental auth-tokens list -o wide

  # Output as JSON
  dash0 --experimental auth-tokens list -o json

  # Output as YAML
  dash0 --experimental auth-tokens list -o yaml

  # Output as CSV
  dash0 --experimental auth-tokens list -o csv`,
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
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, wide, json, yaml, csv (default: table)")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table, wide, and CSV output")
	cmd.Flags().StringArrayVar(&flags.Column, "column", nil, "Column to display (alias or attribute key; repeatable; table, wide, and CSV only)")

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
	cols, err := resolveListColumns(flags.Column, format)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	items, err := apiClient.ListAuthTokens(ctx)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType})
	}
	for _, item := range items {
		takeSecret(item)
	}

	switch format {
	case listFormatJSON:
		if items == nil {
			items = []*dash0api.AuthTokenResponseDefinition{}
		}
		return printJSON(os.Stdout, items)
	case listFormatYAML:
		return renderYAML(items)
	case listFormatTable, listFormatWide:
		return renderTable(items, cols, flags.SkipHeader)
	case listFormatCSV:
		return renderCSV(items, cols, flags.SkipHeader)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func resolveListColumns(columns []string, format listFormat) ([]query.ColumnDef, error) {
	defaults := tableColumns
	if format == listFormatWide || format == listFormatCSV {
		defaults = wideColumns
	}
	if len(columns) == 0 {
		return defaults, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, wideColumns), nil
}

func tokenValues(item *dash0api.AuthTokenResponseDefinition) map[string]string {
	origin := originOf(&item.Metadata)
	if origin == "" {
		origin = "-"
	}
	return map[string]string{
		"name":    item.Metadata.Name,
		"id":      idOf(&item.Metadata),
		"actions": actionsOf(item),
		"origin":  origin,
	}
}

func renderYAML(items []*dash0api.AuthTokenResponseDefinition) error {
	if len(items) == 0 {
		fmt.Println("No auth tokens found.")
		return nil
	}
	for i, item := range items {
		if i > 0 {
			fmt.Println("---")
		}
		if err := printYAML(os.Stdout, item); err != nil {
			return err
		}
	}
	return nil
}

func renderTable(items []*dash0api.AuthTokenResponseDefinition, cols []query.ColumnDef, skipHeader bool) error {
	if len(items) == 0 {
		fmt.Println("No auth tokens found.")
		return nil
	}
	rows := make([]map[string]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, tokenValues(item))
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func renderCSV(items []*dash0api.AuthTokenResponseDefinition, cols []query.ColumnDef, skipHeader bool) error {
	writer := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(writer, cols); err != nil {
			return err
		}
	}
	for _, item := range items {
		if err := query.WriteCSVRow(writer, cols, tokenValues(item)); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
