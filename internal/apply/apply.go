package apply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
  - View
  - Dash0SpamFilter
  - Dash0NotificationChannel
  - Dash0Team
  - Dash0TimeSeriesAggregation

A PrometheusRule CRD that mixes alerting and recording rules is dispatched to both endpoints; alerting rules become check rules and recording rules become a recording rule.

If an asset exists, it will be updated. If it doesn't exist, it will be created.

[experimental] Pass --since <ref> (requires --experimental/-X) to also delete assets whose definition existed at <ref> but is no longer present in -f's current contents, detected by identifier (id or origin), never by file path. --force skips the per-deletion confirmation prompt and accepts a non-ancestor --since ref; --accept-non-ancestor-ref accepts a non-ancestor ref on its own, without also skipping the per-deletion prompt.` + internal.CONFIG_HINT,
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

// assetDocument represents a parsed YAML document with its kind
type assetDocument struct {
	kind     string
	name     string // human-readable name extracted from the document
	id       string // asset ID extracted from the document (location varies by kind)
	raw      []byte
	filePath string // relative path when loaded from a directory, empty for stdin/single-file
	docIndex int    // 1-based index within the file
	docCount int    // total number of documents in the file
}

// location returns a human-readable string describing where this document came from.
func (d assetDocument) location() string {
	if d.filePath == "" {
		return fmt.Sprintf("document %d", d.docIndex)
	}
	if d.docCount == 1 {
		return d.filePath
	}
	return fmt.Sprintf("%s: document %d", d.filePath, d.docIndex)
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
	var documents []assetDocument
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
		documents, err = readMultiDocumentYAML("-", os.Stdin)
		if err != nil {
			return validationError(err.Error())
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
			documents, err = readDirectory(flags.File)
			if err != nil {
				if flags.SinceFlagSet && errors.Is(err, errNoYAMLFilesFound) {
					// Every asset definition that used to live in this
					// directory was deleted, but the (now-empty) directory
					// itself survives. Same all-deletions case as above.
					documents = nil
				} else {
					return validationError(err.Error())
				}
			}
		default:
			documents, err = readMultiDocumentYAML(flags.File, nil)
			if err != nil {
				return validationError(err.Error())
			}
		}
	}

	if len(documents) == 0 && !flags.SinceFlagSet {
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
		if targetVanished {
			fromDirectory = plan.targetWasDirectoryAtRef
		}
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
			label := formatNameAndId(r.name, r.id)
			applied = append(applied, fmt.Sprintf("%s %s", displayKind, label))

			if r.action == actionUpdated && r.before != nil {
				if err := asset.PrintDiff(os.Stdout, displayKind, r.name, r.before, r.after); err != nil {
					return err
				}
			} else if fromDirectory {
				fmt.Printf("%s: %s %s %s\n", doc.filePath, displayKind, label, r.action)
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
			return fmt.Errorf("%s (%s): %w", doc.location(), doc.kind, applyErr)
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
			// --force also accepts this (backward compatible: it always
			// implied "proceed unattended"), but --accept-non-ancestor-ref
			// lets a caller accept a doubtful ref on its own, without also
			// giving up the per-asset deletion prompts below.
			acceptRef := flags.Force || flags.AcceptNonAncestorRef
			confirmed, confirmErr := confirmation.ConfirmDestructiveOperation(ctx, "Continue with --since's deletions? [y/N]: ", acceptRef)
			if confirmErr != nil || !confirmed {
				skipped := len(deletionPlan.plan.ByIdentifier) + len(deletionPlan.plan.AlertsByName)
				fmt.Fprintf(os.Stderr, "--since's deletion phase skipped; the rest of the run already completed\n")
				return fmt.Errorf("%s not confirmed for deletion (--since ref is not an ancestor of HEAD)", pluralize(skipped, "asset"))
			}
		}
		declined, err := applyDeletions(ctx, apiClient, dataset, deletionPlan, flags.Force)
		if err != nil {
			return err
		}
		if declined > 0 {
			return fmt.Errorf("%s declined; the rest of the --since run completed", pluralize(declined, "deletion"))
		}
	}

	return nil
}

