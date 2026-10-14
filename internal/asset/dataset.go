package asset

import (
	"context"
	"errors"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ErrDatasetSlugRequired is returned when a dataset definition carries no dash0.com/id label.
// The slug is the dataset's only identifier: it is the path id and cannot be assigned by the server.
var ErrDatasetSlugRequired = errors.New(`dataset definition requires metadata.labels["dash0.com/id"] (the dataset slug)`)

// ImportDataset creates or updates a dataset, keyed by its slug (the dash0.com/id label).
//
// A preflight GET decides the route: on hit, PUT updates in place; on a genuine 404, POST creates.
// PUT is not used for the miss path because the server upserts on PUT for most tokens, which would turn
// a mistyped slug into a new dataset.
// A dataset deleted within the last 30 days answers 404, so the POST restores it.
// Any other preflight error surfaces instead of creating a duplicate.
//
// The server ignores spec.permissions on create, so a create that carries permissions is followed by an update.
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
			result, err = apiClient.UpdateDataset(ctx, slug, dataset)
		}
	}
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{Name: dash0api.GetDatasetName(result), ID: dash0api.GetDatasetID(result), Action: action, Before: before, After: result}, nil
}
