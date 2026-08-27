package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
)

// deletionPlan wraps the identifier-diffing result from internal/git with a
// human-readable warning to surface when --since's ref resolved but is not
// an ancestor of HEAD.
type deletionPlan struct {
	plan    gitutil.DeletionPlan
	warning string
	// names best-effort maps each ByIdentifier deletion's Path (the
	// git-recorded path, unique within one plan) to its display name,
	// resolved by re-reading the asset's content from git history at the
	// --since ref. A missing entry means the lookup failed (e.g. a rewritten
	// or gc'd blob) or the asset had no name in the first place; callers
	// fall back to a placeholder. This is cosmetic only — deletion dispatch
	// (deleteAssetByKindAndIdentifier) never depends on it, only on
	// (kind, identifier).
	names map[string]string
	// scope is the --since target's path relative to the repository root
	// (forward-slashed, "" when the target is the repo root itself). Deletion
	// paths (gitutil.Deletion.Path, from git ls-tree) are always repo-root-
	// relative, while validated documents' assetDocument.filePath is always
	// relative to the -f target itself -- when the target is a subdirectory,
	// those two bases differ, so rendering must strip this prefix from a
	// deletion path before grouping it with validated documents from the same
	// file. See stripScope in dryrun.go.
	scope string
	// declaredCheckRuleIDs is every check-rule id -f's current contents still
	// declare, so a deletion dispatched by identifier never removes an asset
	// this same run created or updated. See declaredCheckRuleIDs.
	declaredCheckRuleIDs map[string]bool
	// targetWasDirectoryAtRef reports whether -f's target was a directory
	// (rather than a single file) the last time it existed, per git history
	// at --since's ref. runApply uses this to correct its own fromDirectory
	// guess for a target that no longer exists on disk at all: os.Stat can't
	// tell file from directory once the path is gone, but the ref usually
	// still can.
	targetWasDirectoryAtRef bool
}

