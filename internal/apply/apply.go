package apply

import (
	"context"
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
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
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
  - View
  - Dash0SpamFilter
  - Dash0NotificationChannel
  - Dash0Team

A PrometheusRule CRD that mixes alerting and recording rules is dispatched to both endpoints; alerting rules become check rules and recording rules become a recording rule.

If an asset exists, it will be updated. If it doesn't exist, it will be created.

[experimental] Pass --since <ref> (requires --experimental/-X) to also delete assets whose definition existed at <ref> but is no longer present in -f's current contents, detected by identifier (id or origin), never by file path. --force skips the per-deletion confirmation prompt.` + internal.CONFIG_HINT,
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
  dash0 --experimental apply -f dashboards/ --since HEAD~1 --force`,
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
				return fmt.Errorf("--since '%s' cannot be used with -f - (stdin); it needs a file or directory path to compare against git history", flags.Since)
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
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip the confirmation prompt for deletions triggered by --since")

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
	var err error

	if flags.File == "-" {
		// Read from stdin
		documents, err = asset.ReadMultiDocumentYAML("-", os.Stdin)
		if err != nil {
			return validationError(err.Error())
		}
	} else {
		info, statErr := os.Stat(flags.File)
		if statErr != nil {
			return fmt.Errorf("failed to read input: %w", statErr)
		}
		if info.IsDir() {
			fromDirectory = true
			documents, err = asset.ReadDirectory(flags.File)
			if err != nil {
				return validationError(err.Error())
			}
		} else {
			documents, err = asset.ReadMultiDocumentYAML(flags.File, nil)
			if err != nil {
				return validationError(err.Error())
			}
		}
	}

	if len(documents) == 0 {
		return validationError("no documents found in input")
	}

	validationErrors, validationWarnings := validateDocuments(documents)
	if len(validationErrors) > 0 {
		return validationError(validationErrors...)
	}
	for _, warning := range validationWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	var deletionPlan *deletionPlan
	if flags.SinceFlagSet {
		plan, err := computeDeletionPlan(ctx, flags)
		if err != nil {
			return err
		}
		deletionPlan = plan
	}

	if flags.DryRun {
		return runDryRun(documents, fromDirectory, flags.File, flags.Since, deletionPlan)
	}

	// Create API client
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// Apply each document
	var applied []string
	for _, doc := range documents {
		results, applyErr := applyDocument(ctx, apiClient, doc, dataset)
		for _, r := range results {
			displayKind := asset.KindDisplayName(r.kind)
			label := asset.FormatNameAndID(r.name, r.id)
			applied = append(applied, fmt.Sprintf("%s %s", displayKind, label))

			if r.action == actionUpdated && r.before != nil {
				if err := asset.PrintDiff(os.Stdout, displayKind, r.name, r.before, r.after); err != nil {
					return err
				}
			} else if fromDirectory {
				fmt.Printf("%s: %s %s %s\n", doc.FilePath, displayKind, label, r.action)
			} else {
				fmt.Printf("%s %s %s\n", displayKind, label, r.action)
			}
		}
		if applyErr != nil {
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
		if deletionPlan.warning != "" {
			fmt.Fprintf(os.Stderr, "warning: %s\n", deletionPlan.warning)
			confirmed, confirmErr := confirmation.ConfirmDestructiveOperation(ctx, "Continue with --since's deletions? [y/N]: ", flags.Force)
			if confirmErr != nil || !confirmed {
				skipped := len(deletionPlan.plan.ByIdentifier) + len(deletionPlan.plan.AlertsByName)
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

// validateDocuments checks all documents up front, collecting all errors so a multi-doc apply is
// never partially triggered by a problem detectable before the first API call. Non-fatal warnings
// are collected separately — callers only print them when validation succeeds, since a warning
// about a document that never gets applied would be noise next to a hard error.
func validateDocuments(documents []asset.Document) (validationErrors, validationWarnings []string) {
	for _, doc := range documents {
		if doc.Kind == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: missing 'kind' field", doc.Location()))
		} else if !isValidKind(doc.Kind) {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: unsupported kind %q (supported: Dashboard, PersesDashboard, CheckRule, PrometheusRule, SyntheticCheck, View, Dash0SpamFilter, Dash0NotificationChannel, Dash0Team)", doc.Location(), doc.Kind))
		} else if normalizeKind(doc.Kind) == "spamfilter" {
			// Catch unknown spam filter apiVersions during validation rather
			// than after the first PUT, so a partial apply of a multi-doc input
			// is never triggered by a typo in apiVersion.
			if _, err := asset.DetectSpamFilterAPIVersion(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if normalizeKind(doc.Kind) == "prometheusrule" {
			// Catch CRDs that contain no usable rules at all up front, before
			// any API call. ParseAsPrometheusAlertRules already rejects
			// alert-only-empty CRDs, but a CRD with zero rules of either kind
			// would otherwise slip through to applyDocument.
			if err := validatePrometheusRule(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if normalizeKind(doc.Kind) == "notificationchannel" {
			// A document carrying spec.routing.assets gets a non-fatal warning — the API treats
			// the field as API-managed and silently ignores it on write; the apply proceeds as
			// usual. Parse errors are already caught during metadata extraction in
			// asset.ReadMultiDocumentYAML.
			var channel dash0api.NotificationChannelDefinition
			if err := sigsyaml.Unmarshal(doc.Raw, &channel); err == nil {
				if warning := asset.RoutingAssetsWarning(&channel); warning != "" {
					validationWarnings = append(validationWarnings, fmt.Sprintf("%s: %s", doc.Location(), warning))
				}
			}
		}
	}
	return validationErrors, validationWarnings
}

// validationError formats one or more validation issues into a consistent
// "validation failed with N error/errors:" message.
func validationError(issues ...string) error {
	return fmt.Errorf("validation failed with %s:\n  %s", asset.Pluralize(len(issues), "error"), strings.Join(issues, "\n  "))
}

func isValidKind(kind string) bool {
	return asset.IsValidKind(kind)
}

func normalizeKind(kind string) string {
	return asset.NormalizeKind(kind)
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
		if err := sigsyaml.Unmarshal(doc.Raw, &check); err != nil {
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

	case "view":
		var view dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(doc.Raw, &view); err != nil {
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
		if err := sigsyaml.Unmarshal(doc.Raw, &channel); err != nil {
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

	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := sigsyaml.Unmarshal(doc.Raw, &team); err != nil {
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
	crd, err := parsePrometheusRuleCRD(doc.Raw)
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

// parsePrometheusRuleCRD parses raw bytes as a PrometheusRule CRD (the typed
// dash0api.RecordingRule, an alias for the generated PrometheusRule type that
// captures both Alert and Record per rule).
func parsePrometheusRuleCRD(data []byte) (*dash0api.RecordingRule, error) {
	var crd dash0api.RecordingRule
	if err := sigsyaml.Unmarshal(data, &crd); err != nil {
		return nil, fmt.Errorf("failed to parse PrometheusRule: %w", err)
	}
	return &crd, nil
}

// validatePrometheusRule rejects a PrometheusRule CRD that contains no
// alerting and no recording rules, so the failure surfaces in the validation
// phase rather than after the first request.
func validatePrometheusRule(data []byte) error {
	crd, err := parsePrometheusRuleCRD(data)
	if err != nil {
		return err
	}
	if !asset.PrometheusRuleHasAlerts(crd) && asset.RecordingOnlyPrometheusRule(crd) == nil {
		return fmt.Errorf("PrometheusRule contains no alerting or recording rules")
	}
	return nil
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
		if err := sigsyaml.Unmarshal(doc.Raw, &filter); err != nil {
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
	case string(dash0api.V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := sigsyaml.Unmarshal(doc.Raw, &filter); err != nil {
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
