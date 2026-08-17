package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
)

// dryRunRow is one asset --dry-run reports on: either being validated
// (create/update) or, when --since is set, removed from -f's contents.
// Rows within a file are sorted by originOrID rather than input order, so a
// file with both a surviving and a removed asset presents them together
// instead of as two separately-headed sections.
type dryRunRow struct {
	op         string // "apply" or "delete"
	kind       string
	name       string
	originOrID string
	// detail carries the alert-deletion case's extra context (which
	// PrometheusRule CRD the alert was removed from) for text rendering
	// only -- the JSON schema's {op, name, originOrId} shape has no field
	// for it.
	detail string
}

// dryRunChangeJSON and dryRunFileJSON are --agent-mode --dry-run's JSON
// output shape: an array of {path, changes}, one entry per file, each
// change naming the operation, the asset's display name, and its id/origin.
type dryRunChangeJSON struct {
	Op         string `json:"op"`
	Name       string `json:"name"`
	OriginOrID string `json:"originOrId"`
}

type dryRunFileJSON struct {
	Path    string              `json:"path"`
	Changes []dryRunChangeJSON `json:"changes"`
}

// buildDryRunRows groups documents (always) and dp's deletion plan (only
// when dp is non-nil) into per-file rows, sorted within each file by
// originOrID (id/origin) for both text and JSON rendering to share.
func buildDryRunRows(documents []assetDocument, dp *deletionPlan) (rowsByFile map[string][]dryRunRow, files []string, validatedFileSet map[string]bool) {
	rowsByFile = map[string][]dryRunRow{}
	validatedFileSet = map[string]bool{}
	addRow := func(file string, row dryRunRow) {
		if _, seen := rowsByFile[file]; !seen {
			files = append(files, file)
		}
		rowsByFile[file] = append(rowsByFile[file], row)
	}

	// Needed to place an alert deletion (identified only by its surviving
	// CRD's identifier, not a file path) under the same file as the CRD's
	// own validated entry.
	crdFileByIdentifier := map[string]string{}
	for _, doc := range documents {
		identifier, err := dash0yaml.ExtractIdentifier(doc.raw)
		if err != nil || identifier == "" {
			identifier = doc.id
		}
		if normalizeKind(doc.kind) == "prometheusrule" {
			crdFileByIdentifier[identifier] = doc.filePath
		}
		validatedFileSet[doc.filePath] = true
		addRow(doc.filePath, dryRunRow{op: "apply", kind: doc.kind, name: doc.name, originOrID: identifier})
	}

	if dp != nil {
		for _, d := range dp.plan.ByIdentifier {
			// dp.names is resolved from git history and best-effort: a
			// lookup failure (rewritten/gc'd blob) falls back to an
			// explicit "<name>" placeholder rather than silently omitting
			// it.
			name := dp.names[d.Path]
			if name == "" {
				name = "<name>"
			}
			basePath, _ := splitMultiDocPath(d.Path)
			addRow(basePath, dryRunRow{op: "delete", kind: d.Kind, name: name, originOrID: d.Identifier})
		}
		for _, a := range dp.plan.AlertsByName {
			addRow(crdFileByIdentifier[a.CRDIdentifier], dryRunRow{
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
	return rowsByFile, files, validatedFileSet
}

// runDryRun renders --dry-run's output: validation results, merged with
// --since's deletion plan when dp is non-nil. Emits agent-mode JSON when
// active, plain text otherwise.
func runDryRun(documents []assetDocument, fromDirectory bool, fileArg, since string, dp *deletionPlan) error {
	if dp != nil && dp.warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", dp.warning)
	}

	rowsByFile, files, validatedFileSet := buildDryRunRows(documents, dp)

	if agentmode.Enabled {
		return renderDryRunJSON(rowsByFile, files, fromDirectory, fileArg)
	}
	renderDryRunText(rowsByFile, files, validatedFileSet, fromDirectory, len(documents), since, dp)
	return nil
}

func renderDryRunText(rowsByFile map[string][]dryRunRow, files []string, validatedFileSet map[string]bool, fromDirectory bool, documentCount int, since string, dp *deletionPlan) {
	switch {
	case dp == nil && fromDirectory:
		fmt.Printf("Dry run: %s from %s validated\n", pluralize(documentCount, "document"), pluralize(len(validatedFileSet), "file"))
	case dp == nil:
		fmt.Printf("Dry run: %s validated\n", pluralize(documentCount, "document"))
	default:
		deletionCount := len(dp.plan.ByIdentifier) + len(dp.plan.AlertsByName)
		if deletionCount == 0 {
			fmt.Printf("Dry run: %s%s validated; --since: no deletions\n", pluralize(documentCount, "document"), fileSuffix(fromDirectory, len(validatedFileSet)))
		} else {
			fmt.Printf("Dry run: %s%s validated; %s pending due to --since '%s'\n",
				pluralize(documentCount, "document"), fileSuffix(fromDirectory, len(validatedFileSet)), pluralize(deletionCount, "deletion"), since)
		}
	}

	if !fromDirectory {
		var flat []dryRunRow
		for _, f := range files {
			flat = append(flat, rowsByFile[f]...)
		}
		sort.SliceStable(flat, func(i, j int) bool { return flat[i].originOrID < flat[j].originOrID })
		for _, r := range flat {
			fmt.Printf("  * %s\n", renderDryRunLine(r))
		}
		return
	}

	for _, f := range files {
		fmt.Printf("  %s\n", f)
		for _, r := range rowsByFile[f] {
			fmt.Printf("    * %s\n", renderDryRunLine(r))
		}
	}
}

func renderDryRunLine(r dryRunRow) string {
	verb := "Apply"
	if r.op == "delete" {
		verb = "Delete"
	}
	if r.detail != "" {
		return fmt.Sprintf("%s %s %q (%s)", verb, asset.KindDisplayName(r.kind), r.name, r.detail)
	}
	return fmt.Sprintf("%s %s %s", verb, asset.KindDisplayName(r.kind), formatNameAndId(r.name, r.originOrID))
}

// fileSuffix renders the " from N files" clause the --since summary line
// adds only when the target was a directory — a single-file or stdin target
// has no file count worth stating.
func fileSuffix(fromDirectory bool, fileCount int) string {
	if !fromDirectory {
		return ""
	}
	return " from " + pluralize(fileCount, "file")
}

// renderDryRunJSON emits the {path, changes} array agent mode expects. A
// single-file or stdin target (!fromDirectory) has no real per-document file
// grouping to report, so every row is collected under one entry keyed by the
// literal -f argument.
func renderDryRunJSON(rowsByFile map[string][]dryRunRow, files []string, fromDirectory bool, fileArg string) error {
	toChanges := func(rows []dryRunRow) []dryRunChangeJSON {
		changes := make([]dryRunChangeJSON, 0, len(rows))
		for _, r := range rows {
			changes = append(changes, dryRunChangeJSON{Op: r.op, Name: r.name, OriginOrID: r.originOrID})
		}
		return changes
	}

	out := []dryRunFileJSON{}
	if !fromDirectory {
		var flat []dryRunRow
		for _, f := range files {
			flat = append(flat, rowsByFile[f]...)
		}
		sort.SliceStable(flat, func(i, j int) bool { return flat[i].originOrID < flat[j].originOrID })
		out = append(out, dryRunFileJSON{Path: fileArg, Changes: toChanges(flat)})
	} else {
		for _, f := range files {
			out = append(out, dryRunFileJSON{Path: f, Changes: toChanges(rowsByFile[f])})
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
