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

// ImportDataset creates or updates a dataset by the routing rule, with the
// slug as its id. A PUT creates a dataset that does not exist, and restores
// one deleted within the last 30 days, so a dataset is never created by POST.
func ImportDataset(ctx context.Context, apiClient dash0api.Client, dataset *dash0api.DatasetDefinition) (ImportResult, error) {
	plan, err := PlanDataset(ctx, apiClient, dataset)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setDatasetOrigin(dataset, "")
	}
	result, err := apiClient.UpdateDataset(ctx, plan.Key, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetDatasetName(result), ID: dash0api.GetDatasetID(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanDataset routes the dataset with RouteDocument and reads the stored
// dataset that ImportDataset would write. Key is empty when the document is
// invalid or the slug read failed, because the write has no target.
func PlanDataset(ctx context.Context, apiClient dash0api.Client, dataset *dash0api.DatasetDefinition) (ImportPlan, error) {
	if err := ValidateDataset(dataset); err != nil {
		return ImportPlan{}, err
	}
	identity := DatasetIdentity(dataset)
	dash0api.StripDatasetServerFields(dataset)
	route, err := RouteDocument(ctx, identity, apiClient.GetDataset, dash0api.GetDatasetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: dataset}
	if route.Key == "" || route.Create {
		setDatasetOrigin(dataset, route.Key)
		return plan, err
	}
	// The slug cannot change after creation, so a hit by origin with another
	// slug means a renamed slug or a copied origin.
	if stored := dash0api.GetDatasetID(route.Stored); stored != identity.ID {
		return ImportPlan{}, &DatasetSlugChangeError{Origin: route.Key, Stored: stored, Document: identity.ID}
	}
	// The stored origin keeps the diff to what changes; ImportDataset clears
	// it before the write.
	setDatasetOrigin(dataset, dash0api.GetDatasetOrigin(route.Stored))
	plan.Action, plan.Before = ActionUpdated, route.Stored
	return plan, nil
}

// DatasetIdentity returns what RouteDocument routes a dataset by. The slug is
// the id, and the inferred origin hashes it in place of metadata.name, because
// the slug is required and names the dataset for good.
func DatasetIdentity(dataset *dash0api.DatasetDefinition) Identity {
	slug := dash0api.GetDatasetID(dataset)
	return Identity{ID: slug, Origin: dash0api.GetDatasetOrigin(dataset), Inferred: InferOrigin("dataset", slug, "")}
}

// DatasetSlugChangeError reports a document whose origin belongs to a dataset
// with another slug.
type DatasetSlugChangeError struct {
	Origin   string
	Stored   string
	Document string
}

func (e *DatasetSlugChangeError) Error() string {
	return fmt.Sprintf("origin %q belongs to dataset %q, and a dataset slug cannot change; set the slug back to %q, or remove dash0.com/origin to create dataset %q", e.Origin, e.Stored, e.Stored, e.Document)
}

func setDatasetOrigin(dataset *dash0api.DatasetDefinition, origin string) {
	if dataset.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		dataset.Metadata.Labels = &dash0api.DatasetLabels{}
	}
	dataset.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}
