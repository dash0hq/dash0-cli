package asset

import (
	"context"
	"errors"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ErrDatasetSlugRequired is returned when a dataset definition carries no dash0.com/id label.
// The slug is the dataset's only identifier: it is the path id and cannot be assigned by the server.
var ErrDatasetSlugRequired = errors.New(`dataset definition requires metadata.labels["dash0.com/id"] (the dataset slug)`)

// TelemetryFiltersWarning returns a user-facing warning when the dataset definition carries a
// non-empty spec.telemetryFilters, and an empty string otherwise.
// StripDatasetServerFields clears the field before every write, so the stored filters stay unchanged.
// Call it before ImportDataset, which strips the field.
func TelemetryFiltersWarning(dataset *dash0api.DatasetDefinition) string {
	if dataset == nil || dataset.Spec.TelemetryFilters == nil || len(*dataset.Spec.TelemetryFilters) == 0 {
		return ""
	}
	return "spec.telemetryFilters is not managed by the CLI and is ignored on write: " +
		"the Dash0 CLI strips it so the stored filters stay unchanged. Change filters in the Dash0 UI."
}

// DatasetPermissionsNotApplied wraps err for a dataset that was created but whose follow-up
// spec.permissions update failed, so the user knows how to recover.
func DatasetPermissionsNotApplied(err error) error {
	return fmt.Errorf("%w\nHint: the dataset was created but spec.permissions were not applied; rerun the same command to apply them", err)
}

// ImportDataset creates or updates a dataset, keyed by its slug (the dash0.com/id label).
//
// A preflight GET decides the route: on hit, PUT updates in place; on a genuine 404, POST creates.
// PUT is not used for the miss path because the server upserts on PUT for most tokens, which would turn
// a mistyped slug into a new dataset.
// A dataset deleted within the last 30 days answers 404, so the POST restores it.
// Any other preflight error surfaces instead of creating a duplicate.
//
// The server ignores spec.permissions on create, so a create that carries permissions is followed by an update.
// If that update fails, the dataset exists without its permissions: ImportDataset then returns the created
// dataset (Action created, After set) together with the error, so callers can report it before failing.
func ImportDataset(ctx context.Context, apiClient dash0api.Client, dataset *dash0api.DatasetDefinition) (ImportResult, error) {
	slug := dash0api.GetDatasetID(dataset)
	if slug == "" {
		return ImportResult{}, ErrDatasetSlugRequired
	}
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
		if err == nil && dataset.Spec.Permissions != nil && len(*dataset.Spec.Permissions) > 0 {
			created := result
			result, err = apiClient.UpdateDataset(ctx, slug, dataset)
			if err != nil {
				return ImportResult{Name: dash0api.GetDatasetName(created), ID: dash0api.GetDatasetID(created), Action: ActionCreated, After: created}, err
			}
		}
	}
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{Name: dash0api.GetDatasetName(result), ID: dash0api.GetDatasetID(result), Action: action, Before: before, After: result}, nil
}