// computeDeletionPlan resolves flags.Since against the git repository
// containing flags.File and diffs the identifier set at that ref against
// flags.File's current disk contents. It never talks to the Dash0 API —
// everything it needs comes from git and the local filesystem.
func computeDeletionPlan(ctx context.Context, flags *applyFlags) (*deletionPlan, error) {
	absFile, err := filepath.Abs(flags.File)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %s: %w", flags.File, err)
	}

	// absFile may no longer exist on disk at all: every asset definition
	// under -f's target may have been deleted, taking the directory itself
	// with them. A --since run only needs *some* real, existing path inside
	// the repository to locate its root -- not the target itself -- so walk
	// up to the nearest existing ancestor instead of requiring absFile to
	// exist, then reattach the missing suffix below so scope still reflects
	// the target's (now-vanished) location.
	existingAncestor, missingSuffix, err := nearestExistingAncestor(absFile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", flags.File, err)
	}
	// Resolve symlinks so absFile is comparable with repo.Root()'s output:
	// `git rev-parse --show-toplevel` always prints the fully-resolved real
	// path, but filepath.Abs alone does not resolve symlinks in parent
	// directories (e.g. macOS's /var -> /private/var), which would otherwise
	// make every filepath.Rel(repoRoot, absFile) below compute a bogus
	// "outside the repository" path.
	resolvedAncestor, err := filepath.EvalSymlinks(existingAncestor)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", flags.File, err)
	}
	absFile = filepath.Join(resolvedAncestor, missingSuffix)

	repoDir := resolvedAncestor
	if missingSuffix == "" {
		// absFile exists: preserve the original file-vs-directory dance
		// exactly as before. resolvedAncestor equals absFile itself here
		// (nearestExistingAncestor found no missing suffix because the
		// target already exists), so a single-file target must still be
		// rehomed to its *parent* directory -- `git -C <path-to-a-file>`
		// fails outright ("Not a directory"), unlike `git -C <a-directory>`.
		info, err := os.Stat(absFile)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", flags.File, err)
		}
		if !info.IsDir() {
			repoDir = filepath.Dir(absFile)
		}
	}
	repo := gitutil.Repo{Dir: repoDir}

	repoRoot, err := repo.Root(ctx)
	if err != nil {
		if gitutil.IsNotAGitRepository(err) {
			return nil, fmt.Errorf("--since '%s' requires %s to be inside a git repository, but it is not (no .git found there or in any parent directory)\nHint: point -f at a path inside the repository that tracks these assets, or drop --since to apply without deletion detection", flags.Since, flags.File)
		}
		return nil, fmt.Errorf("--since '%s' requires %s to be inside a git repository: %w", flags.Since, flags.File, err)
	}
	// Re-anchor at repoRoot: every scope-relative pathspec built below (and
	// passed to BuildSnapshotFromRef) is repo-root-relative, matching git
	// ls-tree's own path convention. Running git commands with -C repoDir
	// when repoDir is a subdirectory of the repo would otherwise resolve
	// those pathspecs relative to repoDir instead, silently matching nothing
	// (e.g. -f dashboards/ turning scope "dashboards" into the nonexistent
	// "dashboards/dashboards" once -C is already inside dashboards/).
	repo = gitutil.Repo{Dir: repoRoot}

	refState, sha, err := repo.ClassifyRef(ctx, flags.Since)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --since '%s' as Git reference: %w", flags.Since, err)
	}

	var warning string
	switch refState {
	case gitutil.RefEmpty:
		return nil, fmt.Errorf("--since '%s' resolved to an empty ref; there is no prior state to compare against\nHint: a CI-provided ref variable is likely unset (e.g. github.event.before on a workflow_dispatch or schedule trigger); skip --since for this invocation, or pass an explicit ref", flags.Since)
	case gitutil.RefAllZeros:
		return nil, fmt.Errorf("--since '%s' resolved to git's all-zeros SHA (%s), meaning there is no prior state to compare against\nHint: some CI systems report this value for a branch's first push; skip --since for this invocation, or pass an explicit ref", flags.Since, gitutil.AllZerosSHA)
	case gitutil.RefUnresolvable:
		// Best-effort: a simple "<base>~N"/"<base>^N" ref whose base
		// resolves fine but doesn't have N commits of history behind it
		// (e.g. --since HEAD~1 against a single-commit repo) gets a
		// specific reason instead of the generic typo-or-shallow-clone
		// guess, neither of which applies to that case.
		if reason := repo.ExplainUnresolvableRef(ctx, flags.Since); reason != "" {
			return nil, fmt.Errorf("--since '%s' could not be resolved: %s\nHint: pass a ref that exists this far back in the repository's history, or skip --since for this invocation", flags.Since, reason)
		}
		return nil, fmt.Errorf("--since '%s' could not be resolved\nHint: check the ref for a typo; if this is a shallow clone (actions/checkout defaults to fetch-depth: 1), re-run the checkout with fetch-depth: 0 so the ref's history is available", flags.Since)
	case gitutil.RefResolvedNonAncestor:
		// The confirmation for this case is deliberately NOT done here: doing
		// so would abort the entire apply run (including ordinary, unrelated
		// creates/updates) before any document is even processed, just
		// because the --since ref needs confirming. Instead, runApply asks
		// for confirmation immediately before calling applyDeletions, after
		// every document create/update has already gone through — mirroring
		// how a declined per-asset deletion (applyDeletions itself) never
		// blocks the rest of the run, just the deletions.
		warning = fmt.Sprintf("--since '%s' is not an ancestor of HEAD (likely a force-push or history rewrite); deletion detection may be inaccurate", flags.Since)
	}

	scope, err := filepath.Rel(repoRoot, absFile)
	if err != nil {
		return nil, fmt.Errorf("failed to compute %s's path relative to repository root %s: %w", flags.File, repoRoot, err)
	}
	scope = filepath.ToSlash(scope)
	if scope == "." {
		scope = ""
	}

	// A sparse checkout leaves tracked files off disk, so the disk side of the
	// diff sees fewer than the ref side and every absent one looks deleted.
	// Refuse rather than delete assets git still declares.
	sparse, err := repo.HasSkipWorktreeFiles(ctx, scope)
	if err != nil {
		return nil, err
	}
	if sparse {
		return nil, fmt.Errorf("--since '%s' cannot run against a sparse checkout: files tracked under %s are absent from disk, so they would be detected as deletions\nHint: run from a full checkout (git sparse-checkout disable), or drop --since to apply without deletion detection", flags.Since, flags.File)
	}

	targetWasDirectoryAtRef, err := repo.IsTreeAtRef(ctx, sha, scope)
	if err != nil {
		// The target may not have existed at this ref at all (e.g. it was
		// created after ref, then deleted before now) -- that's a legitimate
		// "nothing to report either way" outcome, not a hard failure; fall
		// back to treating it as not-a-directory, matching absFile's own
		// current-disk-state IsDir() default when nothing else is known.
		targetWasDirectoryAtRef = false
	}

	before, err := gitutil.BuildSnapshotFromRef(ctx, repo, sha, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to read git state at --since ref '%s': %w", flags.Since, err)
	}
	after, err := gitutil.BuildSnapshotFromDisk(ctx, absFile, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read current Git state: %w", err)
	}

	plan := gitutil.Diff(before, after)
	if len(plan.NoIdentifier) > 0 {
		return nil, fmt.Errorf("--since '%s' found %s deleted with no identifier its kind is upserted by, so deletion cannot be determined reliably:\n  %s\nHint: without a stable identifier there is no way to tell which live asset each document was; delete these assets directly in Dash0, or skip --since for this invocation",
			flags.Since, pluralize(len(plan.NoIdentifier), "document"), strings.Join(plan.NoIdentifier, "\n  "))
	}

	names := resolveDeletionNames(before, plan.ByIdentifier)

	return &deletionPlan{plan: plan, warning: warning, names: names, scope: scope, declaredCheckRuleIDs: declaredCheckRuleIDs(after), targetWasDirectoryAtRef: targetWasDirectoryAtRef}, nil
}