// validateDocuments checks all documents up front, collecting all errors so a multi-doc apply is
// never partially triggered by a problem detectable before the first API call. Non-fatal warnings
// are collected separately — callers only print them when validation succeeds, since a warning
// about a document that never gets applied would be noise next to a hard error.
func validateDocuments(documents []assetDocument) (validationErrors, validationWarnings []string) {
	for _, doc := range documents {
		if doc.kind == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: missing 'kind' field", doc.location()))
		} else if !isValidKind(doc.kind) {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: unsupported kind %q (supported: Dashboard, PersesDashboard, CheckRule, PrometheusRule, SyntheticCheck, View, Dash0SpamFilter, Dash0NotificationChannel, Dash0Team, Dash0TimeSeriesAggregation)", doc.location(), doc.kind))
		} else if normalizeKind(doc.kind) == "spamfilter" {
			// Catch unknown spam filter apiVersions during validation rather
			// than after the first PUT, so a partial apply of a multi-doc input
			// is never triggered by a typo in apiVersion.
			if _, err := asset.DetectSpamFilterAPIVersion(doc.raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.location(), err.Error()))
			}
		} else if normalizeKind(doc.kind) == "prometheusrule" {
			// Catch CRDs that contain no usable rules at all up front, before
			// any API call. ParseAsPrometheusAlertRules already rejects
			// alert-only-empty CRDs, but a CRD with zero rules of either kind
			// would otherwise slip through to applyDocument.
			if err := validatePrometheusRule(doc.raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.location(), err.Error()))
			}
		} else if normalizeKind(doc.kind) == "notificationchannel" {
			// A document carrying spec.routing.assets gets a non-fatal warning — the API treats
			// the field as API-managed and silently ignores it on write; the apply proceeds as
			// usual. Parse errors are already caught during metadata extraction in
			// readMultiDocumentYAML.
			var channel dash0api.NotificationChannelDefinition
			if err := sigsyaml.Unmarshal(doc.raw, &channel); err == nil {
				if warning := asset.RoutingAssetsWarning(&channel); warning != "" {
					validationWarnings = append(validationWarnings, fmt.Sprintf("%s: %s", doc.location(), warning))
				}
			}
		} else if normalizeKind(doc.kind) == "timeseriesaggregation" {
			// Origin is mandatory for this kind — the API rejects a create
			// without one, and there is no fallback create path — so a
			// document missing it fails here rather than after the run has
			// already written its other documents.
			var aggregation dash0api.TimeSeriesAggregationDefinition
			if err := sigsyaml.Unmarshal(doc.raw, &aggregation); err == nil {
				if asset.GetTimeSeriesAggregationOrigin(&aggregation) == "" {
					validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.location(), asset.ErrTimeSeriesAggregationMissingOrigin.Error()))
				}
			}
		}
	}
	return validationErrors, validationWarnings
}

// validationError formats one or more validation issues into a consistent
// "validation failed with N error/errors:" message.
func validationError(issues ...string) error {
	return fmt.Errorf("validation failed with %s:\n  %s", pluralize(len(issues), "error"), strings.Join(issues, "\n  "))
}

// pluralize returns "1 thing" or "N things" depending on count.
// This logic is currently naive and assumes all plurals are formed by adding "s".
// This happens to work for all our asset kinds, errors, documents and other situations
// we use this logic.
func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

// formatNameAndId returns a display string with name and optional ID.
func formatNameAndId(name, id string) string {
	if name != "" && id != "" {
		return fmt.Sprintf("%q (%s)", name, id)
	}
	if name != "" {
		return fmt.Sprintf("%q", name)
	}
	if id != "" {
		return fmt.Sprintf("(%s)", id)
	}
	return ""
}

