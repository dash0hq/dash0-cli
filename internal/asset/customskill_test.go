package asset

import (
	"context"
	"errors"
	"net/http"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skillWithManifest(content string) *dash0api.SkillDefinition {
	return &dash0api.SkillDefinition{
		Kind: dash0api.Dash0Skill,
		Spec: dash0api.SkillSpec{Files: []dash0api.SkillFile{{RelativePath: CustomSkillManifestPath, Content: content}}},
	}
}

func TestCustomSkillName(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string
	}{
		"frontmatter":           {"---\nname: incident-triage\ndescription: d\n---\nBody.\n", "incident-triage"},
		"byte order mark":       {"\ufeff---\nname: incident-triage\n---\nBody.\n", "incident-triage"},
		"no frontmatter":        {"# incident-triage\n", ""},
		"unterminated":          {"---\nname: incident-triage\n", ""},
		"invalid frontmatter":   {"---\nname: [\n---\n", ""},
		"frontmatter sans name": {"---\ndescription: d\n---\n", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, CustomSkillName(skillWithManifest(c.content)))
		})
	}
}

func TestCustomSkillName_PrefersServerName(t *testing.T) {
	skill := skillWithManifest("---\nname: from-file\n---\n")
	name := "from-server"
	skill.Metadata.Name = &name

	assert.Equal(t, "from-server", CustomSkillName(skill))
}

func TestValidateCustomSkill(t *testing.T) {
	require.NoError(t, ValidateCustomSkill(skillWithManifest("x")))

	noManifest := &dash0api.SkillDefinition{Spec: dash0api.SkillSpec{Files: []dash0api.SkillFile{{RelativePath: "README.md"}}}}
	assert.ErrorContains(t, ValidateCustomSkill(noManifest), "requires a SKILL.md file")

	wrongKind := skillWithManifest("x")
	wrongKind.Kind = "Dashboard"
	assert.ErrorContains(t, ValidateCustomSkill(wrongKind), `unsupported kind "Dashboard"`)
}

func TestCustomSkillKey(t *testing.T) {
	skill := skillWithManifest("x")
	assert.Empty(t, CustomSkillKey(skill))

	dash0api.SetCustomSkillID(skill, "cskl_1")
	assert.Equal(t, "cskl_1", CustomSkillKey(skill))

	dash0api.SetCustomSkillOrigin(skill, "dash0-cli_triage")
	assert.Equal(t, "dash0-cli_triage", CustomSkillKey(skill))
}

func TestExplainCustomSkillError(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"duplicate name": {
			&dash0api.APIError{StatusCode: http.StatusBadRequest, Message: "Bad Request: ... Skill names must be unique per organization."},
			`another Agent0 skill in this organization is already named "triage"`,
		},
		"defaultEnabled forbidden": {
			&dash0api.APIError{StatusCode: http.StatusForbidden, Message: "Forbidden: You may not change defaultEnabled on the spec.permissions entry with role basic_member."},
			"not allowed to set defaultEnabled",
		},
		"other bad request":      {&dash0api.APIError{StatusCode: http.StatusBadRequest, Message: "Bad Request: no files"}, ""},
		"other forbidden":        {&dash0api.APIError{StatusCode: http.StatusForbidden, Message: "Forbidden: write"}, ""},
		"not an API error":       {errors.New("Skill names must be unique"), ""},
		"name text on a non-400": {&dash0api.APIError{StatusCode: http.StatusConflict, Message: "Skill names must be unique"}, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			explained := ExplainCustomSkillError(c.err, "triage")
			if c.want == "" {
				assert.NoError(t, explained)
				return
			}
			require.Error(t, explained)
			assert.Contains(t, explained.Error(), c.want)
			assert.Contains(t, explained.Error(), "\nHint: ")
		})
	}
}

func TestImportCustomSkill_InvalidMakesNoAPICall(t *testing.T) {
	noManifest := &dash0api.SkillDefinition{Spec: dash0api.SkillSpec{Files: []dash0api.SkillFile{{RelativePath: "README.md"}}}}

	_, err := ImportCustomSkill(context.Background(), nil, noManifest)

	assert.ErrorContains(t, err, "requires a SKILL.md file")
}

// The server adds a version, timestamps, the per-user enabled state, permittedActions and the admin grant to
// every skill it returns, so none of them may show up as a change.
func TestUnifiedDiff_CustomSkillIgnoresServerFields(t *testing.T) {
	sent := skillWithManifest("---\nname: triage\n---\nBody.\n")
	dash0api.SetCustomSkillOrigin(sent, "dash0-cli_triage")
	member := "basic_member"
	off := false
	sent.Spec.Permissions = &[]dash0api.SkillPermission{{Role: &member, Actions: []dash0api.CustomSkillAction{"custom_skill:read"}, DefaultEnabled: &off}}

	returned := skillWithManifest("---\nname: triage\n---\nBody.\n")
	dash0api.SetCustomSkillOrigin(returned, "dash0-cli_triage")
	name, version, enabled := "triage", "7", "true"
	returned.Metadata.Name = &name
	returned.Metadata.Labels.Dash0Comversion = &version
	returned.Metadata.Annotations = &dash0api.SkillAnnotations{Dash0Comenabled: &enabled}
	admin, on := "admin", true
	returned.Spec.Permissions = &[]dash0api.SkillPermission{
		{Role: &admin, Actions: []dash0api.CustomSkillAction{"custom_skill:read", "custom_skill:delete"}, DefaultEnabled: &on},
		{Role: &member, Actions: []dash0api.CustomSkillAction{"custom_skill:read"}, DefaultEnabled: &off},
	}
	returned.Spec.PermittedActions = &[]dash0api.CustomSkillAction{"custom_skill:read"}

	text, err := unifiedDiff("Agent0 skill", sent, returned)

	require.NoError(t, err)
	assert.Empty(t, text)
}
