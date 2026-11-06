package apply

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/asset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyAction_String(t *testing.T) {
	assert.Equal(t, "created", string(actionCreated))
	assert.Equal(t, "updated", string(actionUpdated))
}

func TestValidateDocuments_NotificationChannelAssetsWarning(t *testing.T) {
	yaml := `kind: Dash0NotificationChannel
metadata:
  name: Channel With Assets
spec:
  type: webhook
  config:
    url: https://example.com/webhook
  routing:
    filters: []
    assets:
      - kind: check_rule
        id: 00000000-0000-0000-0000-000000000001
        name: some rule
`
	docs, err := asset.ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)

	validationErrors, validationWarnings := asset.ValidateDocuments(docs)
	assert.Empty(t, validationErrors, "the API-managed assets warning must not become a validation error")
	require.Len(t, validationWarnings, 1)
	assert.Contains(t, validationWarnings[0], "spec.routing.assets is API-managed and ignored on write")
}

func TestValidateDocuments_DatasetMissingSlug(t *testing.T) {
	docs, err := asset.ReadMultiDocumentYAML("-", strings.NewReader("apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: P\nspec: {}\n"))
	require.NoError(t, err)

	validationErrors, _ := asset.ValidateDocuments(docs)
	require.Len(t, validationErrors, 1)
	assert.Contains(t, validationErrors[0], "dash0.com/id")
}

func TestValidateDocuments_AuthTokenRejected(t *testing.T) {
	docs, err := asset.ReadMultiDocumentYAML("-", strings.NewReader("apiVersion: dash0.com/v1alpha1\nkind: Dash0AuthToken\nmetadata:\n  name: ingest\nspec:\n  actions: [\"ingest:*\"]\n"))
	require.NoError(t, err)

	validationErrors, _ := asset.ValidateDocuments(docs)
	require.Len(t, validationErrors, 1)
	assert.Contains(t, validationErrors[0], `apply does not support kind "Dash0AuthToken"`)
	assert.Contains(t, validationErrors[0], "auth-tokens create")
}

func TestValidateDocuments_DatasetAPIVersion(t *testing.T) {
	cases := map[string]struct {
		doc     string
		wantErr string
	}{
		"valid":   {"apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: P\n  labels:\n    dash0.com/id: p\nspec: {}\n", ""},
		"missing": {"kind: Dash0Dataset\nmetadata:\n  name: P\n  labels:\n    dash0.com/id: p\nspec: {}\n", "dataset definition requires apiVersion: dash0.com/v1alpha1"},
		"unknown": {"apiVersion: dash0.com/v2\nkind: Dash0Dataset\nmetadata:\n  name: P\n  labels:\n    dash0.com/id: p\nspec: {}\n", `unsupported dataset apiVersion "dash0.com/v2" (supported: dash0.com/v1alpha1)`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			docs, err := asset.ReadMultiDocumentYAML("-", strings.NewReader(c.doc))
			require.NoError(t, err)

			validationErrors, _ := asset.ValidateDocuments(docs)
			if c.wantErr == "" {
				assert.Empty(t, validationErrors)
				return
			}
			require.Len(t, validationErrors, 1)
			assert.Contains(t, validationErrors[0], c.wantErr)
		})
	}
}

func TestDatasetsFirst(t *testing.T) {
	docs := []asset.Document{
		{Kind: "View", ID: "v1"},
		{Kind: "Dash0Dataset", ID: "d1"},
		{Kind: "Dashboard", ID: "b1"},
		{Kind: "Dash0Dataset", ID: "d2"},
	}

	got := datasetsFirst(docs)

	var ids []string
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	assert.Equal(t, []string{"d1", "d2", "v1", "b1"}, ids)
	assert.Equal(t, "v1", docs[0].ID, "the input must not be reordered")
}

func TestValidateDocuments_CheckRuleIDCollisions(t *testing.T) {
	const (
		checkRuleA = "kind: CheckRule\nid: shared-id\nname: Rule A\nexpression: up == 0\n"
		checkRuleB = "kind: CheckRule\nid: shared-id\nname: Rule B\nexpression: up == 1\n"
	)
	singleAlertPrometheusRule := `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: rules
  labels:
    dash0.com/id: shared-id
spec:
  groups:
    - name: group
      rules:
        - alert: Alert A
          expr: up == 0
`
	multiAlertPrometheusRule := singleAlertPrometheusRule + `        - alert: Alert B
          expr: up == 1
`
	collision := func(uses ...string) []string {
		return []string{`check rule id "shared-id" is used more than once, so only the last one would be kept:` + "\n    " + strings.Join(uses, "\n    ")}
	}

	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "two check rules in one file",
			files: map[string]string{"rules.yaml": checkRuleA + "---\n" + checkRuleB},
			want:  collision(`rules.yaml: document 1: "Rule A"`, `rules.yaml: document 2: "Rule B"`),
		},
		{
			name:  "check rules in two files",
			files: map[string]string{"a.yaml": checkRuleA, "b.yaml": checkRuleB},
			want:  collision(`a.yaml: "Rule A"`, `b.yaml: "Rule B"`),
		},
		{
			name:  "single-alert PrometheusRule and check rule",
			files: map[string]string{"a.yaml": singleAlertPrometheusRule, "b.yaml": checkRuleB},
			want:  collision(`a.yaml: "group - Alert A"`, `b.yaml: "Rule B"`),
		},
		{
			name:  "multi-alert PrometheusRule and check rule with its id",
			files: map[string]string{"a.yaml": multiAlertPrometheusRule, "b.yaml": checkRuleB},
		},
		{
			name:  "distinct ids",
			files: map[string]string{"a.yaml": checkRuleA, "b.yaml": strings.Replace(checkRuleB, "shared-id", "other-id", 1)},
		},
		{
			name: "check rules without id",
			files: map[string]string{
				"a.yaml": "kind: CheckRule\nname: Rule A\nexpression: up == 0\n",
				"b.yaml": "kind: CheckRule\nname: Rule B\nexpression: up == 1\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
			}
			docs, err := asset.ReadDirectory(dir)
			require.NoError(t, err)

			validationErrors, _ := asset.ValidateDocuments(docs)
			assert.Equal(t, tt.want, validationErrors)
		})
	}
}

// Every kind in asset.ValidKinds must have an applyDocument case, or apply
// accepts a document in validation and then fails it with "unsupported kind".
// The document does not parse, so no case reaches the API.
func TestApplyDocument_AppliesEveryValidKind(t *testing.T) {
	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			_, err := applyDocument(context.Background(), nil, asset.Document{Kind: kind, Raw: []byte("{")}, nil)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "unsupported kind")
		})
	}
}
