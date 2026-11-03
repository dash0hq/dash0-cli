package apply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/spf13/cobra"
)

// Flags for the apply command
type applyFlags struct {
	ApiUrl    string
	AuthToken string
	Dataset   string
	File      string
	DryRun    bool
	Since     string
	// SinceFlagSet records whether --since was actually passed on the
	// command line, as opposed to left at its "" zero value. This is
	// distinct from Since != "": a CI script building
	// --since="${{ github.event.before }}" can pass an explicitly empty
	// string (e.g. on a workflow_dispatch/schedule trigger with no prior
	// ref), and that case must still route through computeDeletionPlan to
	// hit the dedicated RefEmpty error, not be silently treated the same as
	// --since never being mentioned at all.
	SinceFlagSet bool
	Force        bool
	// AcceptNonAncestorRef authorizes proceeding past the warning printed
	// when --since's ref resolves but is not an ancestor of HEAD (likely a
	// force-push or history rewrite), without also implying --force's
	// separate job of skipping every per-asset deletion confirmation.
	// --force still authorizes this too, for backward compatibility.
	AcceptNonAncestorRef bool
}

// NewApplyCmd creates the top-level apply command
func NewApplyCmd() *cobra.Command {
	var flags applyFlags

	cmd := &cobra.Command{
		Use:   "apply -f <file|directory>",
		Short: "Apply asset definitions from a file or directory",
		Long: `Apply asset definitions from a YAML file or a directory containing YAML files. Files must have the .yaml or .yml file extension and may contain multiple documents separated by "---".

Each document must have a "kind" field specifying the asset type. Use '-f -' to read documents from stdin.

When a directory is specified, all .yaml and .yml files are discovered recursively. Hidden files and directories (starting with '.') are skipped. All documents are validated before any are applied; if any document fails validation, no changes are made.

Supported asset types:
  - Dashboard (or PersesDashboard CRD)
  - CheckRule (or PrometheusRule CRD with alerting rules)
  - PrometheusRule CRD with recording rules
  - SyntheticCheck
  - SLO
  - View
  - Dash0SpamFilter
  - Dash0NotificationChannel
  - Dash0Team
  - Dash0TimeSeriesAggregation
  - Dash0Dataset
  - Dash0Skill (Agent0 custom skill)

A PrometheusRule CRD that mixes alerting and recording rules is dispatched to both endpoints; alerting rules become check rules and recording rules become a recording rule.

If an asset exists, it will be updated. If it doesn't exist, it will be created.

[experimental] Pass --since <ref> (requires --experimental/-X) to also delete assets whose definition existed at <ref> but is no longer present in -f's current contents, detected by identifier (id or origin), never by file path. A removed Dash0Dataset is deleted like any other kind, as a 30-day soft delete: dashboards, check rules, synthetic checks and views are kept for 30 days, and applying the same slug again within that time restores them; telemetry is removed as its retention expires. --force skips the per-deletion confirmation prompt and accepts a non-ancestor --since ref; --accept-non-ancestor-ref accepts a non-ancestor ref on its own, without also skipping the per-deletion prompt.` + internal.CONFIG_HINT,
		Example: `  # Apply a single asset
  dash0 apply -f dashboard.yaml

  # Apply multiple assets from a single file
  dash0 apply -f assets.yaml

  # Apply all assets from a directory (recursive)
  dash0 apply -f dashboards/

  # Apply from stdin
  cat assets.yaml | dash0 apply -f -

  # Validate without applying
  dash0 apply -f assets.yaml --dry-run

  # Validate a directory without applying
  dash0 apply -f dashboards/ --dry-run

  # Sync a directory to match its state as of a git ref, deleting assets removed since then (experimental)
  dash0 --experimental apply -f dashboards/ --since HEAD~1

  # Same, without the per-deletion confirmation prompt (experimental)
  dash0 --experimental apply -f dashboards/ --since HEAD~1 --force

  # Accept a --since ref from a force-push without skipping per-deletion confirmation (experimental)
  dash0 --experimental apply -f dashboards/ --since HEAD~1 --accept-non-ancestor-ref`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s\nTo apply multiple files, pass a directory with -f instead of a glob pattern", strings.Join(args, " "))
			}
			if flags.File == "" {
				return fmt.Errorf("file is required; use -f to specify the file (use '-' for stdin)")
			}
			if err := experimental.RequireExperimentalFlag(cmd, "since"); err != nil {
				return err
			}
			flags.SinceFlagSet = cmd.Flags().Changed("since")
			if flags.SinceFlagSet && flags.File == "-" {
				return fmt.Errorf("--since '%s' cannot be used with -f - (stdin)\nHint: --since needs a file or directory path to compare against git history; pass -f <path> instead", flags.Since)
			}
			cmd.SilenceUsage = true
			return runApply(cmd.Context(), &flags)
		},
	}

	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to a file or directory containing asset definitions (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate the file without applying changes")
	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset to operate on")
	cmd.Flags().StringVar(&flags.Since, "since", "", "[experimental] Delete assets removed from -f's contents since this git ref (requires --experimental/-X)")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip the confirmation prompt for deletions triggered by --since; also accepts a non-ancestor --since ref")
	cmd.Flags().BoolVar(&flags.AcceptNonAncestorRef, "accept-non-ancestor-ref", false, "Accept a --since ref that is not an ancestor of HEAD (e.g. after a force-push), without also skipping the per-deletion confirmation prompt")

	return cmd
}

