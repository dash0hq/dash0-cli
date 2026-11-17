//go:build integration

package checkrules

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
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

	var rules []dash0api.PrometheusAlertRule
	for _, req := range server.Requests() {
		if req.Method != http.MethodPost || req.Path != apiPathCheckRules {
			continue
		}
		var rule dash0api.PrometheusAlertRule
		require.NoError(t, json.Unmarshal(req.Body, &rule))
		rules = append(rules, rule)
	}
	require.Len(t, rules, 2)

	rulesByName := make(map[string]dash0api.PrometheusAlertRule, len(rules))
	for _, rule := range rules {
		rulesByName[rule.Name] = rule
	}

	latencyRule := rulesByName["Alerting - CheckoutHighLatency"]
	require.NotNil(t, latencyRule.Annotations, "latency rule has no annotations; the rule must inherit the top-level metadata.annotations entry")
	assert.Equal(t, "3fa42d0c-6b8e-4c1a-9f2d-111111111111", latencyRule.Annotations.AdditionalProperties["dash0.com/notification-channel-ids"])

	errorRateRule := rulesByName["Alerting - CheckoutHighErrorRate"]
	require.NotNil(t, errorRateRule.Annotations, "error-rate rule has no annotations; it declares its own dash0.com/notification-channel-ids and runbook_url")
	assert.Equal(t, "3fa42d0c-6b8e-4c1a-9f2d-333333333333", errorRateRule.Annotations.AdditionalProperties["dash0.com/notification-channel-ids"])
	assert.Equal(t, "https://runbooks.example.com/checkout-error-rate", errorRateRule.Annotations.AdditionalProperties["runbook_url"])
}

func TestCreateCheckRule_Restored(t *testing.T) {
	const ruleID = "47b6ccbe-82ab-47c6-a613-ce0d7f34353e"
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathCheckRules+"/"+ruleID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureCheckRulesGetDeleted,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, apiPathCheckRules+"/"+ruleID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureCheckRulesRestoreSuccess,
		Validator:  testutil.RequireHeaders,
	})

	yamlFile := filepath.Join(t.TempDir(), "rule.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: CheckRule
id: `+ruleID+`
name: test-check-rule
expression: up == 0
`), 0644))

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Equal(t, "Check rule \"test-check-rule\" restored\n", output)
}
