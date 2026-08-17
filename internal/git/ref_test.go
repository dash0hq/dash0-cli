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
