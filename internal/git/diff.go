package git

import (
	"sort"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// Deletion is one asset --since determined must be deleted: its identifier
// was present in the "before" Snapshot (git, at <ref>) and is absent from
// the "after" Snapshot (current disk contents).
type Deletion struct {
	Kind       string
	Identifier string
	// Path is the path the asset was found at in the "before" snapshot,
	// kept for diagnostic/logging purposes only — deletion is dispatched by
	// (Kind, Identifier), never by path.
	Path string
	// SpamFilterUsesOrigin records whether the deleted spam filter carried a
	// dash0.com/origin label, when Kind is "spamfilter" (meaningless for
	// every other kind). false means the filter was identified by
	// dash0.com/id alone — its live id may have been reassigned server-side
	// since this identifier was recorded, so the delete dispatch warns
	// rather than deleting silently.
	SpamFilterUsesOrigin bool
	// PrometheusAlerts carries every alerting rule the CRD had at the
	// "before" snapshot, when Kind is "prometheusrule" (nil for every other
	// kind, and for a CRD with zero or one alert). A CRD with two or more
	// alerts has each alert's real check rule living at its own derived id
	// (asset.DeriveAlertCheckRuleID), not at the CRD's literal Identifier --
	// the delete dispatch needs this list to compute and delete each of
	// those derived ids individually, since Identifier alone only ever
	// named a check rule for a single-alert CRD.
	PrometheusAlerts []dash0yaml.PrometheusAlertName
}

// AlertDeletion is a single PrometheusRule alerting rule that disappeared
// from a CRD that otherwise still exists (its own CRD-level identifier is
// present in both snapshots). Detected by (group, alert) name, since the
// CRD's shared identifier cannot distinguish between the alerts it contains.
type AlertDeletion struct {
	// CRDIdentifier is the surviving CRD's own identifier, informational
	// only — dispatch resolves the check rule to delete by name (see
	// PrometheusAlertName.CheckRuleName), not by this identifier.
	CRDIdentifier string
	dash0yaml.PrometheusAlertName
}

// DeletionPlan is the result of diffing two Snapshots: everything --since
// determined must be deleted, plus the set of deleted documents that had no
// stable identifier at all (which must fail the whole run rather than be
// silently skipped or silently applied).
type DeletionPlan struct {
	ByIdentifier []Deletion
	AlertsByName []AlertDeletion
	NoIdentifier []string
}

// IsEmpty reports whether the plan calls for no deletions and has no
// no-identifier failures to surface.
func (p DeletionPlan) IsEmpty() bool {
	return len(p.ByIdentifier) == 0 && len(p.AlertsByName) == 0 && len(p.NoIdentifier) == 0
}

// Diff compares before (the Snapshot at <ref>) against after (the Snapshot
// of current disk contents) and returns everything that must be deleted.
// This is a pure two-point comparison — an asset created and deleted again
// between <ref> and now is invisible to it, by design (see design.md).
func Diff(before, after Snapshot) DeletionPlan {
	var plan DeletionPlan

	for key, path := range before.Identifiers {
		if _, stillPresent := after.Identifiers[key]; stillPresent {
			continue
		}
		deletion := Deletion{
			Kind:                 key.Kind,
			Identifier:           key.Identifier,
			Path:                 path,
			SpamFilterUsesOrigin: before.SpamFilterUsesOriginByIdentifier[key.Identifier],
		}
		if key.Kind == "prometheusrule" {
			deletion.PrometheusAlerts = before.PrometheusAlertsByIdentifier[key.Identifier]
		}
		plan.ByIdentifier = append(plan.ByIdentifier, deletion)
	}

	// A PrometheusRule CRD that survives (its own identifier is present in
	// both snapshots) can still lose its recording-rule role entirely --
	// e.g. its last `record:` entry removed while an `alert:` entry keeps
	// the CRD's identifier alive. Unlike alerting rules, recording rules
	// have no per-item identity to diff by (Dash0 models a CRD's recording
	// rules as one server-side resource), so this is a coarse
	// presence/absence check rather than a name-based diff like AlertsByName
	// below. A CRD whose identifier disappeared entirely is skipped here:
	// deletePrometheusRuleCRD already attempts the recording-rules endpoint
	// unconditionally for a whole-CRD deletion, so adding a second entry
	// for the same identifier would just double-delete it.
	for identifier, hadRecordingRule := range before.PrometheusRecordingRoleByIdentifier {
		if !hadRecordingRule {
			continue
		}
		hasRecordingRuleNow, crdSurvives := after.PrometheusRecordingRoleByIdentifier[identifier]
		if !crdSurvives || hasRecordingRuleNow {
			continue
		}
		plan.ByIdentifier = append(plan.ByIdentifier, Deletion{
			Kind:       "recordingrule",
			Identifier: identifier,
			Path:       before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: identifier}],
		})
	}

	sort.Slice(plan.ByIdentifier, func(i, j int) bool {
		if plan.ByIdentifier[i].Kind != plan.ByIdentifier[j].Kind {
			return plan.ByIdentifier[i].Kind < plan.ByIdentifier[j].Kind
		}
		return plan.ByIdentifier[i].Identifier < plan.ByIdentifier[j].Identifier
	})

	for identifier, beforeAlerts := range before.PrometheusAlertsByIdentifier {
		afterAlerts, crdSurvives := after.PrometheusAlertsByIdentifier[identifier]
		if !crdSurvives {
			// Whole CRD deletion is already covered by the Identifiers loop
			// above.
			continue
		}
		afterSet := make(map[dash0yaml.PrometheusAlertName]bool, len(afterAlerts))
		for _, name := range afterAlerts {
			afterSet[name] = true
		}
		for _, name := range beforeAlerts {
			if !afterSet[name] {
				plan.AlertsByName = append(plan.AlertsByName, AlertDeletion{
					CRDIdentifier:       identifier,
					PrometheusAlertName: name,
				})
			}
		}
	}
	sort.Slice(plan.AlertsByName, func(i, j int) bool {
		a, b := plan.AlertsByName[i], plan.AlertsByName[j]
		if a.CRDIdentifier != b.CRDIdentifier {
			return a.CRDIdentifier < b.CRDIdentifier
		}
		return a.CheckRuleName() < b.CheckRuleName()
	})

	// A no-identifier document carries no id/origin to track across
	// snapshots, so file existence is the finest-grained signal available —
	// but that signal must be file-existence-AND-count, not file-existence
	// alone: a no-identifier document removed from a multi-document file
	// that otherwise survives is just as much a deletion design.md says must
	// never be silently skipped as one whose whole file disappeared.
	beforeDocPathsByFile := map[string][]string{}
	for docPath, doc := range before.NoIdentifier {
		beforeDocPathsByFile[doc.FilePath] = append(beforeDocPathsByFile[doc.FilePath], docPath)
	}
	afterCountByFile := map[string]int{}
	for _, doc := range after.NoIdentifier {
		afterCountByFile[doc.FilePath]++
	}
	for filePath, docPaths := range beforeDocPathsByFile {
		sort.Strings(docPaths)
		if !after.Paths[filePath] {
			// The whole file is gone; every no-identifier document in it
			// counts as deleted.
			plan.NoIdentifier = append(plan.NoIdentifier, docPaths...)
			continue
		}
		survivingCount := afterCountByFile[filePath]
		if survivingCount < len(docPaths) {
			// Some no-identifier document(s) vanished from this
			// otherwise-surviving file. Which exact document(s) can't be
			// known — no-identifier documents have nothing to correlate by
			// besides file path and count — so the trailing docPaths (by
			// the file's original doc-index order) are reported as a
			// deterministic, stable choice.
			plan.NoIdentifier = append(plan.NoIdentifier, docPaths[survivingCount:]...)
		}
	}
	sort.Strings(plan.NoIdentifier)

	return plan
}
