package asset

import (
	"context"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// TeamUsesOrigin reports whether a team document carries a non-empty
// dash0.com/origin label. --since uses it the same way as SLOUsesOrigin: an
// id-only team applied before the router was created by POST, so the live
// team can sit at a server-assigned id the document never learned.
func TeamUsesOrigin(data []byte) (bool, error) {
	var team dash0api.TeamDefinitionV1Alpha1
	if err := dash0yaml.Unmarshal(data, &team); err != nil {
		return false, fmt.Errorf("failed to decode team: %w", err)
	}
	return dash0api.GetTeamOrigin(&team) != "", nil
}

// ImportTeam creates or updates a team through RouteDocument.
//
// Before returning, spec.members on both the before and after states is
// translated from internal IDs (what the server echoes) to email addresses,
// so the apply diff renderer prints legible membership changes.
func ImportTeam(ctx context.Context, apiClient dash0api.Client, team *dash0api.TeamDefinitionV1Alpha1) (ImportResult, error) {
	plan, err := PlanTeam(ctx, apiClient, team)
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setTeamOrigin(team, "")
	}
	result, err := apiClient.UpsertTeam(ctx, plan.Key, team)
	if err != nil {
		return ImportResult{}, err
	}

	// Emails render membership changes as "-bob@example.com" rather than
	// opaque ids. A failed lookup leaves the raw ids in place.
	if before, ok := plan.Before.(*dash0api.TeamDefinitionV1Alpha1); ok {
		_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, before)
	}
	_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, result)

	name := dash0api.GetTeamDisplayName(result)
	if name == "" {
		name = dash0api.GetTeamName(result)
	}
	return ImportResult{Name: name, ID: dash0api.GetTeamID(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanTeam routes the team with RouteDocument and reads the stored team that
// ImportTeam would write. It returns any preflight error other than "not
// found"; an empty Key means the write has no target.
//
// The server stores member ids, so spec.members on Before holds ids. Callers
// resolve them to emails and decide whether a failed lookup matters.
func PlanTeam(ctx context.Context, apiClient dash0api.Client, team *dash0api.TeamDefinitionV1Alpha1) (ImportPlan, error) {
	// StripTeamServerFields clears the dash0.com/id label, so read it first.
	identity := TeamIdentity(team)
	dash0api.StripTeamServerFields(team)
	route, err := RouteDocument(ctx, identity, apiClient.GetTeam, (*dash0api.TeamDefinitionV1Alpha1).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: team}
	if route.Key == "" || route.Create {
		setTeamOrigin(team, route.Key)
		return plan, err
	}
	setTeamOrigin(team, route.Stored.GetOrigin())
	plan.Action, plan.Before = ActionUpdated, route.Stored
	return plan, err
}

// TeamIdentity returns what RouteDocument routes a team by. Teams are
// organization-level, so the inferred origin has no dataset.
func TeamIdentity(team *dash0api.TeamDefinitionV1Alpha1) Identity {
	return identityOf("team", team.Metadata.Name, nil, team)
}

func setTeamOrigin(team *dash0api.TeamDefinitionV1Alpha1, origin string) {
	if team.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		team.Metadata.Labels = &dash0api.TeamLabels{}
	}
	team.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}