// parseDocumentHeader extracts the kind, human-readable name, and ID from
// raw YAML bytes. It unmarshals into typed structs and delegates to the
// Extract* functions in internal/asset/ so that apply and the per-asset
// CRUD commands use the same extraction logic.
func parseDocumentHeader(data []byte) (kind, name, id string, err error) {
	kind, err = dash0yaml.DetectKind(data)
	if err != nil {
		return "", "", "", err
	}

	switch normalizeKind(kind) {
	case "dashboard":
		var dashboard dash0api.DashboardDefinition
		if err := sigsyaml.Unmarshal(data, &dashboard); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetDashboardName(&dashboard)
		if name == "" {
			name = dashboard.Metadata.Name
		}
		id = dash0api.GetDashboardID(&dashboard)

	case "checkrule":
		var rule dash0api.PrometheusAlertRule
		if err := sigsyaml.Unmarshal(data, &rule); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetCheckRuleName(&rule)
		id = dash0api.GetCheckRuleID(&rule)

	case "view":
		var view dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(data, &view); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetViewName(&view)
		id = dash0api.GetViewID(&view)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := sigsyaml.Unmarshal(data, &check); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetSyntheticCheckName(&check)
		id = dash0api.GetSyntheticCheckID(&check)

	case "prometheusrule":
		// We only need metadata (name + ID) here; the Metadata struct has no
		// time.Duration fields, so a partial unmarshal via sigsyaml is safe.
		var partial struct {
			Metadata dash0api.PrometheusRulesMetadata `json:"metadata"`
		}
		if err := sigsyaml.Unmarshal(data, &partial); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = partial.Metadata.Name
		id = partial.Metadata.Labels[dash0api.LabelID]

	case "persesdashboard":
		var perses dash0api.PersesDashboard
		if err := sigsyaml.Unmarshal(data, &perses); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetPersesDashboardName(&perses)
		id = dash0api.GetPersesDashboardID(&perses)

	case "spamfilter":
		// Metadata fields (name + dash0.com/id label) are identical across
		// v1alpha1 and v1alpha2, so we use the v1alpha1 type for the header
		// peek regardless of the document's apiVersion. The apiVersion-aware
		// dispatch happens in applyDocument.
		var filter dash0api.SpamFilter
		if err := sigsyaml.Unmarshal(data, &filter); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetSpamFilterName(&filter)
		id = dash0api.GetSpamFilterID(&filter)

	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := sigsyaml.Unmarshal(data, &channel); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetNotificationChannelName(&channel)
		id = dash0api.GetNotificationChannelID(&channel)
		if id == "" {
			// Notification channels use origin as the upsert key. Surface it
			// as the ID in dry-run/listing output so users can see which
			// existing channel the document will replace.
			id = dash0api.GetNotificationChannelOrigin(&channel)
		}

	case "timeseriesaggregation":
		var aggregation dash0api.TimeSeriesAggregationDefinition
		if err := sigsyaml.Unmarshal(data, &aggregation); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetTimeSeriesAggregationName(&aggregation)
		// Origin, not id, is what this kind upserts by, and it is mandatory.
		// Showing it as the ID in dry-run output names the aggregation the
		// document will replace; an exported document's server-assigned id
		// would name the same object but not the key being used.
		id = asset.GetTimeSeriesAggregationOrigin(&aggregation)

	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := sigsyaml.Unmarshal(data, &team); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetTeamDisplayName(&team)
		if name == "" {
			name = dash0api.GetTeamName(&team)
		}
		id = dash0api.GetTeamID(&team)
		if id == "" {
			// Teams use origin as the upsert key.
			id = dash0api.GetTeamOrigin(&team)
		}

	default:
		var raw map[string]any
		if err := sigsyaml.Unmarshal(data, &raw); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = yamlStringFromMap(raw, "metadata", "name")
	}

	return kind, name, id, nil
}

// yamlString traverses a nested map[string]any by the given keys and returns
// the leaf value as a string, or "" if any key is missing or the value is not
// a string.
func yamlStringFromMap(m map[string]any, keys ...string) string {
	for i, key := range keys {
		val, ok := m[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			s, _ := val.(string)
			return s
		}
		m, ok = val.(map[string]any)
		if !ok {
			return ""
		}
	}
	return ""
}