// nearestExistingAncestor walks up from path until it finds an entry that
// exists on disk, returning that ancestor plus the path components between
// it and path, joined back together with filepath.Join's separator so a
// caller can filepath.Join them straight onto the ancestor's symlink-resolved
// form. missingSuffix is "" when path itself already exists (the common
// case, unaffected by --since: the ancestor returned is then path itself).
//
// This lets computeDeletionPlan resolve a --since target that no longer
// exists on disk at all -- every asset definition under it may have been
// deleted, taking the directory with them -- without needing path itself to
// exist: locating the git repository only needs *some* real path inside it.
func nearestExistingAncestor(path string) (ancestor string, missingSuffix string, err error) {
	current := path
	var missing []string
	for {
		if _, statErr := os.Stat(current); statErr == nil {
			return current, filepath.Join(missing...), nil
		} else if !os.IsNotExist(statErr) {
			return "", "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", fmt.Errorf("no existing ancestor directory found for %s", path)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}
}

// resolveDeletionNames best-effort looks up each deletion's display name
// from before's content — the "before" (--since ref) Snapshot already read
// while building it (gitutil.Snapshot.RawContent), never a second read of
// the same git blob. before is the exact Snapshot that produced every
// deletion candidate in deletions (via gitutil.Diff), so a lookup miss can
// only mean a document that didn't parse cleanly, never a blob that
// genuinely needs (re-)fetching. Documents are parsed once per file and
// cached in memory for the duration of this call, so a multi-document file
// with several deletion candidates only pays the parse cost once.
//
// This is display polish, not correctness-critical: a lookup miss (content
// that no longer parses under today's rules) just omits that entry from the
// returned map rather than failing the run — --since's actual deletion
// dispatch never depends on a name, only on (kind, identifier).
func resolveDeletionNames(before gitutil.Snapshot, deletions []gitutil.Deletion) map[string]string {
	names := make(map[string]string, len(deletions))
	docsByFile := map[string][]assetDocument{}
	for _, d := range deletions {
		basePath, docIndex := splitMultiDocPath(d.Path)
		docs, cached := docsByFile[basePath]
		if !cached {
			if raw, ok := before.RawContent[basePath]; ok {
				docs, _ = parseMultiDocumentYAML(raw)
			}
			docsByFile[basePath] = docs
		}
		if docIndex < len(docs) && docs[docIndex].name != "" {
			names[d.Path] = docs[docIndex].name
		}
	}
	return names
}

// splitMultiDocPath splits a Deletion.Path — possibly suffixed "#<index>"
// for the second and later documents in a multi-document file, per
// internal/git/snapshot.go's ingestDocuments — into the base file path and
// the document's 0-based index within it, matching parseMultiDocumentYAML's
// return-slice indexing.
func splitMultiDocPath(path string) (basePath string, docIndex int) {
	idx := strings.LastIndex(path, "#")
	if idx == -1 {
		return path, 0
	}
	n, err := strconv.Atoi(path[idx+1:])
	if err != nil {
		return path, 0
	}
	return path[:idx], n
}

// Rendering (text and agent-mode JSON) for --dry-run, with or without a
// deletion plan, lives in dryrun.go.

// applyDeletions carries out dp's deletion plan against the Dash0 API,
// prompting per asset (skipped when force is set) exactly like every
// standalone `<kind> delete --force`. It returns the number of deletions the
// caller declined so runApply can report a non-zero exit even though the
// rest of the run succeeded.
//
// dp.warning (set when --since's ref is a non-ancestor) is not printed here:
// runApply already surfaced it once, as part of confirming whether to run
// the deletion phase at all, before calling this function. Printing it again
// here would show the identical line to the user twice for no reason.
func applyDeletions(ctx context.Context, apiClient dash0api.Client, dataset *string, dp *deletionPlan, force bool) (int, error) {
	declined := 0

	for _, d := range dp.plan.ByIdentifier {
		displayKind := asset.KindDisplayName(d.Kind)
		// dp.names is resolved from git history and best-effort: a lookup
		// failure falls back to an explicit "<name>" placeholder, matching
		// printDryRunWithDeletions' convention.
		name := dp.names[d.Path]
		if name == "" {
			name = "<name>"
		}
		display := formatNameAndId(name, d.Identifier)
		if d.Kind == "spamfilter" && !d.SpamFilterUsesOrigin {
			fmt.Fprintf(os.Stderr, "warning: spam filter %s was identified by dash0.com/id alone; its live id may have been reassigned by the server since this identifier was recorded (see docs/commands.md's asset-identifiers section), so this delete may miss the actual live filter\n", display)
		}
		var prompt string
		if d.Kind == "recordingrule" {
			// The surviving PrometheusRule CRD's own file is not what's
			// being removed here -- only its recording-rule role. The
			// generic "removed since --since ref" phrasing below would
			// wrongly suggest the whole document is gone.
			prompt = fmt.Sprintf("Are you sure you want to delete %s %s, whose last record was removed from its PrometheusRule since --since ref? [y/N]: ", displayKind, display)
		} else {
			prompt = fmt.Sprintf("Are you sure you want to delete %s %s, removed since --since ref? [y/N]: ", displayKind, display)
		}
		confirmed, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, force)
		if err != nil {
			return declined, err
		}
		if !confirmed {
			fmt.Fprintf(os.Stderr, "%s %s: deletion declined\n", displayKind, display)
			declined++
			continue
		}
		alreadyDeleted, err := deleteAssetByKindAndIdentifier(ctx, apiClient, dataset, d, dp.declaredCheckRuleIDs)
		if err != nil {
			return declined, fmt.Errorf("failed to delete %s %s: %w", displayKind, display, err)
		}
		// alreadyDeleted means IsAlreadyDeleted already printed its own
		// "was already deleted" line -- printing "deleted" here too would
		// contradict it and misrepresent a no-op as a real deletion in a CI
		// log or audit trail.
		if !alreadyDeleted {
			fmt.Printf("%s %s deleted\n", displayKind, display)
		}
	}

	var nameIndex checkRuleNameIndex
	if len(dp.plan.AlertsByName) > 0 {
		// One listing for the whole loop, not one per alert.
		var err error
		nameIndex, err = buildCheckRuleNameIndex(ctx, apiClient, dataset)
		if err != nil {
			return declined, err
		}
	}
	for _, a := range dp.plan.AlertsByName {
		name := a.CheckRuleName()
		prompt := fmt.Sprintf("Are you sure you want to delete check rule %q, an alert removed from a PrometheusRule since --since ref? [y/N]: ", name)
		confirmed, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, force)
		if err != nil {
			return declined, err
		}
		if !confirmed {
			fmt.Fprintf(os.Stderr, "Check rule %q: deletion declined\n", name)
			declined++
			continue
		}
		alreadyDeleted, err := deleteCheckRuleByName(ctx, apiClient, dataset, name, nameIndex)
		if err != nil {
			return declined, fmt.Errorf("failed to delete check rule %q: %w", name, err)
		}
		if !alreadyDeleted {
			fmt.Printf("Check rule %q deleted\n", name)
		}
	}

	return declined, nil
}

