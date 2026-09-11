package asset

import (
	"fmt"
	"net/http"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTimeSeriesAggregationOrigin(t *testing.T) {
	origin := "http-server-request-duration"

	tests := []struct {
		name        string
		aggregation *dash0api.TimeSeriesAggregationDefinition
		want        string
	}{
		{
			name: "origin set",
			aggregation: &dash0api.TimeSeriesAggregationDefinition{
				Metadata: dash0api.TimeSeriesAggregationMetadata{
					Labels: &dash0api.TimeSeriesAggregationLabels{Dash0Comorigin: &origin},
				},
			},
			want: origin,
		},
		{
			name: "labels present, origin absent",
			aggregation: &dash0api.TimeSeriesAggregationDefinition{
				Metadata: dash0api.TimeSeriesAggregationMetadata{
					Labels: &dash0api.TimeSeriesAggregationLabels{},
				},
			},
			want: "",
		},
		{
			name:        "no labels at all",
			aggregation: &dash0api.TimeSeriesAggregationDefinition{},
			want:        "",
		},
		{
			name:        "nil aggregation",
			aggregation: nil,
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetTimeSeriesAggregationOrigin(tt.aggregation))
		})
	}
}

// StripTimeSeriesAggregationServerFields clears the origin label, so reading it
// afterwards yields "" and would fail every apply on a missing origin.
func TestGetTimeSeriesAggregationOrigin_MustBeReadBeforeStrip(t *testing.T) {
	origin := "http-server-request-duration"
	aggregation := &dash0api.TimeSeriesAggregationDefinition{
		Metadata: dash0api.TimeSeriesAggregationMetadata{
			Labels: &dash0api.TimeSeriesAggregationLabels{Dash0Comorigin: &origin},
		},
	}

	require.Equal(t, origin, GetTimeSeriesAggregationOrigin(aggregation))

	dash0api.StripTimeSeriesAggregationServerFields(aggregation)

	assert.Empty(t, GetTimeSeriesAggregationOrigin(aggregation),
		"Strip clears the origin label; the import helper must read it first")
}

func TestIsTimeSeriesAggregationWrongDataset(t *testing.T) {
	wrongDataset := &dash0api.APIError{
		StatusCode: http.StatusBadRequest,
		Message:    "Bad Request: A time series aggregation with this origin exists, but it is associated with a different dataset.",
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "the cross-dataset 400", err: wrongDataset, want: true},
		{
			name: "wrapped cross-dataset 400",
			err:  fmt.Errorf("apply failed: %w", wrongDataset),
			want: true,
		},
		{
			name: "message only in the raw body",
			err: &dash0api.APIError{
				StatusCode: http.StatusBadRequest,
				Body:       `{"message":"A time series aggregation with this origin exists, but it is associated with a different dataset."}`,
			},
			want: true,
		},
		{
			name: "a different 400",
			err: &dash0api.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Bad Request: The time series aggregation origin (dash0.com/origin label) must not be empty.",
			},
			want: false,
		},
		{
			name: "a 404 with the same text",
			err: &dash0api.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "associated with a different dataset",
			},
			want: false,
		},
		{name: "not an APIError", err: fmt.Errorf("connection refused"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsTimeSeriesAggregationWrongDataset(tt.err))
		})
	}
}

func TestWrapTimeSeriesAggregationWrongDataset(t *testing.T) {
	cause := &dash0api.APIError{
		StatusCode: http.StatusBadRequest,
		Message:    "associated with a different dataset",
	}

	err := WrapTimeSeriesAggregationWrongDataset(cause, "my-origin")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"my-origin"`)
	assert.Contains(t, err.Error(), "unique per organization")
	assert.Contains(t, err.Error(), "distinct origin per dataset")
	// The advice must be reachable as a hint, not buried in the message, so
	// agent mode lifts it into the JSON error's own hint field.
	assert.Contains(t, err.Error(), "\nHint:")
	// The error must not invent a replacement origin. The docs teach a naming
	// convention, but a runtime error cannot guess what to call this dataset.
	assert.NotContains(t, err.Error(), "my-origin-staging")
	// The wrapped cause must stay reachable so callers can still classify it.
	assert.True(t, IsTimeSeriesAggregationWrongDataset(err))
}

func TestKindDisplayName_TimeSeriesAggregation(t *testing.T) {
	// Every spelling apply accepts must render the same display name. Without
	// the case, KindDisplayName echoes the raw kind back at the user.
	for _, kind := range []string{
		"Dash0TimeSeriesAggregation",
		"dash0-time-series-aggregation",
		"TimeSeriesAggregation",
	} {
		assert.Equal(t, "Time series aggregation", KindDisplayName(kind), kind)
	}
}

func TestIsValidKind_TimeSeriesAggregation(t *testing.T) {
	assert.True(t, IsValidKind("Dash0TimeSeriesAggregation"))
	assert.True(t, IsValidKind("dash0-time-series-aggregation"))
	assert.False(t, IsValidKind("Dash0TimeSeriesAggregations"))
}

func TestExtractIdentifier_TimeSeriesAggregation(t *testing.T) {
	// The API upserts by origin, so an id-only document has no key that
	// matches a live aggregation.
	identifier, err := ExtractIdentifier([]byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0TimeSeriesAggregation
metadata:
  name: my-aggregation
  labels:
    dash0.com/origin: my-origin
    dash0.com/id: d54caa75-e94b-43c7-8470-23e4ab852ab6
`))
	require.NoError(t, err)
	assert.Equal(t, "my-origin", identifier)
}

func TestExtractIdentifier_TimeSeriesAggregationWithoutOrigin(t *testing.T) {
	// Returning "" routes the document to --since's no-identifier hard-fail
	// instead of a delete call against an id the server never keys on.
	identifier, err := ExtractIdentifier([]byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0TimeSeriesAggregation
metadata:
  name: my-aggregation
  labels:
    dash0.com/id: d54caa75-e94b-43c7-8470-23e4ab852ab6
`))
	require.NoError(t, err)
	assert.Empty(t, identifier)
}
