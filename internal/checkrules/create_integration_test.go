//go:build integration

package checkrules

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercises the fix through the full `check-rules create` command path
// (parse -> import -> API call), not just the parsing helper covered by
// TestParseCheckRules_TopLevelAnnotationsMergeIntoRules.
func TestCreateCheckRule_PrometheusRuleCRD_TopLevelAnnotationsMerge(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathCheckRules, testutil.MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rules.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: checkout-check-rules
  namespace: monitoring
  annotations:
    dash0.com/notification-channel-ids: "3fa42d0c-6b8e-4c1a-9f2d-111111111111"
spec:
  groups:
    - name: Alerting
      interval: 1m
      rules:
        - alert: CheckoutHighLatency
          expr: up == 0
        - alert: CheckoutHighErrorRate
          expr: up == 0
          annotations:
            dash0.com/notification-channel-ids: "3fa42d0c-6b8e-4c1a-9f2d-333333333333"
            runbook_url: "https://runbooks.example.com/checkout-error-rate"
`), 0644))

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	require.NoError(t, cmd.Execute())

	posts := server.RequestBodies(t, http.MethodPost, apiPathCheckRules)
	require.Len(t, posts, 2)

	bodiesByName := make(map[string]map[string]any, len(posts))
	for _, body := range posts {
		name, ok := body["name"].(string)
		require.True(t, ok, "check rule body missing a string name: %v", body)
		bodiesByName[name] = body
	}

	latencyAnnotations, ok := bodiesByName["Alerting - CheckoutHighLatency"]["annotations"].(map[string]any)
	require.True(t, ok, "latency rule has no annotations; the rule must inherit the top-level metadata.annotations entry")
	assert.Equal(t, "3fa42d0c-6b8e-4c1a-9f2d-111111111111", latencyAnnotations["dash0.com/notification-channel-ids"])

	errorRateAnnotations, ok := bodiesByName["Alerting - CheckoutHighErrorRate"]["annotations"].(map[string]any)
	require.True(t, ok, "error-rate rule has no annotations; it declares its own dash0.com/notification-channel-ids and runbook_url")
	assert.Equal(t, "3fa42d0c-6b8e-4c1a-9f2d-333333333333", errorRateAnnotations["dash0.com/notification-channel-ids"])
	assert.Equal(t, "https://runbooks.example.com/checkout-error-rate", errorRateAnnotations["runbook_url"])
}
