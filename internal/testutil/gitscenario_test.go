package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"
)

func runGitScenario(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return strings.TrimSpace(string(out))
}

// compileGitRepoFixtureSchema compiles internal/testutil/git_repo_fixture.schema.json.
func compileGitRepoFixtureSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	const schemaPath = "git_repo_fixture.schema.json"
	data, err := os.ReadFile(schemaPath)
	require.NoErrorf(t, err, "failed to read %s", schemaPath)

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	require.NoErrorf(t, err, "failed to parse %s", schemaPath)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource(schemaPath, doc))

	schema, err := compiler.Compile(schemaPath)
	require.NoErrorf(t, err, "failed to compile %s", schemaPath)
	return schema
}

// TestGitScenarioFixtures_MatchSchema validates every checked-in git-scenario
// fixture against git_repo_fixture.schema.json, so a malformed fixture (a
// typo'd field name, a missing required key) fails loudly here instead of
// surfacing as a confusing zero-value somewhere inside BuildGitScenario.
func TestGitScenarioFixtures_MatchSchema(t *testing.T) {
	schema := compileGitRepoFixtureSchema(t)

	entries, err := os.ReadDir(GitScenariosDir())
	require.NoError(t, err)

	var fixtureCount int
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yml" {
			continue
		}
		fixtureCount++

		t.Run(entry.Name(), func(t *testing.T) {
			fixturePath := filepath.Join(GitScenariosDir(), entry.Name())
			yamlData, err := os.ReadFile(fixturePath)
			require.NoError(t, err)

			jsonData, err := sigsyaml.YAMLToJSON(yamlData)
			require.NoErrorf(t, err, "failed to convert %s to JSON", fixturePath)

			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonData))
			require.NoError(t, err)

			if err := schema.Validate(instance); err != nil {
				t.Errorf("%s does not match the GitRepoFixture schema:\n%v", fixturePath, err)
			}
		})
	}
	require.NotZero(t, fixtureCount, "expected at least one .yml fixture in %s", GitScenariosDir())
}

func TestBuildGitScenario_WholeFileDeletion(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "whole-file-deletion")

	assert.Len(t, ref, 40, "ref should be a full commit SHA")
	assert.NoFileExists(t, filepath.Join(repoDir, "dashboard-a.yaml"), "the deleted file must not exist at HEAD")
	assert.FileExists(t, filepath.Join(repoDir, "view-b.yaml"))

	atRef := runGitScenario(t, repoDir, "cat-file", "-p", ref+":dashboard-a.yaml")
	assert.Contains(t, atRef, "dash-a")
}

func TestBuildGitScenario_DirectoryRename(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "directory-rename")

	assert.Len(t, ref, 40, "ref should be a full commit SHA")
	assert.NoFileExists(t, filepath.Join(repoDir, "team-a", "dashboard-a.yaml"), "the old path must not exist at HEAD")
	assert.FileExists(t, filepath.Join(repoDir, "team-b", "dashboard-a.yaml"))

	atRef := runGitScenario(t, repoDir, "cat-file", "-p", ref+":team-a/dashboard-a.yaml")
	assert.Contains(t, atRef, "dash-a")
}

func TestBuildGitScenario_WholeDirectoryDeletion(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "whole-directory-deletion")

	assert.Len(t, ref, 40, "ref should be a full commit SHA")
	assert.NoFileExists(t, filepath.Join(repoDir, "dashboard-a.yaml"), "the deleted file must not exist at HEAD")
	assert.NoFileExists(t, filepath.Join(repoDir, "view-b.yaml"), "the deleted file must not exist at HEAD")

	entries, err := os.ReadDir(repoDir)
	require.NoError(t, err)
	var nonGitEntries []string
	for _, e := range entries {
		if e.Name() != ".git" {
			nonGitEntries = append(nonGitEntries, e.Name())
		}
	}
	assert.Empty(t, nonGitEntries, "the repo's working tree should have nothing left besides .git")

	atRef := runGitScenario(t, repoDir, "cat-file", "-p", ref+":dashboard-a.yaml")
	assert.Contains(t, atRef, "dash-a")
}

func TestBuildGitScenario_MultiDocumentPartialDeletion(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "multi-document-partial-deletion")

	current, err := os.ReadFile(filepath.Join(repoDir, "combined.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(current), "view-combined", "the view document must be gone from the current file")
	assert.Contains(t, string(current), "dash-combined")

	atRef := runGitScenario(t, repoDir, "cat-file", "-p", ref+":combined.yaml")
	assert.Contains(t, atRef, "view-combined", "the view document must still be present at ref")
	assert.Contains(t, atRef, "dash-combined")
}

func TestBuildGitScenario_PrometheusAlertPartialDeletion(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "prometheus-alert-partial-deletion")

	current, err := os.ReadFile(filepath.Join(repoDir, "rules.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(current), "DiskFull")
	assert.Contains(t, string(current), "HighErrorRate")
	assert.Contains(t, string(current), "shared-rule-id")

	atRef := runGitScenario(t, repoDir, "cat-file", "-p", ref+":rules.yaml")
	assert.Contains(t, atRef, "DiskFull")
	assert.Contains(t, atRef, "HighErrorRate")
}

func TestBuildGitScenario_PrometheusRecordingPartialRemoval(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "prometheus-recording-partial-removal")

	current, err := os.ReadFile(filepath.Join(repoDir, "rules.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(current), "instance:cpu_usage:avg5m")
	assert.Contains(t, string(current), "HighErrorRate")

	atRef := runGitScenario(t, repoDir, "cat-file", "-p", ref+":rules.yaml")
	assert.Contains(t, atRef, "instance:cpu_usage:avg5m")
}

func TestBuildGitScenario_FirstPushNewBranch(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "first-push-new-branch")

	assert.Equal(t, "0000000000000000000000000000000000000000", ref)
	assert.FileExists(t, filepath.Join(repoDir, "keep.yaml"))
}

func TestBuildGitScenario_NonAncestorForcePush(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "non-ancestor-force-push")

	// ref must resolve...
	sha := runGitScenario(t, repoDir, "rev-parse", "--verify", ref+"^{commit}")
	assert.Equal(t, ref, sha)

	// ...but must not be an ancestor of HEAD (git merge-base --is-ancestor
	// exits 1 for "not an ancestor", which Run() surfaces as a non-nil err).
	cmd := exec.Command("git", "-C", repoDir, "merge-base", "--is-ancestor", ref, "HEAD")
	err := cmd.Run()
	require.Error(t, err, "the orphaned commit must not be an ancestor of the rewritten HEAD")
}

func TestBuildGitScenario_TooShallowClone(t *testing.T) {
	repoDir, ref := BuildGitScenario(t, "too-shallow-clone")

	// The checked-in fixture carries full history: ref must resolve directly.
	runGitScenario(t, repoDir, "rev-parse", "--verify", ref+"^{commit}")

	shallowDir := t.TempDir() + "/shallow"
	runGitScenario(t, ".", "clone", "-q", "--depth", "1", "file://"+repoDir, shallowDir)

	cmd := exec.Command("git", "-C", shallowDir, "rev-parse", "--verify", ref+"^{commit}")
	err := cmd.Run()
	require.Error(t, err, "a --depth 1 clone must not have the older ref commit")
}