// deleteAssetByKindAndIdentifier dispatches a whole-asset deletion (an
// asset whose identifier disappeared entirely) to the matching per-kind
// delete API call, mirroring the dispatch applyDocument already uses for
// create/update.
//
// A 404 here always means "already gone" and is always tolerated,
// regardless of --force: --since's deletion phase is reconciling Dash0 to
// match git, and an asset that's already absent already matches the
// desired end state, whether or not the caller passed --force. --force
// keeps its own, separate job of skipping the confirmation prompt in
// applyDeletions, before this function is ever called. This is
// deliberately unlike the force-gated tolerance a standalone `<kind> delete`
// command uses: that command acts on one asset the caller named by hand, so
// a 404 without --force there is more likely a typo'd id worth surfacing
// loudly than a benign race.
//
// The returned bool reports whether the asset was already gone (rather
// than genuinely deleted by this call), so applyDeletions can avoid
// printing a "deleted" line that would contradict IsAlreadyDeleted's own
// "was already deleted" message -- printing both claims a deletion that
// never happened, which a CI log or audit trail would then take at face
// value.
func deleteAssetByKindAndIdentifier(ctx context.Context, apiClient dash0api.Client, dataset *string, d gitutil.Deletion, declared map[string]bool) (alreadyDeleted bool, err error) {
	kind, identifier := d.Kind, d.Identifier
	if kind == "prometheusrule" {
		return deletePrometheusRuleCRD(ctx, apiClient, dataset, identifier, d.PrometheusAlerts, declared)
	}

	switch kind {
	case "dashboard", "persesdashboard":
		err = apiClient.DeleteDashboard(ctx, identifier, dataset)
	case "checkrule":
		if declared[identifier] {
			// A surviving single-alert CRD's check rule lives here. Reporting
			// "nothing deleted" keeps applyDeletions from printing "deleted".
			warnSkippedDeclaredCheckRule(identifier, fmt.Sprintf("check rule %q", identifier))
			return true, nil
		}
		err = apiClient.DeleteCheckRule(ctx, identifier, dataset)
	case "syntheticcheck":
		err = apiClient.DeleteSyntheticCheck(ctx, identifier, dataset)
	case "recordingrule":
		// A surviving PrometheusRule CRD whose recording-rule role
		// disappeared entirely (see Diff's PrometheusRecordingRoleByIdentifier
		// handling) -- distinct from the whole-CRD "prometheusrule" case
		// above, which already attempts this endpoint unconditionally.
		err = apiClient.DeleteRecordingRule(ctx, identifier, dataset)
	case "view":
		err = apiClient.DeleteView(ctx, identifier, dataset)
	case "spamfilter":
		err = apiClient.DeleteSpamFilter(ctx, identifier, dataset)
	case "notificationchannel":
		err = apiClient.DeleteNotificationChannel(ctx, identifier)
	case "team":
		err = apiClient.DeleteTeam(ctx, identifier)
	default:
		return false, fmt.Errorf("unsupported kind for deletion: %s", kind)
	}

	ectx := client.ErrorContext{AssetType: asset.KindDisplayName(kind), AssetID: identifier}
	if err != nil {
		if client.IsAlreadyDeleted(err, true, ectx) {
			return true, nil
		}
		return false, client.HandleAPIError(err, ectx)
	}
	return false, nil
}

