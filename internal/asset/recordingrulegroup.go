package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// StripRecordingRuleGroupServerFields removes server-generated fields from a
// recording rule definition. Used by both Import (to avoid sending
// rejected fields to the API) and diff rendering (to suppress noise).
//
// Fields NOT stripped (user-specified, round-trippable):
//   - dash0.com/origin — external identifier set by Terraform/operator/user
//   - dash0.com/dataset — dataset routing, user-supplied
//   - annotations.dash0.com/folder-path — optional UI folder path
//   - annotations.dash0.com/sharing — optional sharing config
func StripRecordingRuleGroupServerFields(g *dash0api.RecordingRuleGroupDefinition) {
	if g.Metadata.Labels != nil {
		g.Metadata.Labels.Dash0Comid = nil
		g.Metadata.Labels.Dash0Comversion = nil
		g.Metadata.Labels.Dash0Comsource = nil
	}
	if g.Metadata.Annotations != nil {
		g.Metadata.Annotations.Dash0ComcreatedAt = nil
		g.Metadata.Annotations.Dash0ComdeletedAt = nil
		g.Metadata.Annotations.Dash0ComupdatedAt = nil
	}
	g.Spec.Permissions = nil
	g.Spec.PermittedActions = nil
}

// InjectRecordingRuleGroupDataset injects the dataset into metadata.labels, which
// is how Create and Update receive the dataset (no query param for these endpoints).
// If dataset is nil or empty, the existing value in the file is left unchanged.
func InjectRecordingRuleGroupDataset(group *dash0api.RecordingRuleGroupDefinition, dataset *string) {
	if dataset == nil || *dataset == "" {
		return
	}
	if group.Metadata.Labels == nil {
		group.Metadata.Labels = &dash0api.RecordingRuleGroupLabels{}
	}
	group.Metadata.Labels.Dash0Comdataset = dataset
}

// InjectRecordingRuleGroupVersion copies the dash0.com/version label from source
// into group before an update call, to satisfy the API's optimistic concurrency check.
func InjectRecordingRuleGroupVersion(group, source *dash0api.RecordingRuleGroupDefinition) {
	if source.Metadata.Labels == nil || source.Metadata.Labels.Dash0Comversion == nil {
		return
	}
	if group.Metadata.Labels == nil {
		group.Metadata.Labels = &dash0api.RecordingRuleGroupLabels{}
	}
	group.Metadata.Labels.Dash0Comversion = source.Metadata.Labels.Dash0Comversion
}

// ImportRecordingRuleGroup checks existence by origin or ID, strips
// server-generated fields, injects the dataset into the body, and creates or
// updates the recording rule via the standard CRUD APIs.
func ImportRecordingRuleGroup(ctx context.Context, apiClient dash0api.Client, group *dash0api.RecordingRuleGroupDefinition, dataset *string) (ImportResult, error) {
	StripRecordingRuleGroupServerFields(group)

	// Override dataset in body if --dataset flag was provided.
	InjectRecordingRuleGroupDataset(group, dataset)

	action := ActionCreated
	var before any
	id := ExtractRecordingRuleGroupID(group)
	if id != "" {
		existing, err := apiClient.GetRecordingRuleGroup(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
			// Inject the current version for optimistic concurrency control.
			InjectRecordingRuleGroupVersion(group, existing)
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
// rule definition. The origin label (set by Terraform/operator) is
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
// rule definition, falling back to metadata.name if no display name is set.
func ExtractRecordingRuleGroupName(group *dash0api.RecordingRuleGroupDefinition) string {
	if group.Spec.Display.Name != "" {
		return group.Spec.Display.Name
	}
	return group.Metadata.Name
}
