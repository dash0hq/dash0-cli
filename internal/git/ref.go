package git

import "context"

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