// readMultiDocumentYAML splits a YAML stream into individual documents.
// This is the only place that requires gopkg.in/yaml.v3 directly —
// sigs.k8s.io/yaml doesn't provide a streaming decoder for multi-document YAML.
func readMultiDocumentYAML(filePath string, stdin io.Reader) ([]assetDocument, error) {
	var data []byte
	var err error

	if filePath == "-" {
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("no input provided on stdin")
		}
	} else {
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	return parseMultiDocumentYAML(data)
}

// parseMultiDocumentYAML splits data on YAML document boundaries and parses
// each into an assetDocument. Factored out of readMultiDocumentYAML so
// callers that already have file content in memory (e.g. --since's
// git-history name lookups, which read a blob via ReadFileAtRef rather than
// a path on disk) can reuse the same parsing without a round trip through
// the filesystem.
func parseMultiDocumentYAML(data []byte) ([]assetDocument, error) {
	var documents []assetDocument
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse YAML:\n    %w", err)
		}

		// Skip empty documents
		if node.Kind == 0 {
			continue
		}

		// Re-encode the node to get the raw bytes for this document
		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(&node); err != nil {
			return nil, fmt.Errorf("failed to re-encode document: %w", err)
		}
		encoder.Close()

		kind, name, id, err := parseDocumentHeader(buf.Bytes())
		if err != nil {
			return nil, err
		}

		documents = append(documents, assetDocument{
			kind:     kind,
			name:     name,
			id:       id,
			raw:      buf.Bytes(),
			docIndex: len(documents) + 1,
		})
	}

	// Handle single-document files without YAML document markers
	if len(documents) == 0 && len(data) > 0 {
		kind, name, id, _ := parseDocumentHeader(data)
		if kind != "" {
			documents = append(documents, assetDocument{
				kind:     kind,
				name:     name,
				id:       id,
				raw:      data,
				docIndex: 1,
			})
		}
	}

	// Set docCount on all documents
	for i := range documents {
		documents[i].docCount = len(documents)
	}

	return documents, nil
}

// errNoYAMLFilesFound is wrapped into discoverFiles' "no .yaml or .yml files
// found" error so callers can distinguish "the directory is legitimately
// empty" from any other failure via errors.Is, without matching on message
// text. runApply uses this to tolerate an empty directory specifically when
// --since is set: every asset that used to live there may simply have been
// deleted, which is a valid all-deletions run, not a usage error.
var errNoYAMLFilesFound = errors.New("no .yaml or .yml files found")

// discoverFiles recursively finds all .yaml/.yml files under dirPath,
// skipping hidden entries (names starting with '.').
// Returns paths relative to dirPath, sorted lexicographically.
func discoverFiles(dirPath string) ([]string, error) {
	var paths []string
	var hasNestedDirs bool
	if err := filepath.WalkDir(dirPath, asset.FindNonHiddenYAMLFiles(dirPath, &paths, &hasNestedDirs)); err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	var files []string
	for _, path := range paths {
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return nil, err
		}
		files = append(files, rel)
	}
	if len(files) == 0 {
		if hasNestedDirs {
			return nil, fmt.Errorf("%w in %s and nested directories", errNoYAMLFilesFound, dirPath)
		}
		return nil, fmt.Errorf("%w in %s", errNoYAMLFilesFound, dirPath)
	}
	sort.Strings(files)
	return files, nil
}

// readDirectory reads all YAML files from a directory recursively.
func readDirectory(dirPath string) ([]assetDocument, error) {
	files, err := discoverFiles(dirPath)
	if err != nil {
		return nil, err
	}

	var allDocs []assetDocument
	for _, relPath := range files {
		fullPath := filepath.Join(dirPath, relPath)
		docs, err := readMultiDocumentYAML(fullPath, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relPath, err)
		}
		for i := range docs {
			docs[i].filePath = relPath
		}
		allDocs = append(allDocs, docs...)
	}

	return allDocs, nil
}

