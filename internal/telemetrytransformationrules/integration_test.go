//go:build integration

package telemetrytransformationrules

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPath = "/api/telemetry-transformation-rules"
	origin  = "dash0-cli-normalize-logs"
	fixture = "telemetrytransformationrules/get_success.json"
)

func fixtureRule(t *testing.T) *dash0api.TelemetryTransformationRuleDefinition {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), fixture))
	require.NoError(t, err)
	rule, err := asset.ParseTelemetryTransformationRule(raw)
	require.NoError(t, err)
	return rule
}

func inputFile(t *testing.T, rule *dash0api.TelemetryTransformationRuleDefinition) string {
	t.Helper()
	raw, err := json.Marshal(rule)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "rule.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func execute(t *testing.T, server *testutil.MockServer, args ...string) (string, error) {
	t.Helper()
	cmd := NewTelemetryTransformationRulesCmd()
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs(append(args, "--api-url", server.URL, "--auth-token", "auth_test", "--dataset", "production"))
	var err error
	out := testutil.CaptureStdout(t, func() { err = cmd.Execute() })
	return out, err
}

func requireRequest(r *http.Request) error {
	if err := testutil.RequireHeaders(r); err != nil {
		return err
	}
	if r.URL.Query().Get("dataset") != "production" {
		return assert.AnError
	}
	return nil
}

func TestListAndGet(t *testing.T) {
	testutil.SetupTestEnv(t)
	for _, format := range []string{"table", "wide", "csv", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			rule := fixtureRule(t)
			rule.Spec.Examples = nil
			server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, Body: dash0api.TelemetryTransformationRuleListResponse{TelemetryTransformationRules: []dash0api.TelemetryTransformationRuleDefinition{*rule, *rule}}, Validator: requireRequest})
			server.On(http.MethodGet, apiPath+"/"+origin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixture, Validator: requireRequest})
			out, err := execute(t, server, "list", "-o", format, "--limit", "1")
			require.NoError(t, err)
			assert.Contains(t, out, "Normalize logs")
			assert.NotContains(t, out, "Synthetic log")
			if format == "json" {
				var items []dash0api.TelemetryTransformationRuleDefinition
				require.NoError(t, json.Unmarshal([]byte(out), &items))
				require.Len(t, items, 1)
				assert.Nil(t, items[0].Spec.Examples)
			}
			out, err = execute(t, server, "get", origin, "-o", format)
			require.NoError(t, err)
			assert.Contains(t, out, "Normalize logs")
			if format == "json" || format == "yaml" {
				exported, err := asset.ParseTelemetryTransformationRule([]byte(out))
				require.NoError(t, err)
				assert.Equal(t, fixtureRule(t), exported)
			}
		})
	}
	t.Run("empty JSON and agent default", func(t *testing.T) {
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, Body: dash0api.TelemetryTransformationRuleListResponse{}, Validator: requireRequest})
		old := agentmode.Enabled
		agentmode.Enabled = true
		t.Cleanup(func() { agentmode.Enabled = old })
		out, err := execute(t, server, "ls")
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, out)
	})
}

