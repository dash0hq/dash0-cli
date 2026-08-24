package git

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyRef_Empty(t *testing.T) {
	repo := testRepo(t)
	state, sha, err := repo.ClassifyRef(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, RefEmpty, state)
	assert.Empty(t, sha)
}

func TestClassifyRef_AllZeros(t *testing.T) {
	repo := testRepo(t)
	state, sha, err := repo.ClassifyRef(context.Background(), AllZerosSHA)
	require.NoError(t, err)
	assert.Equal(t, RefAllZeros, state)
	assert.Empty(t, sha)
}

func TestClassifyRef_Unresolvable(t *testing.T) {
	repo := testRepo(t)
	state, sha, err := repo.ClassifyRef(context.Background(), "totally-bogus-ref")
	require.NoError(t, err)
	assert.Equal(t, RefUnresolvable, state)
	assert.Empty(t, sha)
}

func TestClassifyRef_ResolvedAncestor(t *testing.T) {
	repo := testRepo(t)
	base := runGit(t, repo.Dir, "rev-parse", "HEAD")
	writeFile(t, repo.Dir, "f.yaml", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "add file")

	state, sha, err := repo.ClassifyRef(context.Background(), base)
	require.NoError(t, err)
	assert.Equal(t, RefResolvedAncestor, state)
	assert.Equal(t, base, sha)
}

func TestClassifyRef_ResolvedNonAncestor(t *testing.T) {
	repo := testRepo(t)

	// Branch A: diverges from main.
	runGit(t, repo.Dir, "checkout", "-q", "-b", "branch-a")
	writeFile(t, repo.Dir, "a.yaml", "kind: Dashboard\n")
	branchA := commitAll(t, repo.Dir, "branch a commit")

	// main moves on independently, so branchA is not its ancestor.
	runGit(t, repo.Dir, "checkout", "-q", "main")
	writeFile(t, repo.Dir, "b.yaml", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "main commit")

	state, sha, err := repo.ClassifyRef(context.Background(), branchA)
	require.NoError(t, err)
	assert.Equal(t, RefResolvedNonAncestor, state)
	assert.Equal(t, branchA, sha)
}

// TestExplainUnresolvableRef_InsufficientHistory is a regression test for a
// bug where --since HEAD~1 against a fresh, single-commit repository (the
// first thing many people try when setting up a --since test or demo repo)
// was told to check for a typo or a too-shallow clone -- neither applies:
// there is simply no earlier commit yet.
func TestExplainUnresolvableRef_InsufficientHistory(t *testing.T) {
	repo := testRepo(t) // exactly one commit

	reason := repo.ExplainUnresolvableRef(context.Background(), "HEAD~1")
	assert.Contains(t, reason, "1 commit")
	assert.Contains(t, reason, `"HEAD~1"`)
}

// TestExplainUnresolvableRef_BareTildeShorthand pins that the bare "~"
// suffix (shorthand for "~1") is recognized the same as an explicit "~1".
func TestExplainUnresolvableRef_BareTildeShorthand(t *testing.T) {
	repo := testRepo(t)

	reason := repo.ExplainUnresolvableRef(context.Background(), "HEAD~")
	assert.Contains(t, reason, "1 commit")
}

// TestExplainUnresolvableRef_CaretSuffix pins that "^N" (not just "~N") is
// also recognized.
func TestExplainUnresolvableRef_CaretSuffix(t *testing.T) {
	repo := testRepo(t)

	reason := repo.ExplainUnresolvableRef(context.Background(), "HEAD^1")
	assert.Contains(t, reason, "1 commit")
}

// TestExplainUnresolvableRef_EnoughHistoryReturnsEmpty pins the negative
// case: when the base actually has enough history, there's nothing to
// explain -- the ref would have resolved, so callers should never reach
// this function for it in practice, but the function's own logic must
// still not fabricate a reason if asked anyway.
func TestExplainUnresolvableRef_EnoughHistoryReturnsEmpty(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "f.yaml", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "second commit")

	assert.Empty(t, repo.ExplainUnresolvableRef(context.Background(), "HEAD~1"))
}

// TestExplainUnresolvableRef_TypoBaseReturnsEmpty pins that a base which
// doesn't resolve at all (a genuine typo, not an insufficient-history
// case) yields no explanation, leaving the caller to fall back to the
// generic typo-or-shallow-clone message.
func TestExplainUnresolvableRef_TypoBaseReturnsEmpty(t *testing.T) {
	repo := testRepo(t)

	assert.Empty(t, repo.ExplainUnresolvableRef(context.Background(), "totally-bogus-base~1"))
}

// TestExplainUnresolvableRef_NonMatchingShapeReturnsEmpty pins that a ref
// with no trailing ~N/^N shape at all (e.g. a plain typo'd branch name)
// yields no explanation.
func TestExplainUnresolvableRef_NonMatchingShapeReturnsEmpty(t *testing.T) {
	repo := testRepo(t)

	assert.Empty(t, repo.ExplainUnresolvableRef(context.Background(), "totally-bogus-ref"))
}

// TestRefState_PrintsSymbolicName confirms each constant's declared string
// value doubles as a readable representation in log lines and test failure
// output (fmt.Sprintf("%v", ...) / %s), with no separate Stringer needed.
func TestRefState_PrintsSymbolicName(t *testing.T) {
	cases := map[RefState]string{
		RefEmpty:               "RefEmpty",
		RefAllZeros:            "RefAllZeros",
		RefResolvedAncestor:    "RefResolvedAncestor",
		RefResolvedNonAncestor: "RefResolvedNonAncestor",
		RefUnresolvable:        "RefUnresolvable",
	}
	for state, want := range cases {
		assert.Equal(t, want, fmt.Sprintf("%v", state))
	}
}
