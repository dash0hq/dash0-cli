package apply

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/dash0hq/dash0-cli/internal/preview"
)

// buildDryRunPlan lists every document as an "apply" change, plus dp's
// deletions when dp is non-nil. It returns the plan, the number of deletions,
// and the set of files that hold a validated document.
func buildDryRunPlan(documents []asset.Document, dp *gitutil.SincePlan) (plan *preview.Plan, deletionCount int, validatedFileSet map[string]bool) {
	plan = &preview.Plan{}
	validatedFileSet = map[string]bool{}
	for _, doc := range documents {
		identifier, err := asset.ExtractIdentifier(doc.Raw)
		if err != nil || identifier == "" {
			identifier = doc.ID
		}
		validatedFileSet[doc.FilePath] = true
		plan.Add(doc.FilePath, preview.Change{Op: "apply", Kind: doc.Kind, Name: doc.Name, OriginOrID: identifier})
	}
	if dp != nil {
		deletionCount = plan.AddDeletions(dp, documents)
	}
	return plan, deletionCount, validatedFileSet
}

// runDryRun renders --dry-run's output: validation results, merged with
// --since's deletion plan when dp is non-nil. Emits agent-mode JSON when
// active, plain text otherwise.
func runDryRun(documents []asset.Document, fromDirectory bool, fileArg, since string, dp *gitutil.SincePlan) error {
	if dp != nil && dp.Warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", dp.Warning)
	}

	plan, deletionCount, validatedFileSet := buildDryRunPlan(datasetsFirst(documents), dp)

	if agentmode.Enabled {
		return plan.WriteJSON(os.Stdout, fromDirectory, fileArg, since)
	}
	renderDryRunText(plan, validatedFileSet, fromDirectory, len(documents), deletionCount, since, dp != nil)
	return nil
}

func renderDryRunText(plan *preview.Plan, validatedFileSet map[string]bool, fromDirectory bool, documentCount, deletionCount int, since string, withSince bool) {
	switch {
	case !withSince && fromDirectory:
		fmt.Printf("Dry run: %s from %s validated\n", asset.Pluralize(documentCount, "document"), asset.Pluralize(len(validatedFileSet), "file"))
	case !withSince:
		fmt.Printf("Dry run: %s validated\n", asset.Pluralize(documentCount, "document"))
	default:
		if deletionCount == 0 {
			fmt.Printf("Dry run: %s%s validated; --since: no deletions\n", asset.Pluralize(documentCount, "document"), fileSuffix(fromDirectory, len(validatedFileSet)))
		} else {
			fmt.Printf("Dry run: %s%s validated; %s pending due to --since '%s'\n",
				asset.Pluralize(documentCount, "document"), fileSuffix(fromDirectory, len(validatedFileSet)), asset.Pluralize(deletionCount, "deletion"), since)
		}
	}

	if !fromDirectory {
		for _, change := range plan.Flat() {
			fmt.Printf("  * %s\n", renderDryRunLine(change))
		}
		return
	}

	for _, file := range plan.Files() {
		fmt.Printf("  %s\n", file)
		for _, change := range plan.Changes(file) {
			fmt.Printf("    * %s\n", renderDryRunLine(change))
		}
	}
}

func renderDryRunLine(change preview.Change) string {
	verb := "Apply"
	if change.Op == "delete" {
		verb = "Delete"
	}
	if change.Detail != "" {
		return fmt.Sprintf("%s %s %q (%s)", verb, asset.KindDisplayName(change.Kind), change.Name, change.Detail)
	}
	return fmt.Sprintf("%s %s %s", verb, asset.KindDisplayName(change.Kind), asset.FormatNameAndID(change.Name, change.OriginOrID))
}

// adds only when the target was a directory — a single-file or stdin target
// has no file count worth stating.
func fileSuffix(fromDirectory bool, fileCount int) string {
	if !fromDirectory {
		return ""
	}
	return " from " + asset.Pluralize(fileCount, "file")
}
