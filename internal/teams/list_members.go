package teams

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/members"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

type listMembersFlags struct {
	ApiUrl     string
	AuthToken  string
	Output     string
	SkipHeader bool
	Column     []string
}

func newListMembersCmd() *cobra.Command {
	flags := &listMembersFlags{}

	cmd := &cobra.Command{
		Use:   "list-members <team-id>",
		Short: "[experimental] List members of a team",
		Long:  `List all members of a team.` + internal.CONFIG_HINT,
		Example: `  # List members of a team
  dash0 --experimental teams list-members <id>

  # Output as JSON
  dash0 --experimental teams list-members <id> -o json

  # Output as CSV
  dash0 --experimental teams list-members <id> -o csv

  # Show only specific columns
  dash0 --experimental teams list-members <id> --column name --column email`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runListMembers(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, csv (default: table)")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table and CSV output")
	cmd.Flags().StringArrayVar(&flags.Column, "column", nil, "Column to display (alias or attribute key; repeatable; table and CSV only)")

	return cmd
}

func runListMembers(cmd *cobra.Command, teamID string, flags *listMembersFlags) error {
	ctx := cmd.Context()

	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	if err := query.ValidateColumnFormat(flags.Column, flags.Output); err != nil {
		return err
	}

	format, err := parseMemberListFormat(flags.Output)
	if err != nil {
		return err
	}

	cols, err := members.ResolveMemberListColumns(flags.Column)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	resp, err := apiClient.GetTeam(ctx, teamID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   teamID,
		})
	}

	items := make([]*dash0api.MemberDefinition, len(resp.Members))
	for i := range resp.Members {
		items[i] = &resp.Members[i]
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)

	switch format {
	case memberListFormatJSON:
		return members.RenderMembersJSON(items)
	case memberListFormatTable:
		return members.RenderMembersTable(items, cols, flags.SkipHeader, apiUrl)
	case memberListFormatCSV:
		return members.RenderMembersCSV(items, cols, flags.SkipHeader, apiUrl)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

type memberListFormat string

const (
	memberListFormatTable memberListFormat = "table"
	memberListFormatJSON  memberListFormat = "json"
	memberListFormatCSV   memberListFormat = "csv"
)

func parseMemberListFormat(s string) (memberListFormat, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return memberListFormatTable, nil
	case "json":
		return memberListFormatJSON, nil
	case "csv":
		return memberListFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, csv)", s)
	}
}