func TestCreateAndUpdateContracts(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Setenv("DASH0_MAX_RETRIES", "0")
	for _, tc := range []struct {
		name, command               string
		omitExamples, stale, dryRun bool
	}{
		{name: "POST create from stdin", command: "create"},
		{name: "replacement preserves examples", command: "update"},
		{name: "replacement omission deletes examples", command: "update", omitExamples: true},
		{name: "stale token is not refreshed", command: "update", stale: true},
		{name: "dry run shows example deletion without writing", command: "update", omitExamples: true, dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, apiPath+"/"+origin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixture, Validator: requireRequest})
			rule := fixtureRule(t)
			if tc.omitExamples {
				rule.Spec.Examples = nil
			}
			if !tc.stale {
				rule.Metadata.Annotations.Dash0ComupdatedAt = nil
			}
			path := inputFile(t, rule)
			method, target := http.MethodPut, apiPath+"/"+origin
			if tc.command == "create" {
				method, target = http.MethodPost, apiPath
			}
			response := testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixture, Validator: requireRequest}
			if tc.stale {
				response.StatusCode = http.StatusBadRequest
				response.BodyFile = ""
				response.Body = map[string]any{"error": map[string]any{"message": "superseded_version: fetch the current rule", "traceId": "trace-stale"}}
			}
			server.On(method, target, response)
			args := []string{tc.command, "-f", path}
			if tc.dryRun {
				args = append(args, "--dry-run")
			}
			if tc.command == "create" {
				in, err := os.Open(path)
				require.NoError(t, err)
				old := os.Stdin
				os.Stdin = in
				t.Cleanup(func() { os.Stdin = old; _ = in.Close() })
				args[2] = "-"
			}
			out, err := execute(t, server, args...)
			if tc.stale {
				require.ErrorContains(t, err, "superseded_version")
				assert.Contains(t, err.Error(), "status: 400")
				assert.Contains(t, err.Error(), "trace-stale")
			} else {
				require.NoError(t, err)
			}
			if tc.dryRun {
				assert.Contains(t, out, "-  examples:")
				require.Len(t, server.Requests(), 1)
			}
			if tc.command == "create" {
				assert.Contains(t, out, "created")
				require.Len(t, server.Requests(), 1)
			}
			if !tc.dryRun {
				var sent dash0api.TelemetryTransformationRuleDefinition
				require.NoError(t, json.Unmarshal(server.LastRequest().Body, &sent))
				assert.Empty(t, dash0api.GetTelemetryTransformationRuleID(&sent))
				assert.Nil(t, sent.Metadata.Labels.Dash0Comsource)
				assert.Equal(t, "production", dash0api.GetTelemetryTransformationRuleDataset(&sent))
				assert.Equal(t, fixtureRule(t).Spec.Transformations, sent.Spec.Transformations)
				assert.Equal(t, fixtureRule(t).Spec.Conditions, sent.Spec.Conditions)
				if tc.omitExamples {
					assert.Nil(t, sent.Spec.Examples)
				} else {
					require.NotNil(t, sent.Spec.Examples)
					assert.Equal(t, "Synthetic log", (*sent.Spec.Examples)[0].Name)
				}
				if tc.command == "update" {
					require.NotNil(t, sent.Metadata.Annotations)
					assert.Equal(t, fixtureRule(t).Metadata.Annotations.Dash0ComupdatedAt, sent.Metadata.Annotations.Dash0ComupdatedAt)
				}
			}
		})
	}
}

func TestErrorsAndDelete(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Setenv("DASH0_MAX_RETRIES", "0")
	for _, tc := range []struct {
		name         string
		args         []string
		method, path string
		status       int
		want         string
	}{
		{"create conflict is not upsert", []string{"create"}, http.MethodPost, apiPath, http.StatusConflict, "conflicts"},
		{"unknown update does not create", []string{"update"}, http.MethodGet, apiPath + "/" + origin, http.StatusNotFound, "not found"},
		{"unauthorized", []string{"list"}, http.MethodGet, apiPath, http.StatusUnauthorized, "authentication failed"},
		{"forbidden", []string{"get", origin}, http.MethodGet, apiPath + "/" + origin, http.StatusForbidden, "access denied"},
		{"delete", []string{"delete", origin, "--force"}, http.MethodDelete, apiPath + "/" + origin, http.StatusOK, ""},
		{"forced absent delete", []string{"remove", origin, "--force"}, http.MethodDelete, apiPath + "/" + origin, http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(tc.method, tc.path, testutil.MockResponse{StatusCode: tc.status, Body: map[string]any{"error": map[string]any{"message": "fixture detail", "traceId": "trace-errors"}}, Validator: requireRequest})
			args := append([]string{}, tc.args...)
			if tc.args[0] == "create" || tc.args[0] == "update" {
				args = append(args, "-f", inputFile(t, fixtureRule(t)))
			}
			_, err := execute(t, server, args...)
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
				assert.Contains(t, err.Error(), "fixture detail")
				assert.Contains(t, err.Error(), "trace-errors")
			}
			require.Len(t, server.Requests(), 1)
		})
	}
	t.Run("mismatched update target fails before API", func(t *testing.T) {
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		_, err := execute(t, server, "update", "unrelated", "-f", inputFile(t, fixtureRule(t)))
		require.ErrorContains(t, err, "matches neither")
		assert.Empty(t, server.Requests())
	})
	t.Run("delete cancellation", func(t *testing.T) {
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		t.Cleanup(confirmation.SetReaderForTest(strings.NewReader("n\n")))
		out, err := execute(t, server, "delete", origin)
		require.NoError(t, err)
		assert.Contains(t, out, "cancelled")
		assert.Empty(t, server.Requests())
	})
}