// deletePrometheusRuleCRD deletes a whole PrometheusRule CRD by identifier.
// The same identifier may back a check rule (from the CRD's alerting
// rules), a recording rule (from its recording rules), or both — apply's
// own create/update dispatch (applyPrometheusRule) sends a mixed CRD to
// both endpoints, so a mixed CRD's deletion attempts both too.
//
// alerts is the CRD's alerting-rule list at the "before" (--since ref)
// snapshot (gitutil.Deletion.PrometheusAlerts). A CRD with two or more
// alerts has each alert's real check rule living at its own derived id
// (asset.DeriveAlertCheckRuleID), never at the CRD's literal identifier --
// see composePrometheusRuleNames' doc comment for why -- so deleting such a
// CRD must attempt each alert's derived id individually. A CRD with zero or
// one alert keeps using the literal identifier directly, matching how
// applyPrometheusRule creates it.
//
// Every endpoint attempted (every check-rule id, plus the recording-rule
// endpoint) tolerates a 404: this used to be gated on which endpoint(s) the
// CRD's content at --since's ref showed it using, but that signal is a
// single point in time, not a history of everything the identifier has
// ever used. A CRD that had a recording rule stripped from it in an
// earlier commit (leaving only its alerting rules), followed by the whole
// file being deleted, showed --since a ref where the file only ever had
// alerts — silently orphaning the recording rule created earlier, with no
// later git state able to recover that fact once the file is gone.
// Deleting is naturally idempotent (a 404 just means this endpoint never
// had anything for this identifier), so attempting all of them
// unconditionally is safe by the same logic every other kind already
// relies on. The one tradeoff: a check rule or recording rule that happens
// to share one of these identifiers by coincidence (not because it came
// from this CRD) would be deleted too — accepted as an edge case narrow
// enough not to justify leaving real orphaned assets behind.
//
// A 404 on any endpoint always means "already gone" (see
// deleteAssetByKindAndIdentifier's doc comment for why this is
// unconditional, unlike a standalone `<kind> delete` command).
//
// The returned bool follows deleteAssetByKindAndIdentifier's contract: true
// only when *nothing* attempted had anything left to delete (every check
// rule id and the recording rule all 404), so a mixed outcome -- anything
// genuinely deleted, the rest already gone -- is reported as a real
// deletion, matching the fact that something was.
func deletePrometheusRuleCRD(ctx context.Context, apiClient dash0api.Client, dataset *string, identifier string, alerts []asset.PrometheusAlertName, declared map[string]bool) (alreadyDeleted bool, err error) {
	checkRuleIDs := asset.CheckRuleIDsOccupiedByCRD(identifier, alerts)
	if len(alerts) > 1 {
		// A multi-alert CRD applied before per-alert derivation existed left
		// its one check rule at the literal id (docs/commands.md's multi-alert
		// migration note); the per-id 404 tolerance below makes this extra
		// attempt free when there is no such orphan.
		checkRuleIDs = append([]string{identifier}, checkRuleIDs...)
	}

	var lastCheckRuleNotFoundErr error
	checkRulesAllNotFound := true
	for _, id := range checkRuleIDs {
		if declared[id] {
			// Not attempted, so not a "found nothing" either.
			warnSkippedDeclaredCheckRule(id, fmt.Sprintf("PrometheusRule %q", identifier))
			continue
		}
		checkRuleErr := apiClient.DeleteCheckRule(ctx, id, dataset)
		if checkRuleErr == nil {
			checkRulesAllNotFound = false
			continue
		}
		if !dash0api.IsNotFound(checkRuleErr) {
			return false, client.HandleAPIError(checkRuleErr, client.ErrorContext{AssetType: "check rule", AssetID: id})
		}
		lastCheckRuleNotFoundErr = checkRuleErr
	}

	recordingRuleErr := apiClient.DeleteRecordingRule(ctx, identifier, dataset)
	recordingRuleNotFound := recordingRuleErr != nil && dash0api.IsNotFound(recordingRuleErr)
	if recordingRuleErr != nil && !recordingRuleNotFound {
		return false, client.HandleAPIError(recordingRuleErr, client.ErrorContext{AssetType: "recording rule", AssetID: identifier})
	}

	// "Genuinely gone" means 404 on every check-rule id attempted and on
	// the recording-rule endpoint -- nothing had anything for this CRD.
	if checkRulesAllNotFound && recordingRuleNotFound {
		notFoundErr := lastCheckRuleNotFoundErr
		if notFoundErr == nil {
			notFoundErr = recordingRuleErr
		}
		ectx := client.ErrorContext{AssetType: "PrometheusRule", AssetID: identifier}
		if client.IsAlreadyDeleted(notFoundErr, true, ectx) {
			return true, nil
		}
		return false, client.HandleAPIError(notFoundErr, ectx)
	}
	return false, nil
}

