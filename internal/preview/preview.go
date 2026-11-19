// Package preview holds the change list that `apply --dry-run` and `diff`
// print, so both report the same rows and the same agent-mode JSON.
package preview

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
)

// Change is one asset a run would apply, create, update, or delete.
type Change struct {
	Op         string // "apply", "create", "update", or "delete"
	Kind       string
	Name       string
	OriginOrID string
	// Detail names the PrometheusRule CRD a removed alert came from. Text only.
	Detail string
	// Diff is the unified diff of an update, empty when nothing changed.
	Diff string
	// TextKind replaces Kind in text output when set. diff prints the kind
	// apply prints, and its JSON carries the declared kind, as apply --dry-run
	// does, so one PersesDashboard is "Dashboard" in text and
	// "PersesDashboard" in JSON for both commands.
	TextKind string
}

// TextKindName is the kind name text output shows for the change.
func (c Change) TextKindName() string {
	if c.TextKind != "" {
		return asset.KindDisplayName(c.TextKind)
	}
	return asset.KindDisplayName(c.Kind)
}

// Pending reports whether the change would modify Dash0. An update with no
// diff would not.
func (c Change) Pending() bool {
	return c.Op != "update" || c.Diff != ""
}

// Plan groups changes by the file they come from.
type Plan struct {
	byFile map[string][]Change
	files  []string
}

func (p *Plan) Add(file string, change Change) {
	if p.byFile == nil {
		p.byFile = map[string][]Change{}
	}
	if _, seen := p.byFile[file]; !seen {
		p.files = append(p.files, file)
	}
	p.byFile[file] = append(p.byFile[file], change)
}

// AddDeletions adds the assets sincePlan would delete and returns how many it
// added. A check rule that documents still declare is left out, because apply
// --since skips its deletion.
func (p *Plan) AddDeletions(sincePlan *gitutil.SincePlan, documents []asset.Document) int {
	// An alert deletion carries only its CRD's identifier, so it goes under
	// the file that still declares that CRD.
	crdFileByIdentifier := map[string]string{}
	for _, doc := range documents {
		if asset.NormalizeKind(doc.Kind) != "prometheusrule" {
			continue
		}
		identifier, err := asset.ExtractIdentifier(doc.Raw)
		if err != nil || identifier == "" {
			identifier = doc.ID
		}
		crdFileByIdentifier[identifier] = doc.FilePath
	}

	added := 0
	for _, deletion := range sincePlan.Plan.ByIdentifier {
		if deletion.Kind == "checkrule" && sincePlan.DeclaredCheckRuleIDs[deletion.Identifier] {
			continue
		}
		// Names come from git history; a failed lookup shows a placeholder.
		name := sincePlan.Names[deletion.Path]
		if name == "" {
			name = "<name>"
		}
		basePath, _ := gitutil.SplitMultiDocPath(deletion.Path)
		file := gitutil.StripScope(basePath, sincePlan.Scope)
		if deletion.Kind == "prometheusrule" && len(deletion.PrometheusAlerts) > 1 {
			added += p.addMultiAlertCRDDeletion(file, name, deletion, sincePlan.DeclaredCheckRuleIDs)
			continue
		}
		p.Add(file, Change{Op: "delete", Kind: deletion.Kind, Name: name, OriginOrID: deletion.Identifier})
		added++
	}
	for _, alert := range sincePlan.Plan.AlertsByName {
		p.Add(crdFileByIdentifier[alert.CRDIdentifier], Change{
			Op:         "delete",
			Kind:       "checkrule",
			Name:       alert.CheckRuleName(),
			OriginOrID: alert.CRDIdentifier,
			Detail:     fmt.Sprintf("alert removed from PrometheusRule %s", alert.CRDIdentifier),
		})
		added++
	}
	return added
}

