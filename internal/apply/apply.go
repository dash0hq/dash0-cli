package apply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
  - EphemeralDashboard
  - CheckRule (or PrometheusRule CRD with alerting rules)
  - PrometheusRule CRD with recording rules
  - SyntheticCheck
  - SLO
  - View
  - Dash0SpamFilter
  - Dash0NotificationChannel
  - Dash0Team
  - Dash0TimeSeriesAggregation
  - Dash0SignalToMetrics (signal-to-metrics rule)
  - Dash0Sampling (sampling rule)
  - Dash0Dataset
  - Dash0Skill (Agent0 custom skill)
  - Dash0AwsMetricScrapeConfig
  - Dash0GenerativeAiEvaluation (eval)

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

func runApply(ctx context.Context, flags *applyFlags) error {
	// When the target vanished, fromDirectory is only a guess; computeDeletionPlan
	// answers it from git history at the --since ref below, so a vanished
	// single-file target does not render like a directory scan.
	documents, fromDirectory, targetVanished, err := asset.ReadInput(flags.File, flags.SinceFlagSet)
	if err != nil {
		return err
	}

	if len(documents) == 0 && !flags.SinceFlagSet {
		return asset.FormatValidationError("no documents found in input")
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	validationErrors, validationWarnings := asset.ValidateDocuments(documents)
	validationErrors = append(validationErrors, asset.CheckInferredOriginCollisions(documents, dataset)...)
	if len(validationErrors) > 0 {
		return asset.FormatValidationError(validationErrors...)
	}
	for _, warning := range validationWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	var deletionPlan *gitutil.SincePlan
	if flags.SinceFlagSet {
		plan, err := computeDeletionPlan(ctx, flags, dataset)
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

	// A dataset delete starts the 30-day countdown to removing its assets, and
	// a sampling rule delete is permanent and changes which traces Dash0
	// keeps. Like `datasets delete` and `sampling-rules delete`, agent mode
	// must pass --force deliberately instead of getting the usual agent-mode
	// auto-confirmation. Checked before any document is applied, so a refusal
	// leaves the run untouched.
	if deletionPlan != nil {
		for _, d := range deletionPlan.Plan.ByIdentifier {
			var note string
			switch d.Kind {
			case "dataset":
				note = asset.DatasetSoftDeleteNote
			case "sampling":
				note = asset.SamplingRuleDeleteNote
			default:
				continue
			}
			if err := confirmation.RequireForceInAgentMode(flags.Force, fmt.Sprintf("deleting %s %q with --since", asset.KindDisplayName(d.Kind), d.Identifier),
				"re-run with --force to confirm. "+note); err != nil {
				return err
			}
		}
	}

	// Create API client
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// Apply each document
	var applied []string
	writes := asset.InferredOriginWrites{}
	for _, doc := range datasetsFirst(documents) {
		results, applyErr := asset.ApplyDocument(asset.WithInferredOriginWrites(ctx, writes, doc.Location()), apiClient, doc, dataset)
		for _, r := range results {
			displayKind := asset.KindDisplayName(r.Kind)
			label := asset.FormatNameAndID(r.Name, r.ID)
			applied = append(applied, fmt.Sprintf("%s %s", displayKind, label))

			if r.Action == asset.ActionUpdated && r.Before != nil {
				if err := asset.PrintDiff(os.Stdout, displayKind, r.Name, r.Before, r.After); err != nil {
					return err
				}
				continue
			}
			if fromDirectory {
				fmt.Printf("%s: %s %s %s\n", doc.FilePath, displayKind, label, r.Action)
			} else {
				fmt.Printf("%s %s %s\n", displayKind, label, r.Action)
			}
			if r.Action == asset.ActionRestored {
				if err := asset.PrintChanges(os.Stdout, displayKind, r.Before, r.After); err != nil {
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
		deletionPlan.PrintSkippedDeletionWarnings(os.Stderr)
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

// datasetsFirst returns documents with every Dash0Dataset document moved to the front, keeping
// the relative order of everything else. Assets live inside a dataset, and the API answers 403
// for an asset whose dataset does not exist yet, so a dataset file that sorts after its assets
// would otherwise stop the run before the dataset is created.
func datasetsFirst(documents []asset.Document) []asset.Document {
	var datasets, rest []asset.Document
	for _, doc := range documents {
		if asset.NormalizeKind(doc.Kind) == "dataset" {
			datasets = append(datasets, doc)
		} else {
			rest = append(rest, doc)
		}
	}
	return append(datasets, rest...)
}
