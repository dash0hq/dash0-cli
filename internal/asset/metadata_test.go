package asset

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLossyMetadataWarnings_AllKinds(t *testing.T) {
	for _, kind := range append(append([]string{}, ValidKinds...), "authtoken") {
		t.Run(kind, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`kind: %s
metadata:
  name: example
  labels:
    team-lead: alice
    dash0.com/typo: value
  annotations:
    example.com/note: private-value
`, kind))
			original := bytes.Clone(raw)
			if kind == "awsmetricscrapeconfig" {
				// This parser already has a more specific diagnostic.
				assert.Empty(t, LossyMetadataWarnings(raw, ""))
				return
			}
			assert.Equal(t, []string{
				`metadata.annotations["example.com/note"] will not be stored by Dash0`,
				`metadata.labels["dash0.com/typo"] will not be stored by Dash0`,
				`metadata.labels["team-lead"] will not be stored by Dash0`,
			}, LossyMetadataWarnings(raw, ""))
			assert.Equal(t, original, raw)
		})
	}
}

func TestLossyMetadataWarnings_SupportedAndExportedMetadata(t *testing.T) {
	for _, tc := range []struct{ kind, labels, annotations string }{
		{"Dashboard", "dash0.com/source: api", "dash0.com/folder-path: /backend\n    dash0.com/sharing: 'team:team_123'"},
		{"PersesDashboard", "dash0.com/id: dashboard-id", "dash0.com/source: api"},
		{"View", "dash0.com/id: view-id\n    dash0.com/origin: terraform", "dash0.com/sharing: 'team:team_123'"},
		{"SyntheticCheck", "custom: {team-lead: alice}", "dash0.com/folder-path: /backend"},
		{"Dash0SpamFilter", "custom: {team-lead: alice}", "dash0.com/enabled: 'true'"},
		{"SLO", "dash0.com/id: slo_123\n    dash0.com/origin: git", "dash0.com/enabled: 'true'"},
		{"Dash0Team", "dash0.com/id: team_123\n    dash0.com/origin: git\n    dash0.com/source: cli", "{}"},
		{"Dash0NotificationChannel", "dash0.com/origin: git", "{}"},
		{"Dash0Dataset", "dash0.com/id: production\n    dash0.com/version: '1'", "{}"},
		{"Dash0Skill", "dash0.com/origin: git", "dash0.com/creator-name: Alice\n    dash0.com/enabled: 'true'"},
		{"Dash0GenerativeAiEvaluation", "dash0.com/id: eval_123", "dash0.com/created-by: alice\n    dash0.com/sharing: 'team:team_123'"},
		{"Dash0TimeSeriesAggregation", "dash0.com/origin: git\n    custom: {team-lead: alice}", "{}"},
		{"Dash0AuthToken", "dash0.com/origin: git", "{}"},
		{"PrometheusRule", "dash0.com/id: rule-id", "dash0.com/enabled: 'true'"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			raw := fmt.Sprintf("kind: %s\nmetadata:\n  labels:\n    %s\n  annotations: %s\n", tc.kind, tc.labels, tc.annotations)
			if tc.annotations != "{}" {
				raw = fmt.Sprintf("kind: %s\nmetadata:\n  labels:\n    %s\n  annotations:\n    %s\n", tc.kind, tc.labels, tc.annotations)
			}
			assert.Empty(t, LossyMetadataWarnings([]byte(raw), ""))
			assert.Empty(t, LossyMetadataWarnings([]byte(fmt.Sprintf(`kind: %s
metadata:
  annotations:
    dash0.com/created-at: '2026-01-01T00:00:00Z'
    dash0.com/updated-at: '2026-01-02T00:00:00Z'
    dash0.com/deleted-at: '2026-01-03T00:00:00Z'
`, tc.kind)), ""))
		})
	}
}

