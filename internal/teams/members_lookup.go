package teams

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// resolveTeamMembers fetches the organization's members and returns the ones
// whose dash0.com/id label matches an entry in team.Spec.Members. The result
// preserves the order of team.Spec.Members; entries with no matching member are
// skipped silently (the server may still be reconciling, or the caller may have
// listed an email that no longer resolves).
func resolveTeamMembers(
	ctx context.Context,
	apiClient dash0api.Client,
	team *dash0api.TeamDefinitionV1Alpha1,
) ([]*dash0api.MemberDefinition, error) {
	if team == nil || len(team.Spec.Members) == 0 {
		return nil, nil
	}

	all, err := apiClient.ListMembers(ctx)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*dash0api.MemberDefinition, len(all))
	for _, m := range all {
		if m == nil || m.Metadata.Labels == nil || m.Metadata.Labels.Dash0Comid == nil {
			continue
		}
		byID[*m.Metadata.Labels.Dash0Comid] = m
	}

	out := make([]*dash0api.MemberDefinition, 0, len(team.Spec.Members))
	for _, id := range team.Spec.Members {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}