// addMultiAlertCRDDeletion lists a removed CRD with two or more alerts as one
// check rule per alert, at the derived id apply --since deletes, because
// nothing is stored at the CRD's own identifier. A declared id is left out,
// as apply skips it.
func (p *Plan) addMultiAlertCRDDeletion(file, crdName string, deletion gitutil.Deletion, declared map[string]bool) int {
	added := 0
	ids := asset.CheckRuleIDsOccupiedByCRD(deletion.Identifier, deletion.PrometheusAlerts)
	for i, alert := range deletion.PrometheusAlerts {
		if declared[ids[i]] {
			continue
		}
		p.Add(file, Change{Op: "delete", Kind: "checkrule", Name: alert.CheckRuleName(), OriginOrID: ids[i]})
		added++
	}
	if deletion.PrometheusHadRecordingRule {
		p.Add(file, Change{Op: "delete", Kind: "recordingrule", Name: crdName, OriginOrID: deletion.Identifier})
		added++
	}
	return added
}

// Files returns the files in sorted order, each one's changes sorted by
// OriginOrID, so a surviving and a removed asset in one file show together.
func (p *Plan) Files() []string {
	sort.Strings(p.files)
	for _, file := range p.files {
		changes := p.byFile[file]
		sort.SliceStable(changes, func(i, j int) bool { return changes[i].OriginOrID < changes[j].OriginOrID })
	}
	return p.files
}

func (p *Plan) Changes(file string) []Change {
	return p.byFile[file]
}

// Group is the changes of one file. File is empty for a single-file or stdin
// target.
type Group struct {
	File    string
	Changes []Change
}

// Groups returns one group per file for a directory target. A single-file or
// stdin target has no file grouping to show, so it gets one group with every
// change sorted by OriginOrID.
func (p *Plan) Groups(fromDirectory bool) []Group {
	var groups []Group
	if fromDirectory {
		for _, file := range p.Files() {
			groups = append(groups, Group{File: file, Changes: p.byFile[file]})
		}
		return groups
	}
	var flat []Change
	for _, file := range p.Files() {
		flat = append(flat, p.byFile[file]...)
	}
	sort.SliceStable(flat, func(i, j int) bool { return flat[i].OriginOrID < flat[j].OriginOrID })
	return []Group{{Changes: flat}}
}

// DeleteLine is the line apply --dry-run and diff print for a deletion.
func DeleteLine(change Change) string {
	kind := change.TextKindName()
	if change.Detail != "" {
		return fmt.Sprintf("Delete %s %q (%s)", kind, change.Name, change.Detail)
	}
	return fmt.Sprintf("Delete %s %s", kind, asset.FormatNameAndID(change.Name, change.OriginOrID))
}

// PendingCount returns how many changes would modify Dash0.
func (p *Plan) PendingCount() int {
	count := 0
	for _, changes := range p.byFile {
		for _, change := range changes {
			if change.Pending() {
				count++
			}
		}
	}
	return count
}

type changeJSON struct {
	Op         string `json:"op"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	OriginOrID string `json:"originOrId"`
	// Since is the --since ref a deletion came from. An agent deciding whether
	// to approve a deletion needs it.
	Since string `json:"since,omitempty"`
}

type fileJSON struct {
	Path    string       `json:"path"`
	Changes []changeJSON `json:"changes"`
}

// WriteJSON writes the agent-mode {path, changes} array. A single-file or stdin
// target is one entry keyed by fileArg. Changes that are not pending are left
// out, and so is a file that has no pending change.
func (p *Plan) WriteJSON(w io.Writer, fromDirectory bool, fileArg, since string) error {
	toJSON := func(changes []Change) []changeJSON {
		out := make([]changeJSON, 0, len(changes))
		for _, change := range changes {
			if !change.Pending() {
				continue
			}
			entry := changeJSON{Op: change.Op, Kind: asset.KindDisplayName(change.Kind), Name: change.Name, OriginOrID: change.OriginOrID}
			if change.Op == "delete" {
				entry.Since = since
			}
			out = append(out, entry)
		}
		return out
	}

	out := []fileJSON{}
	for _, group := range p.Groups(fromDirectory) {
		path := group.File
		if !fromDirectory {
			path = fileArg
		}
		changes := toJSON(group.Changes)
		// A file whose changes are all unchanged updates is left out. A file
		// with no changes at all is kept, because a single-file target with an
		// empty plan must still print [{path, changes: []}].
		if len(changes) == 0 && len(group.Changes) > 0 {
			continue
		}
		out = append(out, fileJSON{Path: path, Changes: changes})
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
