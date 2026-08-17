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

// SincePlan wraps the identifier-diffing result (DeletionPlan) with a
// human-readable warning to surface when --since's ref resolved but is not
// an ancestor of HEAD. Shared between apply --since (which acts on the plan)
// and diff --since (which only previews it).
type SincePlan struct {
	Plan    DeletionPlan
	Warning string
	// Names best-effort maps each ByIdentifier deletion's Path (the
	// git-recorded path, unique within one plan) to its display name,
	// resolved by re-reading the asset's content from git history at the
	// --since ref. A missing entry means the lookup failed (e.g. a rewritten
	// or gc'd blob) or the asset had no name in the first place; callers
	// fall back to a placeholder. This is cosmetic only — deletion dispatch
	// never depends on it, only on (kind, identifier).
	Names map[string]string
	// Scope is the --since target's path relative to the repository root
	// (forward-slashed, "" when the target is the repo root itself). Deletion
	// paths (Deletion.Path, from git ls-tree) are always repo-root-relative,
	// while validated documents' asset.Document.FilePath is always relative
	// to the -f target itself -- when the target is a subdirectory, those two
	// bases differ, so rendering must strip this prefix from a deletion path
	// before grouping it with validated documents from the same file. See
	// StripScope.
	Scope string
}

// ComputeSincePlan resolves since against the git repository containing file
// and diffs the identifier set at that ref against file's current disk
// contents. It never talks to the Dash0 API — everything it needs comes from
// git and the local filesystem.
func ComputeSincePlan(ctx context.Context, file, since string) (*SincePlan, error) {
	absFile, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %s: %w", file, err)
	}
	// Resolve symlinks so absFile is comparable with repo.Root()'s output:
	// `git rev-parse --show-toplevel` always prints the fully-resolved real
	// path, but filepath.Abs alone does not resolve symlinks in parent
	// directories (e.g. macOS's /var -> /private/var), which would otherwise
	// make every filepath.Rel(repoRoot, absFile) below compute a bogus
	// "outside the repository" path.
	absFile, err = filepath.EvalSymlinks(absFile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", file, err)
	}

	info, err := os.Stat(absFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", file, err)
	}
	repoDir := absFile
	if !info.IsDir() {
		repoDir = filepath.Dir(absFile)
	}
	repo := Repo{Dir: repoDir}

	repoRoot, err := repo.Root(ctx)
	if err != nil {
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
		return nil, fmt.Errorf("--since '%s' resolved to an empty ref; there is no prior state to compare against (this can happen when a CI-provided ref variable is unset — check the workflow's before/after ref inputs)", since)
	case RefAllZeros:
		return nil, fmt.Errorf("--since '%s' resolved to git's all-zeros SHA (%s), meaning there is no prior state to compare against (some CI systems report this value for a ref's first push)", since, AllZerosSHA)
	case RefUnresolvable:
		return nil, fmt.Errorf("--since '%s' could not be resolved (check for a typo, or a too-shallow clone missing the needed history)", since)
	case RefResolvedNonAncestor:
		// The confirmation for this case is deliberately NOT done here: doing
		// so would abort the entire calling command before any document is
		// even processed, just because the --since ref needs confirming.
		// Callers that mutate (apply) ask for confirmation immediately before
		// carrying out deletions, after every other document has already been
		// processed; callers that only preview (diff) never mutate at all, so
		// they just show this warning alongside the plan.
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
		return nil, fmt.Errorf("--since '%s' found %s deleted with no dash0.com/id or dash0.com/origin label, so deletion cannot be determined reliably:\n  %s",
			since, asset.Pluralize(len(plan.NoIdentifier), "document"), strings.Join(plan.NoIdentifier, "\n  "))
	}

	names := resolveDeletionNames(ctx, repo, sha, plan.ByIdentifier)

	return &SincePlan{Plan: plan, Warning: warning, Names: names, Scope: scope}, nil
}

// resolveDeletionNames best-effort looks up each deletion's display name by
// re-reading its content from git history at sha (the resolved --since
// ref). Reads are cached per file so a multi-document file with several
// deletion candidates only costs one `git cat-file` call.
//
// This is display polish, not correctness-critical: a lookup failure (a
// since-rewritten blob, content that no longer parses under today's rules)
// just omits that entry from the returned map rather than failing the run.
func resolveDeletionNames(ctx context.Context, repo Repo, sha string, deletions []Deletion) map[string]string {
	names := make(map[string]string, len(deletions))
	docsByFile := map[string][]asset.Document{}
	for _, d := range deletions {
		basePath, docIndex := SplitMultiDocPath(d.Path)
		docs, cached := docsByFile[basePath]
		if !cached {
			raw, err := repo.ReadFileAtRef(ctx, sha, basePath)
			if err == nil {
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
// snapshot.go's ingestDocuments — into the base file path and the document's
// 0-based index within it, matching asset.ParseMultiDocumentYAML's
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

// StripScope removes scope's directory prefix from path, converting a
// repo-root-relative deletion path (Deletion.Path, as read via git ls-tree)
// into the same -f-target-relative basis validated documents'
// asset.Document.FilePath already uses — otherwise a deletion from a
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
