package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// StripSyntheticCheckServerFields removes server-generated fields from a
// synthetic check definition. Used by both Import (to avoid sending rejected
// fields to the API) and diff rendering (to suppress noise).
func StripSyntheticCheckServerFields(c *dash0api.SyntheticCheckDefinition) {
	c.Metadata.Annotations = nil
	if c.Metadata.Labels == nil {
		c.Metadata.Labels = &dash0api.SyntheticCheckLabels{}
	}
	c.Metadata.Labels.Dash0Comversion = nil
	c.Metadata.Labels.Custom = nil
	c.Metadata.Labels.Dash0Comdataset = nil
	c.Metadata.Labels.Dash0Comorigin = nil
	c.Spec.Permissions = nil
}

// ImportSyntheticCheck creates or updates a synthetic check via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// check already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportSyntheticCheck(ctx context.Context, apiClient dash0api.Client, check *dash0api.SyntheticCheckDefinition, dataset *string) (ImportResult, error) {
	StripSyntheticCheckServerFields(check)

	action := ActionCreated
	var before any
	id := ""
	if check.Metadata.Labels.Dash0Comid != nil && *check.Metadata.Labels.Dash0Comid != "" {
		id = *check.Metadata.Labels.Dash0Comid
		existing, err := apiClient.GetSyntheticCheck(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.SyntheticCheckDefinition
	var err error
	if id != "" {
		result, err = apiClient.UpdateSyntheticCheck(ctx, id, check, dataset)
	} else {
		result, err = apiClient.CreateSyntheticCheck(ctx, check, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	if result.Metadata.Labels != nil && result.Metadata.Labels.Dash0Comid != nil {
		id = *result.Metadata.Labels.Dash0Comid
	}
	return ImportResult{Name: ExtractSyntheticCheckName(result), ID: id, Action: action, Before: before, After: result}, nil
}

// ExtractSyntheticCheckID extracts the ID from a synthetic check definition.
func ExtractSyntheticCheckID(check *dash0api.SyntheticCheckDefinition) string {
	if check.Metadata.Labels != nil && check.Metadata.Labels.Dash0Comid != nil {
		return *check.Metadata.Labels.Dash0Comid
	}
	return ""
}

// ExtractSyntheticCheckName extracts the display name from a synthetic check
// definition, falling back to metadata.name if no display name is set.
func ExtractSyntheticCheckName(check *dash0api.SyntheticCheckDefinition) string {
	if check.Spec.Display != nil && check.Spec.Display.Name != "" {
		return check.Spec.Display.Name
	}
	return check.Metadata.Name
}
