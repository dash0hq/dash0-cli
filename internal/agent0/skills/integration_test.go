//go:build integration

package skills

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAuthToken = "auth_test_token"
	apiPathSkills = "/api/agent0/custom-skills"
	skillOrigin   = "dash0-cli_incident-triage"
	skillID       = "cskl_01jb8k3m4n5p6q7r8s9t0v1w2x"
	pathByOrigin  = apiPathSkills + "/" + skillOrigin
	pathByID      = apiPathSkills + "/" + skillID

	skillYAML = `kind: Dash0Skill
metadata:
  labels:
    dash0.com/origin: dash0-cli_incident-triage
spec:
  files:
    - relativePath: SKILL.md
      content: |
        ---
        name: incident-triage
        description: Triage a production incident from its failed checks.
        ---

        Start from the failed check, then follow the trace to the failing service.
    - relativePath: references/runbook.md
      content: |
        # Runbook

        Check the deploy log, then the feature flags.
  permissions:
    - role: basic_member
      actions:
        - custom_skill:read
      defaultEnabled: false
`
)

var skillPathPattern = regexp.MustCompile(`^/api/agent0/custom-skills/[^/]+$`)

func newExperimentalSkillsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	agent0 := &cobra.Command{Use: "agent0"}
	agent0.AddCommand(NewSkillsCmd())
	root.AddCommand(agent0)
	return root
}

func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newExperimentalSkillsCmd()
	cmd.SetArgs(append(args, "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "skill.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func requestsFor(server *testutil.MockServer, method, path string) []testutil.RecordedRequest {
	var out []testutil.RecordedRequest
	for _, r := range server.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func decodeSkill(t *testing.T, req testutil.RecordedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	return body
}

// assertWriteBody checks that a write carries what the document declares and none of the fields the API
// rejects or derives: spec.source, the per-user dash0.com/enabled, and the admin grant the server adds.
func assertWriteBody(t *testing.T, body map[string]any) {
	t.Helper()
	assert.Equal(t, "Dash0Skill", body["kind"])
	metadata := body["metadata"].(map[string]any)
	assert.NotContains(t, metadata, "annotations")
	assert.NotContains(t, metadata, "name")
	spec := body["spec"].(map[string]any)
	assert.NotContains(t, spec, "source")
	assert.NotContains(t, spec, "permittedActions")
	assert.Len(t, spec["files"], 2)
	assert.Equal(t, []any{
		map[string]any{"role": "basic_member", "actions": []any{"custom_skill:read"}, "defaultEnabled": false},
	}, spec["permissions"])
}

func mockGet(server *testutil.MockServer, path string, status int, fixture string) {
	server.On(http.MethodGet, path, testutil.MockResponse{StatusCode: status, BodyFile: fixture, Validator: testutil.RequireHeaders})
}

func TestSkills_RequireExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	for _, args := range [][]string{
		{"agent0", "skills", "list"},
		{"agent0", "skills", "get", skillOrigin},
		{"agent0", "skills", "create", "-f", writeYAML(t, skillYAML)},
		{"agent0", "skills", "update", "-f", writeYAML(t, skillYAML)},
		{"agent0", "skills", "delete", skillOrigin, "--force"},
	} {
		_, _, err := runCmd(t, server.URL, args...)
		require.Error(t, err, args)
		assert.Contains(t, err.Error(), "experimental command")
	}
	assert.Empty(t, server.Requests())
}

func TestListSkills_Table(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 3)
	assert.Regexp(t, `^NAME\s+ID$`, strings.TrimSpace(lines[0]))
	assert.Regexp(t, `^incident-triage\s+`+skillID+`$`, strings.TrimSpace(lines[1]))
	assert.Regexp(t, `^release-notes\s+cskl_`, strings.TrimSpace(lines[2]))
}

func TestListSkills_Wide(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list", "-o", "wide")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 3)
	assert.Regexp(t, `^NAME\s+ID\s+ORIGIN\s+ENABLED$`, strings.TrimSpace(lines[0]))
	assert.Regexp(t, `^incident-triage\s+\S+\s+`+skillOrigin+`\s+true$`, strings.TrimSpace(lines[1]))
	assert.Regexp(t, `^release-notes\s+\S+\s+-\s+false$`, strings.TrimSpace(lines[2]))
}

func TestListSkills_Filters(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListEmpty)

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list", "--origin-prefix", "dash0-cli_", "--enabled=false")

	require.NoError(t, err)
	reqs := requestsFor(server, http.MethodGet, apiPathSkills)
	require.Len(t, reqs, 1)
	assert.Contains(t, reqs[0].Query, "originPrefix=dash0-cli_")
	assert.Contains(t, reqs[0].Query, "enabled=false")
}

func TestListSkills_NoFiltersByDefault(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListEmpty)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list")

	require.NoError(t, err)
	assert.Contains(t, output, "No Agent0 skills found")
	reqs := requestsFor(server, http.MethodGet, apiPathSkills)
	require.Len(t, reqs, 1)
	assert.Empty(t, reqs[0].Query)
}

