package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

// Exit codes of `dash0 diff`, matching `kubectl diff`.
const (
	ExitCodeDiffFound = 1
	ExitCodeDiffError = 2
)

// ExitError asks main to exit with Code. A nil Err exits silently, which is how
// `diff` reports "differences found" without printing an error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// NewDiffCmd creates the top-level diff command.
func NewDiffCmd() *cobra.Command {
	var flags applyFlags

	cmd := &cobra.Command{
		Use:   "diff -f <file|directory>",
		Short: "[experimental] Show what apply would change, without changing anything",
		Long: `Show the difference between asset definitions in a YAML file or directory and their current state in Dash0, without creating, updating, or deleting anything.

Accepts the same input as apply. Assets that do not exist yet are reported as would-be-created; existing assets show a unified diff, or "no changes".

With --since <ref>, assets removed from -f's contents since that git ref are reported as would-be-deleted, using the same rules as apply --since.

Exit codes: 0 when nothing would change, 1 when at least one create, update, or deletion is pending (not a failure), and 2 on error. In agent mode, read the JSON on stdout for the plan. Creates are inferred from a 404, so a plan made of creates alone can still be produced with an invalid token.` + internal.CONFIG_HINT,
		Example: `  # Preview the changes apply would make for a file
  dash0 --experimental diff -f dashboard.yaml

  # Preview the changes for a directory
  dash0 --experimental diff -f dashboards/

  # Preview changes, including deletions since a git ref
  dash0 --experimental diff -f dashboards/ --since HEAD~1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := validateDiffInvocation(cmd, &flags, args)
			if err == nil {
				cmd.SilenceUsage = true
				err = runDiff(cmd.Context(), &flags)
			}
			var exitErr *ExitError
			if err != nil && !errors.As(err, &exitErr) {
				return &ExitError{Code: ExitCodeDiffError, Err: err}
			}
			return err
		},
	}
	// Usage errors must not look like "changes pending" (exit 1) to CI.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{Code: ExitCodeDiffError, Err: err}
	})

	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to a file or directory containing asset definitions (use '-' for stdin)")
	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset to operate on")
	cmd.Flags().StringVar(&flags.Since, "since", "", "Also report assets removed from -f's contents since this git ref as would-be-deleted")

	return cmd
}

func validateDiffInvocation(cmd *cobra.Command, flags *applyFlags, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %s\nTo diff multiple files, pass a directory with -f instead of a glob pattern", strings.Join(args, " "))
	}
	if err := experimental.RequireExperimental(cmd); err != nil {
		return err
	}
	if flags.File == "" {
		return fmt.Errorf("file is required; use -f to specify the file (use '-' for stdin)")
	}
	flags.SinceFlagSet = cmd.Flags().Changed("since")
	if flags.SinceFlagSet && flags.File == "-" {
		return fmt.Errorf("--since '%s' cannot be used with -f - (stdin)\nHint: --since needs a file or directory path to compare against git history; pass -f <path> instead", flags.Since)
	}
	return nil
}

// previewClient runs apply's import logic without writing: reads go to Dash0,
// writes echo their input back, so the import result carries the before and
// after states a diff needs. The embedded Client is deliberately nil, so a
// write method that is not overridden below panics instead of reaching Dash0.
// Non-404 read failures are recorded because the
// import helpers would otherwise mistake them for "does not exist yet".
type previewClient struct {
	dash0api.Client
	real     dash0api.Client
	fetchErr error
}

func track[T any](c *previewClient, v T, err error) (T, error) {
	if err != nil && !dash0api.IsNotFound(err) && c.fetchErr == nil {
		c.fetchErr = err
	}
	return v, err
}

func (c *previewClient) GetDashboard(ctx context.Context, id string, ds *string) (*dash0api.DashboardDefinition, error) {
	v, err := c.real.GetDashboard(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetCheckRule(ctx context.Context, id string, ds *string) (*dash0api.PrometheusAlertRule, error) {
	v, err := c.real.GetCheckRule(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetSyntheticCheck(ctx context.Context, id string, ds *string) (*dash0api.SyntheticCheckDefinition, error) {
	v, err := c.real.GetSyntheticCheck(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetSLO(ctx context.Context, id string, ds *string) (*dash0api.SloDefinition, error) {
	v, err := c.real.GetSLO(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetView(ctx context.Context, id string, ds *string) (*dash0api.ViewDefinition, error) {
	v, err := c.real.GetView(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetTimeSeriesAggregation(ctx context.Context, id string, ds *string) (*dash0api.TimeSeriesAggregationDefinition, error) {
	v, err := c.real.GetTimeSeriesAggregation(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetTeam(ctx context.Context, id string) (*dash0api.TeamDefinitionV1Alpha1, error) {
	v, err := c.real.GetTeam(ctx, id)
	return track(c, v, err)
}

func (c *previewClient) GetRecordingRule(ctx context.Context, id string, ds *string) (*dash0api.RecordingRule, error) {
	v, err := c.real.GetRecordingRule(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) GetNotificationChannel(ctx context.Context, id string) (*dash0api.NotificationChannelDefinition, error) {
	v, err := c.real.GetNotificationChannel(ctx, id)
	return track(c, v, err)
}

func (c *previewClient) GetSpamFilter(ctx context.Context, id string, ds *string) (dash0api.SpamFilterObject, error) {
	v, err := c.real.GetSpamFilter(ctx, id, ds)
	return track(c, v, err)
}

func (c *previewClient) CreateDashboard(_ context.Context, v *dash0api.DashboardDefinition, _ *string) (*dash0api.DashboardDefinition, error) {
	return v, nil
}

func (c *previewClient) UpdateDashboard(_ context.Context, id string, v *dash0api.DashboardDefinition, _ *string) (*dash0api.DashboardDefinition, error) {
	// ImportDashboard clears the id from the body before updating; the state
	// fetched from Dash0 has it, so put it back or every dashboard looks changed.
	dash0api.SetDashboardID(v, id)
	return v, nil
}

func (c *previewClient) CreateCheckRule(_ context.Context, v *dash0api.PrometheusAlertRule, _ *string) (*dash0api.PrometheusAlertRule, error) {
	return v, nil
}

func (c *previewClient) UpdateCheckRule(_ context.Context, _ string, v *dash0api.PrometheusAlertRule, _ *string) (*dash0api.PrometheusAlertRule, error) {
	return v, nil
}

func (c *previewClient) CreateSyntheticCheck(_ context.Context, v *dash0api.SyntheticCheckDefinition, _ *string) (*dash0api.SyntheticCheckDefinition, error) {
	return v, nil
}

func (c *previewClient) UpdateSyntheticCheck(_ context.Context, _ string, v *dash0api.SyntheticCheckDefinition, _ *string) (*dash0api.SyntheticCheckDefinition, error) {
	return v, nil
}

func (c *previewClient) CreateSLO(_ context.Context, v *dash0api.SloDefinition, _ *string) (*dash0api.SloDefinition, error) {
	return v, nil
}

func (c *previewClient) UpdateSLO(_ context.Context, _ string, v *dash0api.SloDefinition, _ *string) (*dash0api.SloDefinition, error) {
	return v, nil
}

func (c *previewClient) CreateView(_ context.Context, v *dash0api.ViewDefinition, _ *string) (*dash0api.ViewDefinition, error) {
	return v, nil
}

func (c *previewClient) UpdateView(_ context.Context, _ string, v *dash0api.ViewDefinition, _ *string) (*dash0api.ViewDefinition, error) {
	return v, nil
}

func (c *previewClient) CreateTimeSeriesAggregation(_ context.Context, v *dash0api.TimeSeriesAggregationDefinition, _ *string) (*dash0api.TimeSeriesAggregationDefinition, error) {
	return v, nil
}

func (c *previewClient) UpdateTimeSeriesAggregation(_ context.Context, _ string, v *dash0api.TimeSeriesAggregationDefinition, _ *string) (*dash0api.TimeSeriesAggregationDefinition, error) {
	return v, nil
}

func (c *previewClient) CreateTeam(_ context.Context, v *dash0api.TeamDefinitionV1Alpha1) (*dash0api.TeamDefinitionV1Alpha1, error) {
	return v, nil
}

func (c *previewClient) UpsertTeam(_ context.Context, _ string, v *dash0api.TeamDefinitionV1Alpha1) (*dash0api.TeamDefinitionV1Alpha1, error) {
	return v, nil
}

func (c *previewClient) CreateRecordingRule(_ context.Context, v *dash0api.RecordingRule, _ *string) (*dash0api.RecordingRule, error) {
	return v, nil
}

func (c *previewClient) UpdateRecordingRule(_ context.Context, _ string, v *dash0api.RecordingRule, _ *string) (*dash0api.RecordingRule, error) {
	return v, nil
}

func (c *previewClient) CreateNotificationChannel(_ context.Context, v *dash0api.NotificationChannelDefinition) (*dash0api.NotificationChannelDefinition, error) {
	return v, nil
}

func (c *previewClient) UpdateNotificationChannel(_ context.Context, _ string, v *dash0api.NotificationChannelDefinition) (*dash0api.NotificationChannelDefinition, error) {
	return v, nil
}

func (c *previewClient) CreateSpamFilter(_ context.Context, v *dash0api.SpamFilter, _ *string) (*dash0api.SpamFilter, error) {
	return v, nil
}

func (c *previewClient) UpdateSpamFilter(_ context.Context, _ string, v *dash0api.SpamFilter, _ *string) (*dash0api.SpamFilter, error) {
	return v, nil
}

func (c *previewClient) CreateSpamFilterV1Alpha2(_ context.Context, v *dash0api.SpamFilterV1Alpha2, _ *string) (*dash0api.SpamFilterV1Alpha2, error) {
	return v, nil
}

func (c *previewClient) UpdateSpamFilterV1Alpha2(_ context.Context, _ string, v *dash0api.SpamFilterV1Alpha2, _ *string) (*dash0api.SpamFilterV1Alpha2, error) {
	return v, nil
}

// diffEntry is one asset's outcome: op is "create", "update", "unchanged", or "delete".
type diffEntry struct {
	path       string
	op         string
	kind       string
	name       string
	originOrID string
	detail     string
	diff       string
}

func runDiff(ctx context.Context, flags *applyFlags) error {
	documents, fromDirectory, deletionPlan, err := loadDocumentsAndPlan(ctx, flags)
	if err != nil {
		return err
	}
	if deletionPlan != nil && deletionPlan.warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", deletionPlan.warning)
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	dataset := client.ResolveDataset(ctx, flags.Dataset)
	preview := &previewClient{real: apiClient}

	var entries []diffEntry
	for _, doc := range documents {
		path := flags.File
		if fromDirectory {
			path = doc.filePath
		}
		results, applyErr := applyDocument(ctx, preview, doc, dataset)
		if preview.fetchErr != nil {
			applyErr = client.HandleAPIError(preview.fetchErr, client.ErrorContext{
				AssetType: strings.ToLower(asset.KindDisplayName(doc.kind)),
				AssetName: doc.name,
			})
		}
		// Abort the whole plan: a partial one would under-report changes.
		if applyErr != nil {
			return fmt.Errorf("%s (%s): %w", doc.location(), doc.kind, applyErr)
		}
		for _, r := range results {
			entry := diffEntry{path: path, kind: r.kind, name: r.name, originOrID: r.id, op: "create"}
			if entry.originOrID == "" {
				entry.originOrID = doc.id
			}
			if r.action == actionUpdated && r.before != nil {
				text, err := asset.UnifiedDiff(asset.KindDisplayName(r.kind), r.before, r.after)
				if err != nil {
					return err
				}
				entry.op, entry.diff = "update", text
				if text == "" {
					entry.op = "unchanged"
				}
			}
			entries = append(entries, entry)
		}
	}

	if deletionPlan != nil {
		rowsByFile, files, _ := buildDryRunRows(documents, deletionPlan)
		for _, f := range files {
			for _, r := range rowsByFile[f] {
				if r.op == "delete" {
					path := f
					if !fromDirectory {
						// Same grouping as creates and updates for a single-file target.
						path = flags.File
					}
					entries = append(entries, diffEntry{path: path, op: "delete", kind: r.kind, name: r.name, originOrID: r.originOrID, detail: r.detail})
				}
			}
		}
	}

	if agentmode.Enabled {
		err = renderDiffJSON(entries, flags.Since)
	} else {
		err = renderDiffText(entries, fromDirectory)
	}
	if err != nil {
		return err
	}

	if slices.ContainsFunc(entries, func(e diffEntry) bool { return e.op != "unchanged" }) {
		return &ExitError{Code: ExitCodeDiffFound}
	}
	return nil
}

func renderDiffText(entries []diffEntry, fromDirectory bool) error {
	for _, e := range entries {
		displayKind := asset.KindDisplayName(e.kind)
		prefix := ""
		if fromDirectory {
			prefix = e.path + ": "
		}
		switch e.op {
		case "create":
			fmt.Printf("%s%s %s would be created\n", prefix, displayKind, formatNameAndId(e.name, e.originOrID))
		case "delete":
			if e.detail != "" {
				fmt.Printf("%s%s %q (%s) would be deleted\n", prefix, displayKind, e.name, e.detail)
			} else {
				fmt.Printf("%s%s %s would be deleted\n", prefix, displayKind, formatNameAndId(e.name, e.originOrID))
			}
		default:
			if err := asset.WriteDiff(os.Stdout, displayKind, e.name, e.diff); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderDiffJSON(entries []diffEntry, since string) error {
	out := []dryRunFileJSON{}
	index := map[string]int{}
	for _, e := range entries {
		i, ok := index[e.path]
		if !ok {
			i = len(out)
			index[e.path] = i
			out = append(out, dryRunFileJSON{Path: e.path, Changes: []dryRunChangeJSON{}})
		}
		change := dryRunChangeJSON{Op: e.op, Kind: asset.KindDisplayName(e.kind), Name: e.name, OriginOrID: e.originOrID, Diff: e.diff}
		if e.op == "delete" {
			change.Since = since
		}
		out[i].Changes = append(out[i].Changes, change)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
