// Package git wraps the git plumbing commands `--since` needs
// (rev-parse, merge-base, cat-file, ls-tree) via the system git binary,
// never porcelain commands (git diff, git show, git log) — porcelain output
// is for human consumption and isn't a stable, documented contract across
// git versions.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/asset"
)

// AllZerosSHA is git's sentinel for "this ref did not exist" — the value
// GitHub gives github.event.before on a branch's first push, and the value
// git's own pre-receive/post-receive hooks use for a created/deleted ref.
const AllZerosSHA = "0000000000000000000000000000000000000000"

// ErrRefNotFound is returned by resolveCommit when git could not resolve a
// ref to a commit — a nonexistent ref, a too-shallow history, or any other
// plain git-resolution failure. It is not returned for infrastructure
// failures (git binary missing, context canceled), which propagate as-is.
var ErrRefNotFound = errors.New("git ref not found")

// Repo is a lightweight handle to a git working tree, used to run plumbing
// commands against it via the system git binary. Dir may be the working
// tree's root or any directory inside it — git -C resolves it either way.
type Repo struct {
	Dir string
}

// run invokes `git -C <r.Dir> <args...>` and returns stdout on success. A
// non-zero exit still returns an error usable with errors.As(&exitErr) to
// distinguish "git said no" from an infrastructure failure (binary missing,
// context canceled) — the wrapping here uses %w specifically to preserve
// that distinction for callers further up the chain.
func (r Repo) run(ctx context.Context, args ...string) ([]byte, error) {
	fullArgs := append([]string{"-C", r.Dir}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// resolveCommit runs `git rev-parse --verify <ref>^{commit}`, returning the
// resolved commit SHA. Returns ErrRefNotFound when git exits non-zero
// because the ref itself doesn't resolve (as opposed to an infrastructure
// failure, which is returned unwrapped).
func (r Repo) resolveCommit(ctx context.Context, ref string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", ErrRefNotFound
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAncestor runs `git merge-base --is-ancestor <ancestor> <descendant>`,
// reporting whether ancestor is reachable from descendant. Per git's own
// documented convention for --is-ancestor, exit 0 means true and exit 1
// means false; any other outcome is a genuine error (e.g. one of the refs
// doesn't exist).
func (r Repo) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	_, err := r.run(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("failed to check ancestry of %s against %s: %w", ancestor, descendant, err)
}

// Root runs `git rev-parse --show-toplevel`, returning the absolute path to
// the repository root containing r.Dir. Callers use this to anchor
// repo-relative paths consistently between BuildSnapshotFromRef (whose paths
// are always repo-root-relative, per git ls-tree's own behavior) and
// BuildSnapshotFromDisk.
//
// Its own error is deliberately unadorned (no "failed to determine
// repository root for %s" wrapping): callers that need a clean, single-line
// message for the common "not a git repository at all" case should check
// IsNotAGitRepository(err) themselves and build their own message from
// scratch, rather than trying to make a wrapped chain of "failed to
// determine..." / "git rev-parse ...: exit status 128 (stderr: fatal: not
// a git repository...)" read well. r.Dir is included so a caller that does
// want the raw error for anything else still knows which directory failed.
func (r Repo) Root(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("failed to determine repository root for %s: %w", r.Dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsNotAGitRepository reports whether err (as returned by Root, or any
// other Repo method) is git's own "fatal: not a git repository (or any of
// the parent directories): .git" failure -- the common case of running
// --since against a directory that was never a git repository at all, as
// opposed to some other infrastructure failure. Matched by substring
// against git's own stable, documented wording, since git's rev-parse
// exit code (128) is a generic "fatal error" shared by many unrelated
// failures and can't distinguish this case on its own.
func IsNotAGitRepository(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not a git repository")
}

// ReadFileAtRef runs `git cat-file -p <ref>:<path>`, returning the file's
// content at that ref. path must be relative to the repo root, using
// forward slashes (git's own path convention).
func (r Repo) ReadFileAtRef(ctx context.Context, ref, path string) ([]byte, error) {
	out, err := r.run(ctx, "cat-file", "-p", ref+":"+path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s at %s: %w", path, ref, err)
	}
	return out, nil
}

// IsTreeAtRef runs `git cat-file -t <ref>:<path>` and reports whether path
// was a directory (a "tree" object) at ref, as opposed to a file (a "blob").
// path == "" means the repository root itself, which is always a tree.
//
// Used to recover whether a --since target that no longer exists on disk at
// all was a single file or a directory the last time it did exist, so
// dry-run rendering can group its output the same way it would have while
// the target still existed, instead of defaulting to one shape regardless.
func (r Repo) IsTreeAtRef(ctx context.Context, ref, path string) (bool, error) {
	if path == "" {
		return true, nil
	}
	out, err := r.run(ctx, "cat-file", "-t", ref+":"+path)
	if err != nil {
		return false, fmt.Errorf("failed to determine object type of %s at %s: %w", path, ref, err)
	}
	return strings.TrimSpace(string(out)) == "tree", nil
}

// HasSkipWorktreeFiles reports whether any tracked file under scope carries
// git's skip-worktree bit, which sparse-checkout sets on everything outside
// the cone. Such a file is in the commit but not on disk, so the ref side of a
// --since diff sees it and the disk side does not, making it look deleted.
func (r Repo) HasSkipWorktreeFiles(ctx context.Context, scope string) (bool, error) {
	args := []string{"ls-files", "-t", "-z"}
	if scope != "" {
		args = append(args, "--", scope)
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return false, fmt.Errorf("failed to list tracked files under %s: %w", scope, err)
	}
	// Entries are "<tag> <path>" separated by NUL; S is skip-worktree.
	return bytes.HasPrefix(out, []byte("S ")) || bytes.Contains(out, []byte("\x00S ")), nil
}

// ListYAMLFilesAtRef runs `git ls-tree -r -z --name-only <ref> [-- <scope>]`,
// returning every .yaml/.yml file at that ref within scope (a repo-relative
// directory or file path; empty scope lists the whole tree). Hidden files
// and directories (any path component starting with ".") are skipped,
// matching apply's existing discoverFiles behavior for disk scans, so the
// git-side and disk-side listings stay consistent — including exempting the
// scope itself from the hidden check: a dot-prefixed -f target (e.g.
// -f .dash0-assets/) is a deliberate user choice, not something to skip, the
// same way FindNonHiddenYAMLFiles never applies IsHiddenPath to its walk
// root. Every path component *inside* scope is still checked normally.
//
// The .yaml/.yml extension check is likewise skipped when scope names a
// single file exactly (line == scope; a directory scope's entries are always
// listed as scope/<something>, never scope itself, so this can only match a
// genuine single-file target) — apply's own single-file create/update path
// (readMultiDocumentYAML) has no extension check at all, so -f config.json
// must be scanned by --since the same way it's read by every other apply
// path, not silently excluded from both snapshots because of its extension.
func (r Repo) ListYAMLFilesAtRef(ctx context.Context, ref, scope string) ([]string, error) {
	// -z is load-bearing: without it git C-quotes a non-ASCII path
	// ("a/caf\303\251.yaml") and the trailing quote fails IsYAMLFile.
	args := []string{"ls-tree", "-r", "-z", "--name-only", ref}
	if scope != "" {
		args = append(args, "--", scope)
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list files at %s: %w", ref, err)
	}

	var files []string
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if line == "" {
			continue
		}
		isExplicitSingleFileTarget := scope != "" && line == scope
		if !isExplicitSingleFileTarget && !asset.IsYAMLFile(line) {
			continue
		}
		pathBelowScope := strings.TrimPrefix(line, scope)
		pathBelowScope = strings.TrimPrefix(pathBelowScope, "/")
		if asset.IsHiddenPath(pathBelowScope) {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}