// declaredCheckRuleIDs collects every check-rule id -f's current contents
// still declare. Dispatch resolves an identifier to an endpoint, dropping the
// kind half of the (kind, identifier) key Snapshot.Identifiers uses elsewhere:
// a removed PrometheusRule and a surviving CheckRule sharing an identifier are
// two plan entries but one DELETE /api/alerting/check-rules/<id>, which would
// undo phase 1's own create/update. The collision runs both ways.
//
// Tolerating an orphan here is deliberate: leaving an asset behind is
// recoverable by hand, deleting one the user just declared is not.
func declaredCheckRuleIDs(after gitutil.Snapshot) map[string]bool {
	declared := map[string]bool{}
	for key := range after.Identifiers {
		switch key.Kind {
		case "checkrule":
			declared[key.Identifier] = true
		case "prometheusrule":
			// Occupied ids only: a multi-alert CRD's literal identifier holds
			// an orphan, not something it declares, so it stays reclaimable.
			for _, id := range asset.CheckRuleIDsOccupiedByCRD(key.Identifier, after.PrometheusAlertsByIdentifier[key.Identifier]) {
				declared[id] = true
			}
		}
	}
	return declared
}

// warnSkippedDeclaredCheckRule reports a skipped check-rule deletion, so the
// orphan it leaves behind is not invisible.
func warnSkippedDeclaredCheckRule(id, removedDisplay string) {
	fmt.Fprintf(os.Stderr, "warning: not deleting check rule %q on behalf of removed %s: the current contents of -f still declare a check rule with that identifier, and deleting it would undo this run's own create/update; remove it by hand if it is a leftover\n", id, removedDisplay)
}

