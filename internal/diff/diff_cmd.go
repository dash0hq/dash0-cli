package diff

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/spf13/cobra"
)

// diffFlags holds the flags for the diff command.
type diffFlags struct {
	ApiUrl       string
	AuthToken    string
	Dataset      string
	File         string
	Since        string
	SinceFlagSet bool
}

// NewDiffCmd creates the top-level diff command.
func NewDiffCmd() *cobra.Command {
	var flags diffFlags

	cmd := &cobra.Command{
		Use:   "diff -f <file|directory>",
		Short: "[experimental] Preview what apply would change, without changing anything",
		Long: `Preview the differences between local asset definitions and their current state in Dash0, without creating, updating, or deleting anything.

For each document, diff fetches its current state from Dash0 (when it carries an identifier and already exists) and prints a unified diff against the local definition. A document with no matching asset yet is reported as a create, so diff distinguishes create from update accurately -- unlike 'apply --dry-run', which is local-only and cannot tell the two apart.

All documents are fetched before anything is printed: if any fetch fails for a reason other than the asset simply not existing yet, the whole preview is aborted and nothing is printed.

Pass --since <ref> to also preview deletions: assets whose definition existed at <ref> but is no longer present in -f's current contents, detected by identifier (id or origin), never by file path. This mirrors 'apply --since' exactly, but never deletes anything.

Exit code: 0 when there are no differences, 1 when there are differences pending (creates, updates, or deletions), 2 on error.

Requires the -X (or --experimental) flag.` + internal.CONFIG_HINT,
		Example: `  # Preview changes for a single file
  dash0 --experimental diff -f dashboard.yaml

  # Preview changes for a directory (recursive)
  dash0 --experimental diff -f dashboards/

  # Preview changes from stdin
  cat assets.yaml | dash0 --experimental diff -f -

  # Also preview deletions detected since a git ref
  dash0 --experimental diff -f dashboards/ --since HEAD~1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s\nTo diff multiple files, pass a directory with -f instead of a glob pattern", strings.Join(args, " "))
			}
			if flags.File == "" {
				return fmt.Errorf("file is required; use -f to specify the file (use '-' for stdin)")
			}
			flags.SinceFlagSet = cmd.Flags().Changed("since")
			if flags.SinceFlagSet && flags.File == "-" {
				return fmt.Errorf("--since '%s' cannot be used with -f - (stdin); it needs a file or directory path to compare against git history", flags.Since)
			}
			cmd.SilenceUsage = true
			return runDiff(cmd.Context(), &flags)
		},
	}

	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to a file or directory containing asset definitions (use '-' for stdin)")
	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset to operate on")
	cmd.Flags().StringVar(&flags.Since, "since", "", "Also preview deletions: assets removed from -f's contents since this git ref")

	return cmd
}

func runDiff(ctx context.Context, flags *diffFlags) error {
	documents, fromDirectory, err := readDocuments(flags.File)
	if err != nil {
		return err
	}
	if len(documents) == 0 {
		return asset.FormatValidationError("no documents found in input")
	}

	validationErrors, validationWarnings := asset.ValidateDocuments(documents)
	if len(validationErrors) > 0 {
		return asset.FormatValidationError(validationErrors...)
	}
	for _, warning := range validationWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	var sincePlan *gitutil.SincePlan
	if flags.SinceFlagSet {
		plan, err := gitutil.ComputeSincePlan(ctx, flags.File, flags.Since)
		if err != nil {
			return err
		}
		sincePlan = plan
		if sincePlan.Warning != "" {
			// Unlike apply, diff never mutates, so a non-ancestor ref only
			// needs a warning -- there is nothing to confirm before
			// proceeding.
			fmt.Fprintf(os.Stderr, "warning: %s\n", sincePlan.Warning)
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// All-or-nothing fetch gate: plan (fetch) every document before printing
	// anything. A failure here aborts the whole preview -- no partial report
	// covering only the documents that happened to succeed.
	planned := make([]plannedDoc, 0, len(documents))
	for _, doc := range documents {
		plans, err := planDocument(ctx, apiClient, doc, dataset)
		if err != nil {
			return fmt.Errorf("%s (%s): %w", doc.Location(), doc.Kind, err)
		}
		planned = append(planned, plannedDoc{doc: doc, plans: plans})
	}

	pending, err := renderReport(planned, sincePlan, fromDirectory, flags.File)
	if err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	return &PendingDifferencesError{Count: pending}
}

// readDocuments reads flags.File the same way apply does: stdin ("-"), a
// single file, or a directory (recursively). Returns whether the target was
// a directory, since that affects how the report groups rows.
func readDocuments(file string) (documents []asset.Document, fromDirectory bool, err error) {
	if file == "-" {
		documents, err = asset.ReadMultiDocumentYAML("-", os.Stdin)
		if err != nil {
			return nil, false, asset.FormatValidationError(err.Error())
		}
		return documents, false, nil
	}

	info, statErr := os.Stat(file)
	if statErr != nil {
		return nil, false, fmt.Errorf("failed to read input: %w", statErr)
	}
	if info.IsDir() {
		documents, err = asset.ReadDirectory(file)
		if err != nil {
			return nil, true, asset.FormatValidationError(err.Error())
		}
		return documents, true, nil
	}

	documents, err = asset.ReadMultiDocumentYAML(file, nil)
	if err != nil {
		return nil, false, asset.FormatValidationError(err.Error())
	}
	return documents, false, nil
}
