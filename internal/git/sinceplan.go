package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/asset"
)

// SincePlan wraps the identifier-diffing result from internal/git with a
// human-readable warning to surface when --since's ref resolved but is not
// an ancestor of HEAD.
type SincePlan struct {
	Plan    DeletionPlan
	Warning string
	// names best-effort maps each ByIdentifier deletion's Path (the
	// git-recorded path, unique within one plan) to its display name,
	// resolved by re-reading the asset's content from git history at the
	// --since ref. A missing entry means the lookup failed (e.g. a rewritten
	// or gc'd blob) or the asset had no name in the first place; callers
	// fall back to a placeholder. This is cosmetic only — deletion dispatch
	// (deleteAssetByKindAndIdentifier) never depends on it, only on
	// (kind, identifier).
	Names map[string]string
	// scope is the --since target's path relative to the repository root
	// (forward-slashed, "" when the target is the repo root itself). Deletion
	// paths (Deletion.Path, from git ls-tree) are always repo-root-
	// relative, while validated documents' asset.Document.FilePath is always
	// relative to the -f target itself -- when the target is a subdirectory,
	// those two bases differ, so rendering must strip this prefix from a
	// deletion path before grouping it with validated documents from the same
	// file. See StripScope.
	Scope string
	// declaredCheckRuleIDs is every check-rule id -f's current contents still
	// declare, so a deletion dispatched by identifier never removes an asset
	// this same run created or updated. See declaredCheckRuleIDs.
	DeclaredCheckRuleIDs map[string]bool
	// targetWasDirectoryAtRef reports whether -f's target was a directory
	// (rather than a single file) the last time it existed, per git history
	// at --since's ref. apply uses this to correct its own fromDirectory
	// guess for a target that no longer exists on disk at all: os.Stat can't
	// tell file from directory once the path is gone, but the ref usually
	// still can.
	TargetWasDirectoryAtRef bool
}

// ComputeSincePlan resolves since against the git repository
// containing file and diffs the identifier set at that ref against
// file's current disk contents. It never talks to the Dash0 API —
// everything it needs comes from git and the local filesystem.
func ComputeSincePlan(ctx context.Context, file, since string) (*SincePlan, error) {
	absFile, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %s: %w", file, err)
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
		return nil, fmt.Errorf("failed to resolve %s: %w", file, err)
	}
	// Resolve symlinks so absFile is comparable with repo.Root()'s output:
	// `git rev-parse --show-toplevel` always prints the fully-resolved real
	// path, but filepath.Abs alone does not resolve symlinks in parent
	// directories (e.g. macOS's /var -> /private/var), which would otherwise
	// make every filepath.Rel(repoRoot, absFile) below compute a bogus
	// "outside the repository" path.
	resolvedAncestor, err := filepath.EvalSymlinks(existingAncestor)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", file, err)
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
			return nil, fmt.Errorf("failed to stat %s: %w", file, err)
		}
		if !info.IsDir() {
			repoDir = filepath.Dir(absFile)
		}
	}
	repo := Repo{Dir: repoDir}

	repoRoot, err := repo.Root(ctx)
	if err != nil {
		if IsNotAGitRepository(err) {
			return nil, fmt.Errorf("--since '%s' requires %s to be inside a git repository, but it is not (no .git found there or in any parent directory)\nHint: point -f at a path inside the repository that tracks these assets, or drop --since to apply without deletion detection", since, file)
		}
		return nil, fmt.Errorf("--since '%s' requires %s to be inside a git repository: %w", since, file, err)
	}
	// Re-anchor at repoRoot: every scope-relative pathspec built below (and
	// passed to BuildSnapshotFromRef) is repo-root-relative, matching git
	// ls-tree's own path convention. Running git commands with -C repoDir
	// when repoDir is a subdirectory of the repo would otherwise resolve
	// those pathspecs relative to repoDir instead, silently matching nothing
	// (e.g. -f dashboards/ turning scope "dashboards" into the nonexistent
	// "dashboards/dashboards" once -C is already inside dashboards/).
	repo = Repo{Dir: repoRoot}

	refState, sha, err := repo.ClassifyRef(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --since '%s' as Git reference: %w", since, err)
	}

	var warning string
	switch refState {
	case RefEmpty:
		return nil, fmt.Errorf("--since '%s' resolved to an empty ref; there is no prior state to compare against\nHint: a CI-provided ref variable is likely unset (e.g. github.event.before on a workflow_dispatch or schedule trigger); skip --since for this invocation, or pass an explicit ref", since)
	case RefAllZeros:
		return nil, fmt.Errorf("--since '%s' resolved to git's all-zeros SHA (%s), meaning there is no prior state to compare against\nHint: some CI systems report this value for a branch's first push; skip --since for this invocation, or pass an explicit ref", since, AllZerosSHA)
	case RefUnresolvable:
		// Best-effort: a simple "<base>~N"/"<base>^N" ref whose base
		// resolves fine but doesn't have N commits of history behind it
		// (e.g. --since HEAD~1 against a single-commit repo) gets a
		// specific reason instead of the generic typo-or-shallow-clone
		// guess, neither of which applies to that case.
		if reason := repo.ExplainUnresolvableRef(ctx, since); reason != "" {
			return nil, fmt.Errorf("--since '%s' could not be resolved: %s\nHint: pass a ref that exists this far back in the repository's history, or skip --since for this invocation", since, reason)
		}
		return nil, fmt.Errorf("--since '%s' could not be resolved\nHint: check the ref for a typo; if this is a shallow clone (actions/checkout defaults to fetch-depth: 1), re-run the checkout with fetch-depth: 0 so the ref's history is available", since)
	case RefResolvedNonAncestor:
		// The confirmation for this case is deliberately NOT done here: doing
		// so would abort the entire apply run (including ordinary, unrelated
		// creates/updates) before any document is even processed, just
		// because the --since ref needs confirming. Instead, apply asks
		// for confirmation immediately before calling applyDeletions, after
		// every document create/update has already gone through — mirroring
		// how a declined per-asset deletion (applyDeletions itself) never
		// blocks the rest of the run, just the deletions.
		warning = fmt.Sprintf("--since '%s' is not an ancestor of HEAD (likely a force-push or history rewrite); deletion detection may be inaccurate", since)
	}

	scope, err := filepath.Rel(repoRoot, absFile)
	if err != nil {
		return nil, fmt.Errorf("failed to compute %s's path relative to repository root %s: %w", file, repoRoot, err)
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
		return nil, fmt.Errorf("--since '%s' cannot run against a sparse checkout: files tracked under %s are absent from disk, so they would be detected as deletions\nHint: run from a full checkout (git sparse-checkout disable), or drop --since to apply without deletion detection", since, file)
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

	before, err := BuildSnapshotFromRef(ctx, repo, sha, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to read git state at --since ref '%s': %w", since, err)
	}
	after, err := BuildSnapshotFromDisk(ctx, absFile, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read current Git state: %w", err)
	}

	plan := Diff(before, after)
	if len(plan.NoIdentifier) > 0 {
		return nil, fmt.Errorf("--since '%s' found %s deleted with no identifier its kind is upserted by, so deletion cannot be determined reliably:\n  %s\nHint: without a stable identifier there is no way to tell which live asset each document was; delete these assets directly in Dash0, or skip --since for this invocation",
			since, asset.Pluralize(len(plan.NoIdentifier), "document"), strings.Join(plan.NoIdentifier, "\n  "))
	}

	names := resolveDeletionNames(before, plan.ByIdentifier)

	return &SincePlan{Plan: plan, Warning: warning, Names: names, Scope: scope, DeclaredCheckRuleIDs: declaredCheckRuleIDs(after), TargetWasDirectoryAtRef: targetWasDirectoryAtRef}, nil
}