// applyAction indicates whether an asset was created or updated
type applyAction string

const (
	actionCreated applyAction = "created"
	actionUpdated applyAction = "updated"
)

// applyResult holds the outcome of applying a single asset.
type applyResult struct {
	kind   string
	name   string
	id     string
	action applyAction
	before any // asset state before update (nil for creates)
	after  any // asset state after update/create
}

func runApply(ctx context.Context, flags *applyFlags) error {
	var documents []asset.Document
	var fromDirectory bool
	// targetVanished records that fromDirectory was only a guess (always
	// true) because the target no longer exists on disk at all, so os.Stat
	// couldn't say whether it used to be a file or a directory. Once
	// computeDeletionPlan runs, it can answer that from git history at the
	// --since ref, and the guess is corrected below -- otherwise a vanished
	// single-file target would render like a multi-file directory scan
	// (grouped by its git-recorded path instead of the literal -f argument).
	var targetVanished bool
	var err error

	if flags.File == "-" {
		// Read from stdin
		documents, err = asset.ReadMultiDocumentYAML("-", os.Stdin)
		if err != nil {
			return asset.FormatValidationError(err.Error())
		}
	} else {
		info, statErr := os.Stat(flags.File)
		switch {
		case statErr != nil && flags.SinceFlagSet && os.IsNotExist(statErr):
			// --since's target no longer exists on disk at all: every asset
			// definition under it was deleted, and (for a directory target)
			// the directory itself was removed along with them. This is a
			// legitimate all-deletions run, the same as the existing-but-
			// empty-directory case below -- continue with zero current
			// documents and let computeDeletionPlan report every asset found
			// at the --since ref as a deletion.
			fromDirectory = true
			targetVanished = true
		case statErr != nil:
			return fmt.Errorf("failed to read input: %w", statErr)
		case info.IsDir():
			fromDirectory = true
			documents, err = asset.ReadDirectory(flags.File)
			if err != nil {
				if flags.SinceFlagSet && errors.Is(err, asset.ErrNoYAMLFilesFound) {
					// Every asset definition that used to live in this
					// directory was deleted, but the (now-empty) directory
					// itself survives. Same all-deletions case as above.
					documents = nil
				} else {
					return asset.FormatValidationError(err.Error())
				}
			}
		default:
			documents, err = asset.ReadMultiDocumentYAML(flags.File, nil)
			if err != nil {
				return asset.FormatValidationError(err.Error())
			}
		}
	}

	if len(documents) == 0 && !flags.SinceFlagSet {
		return asset.FormatValidationError("no documents found in input")
	}

	validationErrors, validationWarnings := asset.ValidateDocuments(documents)
	if len(validationErrors) > 0 {
		return asset.FormatValidationError(validationErrors...)
	}
	for _, warning := range validationWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	var deletionPlan *gitutil.SincePlan
	if flags.SinceFlagSet {
		plan, err := computeDeletionPlan(ctx, flags)
		if err != nil {
			return err
		}
		deletionPlan = plan
		if targetVanished {
			fromDirectory = plan.TargetWasDirectoryAtRef
		}
	}

	if flags.DryRun {
		return runDryRun(documents, fromDirectory, flags.File, flags.Since, deletionPlan)
	}

	// A dataset delete starts the 30-day countdown to removing its assets.
	// Like `datasets delete`, agent mode must pass --force deliberately instead
	// of getting the usual agent-mode auto-confirmation. Checked before any
	// document is applied, so a refusal leaves the run untouched.
	if deletionPlan != nil {
		for _, d := range deletionPlan.Plan.ByIdentifier {
			if d.Kind != "dataset" {
				continue
			}
			if err := confirmation.RequireForceInAgentMode(flags.Force, fmt.Sprintf("deleting Dataset %q with --since", d.Identifier),
				"re-run with --force to confirm. "+asset.DatasetSoftDeleteNote); err != nil {
				return err
			}
		}
	}

	// Create API client
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// Apply each document
	var applied []string
	for _, doc := range datasetsFirst(documents) {
		results, applyErr := applyDocument(ctx, apiClient, doc, dataset)
		for _, r := range results {
			displayKind := asset.KindDisplayName(r.kind)
			label := asset.FormatNameAndID(r.name, r.id)
			applied = append(applied, fmt.Sprintf("%s %s", displayKind, label))

			if r.action == actionUpdated && r.before != nil {
				if err := asset.PrintDiff(os.Stdout, displayKind, r.name, r.before, r.after); err != nil {
					return err
				}
				continue
			}
			if fromDirectory {
				fmt.Printf("%s: %s %s %s\n", doc.FilePath, displayKind, label, r.action)
			} else {
				fmt.Printf("%s %s %s\n", displayKind, label, r.action)
			}
			if asset.ImportAction(r.action) == asset.ActionRestored {
				if err := asset.PrintChanges(os.Stdout, displayKind, r.before, r.after); err != nil {
					return err
				}
			}
		}
		if applyErr != nil {
			var stillDeleted *asset.StillDeletedError
			if errors.As(applyErr, &stillDeleted) {
				applied = append(applied, fmt.Sprintf("%s %s", stillDeleted.DisplayKind, asset.FormatNameAndID(stillDeleted.Name, stillDeleted.ID)))
			}
			if len(applied) > 0 {
				fmt.Println("Applied before error:")
				for _, a := range applied {
					fmt.Printf("  - %s\n", a)
				}
			}
			return fmt.Errorf("%s (%s): %w", doc.Location(), doc.Kind, applyErr)
		}
	}

	if deletionPlan != nil {
		// The non-ancestor confirmation happens here, after every document
		// create/update above has already gone through — never before them.
		// Gating it earlier (inside computeDeletionPlan, as this used to
		// work) meant a declined or unconfirmable --since ref aborted the
		// entire apply run, including ordinary creates/updates that have
		// nothing to do with --since's ancestry check.
		if deletionPlan.Warning != "" {
			fmt.Fprintf(os.Stderr, "warning: %s\n", deletionPlan.Warning)
			// --force also accepts this (backward compatible: it always
			// implied "proceed unattended"), but --accept-non-ancestor-ref
			// lets a caller accept a doubtful ref on its own, without also
			// giving up the per-asset deletion prompts below.
			acceptRef := flags.Force || flags.AcceptNonAncestorRef
			confirmed, confirmErr := confirmation.ConfirmDestructiveOperation(ctx, "Continue with --since's deletions? [y/N]: ", acceptRef)
			if confirmErr != nil || !confirmed {
				skipped := len(deletionPlan.Plan.ByIdentifier) + len(deletionPlan.Plan.AlertsByName)
				fmt.Fprintf(os.Stderr, "--since's deletion phase skipped; the rest of the run already completed\n")
				return fmt.Errorf("%s not confirmed for deletion (--since ref is not an ancestor of HEAD)", asset.Pluralize(skipped, "asset"))
			}
		}
		declined, err := applyDeletions(ctx, apiClient, dataset, deletionPlan, flags.Force)
		if err != nil {
			return err
		}
		if declined > 0 {
			return fmt.Errorf("%s declined; the rest of the --since run completed", asset.Pluralize(declined, "deletion"))
		}
	}

	return nil
}

