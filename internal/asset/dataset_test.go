package asset

import (
	"bytes"
	"context"
	"errors"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryFiltersWarning(t *testing.T) {
	assert.Empty(t, TelemetryFiltersWarning(nil))
	assert.Empty(t, TelemetryFiltersWarning(&dash0api.DatasetDefinition{}))
	assert.Empty(t, TelemetryFiltersWarning(&dash0api.DatasetDefinition{Spec: dash0api.DatasetSpec{TelemetryFilters: &[]dash0api.TelemetryFilter{}}}))

	d := &dash0api.DatasetDefinition{Spec: dash0api.DatasetSpec{TelemetryFilters: &[]dash0api.TelemetryFilter{{}}}}
	assert.Equal(t, "spec.telemetryFilters is not managed by the CLI and is ignored on write: "+
		"the Dash0 CLI strips it so the stored filters stay unchanged. Change filters in the Dash0 UI.", TelemetryFiltersWarning(d))
}

func TestSortDatasetPermissions(t *testing.T) {
	d := &dash0api.DatasetDefinition{Spec: dash0api.DatasetSpec{Permissions: &[]dash0api.DatasetPermission{
		{UserId: strPtr("alice@example.com")},
		{TeamId: strPtr("team_1")},
		{Role: strPtr("basic_member")},
		{Role: strPtr("admin")},
	}}}
	SortDatasetPermissions(d)
	perms := *d.Spec.Permissions
	assert.Equal(t, "admin", *perms[0].Role)
	assert.Equal(t, "basic_member", *perms[1].Role)
	assert.Equal(t, "team_1", *perms[2].TeamId)
	assert.Equal(t, "alice@example.com", *perms[3].UserId)

	SortDatasetPermissions(nil)
	SortDatasetPermissions(&dash0api.DatasetDefinition{})
}

func TestPrintDiff_Dataset_PermissionOrderIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	build := func(perms ...dash0api.DatasetPermission) *dash0api.DatasetDefinition {
		return &dash0api.DatasetDefinition{
			Kind:     "Dash0Dataset",
			Metadata: dash0api.DatasetMetadata{Name: "P", Labels: &dash0api.DatasetLabels{Dash0Comid: "p"}},
			Spec:     dash0api.DatasetSpec{Permissions: &perms},
		}
	}
	admin := dash0api.DatasetPermission{Role: strPtr("admin")}
	team := dash0api.DatasetPermission{TeamId: strPtr("team_1")}

	var buf bytes.Buffer
	require.NoError(t, PrintDiff(&buf, "Dataset", "P", build(admin, team), build(team, admin)))
	assert.Contains(t, buf.String(), "no changes")
}

// datasetStub overrides only the dataset calls ImportDataset makes.
type datasetStub struct {
	dash0api.Client
	updateErr error
}

func (datasetStub) GetDataset(context.Context, string) (*dash0api.DatasetDefinition, error) {
	return nil, &dash0api.APIError{StatusCode: 404}
}

func (datasetStub) CreateDataset(_ context.Context, d *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	return d, nil
}

func (s datasetStub) UpdateDataset(context.Context, string, *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	return nil, s.updateErr
}

func TestImportDataset_PermissionsUpdateFailureReturnsCreatedDataset(t *testing.T) {
	d := &dash0api.DatasetDefinition{
		Metadata: dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: "prod"}},
		Spec:     dash0api.DatasetSpec{Permissions: &[]dash0api.DatasetPermission{{Role: strPtr("admin")}}},
	}
	boom := errors.New("boom")

	result, err := ImportDataset(context.Background(), datasetStub{updateErr: boom}, d)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, ActionCreated, result.Action)
	assert.Equal(t, "prod", result.ID)
	assert.Equal(t, "Prod", result.Name)
	assert.NotNil(t, result.After)
	assert.Contains(t, DatasetPermissionsNotApplied(err).Error(), "rerun the same command")
}
