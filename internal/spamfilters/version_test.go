package spamfilters

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectAPIVersion(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    string
		wantErr string // substring of expected error message; empty means no error
	}{
		{
			name: "explicit v1alpha1",
			data: "apiVersion: v1alpha1\nkind: Dash0SpamFilter\n",
			want: string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1),
		},
		{
			name: "explicit v1alpha2",
			data: "apiVersion: v1alpha2\nkind: Dash0SpamFilter\n",
			want: string(dash0api.V1alpha2),
		},
		{
			name: "missing apiVersion defaults to v1alpha1",
			data: "kind: Dash0SpamFilter\nmetadata:\n  name: foo\n",
			want: string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1),
		},
		{
			name: "JSON input is accepted",
			data: `{"apiVersion":"v1alpha2","kind":"Dash0SpamFilter"}`,
			want: string(dash0api.V1alpha2),
		},
		{
			name:    "unknown apiVersion is rejected with the list of supported values",
			data:    "apiVersion: v9999\nkind: Dash0SpamFilter\n",
			wantErr: `unsupported spam filter apiVersion "v9999" (supported: "v1alpha1", "v1alpha2")`,
		},
		{
			name:    "malformed YAML surfaces the underlying parser error",
			data:    "apiVersion: : :\n",
			wantErr: "failed to detect spam filter apiVersion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectAPIVersion([]byte(tt.data))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestObjectAccessors_V1Alpha1(t *testing.T) {
	id := "sf-123"
	origin := "my-origin"
	dataset := "default"

	v1 := dash0api.SpamFilterApiVersionV1Alpha1V1alpha1
	obj := &dash0api.SpamFilter{
		ApiVersion: &v1,
		Kind:       dash0api.SpamFilterDefinitionKindDash0SpamFilter,
		Metadata: dash0api.SpamFilterMetadata{
			Name: "drop noisy",
			Labels: &dash0api.SpamFilterLabels{
				Dash0Comid:      &id,
				Dash0Comorigin:  &origin,
				Dash0Comdataset: &dataset,
			},
		},
		Spec: dash0api.SpamFilterSpec{
			Contexts: []dash0api.TelemetryFilterContext{
				dash0api.TelemetryFilterContextLog,
				dash0api.TelemetryFilterContextSpan,
			},
			Filter: dash0api.FilterCriteria{
				dash0api.AttributeFilter{Key: "k8s.namespace.name", Operator: "is"},
			},
		},
	}

	assert.Equal(t, string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1), objectAPIVersion(obj))
	assert.Equal(t, "Dash0SpamFilter", objectKind(obj))
	assert.Equal(t, "drop noisy", objectName(obj))
	assert.Equal(t, "sf-123", objectID(obj))
	assert.Equal(t, "my-origin", objectOrigin(obj))
	assert.Equal(t, "default", objectDataset(obj))
	assert.Equal(t, 1, objectFilterCount(obj))
}

func TestObjectAccessors_V1Alpha2(t *testing.T) {
	id := "sf-456"

	obj := &dash0api.SpamFilterV1Alpha2{
		ApiVersion: dash0api.V1alpha2,
		Kind:       dash0api.SpamFilterDefinitionV1Alpha2KindDash0SpamFilter,
		Metadata: dash0api.SpamFilterMetadata{
			Name:   "drop noisy v2",
			Labels: &dash0api.SpamFilterLabels{Dash0Comid: &id},
		},
		Spec: dash0api.SpamFilterSpecV1Alpha2{
			Context: dash0api.TelemetryFilterContextLog,
			Filter: dash0api.FilterCriteria{
				dash0api.AttributeFilter{Key: "service.name", Operator: "is"},
				dash0api.AttributeFilter{Key: "http.target", Operator: "ends_with"},
			},
		},
	}

	assert.Equal(t, string(dash0api.V1alpha2), objectAPIVersion(obj))
	assert.Equal(t, "Dash0SpamFilter", objectKind(obj))
	assert.Equal(t, "drop noisy v2", objectName(obj))
	assert.Equal(t, "sf-456", objectID(obj))
	assert.Equal(t, "", objectOrigin(obj))
	assert.Equal(t, 2, objectFilterCount(obj))
}

func TestObjectAPIVersion_V1Alpha1_DefaultsWhenAbsent(t *testing.T) {
	// On the wire, v1alpha1 documents may omit apiVersion. The accessor
	// should report v1alpha1 (the type's own version) instead of "" so the
	// caller can render a useful string without special-casing.
	obj := &dash0api.SpamFilter{}
	assert.Equal(t, string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1), objectAPIVersion(obj))
}
