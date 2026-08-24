package git

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RefState classifies a `--since` ref before any deletion detection runs.
// It is a named string type, rather than an int paired with a separate
// Stringer, so each constant's declaration doubles as its own printable
// representation (in log lines, test failure output, etc.) — there is no
// parallel name-mapping switch that could drift out of sync.
type RefState string

const (
	// RefEmpty means the ref was the empty string — --since was not passed,
	// or was passed as "".
	RefEmpty RefState = "RefEmpty"
	// RefAllZeros means the ref was git's all-zeros sentinel (AllZerosSHA) —
	// e.g. GitHub's github.event.before on a branch's first push. There is
	// no "before" state to compare against.
	RefAllZeros RefState = "RefAllZeros"
	// RefResolvedAncestor means the ref resolved to a real commit that is an
	// ancestor of HEAD — the ordinary, expected case.
	RefResolvedAncestor RefState = "RefResolvedAncestor"
	// RefResolvedNonAncestor means the ref resolved to a real commit, but
	// that commit is not an ancestor of HEAD (e.g. after a force-push or
	// history rewrite). Callers must not silently treat this like the
	// ancestor case.
	RefResolvedNonAncestor RefState = "RefResolvedNonAncestor"
	// RefUnresolvable means git could not resolve the ref to a commit at all
	// (typo, too-shallow clone, ref genuinely doesn't exist).
	RefUnresolvable RefState = "RefUnresolvable"
)

// ClassifyRef resolves ref against repo and classifies it into a RefState.
// resolvedSHA is populated only for RefResolvedAncestor and
// RefResolvedNonAncestor; it is the commit --since's two-point diff should
// read the "before" state from.
//
// err is reserved for infrastructure failures (git binary missing, context
// canceled, HEAD itself unresolvable) — a ref that simply doesn't resolve is
// not an error, it's the RefUnresolvable state.
func (r Repo) ClassifyRef(ctx context.Context, ref string) (state RefState, resolvedSHA string, err error) {
	if ref == "" {
		return RefEmpty, "", nil
	}
	if ref == AllZerosSHA {
		return RefAllZeros, "", nil
	}

	sha, resolveErr := r.resolveCommit(ctx, ref)
	if resolveErr != nil {
		if isRefNotFound(resolveErr) {
			return RefUnresolvable, "", nil
		}
		return RefUnresolvable, "", resolveErr
	}

	isAncestor, ancestorErr := r.IsAncestor(ctx, sha, "HEAD")
	if ancestorErr != nil {
		return RefUnresolvable, "", ancestorErr
	}
	if isAncestor {
		return RefResolvedAncestor, sha, nil
	}
	return RefResolvedNonAncestor, sha, nil
}

func isRefNotFound(err error) bool {
	return err == ErrRefNotFound
}

// simpleAncestorRefPattern matches a ref written as "<base>~N" or
// "<base>^N", including the bare "<base>~"/"<base>^" shorthand for N=1
// (an empty digit group). The base is captured non-greedily so a chained
// expression like "abc~2~3" splits at its last separator, matching git's
// own left-to-right evaluation.
var simpleAncestorRefPattern = regexp.MustCompile(`^(.+?)[~^](\d*)$`)

// ExplainUnresolvableRef returns a best-effort, more specific reason why
// ref (already known to be RefUnresolvable) failed to resolve, or "" if it
// can't tell. Today it recognizes exactly one shape: a simple "<base>~N" or
// "<base>^N" expression (or the bare "~"/"^" shorthand for N=1) whose base
// resolves fine, but whose own history has fewer than N+1 commits -- e.g.
// --since HEAD~1 in a repository with only one commit. That's a materially
// different situation from a typo'd ref or a too-shallow clone (the two
// reasons the generic RefUnresolvable message suggests), and it's often
// the very first wall someone hits setting up a fresh repo to try --since
// against. Deliberately narrow: no attempt is made to parse the rest of
// git's revision syntax (^{...}, @{...}, a mix of ~ and ^, etc.) -- those
// fall through to the generic message unchanged.
func (r Repo) ExplainUnresolvableRef(ctx context.Context, ref string) string {
	m := simpleAncestorRefPattern.FindStringSubmatch(ref)
	if m == nil {
		return ""
	}
	base, digits := m[1], m[2]
	n := 1
	if digits != "" {
		parsed, err := strconv.Atoi(digits)
		if err != nil {
			return ""
		}
		n = parsed
	}

	if _, err := r.resolveCommit(ctx, base); err != nil {
		// base itself doesn't resolve either -- a genuine typo (in base, or
		// in the whole ref), not an insufficient-history case.
		return ""
	}
	out, err := r.run(ctx, "rev-list", "--count", base)
	if err != nil {
		return ""
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return ""
	}
	if n < count {
		// There IS enough history; something else is wrong with ref, and
		// the generic message's suggestions are as good a guess as any.
		return ""
	}

	commitWord := "commit"
	if count != 1 {
		commitWord += "s"
	}
	return fmt.Sprintf("%q has only %d %s of history, not enough for %q to resolve %d commit%s further back", base, count, commitWord, ref, n, pluralSuffix(n))
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
