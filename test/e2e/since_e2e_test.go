//go:build e2e

package e2e

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
)

const (
	testAuthToken     = "auth_test_token"
	apiPathCheckRules = "/api/alerting/check-rules"
)

var (
	dashboardIDPattern = regexp.MustCompile(`^/api/dashboards/[^/]+$`)
	checkRuleIDPattern = regexp.MustCompile(`^/api/alerting/check-rules/[^/]+$`)
	viewIDPattern      = regexp.MustCompile(`^/api/views/[^/]+$`)
)

// drainExecOutput reads an exec result reader to completion. testcontainers'
// Exec streams stdout+stderr multiplexed on one reader; a short read isn't
// meaningful here since these are short-lived, non-interactive CLI runs.
func drainExecOutput(reader io.Reader) string {
	if reader == nil {
		return ""
	}
	data, _ := io.ReadAll(reader)
	return string(data)
}

// execDash0 runs `dash0 <args...>` inside container, targeting the mock API
// server at http://<HostInternal>:<hostPort>, and returns its exit code and
// combined output.
func execDash0(ctx context.Context, t *testing.T, container testcontainers.Container, hostPort int, args ...string) (int, string) {
	t.Helper()

	apiURL := "http://" + testcontainers.HostInternal + ":" + strconv.Itoa(hostPort)
	fullArgs := append([]string{"dash0"}, args...)
	fullArgs = append(fullArgs, "--api-url", apiURL, "--auth-token", testAuthToken)

	exitCode, reader, err := container.Exec(ctx, fullArgs, exec.WithEnv([]string{"DASH0_CONFIG_DIR=/tmp/dash0-config"}))
	if err != nil {
		t.Fatalf("failed to exec %v: %v", fullArgs, err)
	}
	return exitCode, drainExecOutput(reader)
}

func mockServerPort(t *testing.T, server *testutil.MockServer) int {
	t.Helper()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse mock server URL %q: %v", server.URL, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse mock server port from %q: %v", server.URL, err)
	}
	return port
}

func copyScenarioIntoContainer(ctx context.Context, t *testing.T, container testcontainers.Container, repoDir string) {
	t.Helper()
	if err := container.CopyDirToContainer(ctx, repoDir, "/work/repo", 0o755); err != nil {
		t.Fatalf("failed to copy scenario repo into container: %v", err)
	}

	// docker cp preserves the host file owner's UID (the user running this
	// test), which the container's root user doesn't match -- tripping
	// git's post-CVE-2022-24765 "dubious ownership" guard, for both
	// /work/repo itself and, separately, /work/repo/.git when it's later
	// used as a local clone source (too-shallow-clone). A real CI
	// environment hits this same mismatch whenever a checkout is owned by a
	// different UID than the one running commands; actions/checkout works
	// around it by marking the checkout safe (with the same '*' wildcard),
	// which is what we replicate here rather than disabling the protection
	// inside dash0 itself.
	exitCode, output := execCommand(ctx, t, container, "git", "config", "--global", "--add", "safe.directory", "*")
	if exitCode != 0 {
		t.Fatalf("failed to mark /work/repo safe: %s", output)
	}
}

func TestE2E_ApplySince_WholeFileDeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "whole-file-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, Body: map[string]any{}})
	server.OnPattern(http.MethodPut, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureViewsImportSuccess})
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d. Output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "deleted") {
		t.Errorf("expected output to mention a deletion, got:\n%s", output)
	}
}

// TestE2E_ApplySince_WholeDirectoryDeletion covers the same all-deletions
// scenario as TestE2E_ApplySince_WholeFileDeletion but with every file under
// -f's target removed (not just one of several) -- the case that used to
// hard-fail with "no .yaml or .yml files found" before computeDeletionPlan
// ever ran, since -f here points at the repo root, which git always leaves
// on disk even once every tracked file under it is gone.
func TestE2E_ApplySince_WholeDirectoryDeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "whole-directory-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}})
	server.OnPattern(http.MethodDelete, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d. Output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "deleted") {
		t.Errorf("expected output to mention a deletion, got:\n%s", output)
	}
}

// TestE2E_ApplySince_DirectoryRenameIsNotADeletion pins the documented
// contract that deletion detection is by identifier, never by file path
// (see "Deletion detection is by identifier" in docs/commands.md's --since
// section): a dashboard moved into a differently-named subdirectory between
// the ref and HEAD must be treated as a plain update at its new path, not a
// deletion. No DELETE route is registered on the mock server -- if the fix
// regressed and the rename were (mis)treated as a deletion, the delete
// call would hit the mock server's default handler and the exit-code/output
// checks below would surface it, the same technique
// TestE2E_ApplySince_PrometheusRecordingPartialRemovalIsNotADeletion uses.
func TestE2E_ApplySince_DirectoryRenameIsNotADeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "directory-rename")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureDashboardsGetSuccess})
	server.OnPattern(http.MethodPut, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureDashboardsImportSuccess})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d. Output:\n%s", exitCode, output)
	}
	if strings.Contains(output, "deleted") {
		t.Errorf("renaming a subdirectory within the scanned scope must not be treated as a deletion, got:\n%s", output)
	}
	if !strings.Contains(output, "Dashboard") {
		t.Errorf("expected output to mention the dashboard being applied at its new path, got:\n%s", output)
	}
}

