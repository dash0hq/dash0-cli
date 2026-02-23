package teams

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
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

// listFormat represents the output format for team list.
type listFormat string

const (
	listFormatTable listFormat = "table"
	listFormatJSON  listFormat = "json"
	listFormatCSV   listFormat = "csv"
)

var teamListDefaultColumns = []query.ColumnDef{
	{Key: "name", Aliases: []string{"team name"}, Header: internal.HEADER_NAME, Width: 30},
	{Key: "id", Aliases: []string{"team id"}, Header: internal.HEADER_ID, Width: 32},
	{Key: "members", Aliases: []string{"member count"}, Header: internal.HEADER_MEMBERS, Width: 12},
	{Key: "origin", Header: internal.HEADER_ORIGIN, Width: 10},
	{Key: "url", Header: internal.HEADER_URL, Width: 70},
}

func parseListFormat(s string) (listFormat, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return listFormatTable, nil
	case "json":
		return listFormatJSON, nil
	case "csv":
		return listFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, csv)", s)
	}
}

func newListCmd() *cobra.Command {
	flags := &listFlags{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "[experimental] List teams",
		Long:    `List all teams in your Dash0 organization.` + internal.CONFIG_HINT,
		Example: `  # List all teams
  dash0 --experimental teams list

  # Output as JSON
  dash0 --experimental teams list -o json

  # Output as CSV
  dash0 --experimental teams list -o csv

  # Show only specific columns
  dash0 --experimental teams list --column name --column members`,
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
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, csv (default: table)")
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

	iter := apiClient.ListTeamsIter(ctx)

	var items []*dash0api.TeamsListItem
	for iter.Next() {
		items = append(items, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: "team"})
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)

	switch format {
	case listFormatJSON:
		return renderTeamsJSON(items)
	case listFormatTable:
		return renderTeamsTable(items, cols, flags.SkipHeader, apiUrl)
	case listFormatCSV:
		return renderTeamsCSV(items, cols, flags.SkipHeader, apiUrl)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func resolveListColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return teamListDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, teamListDefaultColumns), nil
}

func teamValues(item *dash0api.TeamsListItem, apiUrl string) map[string]string {
	origin := ""
	if item.Origin != nil {
		origin = *item.Origin
	}
	return map[string]string{
		"name":    item.Name,
		"id":      item.Id,
		"members": formatMemberCount(item),
		"origin":  origin,
		"url":     asset.DeeplinkURL(apiUrl, "team", item.Id),
	}
}

func formatMemberCount(item *dash0api.TeamsListItem) string {
	return strconv.Itoa(item.TotalMemberCount)
}

func renderTeamsJSON(items []*dash0api.TeamsListItem) error {
	if items == nil {
		items = []*dash0api.TeamsListItem{}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(items)
}

func renderTeamsTable(items []*dash0api.TeamsListItem, cols []query.ColumnDef, skipHeader bool, apiUrl string) error {
	if len(items) == 0 {
		fmt.Println("No teams found.")
		return nil
	}
	var rows []map[string]string
	for _, item := range items {
		rows = append(rows, teamValues(item, apiUrl))
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func renderTeamsCSV(items []*dash0api.TeamsListItem, cols []query.ColumnDef, skipHeader bool, apiUrl string) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
	}
	for _, item := range items {
		values := teamValues(item, apiUrl)
		if err := query.WriteCSVRow(w, cols, values); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
