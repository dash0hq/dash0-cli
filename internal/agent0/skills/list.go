package skills

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
)

type listFlags struct {
	ApiUrl       string
	AuthToken    string
	Output       string
	SkipHeader   bool
	Column       []string
	OriginPrefix string
	Enabled      bool
}

// listFormat represents the output format for skill list.
type listFormat string

const (
	listFormatTable listFormat = "table"
	listFormatWide  listFormat = "wide"
	listFormatJSON  listFormat = "json"
	listFormatYAML  listFormat = "yaml"
	listFormatCSV   listFormat = "csv"
)

var (
	nameColumn    = query.ColumnDef{Key: "name", Aliases: []string{"skill name"}, Header: internal.HEADER_NAME, Width: 40}
	idColumn      = query.ColumnDef{Key: "id", Aliases: []string{"skill id"}, Header: internal.HEADER_ID, Width: 36}
	originColumn  = query.ColumnDef{Key: "origin", Header: internal.HEADER_ORIGIN, Width: 30}
	enabledColumn = query.ColumnDef{Key: "enabled", Header: internal.HEADER_ENABLED, Width: 8}

	tableColumns = []query.ColumnDef{nameColumn, idColumn}
	wideColumns  = []query.ColumnDef{nameColumn, idColumn, originColumn, enabledColumn}
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
		Short:   "[experimental] List Agent0 custom skills",
		Long:    `List the Agent0 custom skills you can read in your Dash0 organization.` + internal.CONFIG_HINT,
		Example: `  # List all skills
  dash0 --experimental agent0 skills list

  # Include the origin and enabled columns
  dash0 --experimental agent0 skills list -o wide

  # List only the skills managed with a given origin prefix
  dash0 --experimental agent0 skills list --origin-prefix dash0-cli_

  # List only the skills that are turned on for you
  dash0 --experimental agent0 skills list --enabled

  # Output as YAML
  dash0 --experimental agent0 skills list -o yaml

  # Show only specific columns
  dash0 --experimental agent0 skills list --column name --column origin`,
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
	cmd.Flags().StringVar(&flags.OriginPrefix, "origin-prefix", "", "Only list skills whose origin starts with this prefix")
	cmd.Flags().BoolVar(&flags.Enabled, "enabled", false, "Only list skills that are on (--enabled) or off (--enabled=false) for you")

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

	opts := []dash0api.ListOption{dash0api.WithOriginPrefix(flags.OriginPrefix)}
	if cmd.Flags().Changed("enabled") {
		opts = append(opts, dash0api.WithEnabled(flags.Enabled))
	}
	iter := apiClient.ListCustomSkillsIter(ctx, opts...)
	var items []*dash0api.SkillDefinition
	for iter.Next() {
		item := iter.Current()
		asset.SortCustomSkillPermissions(item)
		items = append(items, item)
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType})
	}

	switch format {
	case listFormatJSON:
		if items == nil {
			items = []*dash0api.SkillDefinition{}
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(items)
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

func skillValues(item *dash0api.SkillDefinition) map[string]string {
	origin := dash0api.GetCustomSkillOrigin(item)
	if origin == "" {
		origin = "-"
	}
	return map[string]string{
		"name":    asset.CustomSkillName(item),
		"id":      dash0api.GetCustomSkillID(item),
		"origin":  origin,
		"enabled": enabledValue(item),
	}
}

func enabledValue(item *dash0api.SkillDefinition) string {
	if a := item.Metadata.Annotations; a != nil && a.Dash0Comenabled != nil {
		return *a.Dash0Comenabled
	}
	return "-"
}

func renderYAML(items []*dash0api.SkillDefinition) error {
	if len(items) == 0 {
		fmt.Println("No Agent0 skills found.")
		return nil
	}
	for i, item := range items {
		if i > 0 {
			fmt.Println("---")
		}
		data, err := sigsyaml.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal Agent0 skill as YAML: %w", err)
		}
		fmt.Print(string(data))
	}
	return nil
}

func renderTable(items []*dash0api.SkillDefinition, cols []query.ColumnDef, skipHeader bool) error {
	if len(items) == 0 {
		fmt.Println("No Agent0 skills found.")
		return nil
	}
	rows := make([]map[string]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, skillValues(item))
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func renderCSV(items []*dash0api.SkillDefinition, cols []query.ColumnDef, skipHeader bool) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
	}
	for _, item := range items {
		if err := query.WriteCSVRow(w, cols, skillValues(item)); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