func TestLossyMetadataWarnings_PrometheusRuleAnnotations(t *testing.T) {
	raw := []byte(`kind: PrometheusRule
metadata:
  annotations:
    runbook: https://example.com/runbook
spec:
  groups:
    - name: backend
      rules:
        - alert: Down
          expr: up == 0
          labels: {team-lead: alice}
          annotations: {summary: Backend is down}
`)
	assert.Empty(t, LossyMetadataWarnings(raw, ""), "metadata annotations are inherited by alert rules")
	rules, err := ParseCheckRules(raw)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/runbook", rules[0].Annotations.AdditionalProperties["runbook"])
	assert.Len(t, LossyMetadataWarnings(bytes.ReplaceAll(raw, []byte("alert: Down"), []byte("record: backend_up")), ""), 1)
	assert.Empty(t, LossyMetadataWarnings([]byte("kind: CheckRule\nlabels: {team-lead: alice}\nannotations: {runbook: url}\n"), ""))
}

func TestLossyMetadataWarnings_JSONFallbackAndMalformedInput(t *testing.T) {
	assert.Len(t, LossyMetadataWarnings([]byte(`{"metadata":{"labels":{"team-lead":"alice"}}}`), "team"), 1)
	for _, raw := range []string{"[invalid:", "metadata: scalar", "kind: Unrelated\nmetadata: {labels: {team: backend}}", "kind: Dash0Team\nmetadata: {labels: null, annotations: []}"} {
		assert.Empty(t, LossyMetadataWarnings([]byte(raw), ""))
	}
	var buf bytes.Buffer
	PrintLossyMetadataWarnings(&buf, []byte(`{"metadata":{"labels":{"team-lead":"private-value"}}}`), "team", "-")
	assert.Equal(t, "warning: stdin: metadata.labels[\"team-lead\"] will not be stored by Dash0\n", buf.String())
}

func TestReadDefinitionForWrite_WarnsBeforeMetadataIsLost(t *testing.T) {
	for _, name := range []string{"team.yaml", "team.json", "team", "-"} {
		t.Run(name, func(t *testing.T) {
			raw := `{"kind":"Dash0Team","metadata":{"name":"backend","labels":{"team-lead":"alice","dash0.com/origin":"git"}}}`
			path := name
			if name != "-" {
				path = filepath.Join(t.TempDir(), name)
				require.NoError(t, os.WriteFile(path, []byte(raw), 0600))
			}
			var team dash0api.TeamDefinitionV1Alpha1
			stderr := testutil.CaptureStderr(t, func() {
				require.NoError(t, ReadDefinitionForWrite(path, &team, strings.NewReader(raw), "team"))
			})
			assert.Equal(t, "git", dash0api.GetTeamOrigin(&team))
			assert.Equal(t, 1, strings.Count(stderr, `metadata.labels["team-lead"]`))
			assert.NotContains(t, stderr, "alice")
		})
	}
}

func TestValidateDocuments_LossyMetadataLocations(t *testing.T) {
	docs, err := ParseMultiDocumentYAML([]byte(`kind: Dash0Team
metadata: {name: backend, labels: {team-lead: alice}}
---
kind: View
metadata: {name: errors, annotations: {example.com/note: value}}
`))
	require.NoError(t, err)
	for i := range docs {
		docs[i].FilePath = "assets.yaml"
	}
	errors, warnings := ValidateDocuments(docs)
	assert.Empty(t, errors)
	assert.Equal(t, []string{
		`assets.yaml: document 1: metadata.labels["team-lead"] will not be stored by Dash0`,
		`assets.yaml: document 2: metadata.annotations["example.com/note"] will not be stored by Dash0`,
	}, warnings)
}

func TestReadRawInputForWrite_PreservesInputAndWarns(t *testing.T) {
	// The command supplies the kind when it is absent from a native definition.
	raw := "metadata:\n  name: overview\n  labels: {team-lead: alice}\nspec:\n  layouts: [{kind: Grid, spec: {items: [{x: 0, y: 0}]}}]\n"
	var got []byte
	stderr := testutil.CaptureStderr(t, func() {
		var err error
		got, err = ReadRawInputForWrite("-", strings.NewReader(raw), "dashboard")
		require.NoError(t, err)
	})
	assert.Equal(t, raw, string(got))
	assert.Equal(t, "warning: stdin: metadata.labels[\"team-lead\"] will not be stored by Dash0\n", stderr)
}