// nearestExistingAncestor walks up from path until it finds an entry that
// exists on disk, returning that ancestor plus the path components between
// it and path, joined back together with filepath.Join's separator so a
// caller can filepath.Join them straight onto the ancestor's symlink-resolved
// form. missingSuffix is "" when path itself already exists (the common
// case, unaffected by --since: the ancestor returned is then path itself).
//
// This lets ComputeSincePlan resolve a --since target that no longer
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
// while building it (Snapshot.RawContent), never a second read of
// the same git blob. before is the exact Snapshot that produced every
// deletion candidate in deletions (via Diff), so a lookup miss can
// only mean a document that didn't parse cleanly, never a blob that
// genuinely needs (re-)fetching. Documents are parsed once per file and
// cached in memory for the duration of this call, so a multi-document file
// with several deletion candidates only pays the parse cost once.
//
// This is display polish, not correctness-critical: a lookup miss (content
// that no longer parses under today's rules) just omits that entry from the
// returned map rather than failing the run — --since's actual deletion
// dispatch never depends on a name, only on (kind, identifier).
func resolveDeletionNames(before Snapshot, deletions []Deletion) map[string]string {
	names := make(map[string]string, len(deletions))
	docsByFile := map[string][]asset.Document{}
	for _, d := range deletions {
		basePath, docIndex := SplitMultiDocPath(d.Path)
		docs, cached := docsByFile[basePath]
		if !cached {
			if raw, ok := before.RawContent[basePath]; ok {
				docs, _ = asset.ParseMultiDocumentYAML(raw)
			}
			docsByFile[basePath] = docs
		}
		if docIndex < len(docs) && docs[docIndex].Name != "" {
			names[d.Path] = docs[docIndex].Name
		}
	}
	return names
}

// SplitMultiDocPath splits a Deletion.Path — possibly suffixed "#<index>"
// for the second and later documents in a multi-document file, per
// internal/git/snapshot.go's ingestDocuments — into the base file path and
// the document's 0-based index within it, matching parseMultiDocumentYAML's
// return-slice indexing.
func SplitMultiDocPath(path string) (basePath string, docIndex int) {
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

// declaredCheckRuleIDs collects every check-rule id -f's current contents
// still declare. Dispatch resolves an identifier to an endpoint, dropping the
// kind half of the (kind, identifier) key Snapshot.Identifiers uses elsewhere:
// a removed PrometheusRule and a surviving CheckRule sharing an identifier are
// two plan entries but one DELETE /api/alerting/check-rules/<id>, which would
// undo phase 1's own create/update. The collision runs both ways.
//
// Tolerating an orphan here is deliberate: leaving an asset behind is
// recoverable by hand, deleting one the user just declared is not.
func declaredCheckRuleIDs(after Snapshot) map[string]bool {
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

// StripScope removes scope's directory prefix from path, converting a
// repo-root-relative deletion path (Deletion.Path, as read via git
// ls-tree) into the same -f-target-relative basis validated documents'
// asset.Document.FilePath already uses -- otherwise a deletion from a
// subdirectory -f target groups under a different (repo-root-relative) key
// than that same file's surviving documents, splitting one file into two
// entries with inconsistent prefixing. scope is "" when the target is the
// repository root itself, in which case the two bases already coincide.
func StripScope(path, scope string) string {
	if scope == "" {
		return path
	}
	prefix := scope + "/"
	if rest, ok := strings.CutPrefix(path, prefix); ok {
		return rest
	}
	return path
}
