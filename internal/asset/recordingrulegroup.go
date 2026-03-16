package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// StripRecordingRuleGroupServerFields removes server-generated fields from a
// recording rule group definition. Used by both Import (to avoid sending
// rejected fields to the API) and diff rendering (to suppress noise).
func StripRecordingRuleGroupServerFields(g *dash0api.RecordingRuleGroupDefinition) {
	if g.Metadata.Labels == nil {
		g.Metadata.Labels = &dash0api.RecordingRuleGroupLabels{}
	}
	g.Metadata.Labels.Dash0Comid = nil
	g.Metadata.Labels.Dash0Comorigin = nil
	g.Metadata.Labels.Dash0Comversion = nil
	g.Metadata.Labels.Dash0Comdataset = nil
	g.Metadata.Labels.Dash0Comsource = nil
	g.Spec.Permissions = nil
	g.Spec.PermittedActions = nil
}

// ImportRecordingRuleGroup checks existence by origin or ID, strips
// server-generated fields, injects the dataset into the body, and creates or
// updates the recording rule group via the standard CRUD APIs.
func ImportRecordingRuleGroup(ctx context.Context, apiClient dash0api.Client, group *dash0api.RecordingRuleGroupDefinition, dataset *string) (ImportResult, error) {
	StripRecordingRuleGroupServerFields(group)

	// Inject dataset into body — Create/Update have no dataset query param.
	if dataset != nil && *dataset != "" {
		group.Metadata.Labels.Dash0Comdataset = dataset
	}

	action := ActionCreated
	var before any
	id := ExtractRecordingRuleGroupID(group)
	if id != "" {
		existing, err := apiClient.GetRecordingRuleGroup(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		} else {
			// Asset not found — clear the origin so the API creates a fresh asset.
			group.Metadata.Labels.Dash0Comorigin = nil
			id = ""
		}
	}

	var result *dash0api.RecordingRuleGroupDefinition
	var err error
	if action == ActionUpdated {
		result, err = apiClient.UpdateRecordingRuleGroup(ctx, id, group)
	} else {
		result, err = apiClient.CreateRecordingRuleGroup(ctx, group)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := ExtractRecordingRuleGroupID(result)
	return ImportResult{Name: ExtractRecordingRuleGroupName(result), ID: resultID, Action: action, Before: before, After: result}, nil
}

// ExtractRecordingRuleGroupID extracts the external-facing ID from a recording
// rule group definition. The origin label (set by Terraform/operator) is
// preferred; the internal UUID is used as a fallback.
func ExtractRecordingRuleGroupID(group *dash0api.RecordingRuleGroupDefinition) string {
	if group.Metadata.Labels != nil {
		if group.Metadata.Labels.Dash0Comorigin != nil && *group.Metadata.Labels.Dash0Comorigin != "" {
			return *group.Metadata.Labels.Dash0Comorigin
		}
		if group.Metadata.Labels.Dash0Comid != nil && *group.Metadata.Labels.Dash0Comid != "" {
			return *group.Metadata.Labels.Dash0Comid
		}
	}
	return ""
}

// ExtractRecordingRuleGroupName extracts the display name from a recording
// rule group definition, falling back to metadata.name if no display name is set.
func ExtractRecordingRuleGroupName(group *dash0api.RecordingRuleGroupDefinition) string {
	if group.Spec.Display.Name != "" {
		return group.Spec.Display.Name
	}
	return group.Metadata.Name
}
