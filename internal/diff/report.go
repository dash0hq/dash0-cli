package diff

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/dash0hq/dash0-cli/internal/preview"
)

// plannedDoc pairs one input document with the docPlans it expanded to.
type plannedDoc struct {
	doc   asset.Document
	plans []docPlan
}

// buildPlan turns each docPlan into a create or an update, plus --since's
// deletions when sincePlan is non-nil. A soft-deleted asset is a create,
// because apply restores it. Each update is compared once, and the diff text
// is kept for printing.
func buildPlan(planned []plannedDoc, sincePlan *gitutil.SincePlan) (*preview.Plan, error) {
	plan := &preview.Plan{}
	documents := make([]asset.Document, 0, len(planned))
	for _, pd := range planned {
		documents = append(documents, pd.doc)
		for _, p := range pd.plans {
			change := preview.Change{Op: "create", Kind: p.displayKind, Name: p.name, OriginOrID: p.plan.Key}
			// A PrometheusRule expands to check rules and a recording rule,
			// which apply --dry-run's one CRD row has no kind for.
			if asset.NormalizeKind(pd.doc.Kind) != "prometheusrule" {
				change.Kind, change.TextKind = pd.doc.Kind, p.displayKind
			}
			if p.plan.Action == asset.ActionUpdated {
				text, err := asset.UnifiedDiff(asset.KindDisplayName(p.displayKind), p.plan.Before, p.plan.Proposed)
				if err != nil {
					return nil, err
				}
				change.Op, change.Diff = "update", text
			}
			plan.Add(pd.doc.FilePath, change)
		}
	}
	if sincePlan != nil {
		plan.AddDeletions(sincePlan, documents)
	}
	return plan, nil
}

// renderReport prints the report (text, or JSON in agent mode) and returns the
// number of pending differences.
func renderReport(planned []plannedDoc, sincePlan *gitutil.SincePlan, fromDirectory bool, fileArg, since string) (int, error) {
	plan, err := buildPlan(planned, sincePlan)
	if err != nil {
		return 0, err
	}
	pending := plan.PendingCount()

	if agentmode.Enabled {
		return pending, plan.WriteJSON(os.Stdout, fromDirectory, fileArg, since)
	}
	if err := renderReportText(plan, fromDirectory, pending); err != nil {
		return 0, err
	}
	return pending, nil
}

// renderReportText groups a directory target by file. A single-file or stdin
// target has no grouping, so it lists every change in the JSON's order.
func renderReportText(plan *preview.Plan, fromDirectory bool, pending int) error {
	for _, group := range plan.Groups(fromDirectory) {
		prefix := ""
		if fromDirectory {
			fmt.Printf("%s\n", group.File)
			prefix = "  "
		}
		if err := printChanges(group.Changes, prefix); err != nil {
			return err
		}
	}

	if pending == 0 {
		fmt.Println("No differences")
	}
	return nil
}

func printChanges(changes []preview.Change, prefix string) error {
	for _, change := range changes {
		kind := change.TextKindName()
		switch {
		case change.Op == "create":
			fmt.Printf("%sCreate %s %s\n", prefix, kind, asset.FormatNameAndID(change.Name, change.OriginOrID))
		case change.Op == "delete":
			fmt.Printf("%s%s\n", prefix, preview.DeleteLine(change))
		case change.Diff == "":
			fmt.Printf("%s%s %q: no changes\n", prefix, kind, change.Name)
		default:
			if err := asset.WriteDiff(os.Stdout, change.Diff); err != nil {
				return err
			}
		}
	}
	return nil
}
