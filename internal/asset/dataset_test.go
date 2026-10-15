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
			ApiVersion: DatasetAPIVersion,
			Kind:       "Dash0Dataset",
			Metadata:   dash0api.DatasetMetadata{Name: "P", Labels: &dash0api.DatasetLabels{Dash0Comid: "p"}},
			Spec:       dash0api.DatasetSpec{Permissions: &perms},
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
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: "prod"}},
		Spec:       dash0api.DatasetSpec{Permissions: &[]dash0api.DatasetPermission{{Role: strPtr("admin")}}},
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

func TestValidateDatasetAPIVersion(t *testing.T) {
	assert.NoError(t, ValidateDatasetAPIVersion(&dash0api.DatasetDefinition{ApiVersion: "dash0.com/v1alpha1"}))
	assert.EqualError(t, ValidateDatasetAPIVersion(&dash0api.DatasetDefinition{}),
		"dataset definition requires apiVersion: dash0.com/v1alpha1")
	assert.EqualError(t, ValidateDatasetAPIVersion(&dash0api.DatasetDefinition{ApiVersion: "v1"}),
		`unsupported dataset apiVersion "v1" (supported: dash0.com/v1alpha1)`)
}

// datasetRecorder records what ImportDataset sends.
type datasetRecorder struct {
	dash0api.Client
	created *dash0api.DatasetDefinition
}

func (datasetRecorder) GetDataset(context.Context, string) (*dash0api.DatasetDefinition, error) {
	return nil, &dash0api.APIError{StatusCode: 404}
}

func (r *datasetRecorder) CreateDataset(_ context.Context, d *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	r.created = d
	return d, nil
}

func TestImportDataset_APIVersionRejectedBeforeAnyCall(t *testing.T) {
	for _, v := range []string{"", "dash0.com/v2"} {
		d := &dash0api.DatasetDefinition{
			ApiVersion: v,
			Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: "prod"}},
		}
		// A nil embedded Client panics on any call, proving none is made.
		_, err := ImportDataset(context.Background(), &datasetRecorder{}, d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "apiVersion")
	}
}

func TestImportDataset_SendsNewSpecFields(t *testing.T) {
	logs, traces := dash0api.N90d, dash0api.N60d
	anonymize := dash0api.IpAddressStorageStrategyAnonymize
	upToCountry := dash0api.GeoLocationStorageStrategyUpToCountry
	spanEvents := true
	d := &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: "prod"}},
		Spec: dash0api.DatasetSpec{
			Retention: &dash0api.DatasetRetention{Logs: &logs, Traces: &traces},
			Privacy: &dash0api.DatasetPrivacy{
				IpAddresses: &dash0api.DatasetIpAddressPrivacy{EndUser: &dash0api.DatasetIpAddressStorage{Storage: &anonymize}},
				GeoLocation: &dash0api.DatasetGeoLocationPrivacy{EndUser: &dash0api.DatasetGeoLocationStorage{Storage: &upToCountry}},
			},
			Processing: &dash0api.DatasetProcessing{SpanEventsToLogs: &spanEvents},
		},
	}
	rec := &datasetRecorder{}

	result, err := ImportDataset(context.Background(), rec, d)

	require.NoError(t, err)
	assert.Equal(t, ActionCreated, result.Action)
	require.NotNil(t, rec.created)
	assert.Equal(t, DatasetAPIVersion, rec.created.ApiVersion)
	assert.Equal(t, dash0api.N90d, *rec.created.Spec.Retention.Logs)
	assert.Equal(t, dash0api.N60d, *rec.created.Spec.Retention.Traces)
	assert.Equal(t, anonymize, *rec.created.Spec.Privacy.IpAddresses.EndUser.Storage)
	assert.Equal(t, upToCountry, *rec.created.Spec.Privacy.GeoLocation.EndUser.Storage)
	assert.True(t, *rec.created.Spec.Processing.SpanEventsToLogs)
}