func TestListSkills_EmptyJSONIsArray(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListEmpty)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list", "-o", "json")

	require.NoError(t, err)
	assert.Equal(t, "[]\n", output)
}

func TestListSkills_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list", "-o", "json")

	require.NoError(t, err)
	var parsed []dash0api.SkillDefinition
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed, 2)
	assert.Equal(t, skillOrigin, dash0api.GetCustomSkillOrigin(&parsed[0]))
	require.NotNil(t, parsed[0].Spec.Permissions)
	assert.Equal(t, "admin", *(*parsed[0].Spec.Permissions)[0].Role, "permissions are sorted")
}

func TestListSkills_YAML(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list", "-o", "yaml")

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(output, "\n---\n"))
	assert.Contains(t, output, "kind: Dash0Skill")
}

func TestListSkills_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusOK, testutil.FixtureAgent0SkillsListSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list", "-o", "csv", "--skip-header")

	require.NoError(t, err)
	assert.Equal(t, "incident-triage,"+skillID+","+skillOrigin+",true\nrelease-notes,cskl_01jb8k3m4n5p6q7r8s9t0v1w2y,-,false\n", output)
}

func TestListSkills_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, apiPathSkills, http.StatusUnauthorized, testutil.FixtureAgent0SkillsUnauthorized)

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "list")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestGetSkill_Table(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "get", skillOrigin)

	require.NoError(t, err)
	assert.Equal(t, "Kind:    Dash0Skill\n"+
		"Name:    incident-triage\n"+
		"ID:      "+skillID+"\n"+
		"Origin:  "+skillOrigin+"\n"+
		"Enabled: true\n"+
		"Files:\n"+
		"  SKILL.md\n"+
		"  references/runbook.md\n", output)
}

// The YAML export is what users commit, so spec.permissions, defaultEnabled included, must survive it.
func TestGetSkill_YAMLKeepsPermissions(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "get", skillOrigin, "-o", "yaml")

	require.NoError(t, err)
	var skill dash0api.SkillDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte(output), &skill))
	require.NotNil(t, skill.Spec.Permissions)
	perms := *skill.Spec.Permissions
	require.Len(t, perms, 2)
	assert.Equal(t, "basic_member", *perms[1].Role)
	require.NotNil(t, perms[1].DefaultEnabled)
	assert.False(t, *perms[1].DefaultEnabled)
}

func TestGetSkill_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByID, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "get", skillID, "-o", "json")

	require.NoError(t, err)
	var skill dash0api.SkillDefinition
	require.NoError(t, json.Unmarshal([]byte(output), &skill))
	assert.Equal(t, skillID, dash0api.GetCustomSkillID(&skill))
}

func TestGetSkill_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusNotFound, testutil.FixtureAgent0SkillsNotFound)

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "get", skillOrigin)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `Agent0 skill "`+skillOrigin+`" not found`)
}

func TestCreateSkill_PutsByOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusNotFound, testutil.FixtureAgent0SkillsNotFound)
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsCreateSuccess, Validator: testutil.RequireHeaders})

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, skillYAML))

	require.NoError(t, err)
	assert.Equal(t, `Agent0 skill "incident-triage" created (id: `+skillOrigin+")\n", output)
	puts := requestsFor(server, http.MethodPut, pathByOrigin)
	require.Len(t, puts, 1)
	assertWriteBody(t, decodeSkill(t, puts[0]))
	assert.Empty(t, requestsFor(server, http.MethodPost, apiPathSkills))
}

func TestCreateSkill_UpdatesExistingOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsUpdateSuccess, Validator: testutil.RequireHeaders})

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, skillYAML))

	require.NoError(t, err)
	assert.Equal(t, `Agent0 skill "incident-triage" updated (id: `+skillOrigin+")\n", output)
}

// An exported skill carries server fields; a write must drop them while keeping the grants the user set.
func TestCreateSkill_StripsExportedServerFields(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsUpdateSuccess, Validator: testutil.RequireHeaders})
	exported, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), testutil.FixtureAgent0SkillsGetSuccess))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(exported, &doc))
	doc["spec"].(map[string]any)["source"] = map[string]any{"provider": "github", "repositoryUrl": "https://github.com/dash0hq/agent-skills", "path": "skills/incident-triage"}
	data, err := json.Marshal(doc)
	require.NoError(t, err)

	_, _, err = runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, string(data)))

	require.NoError(t, err)
	puts := requestsFor(server, http.MethodPut, pathByOrigin)
	require.Len(t, puts, 1)
	assertWriteBody(t, decodeSkill(t, puts[0]))
}

func TestCreateSkill_PostsWithoutOriginOrID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSkills, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsCreateSuccess, Validator: testutil.RequireHeaders})
	noOrigin := strings.Replace(skillYAML, "  labels:\n    dash0.com/origin: dash0-cli_incident-triage\n", "  {}\n", 1)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, noOrigin))

	require.NoError(t, err)
	assert.Contains(t, output, `Agent0 skill "incident-triage" created`)
	assert.Len(t, requestsFor(server, http.MethodPost, apiPathSkills), 1)
}

