package asset

import (
	"bytes"
	"context"
	"testing"
	"time"

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
			Metadata:   dash0api.DatasetMetadata{Name: "P", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("p")}},
			Spec:       dash0api.DatasetSpec{Permissions: &perms},
		}
	}
	admin := dash0api.DatasetPermission{Role: strPtr("admin")}
	team := dash0api.DatasetPermission{TeamId: strPtr("team_1")}

	var buf bytes.Buffer
	require.NoError(t, PrintDiff(&buf, "Dataset", "P", build(admin, team), build(team, admin)))
	assert.Contains(t, buf.String(), "no changes")
}

func TestValidateDatasetAPIVersion(t *testing.T) {
	assert.NoError(t, ValidateDatasetAPIVersion(&dash0api.DatasetDefinition{ApiVersion: "dash0.com/v1alpha1"}))
	assert.EqualError(t, ValidateDatasetAPIVersion(&dash0api.DatasetDefinition{}),
		"dataset definition requires apiVersion: dash0.com/v1alpha1")
	assert.EqualError(t, ValidateDatasetAPIVersion(&dash0api.DatasetDefinition{ApiVersion: "v1"}),
		`unsupported dataset apiVersion "v1" (supported: dash0.com/v1alpha1)`)
}

// datasetRecorder records what ImportDataset writes when nothing is stored.
type datasetRecorder struct {
	dash0api.Client
	key     string
	created *dash0api.DatasetDefinition
}

func (datasetRecorder) GetDataset(context.Context, string) (*dash0api.DatasetDefinition, error) {
	return nil, &dash0api.APIError{StatusCode: 404}
}

func (r *datasetRecorder) UpdateDataset(_ context.Context, key string, d *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	r.key, r.created = key, d
	return d, nil
}

func TestImportDataset_APIVersionRejectedBeforeAnyCall(t *testing.T) {
	for _, v := range []string{"", "dash0.com/v2"} {
		d := &dash0api.DatasetDefinition{
			ApiVersion: v,
			Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("prod")}},
		}
		rec := &datasetRecorder{}
		_, err := ImportDataset(context.Background(), rec, d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "apiVersion")
		assert.Nil(t, rec.created, "no create call may reach the API")
	}
}

func TestImportDataset_SendsNewSpecFields(t *testing.T) {
	logs, traces := dash0api.RetentionClassN90d, dash0api.RetentionClassN60d
	anonymize := dash0api.IpAddressStorageStrategyAnonymize
	upToCountry := dash0api.GeoLocationStorageStrategyUpToCountry
	spanEvents := true
	d := &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("prod")}},
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
	inferred := InferOrigin("dataset", "prod", "")
	assert.Equal(t, inferred, rec.key, "a dataset that does not exist is created by its origin")
	assert.Equal(t, inferred, dash0api.GetDatasetOrigin(rec.created), "a create sends its origin")
	assert.Equal(t, "prod", dash0api.GetDatasetID(rec.created), "the slug stays in the body")
	assert.Equal(t, DatasetAPIVersion, rec.created.ApiVersion)
	assert.Equal(t, dash0api.RetentionClassN90d, *rec.created.Spec.Retention.Logs)
	assert.Equal(t, dash0api.RetentionClassN60d, *rec.created.Spec.Retention.Traces)
	assert.Equal(t, anonymize, *rec.created.Spec.Privacy.IpAddresses.EndUser.Storage)
	assert.Equal(t, upToCountry, *rec.created.Spec.Privacy.GeoLocation.EndUser.Storage)
	assert.True(t, *rec.created.Spec.Processing.SpanEventsToLogs)
}

// storedDatasetClient serves a stored dataset that cannot be marshaled to
// JSON, so building the diff preview fails, and records the update.
type storedDatasetClient struct {
	dash0api.Client
	updated *dash0api.DatasetDefinition
}

func (storedDatasetClient) GetDataset(context.Context, string) (*dash0api.DatasetDefinition, error) {
	outOfRange := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	return &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata: dash0api.DatasetMetadata{
			Name:        "Prod",
			Labels:      &dash0api.DatasetLabels{Dash0Comid: strPtr("prod")},
			Annotations: &dash0api.DatasetAnnotations{Dash0ComcreatedAt: &outOfRange},
		},
	}, nil
}

func (c *storedDatasetClient) UpdateDataset(_ context.Context, _ string, d *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	c.updated = d
	return d, nil
}

// The merge only builds the diff preview, so an apply must not depend on it.
func TestImportDataset_DoesNotDependOnThePreviewMerge(t *testing.T) {
	stored := &storedDatasetClient{}
	storedDataset, err := stored.GetDataset(context.Background(), "prod")
	require.NoError(t, err)
	_, err = PreviewUpdate[dash0api.DatasetDefinition](storedDataset, &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("prod")}},
	})
	require.Error(t, err, "the stored dataset must break the preview merge, or this test proves nothing")

	result, err := ImportDataset(context.Background(), stored, &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("prod")}},
	})

	require.NoError(t, err)
	assert.Equal(t, ActionUpdated, result.Action)
	require.NotNil(t, stored.updated)
	assert.Empty(t, dash0api.GetDatasetOrigin(stored.updated), "an update never sends the origin")
}

// datasetAtOrigin stores the dataset "prdo" at origin "o" only.
type datasetAtOrigin struct {
	dash0api.Client
	updated bool
}

func (datasetAtOrigin) GetDataset(_ context.Context, key string) (*dash0api.DatasetDefinition, error) {
	if key != "o" {
		return nil, &dash0api.APIError{StatusCode: 404}
	}
	return &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("prdo"), Dash0Comorigin: strPtr("o")}},
	}, nil
}

func (c *datasetAtOrigin) UpdateDataset(_ context.Context, _ string, d *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	c.updated = true
	return d, nil
}

func TestImportDataset_SlugChangeAtTheSameOriginFailsBeforeTheWrite(t *testing.T) {
	client := &datasetAtOrigin{}
	d := &dash0api.DatasetDefinition{
		ApiVersion: DatasetAPIVersion,
		Metadata:   dash0api.DatasetMetadata{Name: "Prod", Labels: &dash0api.DatasetLabels{Dash0Comid: strPtr("prod"), Dash0Comorigin: strPtr("o")}},
	}

	_, err := ImportDataset(context.Background(), client, d)

	var slugChange *DatasetSlugChangeError
	require.ErrorAs(t, err, &slugChange)
	assert.Equal(t, DatasetSlugChangeError{Origin: "o", Stored: "prdo", Document: "prod"}, *slugChange)
	assert.False(t, client.updated, "no write may reach the API")
}