func normalizeKind(kind string) string {
	// Normalize common variations
	k := strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	k = strings.ReplaceAll(k, "_", "")
	k = strings.TrimPrefix(k, "dash0")
	return k
}

func applyDocument(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]applyResult, error) {
	switch normalizeKind(doc.Kind) {
	case "dashboard", "persesdashboard":
		dashboard, err := dash0yaml.ParseAsDashboard(doc.Raw)
		if err != nil {
			return nil, err
		}
		result, err := asset.ImportDashboard(ctx, apiClient, dashboard, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "dashboard",
				AssetName: dash0api.GetDashboardName(dashboard),
			})
		}
		return []applyResult{{kind: "Dashboard", name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "checkrule":
		return applyCheckRule(ctx, apiClient, doc, dataset)

	case "prometheusrule":
		return applyPrometheusRule(ctx, apiClient, doc, dataset)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &check); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		result, err := asset.ImportSyntheticCheck(ctx, apiClient, &check, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "synthetic check",
				AssetName: check.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.Kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "slo":
		var slo dash0api.SloDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &slo); err != nil {
			return nil, fmt.Errorf("failed to parse SLO: %w", err)
		}
		result, err := asset.ImportSLO(ctx, apiClient, &slo, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "SLO",
				AssetName: dash0api.GetSLOName(&slo),
			})
		}
		return []applyResult{{kind: doc.Kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "view":
		var view dash0api.ViewDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &view); err != nil {
			return nil, fmt.Errorf("failed to parse View: %w", err)
		}
		result, err := asset.ImportView(ctx, apiClient, &view, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "view",
				AssetName: view.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.Kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "spamfilter":
		return applySpamFilter(ctx, apiClient, doc, dataset)

	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &channel); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0NotificationChannel: %w", err)
		}
		result, err := asset.ImportNotificationChannel(ctx, apiClient, &channel)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "notification channel",
				AssetName: dash0api.GetNotificationChannelName(&channel),
			})
		}
		return []applyResult{{kind: "Dash0NotificationChannel", name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "timeseriesaggregation":
		var aggregation dash0api.TimeSeriesAggregationDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &aggregation); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0TimeSeriesAggregation: %w", err)
		}
		result, err := asset.ImportTimeSeriesAggregation(ctx, apiClient, &aggregation, dataset)
		if err != nil {
			// The cross-dataset collision already carries its own explanation
			// and would only be flattened into "invalid request" here.
			if asset.IsTimeSeriesAggregationWrongDataset(err) {
				return nil, err
			}
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "time series aggregation",
				AssetName: dash0api.GetTimeSeriesAggregationName(&aggregation),
			})
		}
		return []applyResult{{kind: "Dash0TimeSeriesAggregation", name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := dash0yaml.Unmarshal(doc.Raw, &team); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Team: %w", err)
		}
		result, err := asset.ImportTeam(ctx, apiClient, &team)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "team",
				AssetName: dash0api.GetTeamDisplayName(&team),
			})
		}
		return []applyResult{{kind: "Dash0Team", name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "dataset":
		var dataset dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &dataset); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Dataset: %w", err)
		}
		result, err := asset.ImportDataset(ctx, apiClient, &dataset)
		if err != nil {
			err = client.HandleAPIError(err, client.ErrorContext{
				AssetType: "dataset",
				AssetID:   dash0api.GetDatasetID(&dataset),
				AssetName: dash0api.GetDatasetName(&dataset),
			})
			return nil, err
		}
		return []applyResult{{kind: "Dash0Dataset", name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "skill":
		var skill dash0api.SkillDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &skill); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Skill: %w", err)
		}
		name := asset.CustomSkillName(&skill)
		result, err := asset.ImportCustomSkill(ctx, apiClient, &skill)
		if err != nil {
			if explained := asset.ExplainCustomSkillError(err, name); explained != nil {
				return nil, explained
			}
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "Agent0 skill", AssetName: name})
		}
		return []applyResult{{kind: "Dash0Skill", name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	default:
		return nil, fmt.Errorf("unsupported kind: %s", doc.Kind)
	}
}