// deleteCheckRuleByName deletes the check rule index resolves name to. Name
// is the only way to target a single alerting rule removed from a
// PrometheusRule CRD that otherwise survives: the CRD's shared identifier
// can't distinguish between the alerts it contains.
//
// Not finding it by name at all, or a 404 on the delete itself, always means
// "already gone" (see deleteAssetByKindAndIdentifier's doc comment for why
// this is unconditional, unlike a standalone `<kind> delete` command). The
// returned bool follows the same contract: true means nothing was actually
// deleted by this call.
func deleteCheckRuleByName(ctx context.Context, apiClient dash0api.Client, dataset *string, name string, index checkRuleNameIndex) (alreadyDeleted bool, err error) {
	id, err := index.resolve(name)
	if err != nil {
		return false, err
	}
	if id == "" {
		fmt.Fprintf(os.Stderr, "Check rule %q was already deleted\n", name)
		return true, nil
	}

	err = apiClient.DeleteCheckRule(ctx, id, dataset)
	ectx := client.ErrorContext{AssetType: "check rule", AssetID: id, AssetName: name}
	if err != nil {
		if client.IsAlreadyDeleted(err, true, ectx) {
			return true, nil
		}
		return false, client.HandleAPIError(err, ectx)
	}
	return false, nil
}

