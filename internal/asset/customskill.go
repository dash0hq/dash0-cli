package asset

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"gopkg.in/yaml.v3"
)

// CustomSkillKind is the kind of an Agent0 custom skill document.
const CustomSkillKind = "Dash0Skill"

// CustomSkillManifestPath is the file every custom skill must carry at its root.
const CustomSkillManifestPath = "SKILL.md"

// ValidateCustomSkill checks what every custom skill entry point needs before an API call:
// the Dash0Skill kind and a SKILL.md file at the skill root.
func ValidateCustomSkill(skill *dash0api.SkillDefinition) error {
	if skill.Kind != "" && NormalizeKind(string(skill.Kind)) != "skill" {
		return fmt.Errorf("unsupported kind %q for an Agent0 skill (expected %s)", skill.Kind, CustomSkillKind)
	}
	if customSkillManifest(skill) == nil {
		return fmt.Errorf("Agent0 skill definition requires a %s file in spec.files", CustomSkillManifestPath)
	}
	return nil
}

func customSkillManifest(skill *dash0api.SkillDefinition) *dash0api.SkillFile {
	for i := range skill.Spec.Files {
		if skill.Spec.Files[i].RelativePath == CustomSkillManifestPath {
			return &skill.Spec.Files[i]
		}
	}
	return nil
}

// CustomSkillName returns the name of a custom skill: metadata.name when the API set it, otherwise
// the name in the SKILL.md frontmatter, which is where the server reads it from. It returns "" when
// neither is available.
func CustomSkillName(skill *dash0api.SkillDefinition) string {
	if name := dash0api.GetCustomSkillName(skill); name != "" {
		return name
	}
	manifest := customSkillManifest(skill)
	if manifest == nil {
		return ""
	}
	return frontmatterName(manifest.Content)
}

func frontmatterName(content string) string {
	rest, ok := strings.CutPrefix(strings.TrimLeft(content, "\ufeff"), "---")
	if !ok {
		return ""
	}
	block, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return ""
	}
	var frontmatter struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(block), &frontmatter); err != nil {
		return ""
	}
	return frontmatter.Name
}

// CustomSkillKey returns the identifier a custom skill is addressed by: its origin, or its id when
// it has no origin.
func CustomSkillKey(skill *dash0api.SkillDefinition) string {
	if origin := dash0api.GetCustomSkillOrigin(skill); origin != "" {
		return origin
	}
	return dash0api.GetCustomSkillID(skill)
}

// ImportCustomSkill creates or updates an Agent0 custom skill.
//
// The origin is the upsert key: PUT by origin creates the skill when the origin is new.
// Without an origin, the dash0.com/id label can only address an existing skill, so a preflight GET
// turns an unknown id into a not-found error instead of a skill created with the id as its origin.
// Without either, POST creates the skill and the server assigns its id.
//
// The origin and id are read before StripCustomSkillServerFields, which removes spec.source and
// the per-user dash0.com/enabled annotation that a write must never send.
func ImportCustomSkill(ctx context.Context, apiClient dash0api.Client, skill *dash0api.SkillDefinition) (ImportResult, error) {
	if err := ValidateCustomSkill(skill); err != nil {
		return ImportResult{}, err
	}
	origin := dash0api.GetCustomSkillOrigin(skill)
	id := dash0api.GetCustomSkillID(skill)
	dash0api.StripCustomSkillServerFields(skill)
	skill.Kind = dash0api.Dash0Skill

	var before *dash0api.SkillDefinition
	action := ActionCreated
	var result *dash0api.SkillDefinition
	var err error
	switch {
	case origin != "":
		before, err = apiClient.GetCustomSkill(ctx, origin)
		switch {
		case err == nil:
			action = actionForExisting(customSkillDeletedAt(before))
		case dash0api.IsNotFound(err):
			before = nil
		default:
			return ImportResult{}, err
		}
		result, err = apiClient.UpdateCustomSkill(ctx, origin, skill)
	case id != "":
		before, err = apiClient.GetCustomSkill(ctx, id)
		if err != nil {
			if dash0api.IsNotFound(err) {
				return ImportResult{}, fmt.Errorf("Agent0 skill with id %q not found"+
					"\nHint: dash0.com/id only addresses an existing skill; set %s to create the skill and manage it as code", id, OriginLabel)
			}
			return ImportResult{}, err
		}
		action = actionForExisting(customSkillDeletedAt(before))
		result, err = apiClient.UpdateCustomSkill(ctx, id, skill)
	default:
		result, err = apiClient.CreateCustomSkill(ctx, skill)
	}
	if err != nil {
		return ImportResult{}, err
	}

	var beforeAny any
	if before != nil {
		SortCustomSkillPermissions(before)
		beforeAny = before
	}
	SortCustomSkillPermissions(result)
	return ImportResult{Name: CustomSkillName(result), ID: CustomSkillKey(result), Action: action, Before: beforeAny, After: result}, nil
}

func customSkillDeletedAt(skill *dash0api.SkillDefinition) *time.Time {
	if skill == nil || skill.Metadata.Annotations == nil {
		return nil
	}
	return skill.Metadata.Annotations.Dash0ComdeletedAt
}

// SortCustomSkillPermissions is the custom skill equivalent of SortViewPermissions.
func SortCustomSkillPermissions(skill *dash0api.SkillDefinition) {
	if skill == nil || skill.Spec.Permissions == nil {
		return
	}
	perms := *skill.Spec.Permissions
	sort.SliceStable(perms, func(i, j int) bool {
		return permissionSortKey(perms[i].Role, perms[i].TeamId, perms[i].UserId) <
			permissionSortKey(perms[j].Role, perms[j].TeamId, perms[j].UserId)
	})
}

// ExplainCustomSkillError adds the cause and the fix to the API errors a custom skill write
// commonly hits: a name another skill already uses, and a defaultEnabled the caller may not set.
// It returns nil for any other error, so the caller falls back to client.HandleAPIError.
//
// The advice goes behind "\nHint:" so agent mode lifts it into the JSON error's hint field.
func ExplainCustomSkillError(err error, name string) error {
	var apiErr *dash0api.APIError
	if !errors.As(err, &apiErr) {
		return nil
	}
	// Matching on the text is unavoidable, because neither error carries a machine-readable discriminator.
	haystack := strings.ToLower(apiErr.Message + " " + apiErr.Body)
	switch {
	case apiErr.StatusCode == http.StatusBadRequest && strings.Contains(haystack, "names must be unique"):
		return fmt.Errorf(
			"another Agent0 skill in this organization is already named %q:\n  %w"+
				"\nHint: the name comes from the SKILL.md frontmatter and is unique per organization. "+
				"Rename the skill, delete the other one, or set the other skill's %s in this document to update it instead",
			name, err, OriginLabel,
		)
	case apiErr.StatusCode == http.StatusForbidden && strings.Contains(haystack, "defaultenabled"):
		return fmt.Errorf(
			"not allowed to set defaultEnabled on a spec.permissions entry of Agent0 skill %q:\n  %w"+
				"\nHint: admins may set defaultEnabled on any grant; members only on their own user grant or on a grant to a team they belong to. "+
				"Remove defaultEnabled from the other entries, or have an admin apply the document",
			name, err,
		)
	default:
		return nil
	}
}
