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

// CustomSkillName returns the name of a custom skill: the name in the SKILL.md frontmatter, which is
// where the server reads it from, otherwise metadata.name. The frontmatter wins because an exported
// document keeps the old metadata.name after the frontmatter renames the skill. It returns "" when
// neither is available.
func CustomSkillName(skill *dash0api.SkillDefinition) string {
	if manifest := customSkillManifest(skill); manifest != nil {
		if name := frontmatterName(manifest.Content); name != "" {
			return name
		}
	}
	return dash0api.GetCustomSkillName(skill)
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
	return PreferredKey(skill)
}

// ImportCustomSkill creates or updates an Agent0 custom skill through
// RouteDocument. The body carries the origin only on create.
//
// The origin and id are read before StripCustomSkillServerFields, which removes spec.source and
// the per-user dash0.com/enabled annotation that a write must never send.
func ImportCustomSkill(ctx context.Context, apiClient dash0api.Client, skill *dash0api.SkillDefinition) (ImportResult, error) {
	plan, err := PlanCustomSkill(ctx, apiClient, skill)
	if err != nil {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		SetCustomSkillOrigin(skill, "")
	}
	result, err := apiClient.UpdateCustomSkill(ctx, plan.Key, skill)
	if err != nil {
		return ImportResult{}, err
	}
	if err := RequireNotDeleted("Agent0 skill", CustomSkillName(result), dash0api.GetCustomSkillID(result), CustomSkillDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	SortCustomSkillPermissions(result)
	return ImportResult{Name: CustomSkillName(result), ID: CustomSkillKey(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanCustomSkill routes the skill with RouteDocument and reads the stored
// skill that ImportCustomSkill would write. Any preflight error other than
// "not found" is returned and stops the write. Only a PUT to the id keeps the
// id in the body.
func PlanCustomSkill(ctx context.Context, apiClient dash0api.Client, skill *dash0api.SkillDefinition) (ImportPlan, error) {
	if err := ValidateCustomSkill(skill); err != nil {
		return ImportPlan{}, err
	}
	// The strip clears metadata.name, which the name falls back to, so read it first.
	identity := CustomSkillIdentity(skill)
	dash0api.StripCustomSkillServerFields(skill)
	skill.Kind = dash0api.Dash0Skill

	route, err := RouteDocument(ctx, identity, apiClient.GetCustomSkill, (*dash0api.SkillDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: skill}
	if route.Key != identity.ID {
		dash0api.ClearCustomSkillID(skill)
	}
	if err != nil {
		return plan, err
	}
	if route.Create {
		SetCustomSkillOrigin(skill, route.Key)
		return plan, nil
	}
	// The stored origin keeps the diff to what changes. ImportCustomSkill
	// clears it before the write: an update never sends the origin.
	SetCustomSkillOrigin(skill, route.Stored.GetOrigin())
	SortCustomSkillPermissions(route.Stored)
	plan.Action, plan.Before = actionForExisting(CustomSkillDeletedAt(route.Stored)), route.Stored
	return plan, nil
}

// CustomSkillIdentity returns what RouteDocument routes a skill by. The
// inferred origin comes from the skill's name, which is the SKILL.md
// frontmatter name and unique per organization, not from metadata.name.
// Skills are organization-level, so it has no dataset.
func CustomSkillIdentity(skill *dash0api.SkillDefinition) Identity {
	return Identity{
		ID:       skill.GetID(),
		Origin:   skill.GetOrigin(),
		Inferred: InferKindOrigin("skill", CustomSkillName(skill), nil),
	}
}

func SetCustomSkillOrigin(skill *dash0api.SkillDefinition, origin string) {
	if skill.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		skill.Metadata.Labels = &dash0api.SkillLabels{}
	}
	skill.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}

// CustomSkillDeletedAt returns when the skill was soft-deleted, or nil if it was not.
func CustomSkillDeletedAt(skill *dash0api.SkillDefinition) *time.Time {
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