// checkRuleNameIndex maps each live check rule's name to every check rule
// carrying it, built once per run so resolving N removed alerts costs one
// listing rather than N, and so a shared name is visible as the ambiguity it
// is rather than resolving to whichever the API listed first.
type checkRuleNameIndex map[string][]checkRuleMatch

type checkRuleMatch struct {
	id     string
	source string // dash0.com/origin-derived system of record, "" when unset
}

// foreignCheckRuleSources are the systems of record --since must not delete
// on behalf of an alert removed from -f: a same-named check rule owned by one
// of them merely collides. A denylist, not an allowlist: the CLI strips
// dash0.com/origin before sending, so its own check rules read back as "api"
// or unset, and CrdSource's contract treats an unknown value as "api" too.
var foreignCheckRuleSources = map[string]bool{
	string(dash0api.Ui):        true,
	string(dash0api.Terraform): true,
	string(dash0api.Operator):  true,
	string(dash0api.Platform):  true,
}

func buildCheckRuleNameIndex(ctx context.Context, apiClient dash0api.Client, dataset *string) (checkRuleNameIndex, error) {
	index := checkRuleNameIndex{}
	iter := apiClient.ListCheckRulesIter(ctx, dataset)
	for iter.Next() {
		item := iter.Current()
		if item.Name == nil {
			continue
		}
		match := checkRuleMatch{id: item.Id}
		if item.Source != nil {
			match.source = string(*item.Source)
		}
		index[*item.Name] = append(index[*item.Name], match)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list check rules: %w", err)
	}
	return index, nil
}

// resolve returns the id of the one deletable check rule named name, "" when
// there is none, or an error when two carry it -- deleting the wrong one is
// unrecoverable, and the CRD's identifier cannot disambiguate them.
func (index checkRuleNameIndex) resolve(name string) (string, error) {
	var deletable, foreign []string
	for _, match := range index[name] {
		if foreignCheckRuleSources[match.source] {
			foreign = append(foreign, match.source)
			continue
		}
		deletable = append(deletable, match.id)
	}

	if len(deletable) > 1 {
		sort.Strings(deletable)
		return "", fmt.Errorf("check rule %q is ambiguous: %d check rules carry that name (%s)\nHint: --since resolves an alert removed from a surviving PrometheusRule CRD by name, since the CRD's shared identifier cannot distinguish its alerts; rename or delete the duplicates in Dash0, or skip --since for this invocation", name, len(deletable), strings.Join(deletable, ", "))
	}
	if len(deletable) == 0 {
		if len(foreign) > 0 {
			sort.Strings(foreign)
			fmt.Fprintf(os.Stderr, "warning: not deleting check rule %q: every check rule with that name is managed by another system (%s), not by this repository\n", name, strings.Join(foreign, ", "))
		}
		return "", nil
	}
	return deletable[0], nil
}