// applyCheckRule handles a single CheckRule (native, non-CRD) document.
// PrometheusRule CRD documents go through applyPrometheusRule, which inspects
// the rule kinds and dispatches to the check-rule or recording-rule endpoint
// (or both, when the CRD is mixed).
func applyCheckRule(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]applyResult, error) {
	rules, err := asset.ParseCheckRules(doc.Raw)
	if err != nil {
		return nil, err
	}
	var results []applyResult
	for _, rule := range rules {
		result, importErr := asset.ImportCheckRule(ctx, apiClient, rule, dataset)
		if importErr != nil {
			return results, client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "check rule",
				AssetName: rule.Name,
			})
		}
		results = append(results, applyResult{
			kind:   "CheckRule",
			name:   result.Name,
			id:     result.ID,
			action: applyAction(result.Action),
			before: result.Before,
			after:  result.After,
		})
	}
	return results, nil
}

// applyPrometheusRule handles a PrometheusRule CRD that may contain alerting
// rules, recording rules, or both. Alerting rules are dispatched to the
// check-rule endpoint via the existing ParseAsPrometheusAlertRules path;
// recording rules are dispatched to the recording-rule endpoint via a
// recording-only copy of the CRD.
func applyPrometheusRule(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]applyResult, error) {
	crd, err := asset.ParsePrometheusRuleCRD(doc.Raw)
	if err != nil {
		return nil, err
	}

	var results []applyResult

	if asset.PrometheusRuleHasAlerts(crd) {
		alertResults, err := applyCheckRule(ctx, apiClient, doc, dataset)
		results = append(results, alertResults...)
		if err != nil {
			return results, err
		}
	}

	recordingOnly := asset.RecordingOnlyPrometheusRule(crd)
	if recordingOnly != nil {
		result, importErr := asset.ImportRecordingRule(ctx, apiClient, recordingOnly, dataset)
		if importErr != nil {
			return results, client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "recording rule",
				AssetName: dash0api.GetRecordingRuleName(recordingOnly),
			})
		}
		results = append(results, applyResult{
			kind:   "RecordingRule",
			name:   result.Name,
			id:     result.ID,
			action: applyAction(result.Action),
			before: result.Before,
			after:  result.After,
		})
	}

	return results, nil
}