func TestE2E_ApplySince_MultiDocumentPartialDeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "multi-document-partial-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, Body: map[string]any{}})
	server.OnPattern(http.MethodPut, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureDashboardsImportSuccess})
	server.OnPattern(http.MethodDelete, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d. Output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "deleted") {
		t.Errorf("expected output to mention a deletion, got:\n%s", output)
	}
}

func TestE2E_ApplySince_PrometheusAlertPartialDeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "prometheus-alert-partial-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, Body: map[string]any{}})
	server.OnPattern(http.MethodPut, checkRuleIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureCheckRulesImportSuccess})
	server.On(http.MethodGet, apiPathCheckRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body: []map[string]any{
			{"dataset": "default", "id": "disk-full-check-rule-id", "name": "rule-group - DiskFull"},
		},
	})
	server.OnPattern(http.MethodDelete, checkRuleIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d. Output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "DiskFull") {
		t.Errorf("expected output to mention the removed alert's check rule, got:\n%s", output)
	}
}

func TestE2E_ApplySince_PrometheusRecordingPartialRemovalIsNotADeletion(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "prometheus-recording-partial-removal")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, Body: map[string]any{}})
	server.OnPattern(http.MethodPut, checkRuleIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureCheckRulesImportSuccess})
	// Deliberately no recording-rules route: hitting one would 404 through
	// the mock server's default handler, which the exit-code check below
	// would surface as a failure.

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d. Output:\n%s", exitCode, output)
	}
	if strings.Contains(output, "recording") {
		t.Errorf("removing a record entry from a surviving CRD must not be treated as a deletion, got:\n%s", output)
	}
}

func TestE2E_ApplySince_FirstPushNewBranch(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "first-push-new-branch")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// No routes registered: the all-zeros sentinel must fail before any API call.

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")

	if exitCode == 0 {
		t.Fatalf("expected a non-zero exit for the all-zeros sentinel, got 0. Output:\n%s", output)
	}
	if !strings.Contains(output, "all-zeros") {
		t.Errorf("expected the all-zeros error message, got:\n%s", output)
	}
}

func TestE2E_ApplySince_NonAncestorForcePush(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "non-ancestor-force-push")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, Body: map[string]any{}})
	server.OnPattern(http.MethodPut, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureViewsImportSuccess})
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}})

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	t.Run("no --force, no terminal: hard fails", func(t *testing.T) {
		exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
			"--experimental", "apply", "-f", "/work/repo", "--since", ref)
		if exitCode == 0 {
			t.Fatalf("expected a non-zero exit with no terminal to confirm against, got 0. Output:\n%s", output)
		}
		if !strings.Contains(output, "ancestor") {
			t.Errorf("expected a non-ancestor-related error, got:\n%s", output)
		}
	})

	t.Run("--force bypasses the confirmation", func(t *testing.T) {
		exitCode, output := execDash0(ctx, t, container, mockServerPort(t, server),
			"--experimental", "apply", "-f", "/work/repo", "--since", ref, "--force")
		if exitCode != 0 {
			t.Fatalf("expected exit 0 with --force, got %d. Output:\n%s", exitCode, output)
		}
		if !strings.Contains(output, "not an ancestor") {
			t.Errorf("expected the non-ancestor warning to still be printed, got:\n%s", output)
		}
	})
}

func TestE2E_ApplySince_TooShallowClone(t *testing.T) {
	ctx := context.Background()
	repoDir, ref := testutil.BuildGitScenario(t, "too-shallow-clone")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// No routes registered: an unresolvable ref must fail before any API call.

	container := startContainer(ctx, t, mockServerPort(t, server))
	copyScenarioIntoContainer(ctx, t, container, repoDir)

	// Perform the shallow clone inside the container, against the just-copied
	// full-history repo -- this is the one step the checked-in fixture
	// deliberately does not bake in (see generate_git_scenarios.sh).
	exitCode, output := execCommand(ctx, t, container, "git", "clone", "-q", "--depth", "1", "file:///work/repo", "/work/shallow")
	if exitCode != 0 {
		t.Fatalf("failed to create the shallow clone: %d\n%s", exitCode, output)
	}

	exitCode, output = execDash0(ctx, t, container, mockServerPort(t, server),
		"--experimental", "apply", "-f", "/work/shallow", "--since", ref, "--force")

	if exitCode == 0 {
		t.Fatalf("expected a non-zero exit for an unresolvable (too-shallow) ref, got 0. Output:\n%s", output)
	}
	if !strings.Contains(output, "could not be resolved") {
		t.Errorf("expected the unresolvable-ref error message, got:\n%s", output)
	}
}

func execCommand(ctx context.Context, t *testing.T, container testcontainers.Container, args ...string) (int, string) {
	t.Helper()
	exitCode, reader, err := container.Exec(ctx, args)
	if err != nil {
		t.Fatalf("failed to exec %v: %v", args, err)
	}
	return exitCode, drainExecOutput(reader)
}
