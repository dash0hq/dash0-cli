package diff

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
)

// plannedDoc pairs one input document with the docPlans it expanded to (more
// than one for a PrometheusRule CRD with several alerts, or a mixed
// alerting+recording CRD).
type plannedDoc struct {
	doc   asset.Document
	plans []docPlan
}

// diffRow is one line of the diff report: a document that would be created
// or updated, or (with --since) an asset that would be deleted.
type diffRow struct {
	op         string // "create", "update", or "delete"
	kind       string
	name       string
	originOrID string
	// changed is only meaningful for op == "update": whether the fetched
	// state actually differs from the proposed content. An unchanged update
	// does not count toward the pending total, matching PrintDiff's own
	// "no changes" convention.
	changed bool
	// detail carries the alert-deletion case's extra context (which
	// PrometheusRule CRD the alert was removed from), text rendering only.
	detail string
}

// diffChangeJSON and diffFileJSON are --agent-mode's JSON output shape: an
// array of {path, changes}, mirroring `apply --dry-run --agent-mode`'s
// {op, name, originOrId} shape so agents already familiar with one recognize
// the other immediately. Unlike apply's "apply" op, diff distinguishes
// "create" from "update" since that is the entire point of a preview.
type diffChangeJSON struct {
	Op         string `json:"op"`
	Name       string `json:"name"`
	OriginOrID string `json:"originOrId"`
}

type diffFileJSON struct {
	Path    string           `json:"path"`
	Changes []diffChangeJSON `json:"changes"`
}

// buildRows classifies each document's plans into diffRow entries (grouped
// by the document's source file), plus --since's deletion candidates when
// sincePlan is non-nil.
func buildRows(planned []plannedDoc, sincePlan *gitutil.SincePlan) (rowsByFile map[string][]diffRow, files []string, err error) {
	rowsByFile = map[string][]diffRow{}
	addRow := func(file string, row diffRow) {
		if _, seen := rowsByFile[file]; !seen {
			files = append(files, file)
		}
		rowsByFile[file] = append(rowsByFile[file], row)
	}

	// Needed to place an alert deletion (identified only by its surviving
	// CRD's identifier, not a file path) under the same file as the CRD's
	// own validated entry -- mirrors apply's dry-run rendering.
	crdFileByIdentifier := map[string]string{}
	for _, pd := range planned {
		identifier, idErr := dash0yaml.ExtractIdentifier(pd.doc.Raw)
		if idErr != nil || identifier == "" {
			identifier = pd.doc.ID
		}
		if asset.NormalizeKind(pd.doc.Kind) == "prometheusrule" {
			crdFileByIdentifier[identifier] = pd.doc.FilePath
		}

		for _, p := range pd.plans {
			if p.before == nil {
				addRow(pd.doc.FilePath, diffRow{op: "create", kind: p.displayKind, name: p.name, originOrID: p.id})
				continue
			}
			changed, hasDiffErr := asset.HasDifference(p.before, p.after)
			if hasDiffErr != nil {
				return nil, nil, hasDiffErr
			}
			addRow(pd.doc.FilePath, diffRow{op: "update", kind: p.displayKind, name: p.name, originOrID: p.id, changed: changed})
		}
	}

	if sincePlan != nil {
		for _, d := range sincePlan.Plan.ByIdentifier {
			name := sincePlan.Names[d.Path]
			if name == "" {
				name = "<name>"
			}
			basePath, _ := gitutil.SplitMultiDocPath(d.Path)
			addRow(gitutil.StripScope(basePath, sincePlan.Scope), diffRow{op: "delete", kind: d.Kind, name: name, originOrID: d.Identifier})
		}
		for _, a := range sincePlan.Plan.AlertsByName {
			addRow(crdFileByIdentifier[a.CRDIdentifier], diffRow{
				op:         "delete",
				kind:       "checkrule",
				name:       a.CheckRuleName(),
				originOrID: a.CRDIdentifier,
				detail:     fmt.Sprintf("alert removed from PrometheusRule %s", a.CRDIdentifier),
			})
		}
	}

	sort.Strings(files)
	for _, f := range files {
		rows := rowsByFile[f]
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].originOrID < rows[j].originOrID })
	}
	return rowsByFile, files, nil
}