// applySpamFilter handles both v1alpha1 and v1alpha2 spam filter documents.
// The apiVersion field on the document selects the schema; an unknown value
// is rejected with the list of supported versions before any API call.
func applySpamFilter(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]applyResult, error) {
	apiVersion, err := asset.DetectSpamFilterAPIVersion(doc.Raw)
	if err != nil {
		return nil, err
	}

	switch apiVersion {
	case string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1):
		var filter dash0api.SpamFilter
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha1 SpamFilter: %w", err)
		}
		result, importErr := asset.ImportSpamFilter(ctx, apiClient, &filter, dataset)
		if importErr != nil {
			return nil, client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "spam filter",
				AssetName: dash0api.GetSpamFilterName(&filter),
			})
		}
		return []applyResult{{kind: doc.Kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil
	case string(dash0api.SpamFilterApiVersionV1Alpha2V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha2 SpamFilter: %w", err)
		}
		result, importErr := asset.ImportSpamFilterV1Alpha2(ctx, apiClient, &filter, dataset)
		if importErr != nil {
			return nil, client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "spam filter",
				AssetName: filter.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.Kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil
	default:
		// Unreachable: DetectSpamFilterAPIVersion only returns supported values or an error.
		return nil, fmt.Errorf("unsupported spam filter apiVersion %q", apiVersion)
	}
}

// datasetsFirst returns documents with every Dash0Dataset document moved to the front, keeping
// the relative order of everything else. Assets live inside a dataset, and the API answers 403
// for an asset whose dataset does not exist yet, so a dataset file that sorts after its assets
// would otherwise stop the run before the dataset is created.
func datasetsFirst(documents []asset.Document) []asset.Document {
	var datasets, rest []asset.Document
	for _, doc := range documents {
		if normalizeKind(doc.Kind) == "dataset" {
			datasets = append(datasets, doc)
		} else {
			rest = append(rest, doc)
		}
	}
	return append(datasets, rest...)
}
