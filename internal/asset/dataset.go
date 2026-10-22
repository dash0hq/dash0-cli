package asset

import (
	"context"
	"errors"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ErrDatasetSlugRequired is returned when a dataset definition carries neither spec.slug nor a dash0.com/id label.
// The slug is the dataset's only identifier: it is the path id and cannot be assigned by the server.
var ErrDatasetSlugRequired = errors.New(`dataset definition requires a slug in spec.slug or metadata.labels["dash0.com/id"]`)

// DatasetSoftDeleteNote says what a dataset delete keeps and removes. Every prompt, hint and help
// text about deleting a dataset uses it, so the wording cannot drift.
const DatasetSoftDeleteNote = "Dashboards, check rules, synthetic checks and views are kept for 30 days; " +
	"creating the same slug within that time restores them. Telemetry is removed as its retention expires."

// DatasetAPIVersion is the only dataset document format the API accepts.
const DatasetAPIVersion = "dash0.com/v1alpha1"

// ValidateDatasetAPIVersion rejects a dataset definition whose apiVersion is missing or unknown,
// so a typo fails before any API call instead of as a server 400.
func ValidateDatasetAPIVersion(dataset *dash0api.DatasetDefinition) error {
	switch {
	case dataset.ApiVersion == "":
		return fmt.Errorf("dataset definition requires apiVersion: %s", DatasetAPIVersion)
	case dataset.ApiVersion != DatasetAPIVersion:
		return fmt.Errorf("unsupported dataset apiVersion %q (supported: %s)", dataset.ApiVersion, DatasetAPIVersion)
	}
	return nil
}

// ValidateDataset checks what every dataset entry point needs before an API call: a slug and a known apiVersion.
func ValidateDataset(dataset *dash0api.DatasetDefinition) error {
	if dash0api.GetDatasetID(dataset) == "" {
		return ErrDatasetSlugRequired
	}
	return ValidateDatasetAPIVersion(dataset)
}

// DatasetOrigin returns the dash0.com/origin label of a dataset, or "" when it has none.
func DatasetOrigin(dataset *dash0api.DatasetDefinition) string {
	if l := dataset.Metadata.Labels; l != nil && l.Dash0Comorigin != nil {
		return *l.Dash0Comorigin
	}
	return ""
}

// ImportDataset creates or updates a dataset, keyed by its slug (the dash0.com/id label).
//
// A preflight GET decides the route: on hit, PUT updates in place; on a genuine 404, POST creates.
// PUT is not used for the miss path because the server upserts on PUT for most tokens, so POST is the
// explicit create. A mistyped slug therefore creates a new dataset here; only `datasets update`
// refuses an unknown slug.
// A dataset deleted within the last 30 days answers 404, so the POST restores it.
// Any other preflight error surfaces instead of creating a duplicate.
func ImportDataset(ctx context.Context, apiClient dash0api.Client, dataset *dash0api.DatasetDefinition) (ImportResult, error) {
	if err := ValidateDataset(dataset); err != nil {
		return ImportResult{}, err
	}
	slug := dash0api.GetDatasetID(dataset)
	dash0api.StripDatasetServerFields(dataset)

	existing, err := apiClient.GetDataset(ctx, slug)
	var result *dash0api.DatasetDefinition
	action := ActionCreated
	var before any
	switch {
	case err == nil:
		action = ActionUpdated
		before = existing
		result, err = apiClient.UpdateDataset(ctx, slug, dataset)
	case dash0api.IsNotFound(err):
		result, err = apiClient.CreateDataset(ctx, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{Name: dash0api.GetDatasetName(result), ID: dash0api.GetDatasetID(result), Action: action, Before: before, After: result}, nil
}
