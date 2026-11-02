//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
)

// TestE2E_DiffSince_WholeFileDeletion runs `diff --since` against a real git
// binary: it must preview the removed dashboard as a deletion and the
// surviving view as a create, exit 1 for pending differences, and send no
// mutating request.
func TestE2E_DiffSince_WholeFileDeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "whole-file-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, Body: map[string]any{}})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "diff", "-f", "/work/repo", "--since", ref)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 (differences pending), got %d. Output:\n%s", exitCode, output)
	}
	for _, want := range []string{`Delete Dashboard "Dashboard A"`, `Create View "View B"`} {
		if !strings.Contains(output, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, output)
		}
	}
	for _, r := range server.Requests() {
		if r.Method != http.MethodGet {
			t.Errorf("diff must not mutate, but sent %s %s", r.Method, r.Path)
		}
	}
}

// TestE2E_DiffSince_UnresolvableRefExitsTwo pins the genuine-error exit code
// through the real binary and main(), with -X before the subcommand.
func TestE2E_DiffSince_UnresolvableRefExitsTwo(t *testing.T) {
	ctx := context.Background()
	repoDir, _ := testutil.BuildGitScenario(t, "whole-file-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "diff", "-f", "/work/repo", "--since", "totally-bogus-ref")

	if exitCode != 2 {
		t.Fatalf("expected exit 2, got %d. Output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "could not be resolved") {
		t.Errorf("expected the unresolvable-ref error, got:\n%s", output)
	}
}