func TestCreateSkill_DuplicateNameExplained(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusNotFound, testutil.FixtureAgent0SkillsNotFound)
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: testutil.FixtureAgent0SkillsDuplicateName, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, skillYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `another Agent0 skill in this organization is already named "incident-triage"`)
	assert.Contains(t, err.Error(), "\nHint: the name comes from the SKILL.md frontmatter")
}

func TestCreateSkill_RequiresManifestWithoutAPICall(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	noManifest := strings.Replace(skillYAML, "relativePath: SKILL.md", "relativePath: README.md", 1)

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, noManifest))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a SKILL.md file in spec.files")
	assert.Empty(t, server.Requests())
}

func TestCreateSkill_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "create", "-f", writeYAML(t, skillYAML), "--dry-run")

	require.NoError(t, err)
	assert.Equal(t, "Dry run: Agent0 skill definition is valid\n", output)
	assert.Empty(t, server.Requests())
}

func TestUpdateSkill_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsUpdateSuccess, Validator: testutil.RequireHeaders})

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "update", "-f", writeYAML(t, skillYAML))

	require.NoError(t, err)
	assert.Contains(t, output, "-      Check the deploy log first.")
	assert.Contains(t, output, "+      Check the deploy log, then the feature flags.")
	assert.NotContains(t, output, "dash0.com/version", "server-managed fields stay out of the diff")
	assert.NotContains(t, output, "role: admin", "the server's admin grant stays out of the diff")
	puts := requestsFor(server, http.MethodPut, pathByOrigin)
	require.Len(t, puts, 1)
	assertWriteBody(t, decodeSkill(t, puts[0]))
}

func TestUpdateSkill_ByIDArgument(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByID, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)
	server.On(http.MethodPut, pathByID, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsUpdateSuccess, Validator: testutil.RequireHeaders})
	noLabels := strings.Replace(skillYAML, "  labels:\n    dash0.com/origin: dash0-cli_incident-triage\n", "  {}\n", 1)

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "update", skillID, "-f", writeYAML(t, noLabels))

	require.NoError(t, err)
	assert.Len(t, requestsFor(server, http.MethodPut, pathByID), 1)
}

func TestUpdateSkill_NotFoundSendsNoPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusNotFound, testutil.FixtureAgent0SkillsNotFound)

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "update", "-f", writeYAML(t, skillYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, requestsFor(server, http.MethodPut, pathByOrigin))
}

func TestUpdateSkill_DefaultEnabledForbiddenExplained(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusForbidden, BodyFile: testutil.FixtureAgent0SkillsDefaultEnabledDenied, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "update", "-f", writeYAML(t, skillYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `not allowed to set defaultEnabled on a spec.permissions entry of Agent0 skill "incident-triage"`)
	assert.Contains(t, err.Error(), "\nHint: admins may set defaultEnabled on any grant")
}

func TestUpdateSkill_ArgumentContradictsFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "update", "other-origin", "-f", writeYAML(t, skillYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `argument "other-origin" does not match the file's origin`)
	assert.Empty(t, server.Requests())
}

func TestUpdateSkill_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	mockGet(server, pathByOrigin, http.StatusOK, testutil.FixtureAgent0SkillsGetSuccess)

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "update", "-f", writeYAML(t, skillYAML), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, output, "+      Check the deploy log, then the feature flags.")
	assert.Empty(t, requestsFor(server, http.MethodPut, pathByOrigin))
}

func TestDeleteSkill_Force(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, skillPathPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}, Validator: testutil.RequireHeaders})

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "delete", skillOrigin, "--force")

	require.NoError(t, err)
	assert.Equal(t, `Agent0 skill "`+skillOrigin+"\" deleted\n", output)
	assert.Len(t, requestsFor(server, http.MethodDelete, pathByOrigin), 1)
}

func TestDeleteSkill_PromptDeclined(t *testing.T) {
	testutil.SetupTestEnv(t)
	restore := confirmation.SetReaderForTest(strings.NewReader("n\n"))
	t.Cleanup(restore)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	output, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "delete", skillOrigin)

	require.NoError(t, err)
	assert.Contains(t, output, "Deletion cancelled")
	assert.Empty(t, server.Requests())
}

func TestDeleteSkill_ForceAlreadyDeleted(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, skillPathPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureAgent0SkillsNotFound, Validator: testutil.RequireHeaders})

	output, stderr, err := runCmd(t, server.URL, "-X", "agent0", "skills", "delete", skillOrigin, "--force")

	require.NoError(t, err)
	assert.Empty(t, output)
	assert.Contains(t, stderr, `Agent0 skill "`+skillOrigin+`" was already deleted`)
}

func TestDeleteSkill_NotFoundWithoutForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	restore := confirmation.SetReaderForTest(strings.NewReader("y\n"))
	t.Cleanup(restore)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, skillPathPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureAgent0SkillsNotFound, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "-X", "agent0", "skills", "delete", skillOrigin)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `Agent0 skill "`+skillOrigin+`" not found`)
}