// pendingCount returns how many rows represent an actual pending difference:
// every create and delete, plus updates where HasDifference found a real
// change.
func pendingCount(rowsByFile map[string][]diffRow) int {
	count := 0
	for _, rows := range rowsByFile {
		for _, r := range rows {
			if r.op != "update" || r.changed {
				count++
			}
		}
	}
	return count
}

// renderReport prints the full diff report (human text or agent-mode JSON)
// and returns the number of pending differences.
func renderReport(planned []plannedDoc, sincePlan *gitutil.SincePlan, fromDirectory bool, fileArg string) (int, error) {
	rowsByFile, files, err := buildRows(planned, sincePlan)
	if err != nil {
		return 0, err
	}
	pending := pendingCount(rowsByFile)

	if agentmode.Enabled {
		return pending, renderReportJSON(rowsByFile, files, fromDirectory, fileArg)
	}
	renderReportText(planned, rowsByFile, files, fromDirectory, pending)
	return pending, nil
}

// updatesByFileAndKey indexes every plan with a non-nil before state (i.e.
// an update) by its source file and id/origin, so renderReportText can find
// the before/after pair for a given diffRow without threading them through
// the row itself.
func updatesByFileAndKey(planned []plannedDoc) map[string]map[string]docPlan {
	byFileAndKey := map[string]map[string]docPlan{}
	for _, pd := range planned {
		for _, p := range pd.plans {
			if p.before == nil {
				continue
			}
			m, ok := byFileAndKey[pd.doc.FilePath]
			if !ok {
				m = map[string]docPlan{}
				byFileAndKey[pd.doc.FilePath] = m
			}
			m[p.id] = p
		}
	}
	return byFileAndKey
}

func renderReportText(planned []plannedDoc, rowsByFile map[string][]diffRow, files []string, fromDirectory bool, pending int) {
	byFileAndKey := updatesByFileAndKey(planned)

	for _, f := range files {
		prefix := ""
		if fromDirectory {
			fmt.Printf("%s\n", f)
			prefix = "  "
		}
		for _, r := range rowsByFile[f] {
			switch r.op {
			case "create":
				fmt.Printf("%sCreate %s %s\n", prefix, asset.KindDisplayName(r.kind), asset.FormatNameAndID(r.name, r.originOrID))
			case "delete":
				if r.detail != "" {
					fmt.Printf("%sDelete %s %q (%s)\n", prefix, asset.KindDisplayName(r.kind), r.name, r.detail)
				} else {
					fmt.Printf("%sDelete %s %s\n", prefix, asset.KindDisplayName(r.kind), asset.FormatNameAndID(r.name, r.originOrID))
				}
			case "update":
				if p, ok := byFileAndKey[f][r.originOrID]; ok {
					_ = asset.PrintDiff(os.Stdout, asset.KindDisplayName(r.kind), r.name, p.before, p.after)
				}
			}
		}
	}

	if pending == 0 {
		fmt.Println("No differences")
	}
}

// renderReportJSON emits the {path, changes} array. A single-file or stdin
// target (!fromDirectory) has no real per-document file grouping to report,
// so every row is collected under one entry keyed by the literal -f
// argument. Rows for an unchanged update are omitted entirely.
func renderReportJSON(rowsByFile map[string][]diffRow, files []string, fromDirectory bool, fileArg string) error {
	toChanges := func(rows []diffRow) []diffChangeJSON {
		changes := make([]diffChangeJSON, 0, len(rows))
		for _, r := range rows {
			if r.op == "update" && !r.changed {
				continue
			}
			changes = append(changes, diffChangeJSON{Op: r.op, Name: r.name, OriginOrID: r.originOrID})
		}
		return changes
	}

	out := []diffFileJSON{}
	if !fromDirectory {
		var flat []diffRow
		for _, f := range files {
			flat = append(flat, rowsByFile[f]...)
		}
		sort.SliceStable(flat, func(i, j int) bool { return flat[i].originOrID < flat[j].originOrID })
		out = append(out, diffFileJSON{Path: fileArg, Changes: toChanges(flat)})
	} else {
		for _, f := range files {
			out = append(out, diffFileJSON{Path: f, Changes: toChanges(rowsByFile[f])})
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