func isValidKind(kind string) bool {
	return asset.IsValidKind(kind)
}

func normalizeKind(kind string) string {
	// Normalize common variations
	k := strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	k = strings.ReplaceAll(k, "_", "")
	k = strings.TrimPrefix(k, "dash0")
	return k
}

func applyDocument(ctx context.Context, apiClient dash0api.Client, doc assetDocument, dataset *string) ([]applyResult, error) {
	switch normalizeKind(doc.kind) {
	case "dashboard", "persesdashboard":
		dashboard, err := dash0yaml.ParseAsDashboard(doc.raw)
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
		if err := sigsyaml.Unmarshal(doc.raw, &check); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		result, err := asset.ImportSyntheticCheck(ctx, apiClient, &check, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "synthetic check",
				AssetName: check.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "view":
		var view dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(doc.raw, &view); err != nil {
			return nil, fmt.Errorf("failed to parse View: %w", err)
		}
		result, err := asset.ImportView(ctx, apiClient, &view, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "view",
				AssetName: view.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil

	case "spamfilter":
		return applySpamFilter(ctx, apiClient, doc, dataset)

	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := sigsyaml.Unmarshal(doc.raw, &channel); err != nil {
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
		if err := sigsyaml.Unmarshal(doc.raw, &aggregation); err != nil {
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
		if err := sigsyaml.Unmarshal(doc.raw, &team); err != nil {
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
		return nil, fmt.Errorf("unsupported kind: %s", doc.kind)
	}
}

// applyCheckRule handles a single CheckRule (native, non-CRD) document.
// PrometheusRule CRD documents go through applyPrometheusRule, which inspects
// the rule kinds and dispatches to the check-rule or recording-rule endpoint
// (or both, when the CRD is mixed).
func applyCheckRule(ctx context.Context, apiClient dash0api.Client, doc assetDocument, dataset *string) ([]applyResult, error) {
	rules, err := asset.ParseCheckRules(doc.raw)
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
func applyPrometheusRule(ctx context.Context, apiClient dash0api.Client, doc assetDocument, dataset *string) ([]applyResult, error) {
	crd, err := parsePrometheusRuleCRD(doc.raw)
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
	names, err := asset.ExtractPrometheusAlertNames(data)
	if err != nil {
		return err
	}
	if err := asset.CheckAlertNameCollisions(names); err != nil {
		return err
	}
	return nil
}

// applySpamFilter handles both v1alpha1 and v1alpha2 spam filter documents.
// The apiVersion field on the document selects the schema; an unknown value
// is rejected with the list of supported versions before any API call.
func applySpamFilter(ctx context.Context, apiClient dash0api.Client, doc assetDocument, dataset *string) ([]applyResult, error) {
	apiVersion, err := asset.DetectSpamFilterAPIVersion(doc.raw)
	if err != nil {
		return nil, err
	}

	switch apiVersion {
	case string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1):
		var filter dash0api.SpamFilter
		if err := sigsyaml.Unmarshal(doc.raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha1 SpamFilter: %w", err)
		}
		result, importErr := asset.ImportSpamFilter(ctx, apiClient, &filter, dataset)
		if importErr != nil {
			return nil, client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "spam filter",
				AssetName: dash0api.GetSpamFilterName(&filter),
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil
	case string(dash0api.V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := sigsyaml.Unmarshal(doc.raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha2 SpamFilter: %w", err)
		}
		result, importErr := asset.ImportSpamFilterV1Alpha2(ctx, apiClient, &filter, dataset)
		if importErr != nil {
			return nil, client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "spam filter",
				AssetName: filter.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action), before: result.Before, after: result.After}}, nil
	default:
		// Unreachable: DetectSpamFilterAPIVersion only returns supported values or an error.
		return nil, fmt.Errorf("unsupported spam filter apiVersion %q", apiVersion)
	}
}
