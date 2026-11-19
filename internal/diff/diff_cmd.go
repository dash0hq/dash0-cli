package diff

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	dash0api "github.com/dash0hq/dash0-api-client-go"

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
	documents, fromDirectory, targetVanished, err := asset.ReadInput(flags.File, flags.SinceFlagSet)
	if err != nil {
		return err
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

	var sincePlan *gitutil.SincePlan
	if flags.SinceFlagSet {
		plan, err := gitutil.ComputeSincePlan(ctx, flags.File, flags.Since)
		if err != nil {
			return err
		}
		sincePlan = plan
		if targetVanished {
			fromDirectory = plan.TargetWasDirectoryAtRef
		}
		if sincePlan.Warning != "" {
			// diff never mutates, so a non-ancestor ref needs only a warning,
			// not the confirmation apply asks for.
			fmt.Fprintf(os.Stderr, "warning: %s\n", sincePlan.Warning)
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// Every document is read before anything is printed, so a failed read
	// aborts the whole preview instead of printing a partial one.
	planned, err := planDocuments(ctx, apiClient, documents, dataset)
	if err != nil {
		return err
	}
	warnDeniedReads(planned, dataset)

	pending, err := renderReport(planned, sincePlan, fromDirectory, flags.File, flags.Since)
	if err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	return &PendingDifferencesError{Count: pending}
}

// maxConcurrentReads bounds the parallel reads against the Dash0 API.
const maxConcurrentReads = 8

// planDocuments plans every document in parallel. The result keeps document
// order, and the error reported is the first one in document order among the
// reads that ran.
//
// Datasets are planned before every other kind, as apply applies them first.
// An asset in a dataset this run creates cannot be stored yet, and the API
// answers 403 instead of 404 for a read in a dataset that does not exist.
func planDocuments(ctx context.Context, apiClient dash0api.Client, documents []asset.Document, dataset *string) ([]plannedDoc, error) {
	planned := make([]plannedDoc, len(documents))
	errs := make([]error, len(documents))
	isDataset := func(doc asset.Document) bool { return asset.NormalizeKind(doc.Kind) == "dataset" }

	planEach(ctx, apiClient, documents, dataset, false, isDataset, planned, errs)
	if err := firstError(errs); err != nil {
		return nil, err
	}
	inNewDataset := createsDataset(planned, dataset)
	planEach(ctx, apiClient, documents, dataset, inNewDataset, func(doc asset.Document) bool { return !isDataset(doc) }, planned, errs)
	if err := firstError(errs); err != nil {
		return nil, err
	}
	return planned, nil
}

// planEach plans, in parallel, the documents that match selects. It starts no
// new read once one has failed, because any failure aborts the preview.
func planEach(ctx context.Context, apiClient dash0api.Client, documents []asset.Document, dataset *string, inNewDataset bool, selects func(asset.Document) bool, planned []plannedDoc, errs []error) {
	slots := make(chan struct{}, maxConcurrentReads)
	var wg sync.WaitGroup
	var failed atomic.Bool
	for i, doc := range documents {
		if !selects(doc) {
			continue
		}
		slots <- struct{}{}
		if failed.Load() {
			<-slots
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			plans, err := planDocument(ctx, apiClient, doc, dataset, inNewDataset)
			if err != nil {
				errs[i] = fmt.Errorf("%s (%s): %w", doc.Location(), doc.Kind, err)
				failed.Store(true)
				return
			}
			planned[i] = plannedDoc{doc: doc, plans: plans}
		}()
	}
	wg.Wait()
}

func firstError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// warnDeniedReads names each asset whose 403 was dropped, because a token
// without access to an existing dataset gets the same answers.
func warnDeniedReads(planned []plannedDoc, dataset *string) {
	for _, pd := range planned {
		for _, p := range pd.plans {
			if p.readDenied {
				fmt.Fprintf(os.Stderr, "warning: %s %q (%s): read denied in dataset %q, assumed new because this run creates %q; a token without access to %q gives the same result\n",
					asset.KindDisplayName(p.displayKind), p.name, p.plan.Key, *dataset, *dataset, *dataset)
			}
		}
	}
}

// createsDataset reports whether a planned Dash0Dataset document creates the
// dataset the dataset-scoped documents of this run are read from.
func createsDataset(planned []plannedDoc, dataset *string) bool {
	if dataset == nil {
		return false
	}
	for _, pd := range planned {
		for _, p := range pd.plans {
			if p.displayKind == "Dash0Dataset" && p.plan.Action == asset.ActionCreated && p.plan.Key == *dataset {
				return true
			}
		}
	}
	return false
}
