package members

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
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

// listFormat represents the output format for member list.
type listFormat string

const (
	listFormatTable listFormat = "table"
	listFormatJSON  listFormat = "json"
	listFormatCSV   listFormat = "csv"
)

var MemberListDefaultColumns = []query.ColumnDef{
	{Key: "name", Aliases: []string{"member name"}, Header: internal.HEADER_NAME, Width: 30},
	{Key: "email", Header: internal.HEADER_EMAIL, Width: 40},
	{Key: "id", Aliases: []string{"member id"}, Header: internal.HEADER_ID, Width: 36},
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
		Short:   "[experimental] List organization members",
		Long:    `List all members of your Dash0 organization.` + internal.CONFIG_HINT,
		Example: `  # List all members
  dash0 --experimental members list

  # Output as JSON
  dash0 --experimental members list -o json

  # Output as CSV
  dash0 --experimental members list -o csv

  # Show only specific columns
  dash0 --experimental members list --column name --column email`,
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

	cols, err := ResolveMemberListColumns(flags.Column)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	iter := apiClient.ListMembersIter(ctx)

	var items []*dash0api.MemberDefinition
	for iter.Next() {
		items = append(items, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: "member"})
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)

	switch format {
	case listFormatJSON:
		return RenderMembersJSON(items)
	case listFormatTable:
		return RenderMembersTable(items, cols, flags.SkipHeader, apiUrl)
	case listFormatCSV:
		return RenderMembersCSV(items, cols, flags.SkipHeader, apiUrl)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func ResolveMemberListColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return MemberListDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, MemberListDefaultColumns), nil
}

func MemberValues(m *dash0api.MemberDefinition, apiUrl string) map[string]string {
	name := MemberDisplayName(m)
	email := ""
	if m.Spec.Display.Email != nil {
		email = *m.Spec.Display.Email
	}
	id := ""
	if m.Metadata.Labels != nil && m.Metadata.Labels.Dash0Comid != nil {
		id = *m.Metadata.Labels.Dash0Comid
	}
	return map[string]string{
		"name":  name,
		"email": email,
		"id":    id,
		"url":   asset.DeeplinkURL(apiUrl, "member", id),
	}
}

func MemberDisplayName(m *dash0api.MemberDefinition) string {
	var parts []string
	if m.Spec.Display.FirstName != nil && *m.Spec.Display.FirstName != "" {
		parts = append(parts, *m.Spec.Display.FirstName)
	}
	if m.Spec.Display.LastName != nil && *m.Spec.Display.LastName != "" {
		parts = append(parts, *m.Spec.Display.LastName)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if m.Spec.Display.Email != nil {
		return *m.Spec.Display.Email
	}
	return m.Metadata.Name
}

func RenderMembersJSON(items []*dash0api.MemberDefinition) error {
	if items == nil {
		items = []*dash0api.MemberDefinition{}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(items)
}

func RenderMembersTable(items []*dash0api.MemberDefinition, cols []query.ColumnDef, skipHeader bool, apiUrl string) error {
	if len(items) == 0 {
		fmt.Println("No members found.")
		return nil
	}
	var rows []map[string]string
	for _, item := range items {
		rows = append(rows, MemberValues(item, apiUrl))
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func RenderMembersCSV(items []*dash0api.MemberDefinition, cols []query.ColumnDef, skipHeader bool, apiUrl string) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
	}
	for _, item := range items {
		values := MemberValues(item, apiUrl)
		if err := query.WriteCSVRow(w, cols, values); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
