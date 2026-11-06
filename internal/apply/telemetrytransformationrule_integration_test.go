//go:build integration

package apply

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const transformationAPIPath = "/api/telemetry-transformation-rules"

func transformationInput(t *testing.T) *dash0api.TelemetryTransformationRuleDefinition {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), "telemetrytransformationrules/get_success.json"))
	require.NoError(t, err)
	rule, err := asset.ParseTelemetryTransformationRule(raw)
	require.NoError(t, err)
	rule.Metadata.Annotations = nil
	return rule
}

func TestApply_TelemetryTransformationRuleLifecycle(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Setenv("DASH0_MAX_RETRIES", "0")
	var mu sync.Mutex
	stored := map[string]*dash0api.TelemetryTransformationRuleDefinition{}
	deleted := map[string]bool{}
	hideOnce := ""
	version := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, testutil.RequireHeaders(r))
		assert.Equal(t, "production", r.URL.Query().Get("dataset"))
		key := strings.TrimPrefix(r.URL.Path, transformationAPIPath+"/")
		switch r.Method {
		case http.MethodGet:
			if stored[key] == nil || deleted[key] || hideOnce == key {
				hideOnce = ""
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(stored[key])
		case http.MethodPost, http.MethodPut:
			var incoming dash0api.TelemetryTransformationRuleDefinition
			if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			assert.Equal(t, "production", dash0api.GetTelemetryTransformationRuleDataset(&incoming))
			if r.Method == http.MethodPost {
				assert.Equal(t, transformationAPIPath, r.URL.Path)
				key = asset.GetTelemetryTransformationRuleOrigin(&incoming)
				if stored[key] != nil {
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"error":{"message":"origin exists"}}`))
					return
				}
				dash0api.SetTelemetryTransformationRuleID(&incoming, fmt.Sprintf("rule-%d", len(stored)+1))
			} else {
				if stored[key] == nil {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
					return
				}
				if incoming.Metadata.Annotations != nil && incoming.Metadata.Annotations.Dash0ComupdatedAt != nil &&
					!incoming.Metadata.Annotations.Dash0ComupdatedAt.Equal(*stored[key].Metadata.Annotations.Dash0ComupdatedAt) {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":{"message":"superseded_version","traceId":"trace-apply"}}`))
					return
				}
				dash0api.SetTelemetryTransformationRuleID(&incoming, dash0api.GetTelemetryTransformationRuleID(stored[key]))
			}
			version++
			updated := time.Date(2026, 10, 27, 14, 3, 8, version, time.UTC)
			incoming.Metadata.Annotations = &dash0api.TelemetryTransformationRuleAnnotations{Dash0ComupdatedAt: &updated}
			stored[key], deleted[key] = &incoming, false
			_ = json.NewEncoder(w).Encode(incoming)
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	first, second := transformationInput(t), transformationInput(t)
	firstOrigin, secondOrigin := "dash0-cli-normalize-logs", "dash0-cli-second"
	second.Metadata.Name = "second"
	second.Metadata.Labels.Dash0Comorigin = &secondOrigin
	path := filepath.Join(t.TempDir(), "rules.yaml")
	run := func(extra ...string) (string, error) {
		t.Helper()
		a, err := json.Marshal(first)
		require.NoError(t, err)
		b, err := json.Marshal(second)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(append(a, []byte("\n---\n")...), b...), 0o600))
		cmd := NewApplyCmd()
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		cmd.SetArgs(append([]string{"-f", path, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "production"}, extra...))
		var runErr error
		out := testutil.CaptureStdout(t, func() { runErr = cmd.Execute() })
		mu.Lock()
		defer mu.Unlock()
		return out, runErr
	}

	out, err := run("--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "2 documents validated")
	assert.Empty(t, stored)
	out, err = run()
	require.NoError(t, err)
	assert.Contains(t, out, "created")
	require.Len(t, stored, 2)
	id := dash0api.GetTelemetryTransformationRuleID(stored[firstOrigin])
	originalVersion := *stored[firstOrigin].Metadata.Annotations.Dash0ComupdatedAt

	out, err = run()
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out, "no changes"))
	slices.Reverse(first.Spec.Transformations)
	(*first.Spec.Examples)[0].Name = "Changed example"
	out, err = run()
	require.NoError(t, err)
	assert.Contains(t, out, "+  - name: a-tag")
	assert.Equal(t, "a-tag", stored[firstOrigin].Spec.Transformations[0].Name)
	assert.Equal(t, "Changed example", (*stored[firstOrigin].Spec.Examples)[0].Name)
	first.Spec.Examples = nil
	out, err = run()
	require.NoError(t, err)
	assert.Contains(t, out, "-  examples:")
	assert.Nil(t, stored[firstOrigin].Spec.Examples)

	mu.Lock()
	deleted[firstOrigin] = true
	mu.Unlock()
	out, err = run()
	require.NoError(t, err)
	assert.Contains(t, out, "restored")
	assert.False(t, deleted[firstOrigin])
	assert.Equal(t, id, dash0api.GetTelemetryTransformationRuleID(stored[firstOrigin]))
	require.Len(t, stored, 2)

	mu.Lock()
	hideOnce = firstOrigin
	mu.Unlock()
	out, err = run()
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out, "no changes"))
	require.Len(t, stored, 2)
	first.Metadata.Annotations = &dash0api.TelemetryTransformationRuleAnnotations{Dash0ComupdatedAt: &originalVersion}
	_, err = run()
	require.ErrorContains(t, err, "superseded_version")
	assert.Contains(t, err.Error(), "trace-apply")
	assert.Equal(t, id, dash0api.GetTelemetryTransformationRuleID(stored[firstOrigin]))
}

func TestApply_TelemetryTransformationRuleValidationBeforeWrites(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	first, err := json.Marshal(transformationInput(t))
	require.NoError(t, err)
	invalid := strings.Replace(string(first), `"enabled":false,`, "", 1)
	path := filepath.Join(t.TempDir(), "rules.yaml")
	require.NoError(t, os.WriteFile(path, []byte(string(first)+"\n---\n"+invalid), 0o600))
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", path, "--api-url", server.URL, "--auth-token", testAuthToken})
	require.ErrorContains(t, cmd.Execute(), "spec.enabled is required")
	assert.Empty(t, server.Requests())
}

func TestApply_TelemetryTransformationRuleCreateBoundaries(t *testing.T) {
	testutil.SetupTestEnv(t)
	for _, tc := range []struct {
		name, lookup, wantError string
		getStatus               int
		loseResponse            bool
		wantMethods             []string
	}{
		{"unknown ID is not cloned", "id", "not found", http.StatusNotFound, false, []string{http.MethodGet}},
		{"preflight denial does not write", "origin", "access denied", http.StatusForbidden, false, []string{http.MethodGet}},
		{"no identifier creates once", "", "", 0, false, []string{http.MethodPost}},
		{"lost create response is not replayed", "origin", "EOF", http.StatusNotFound, true, []string{http.MethodGet, http.MethodPost}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := transformationInput(t)
			if tc.lookup != "origin" {
				rule.Metadata.Labels.Dash0Comorigin = nil
			}
			if tc.lookup != "id" {
				dash0api.ClearTelemetryTransformationRuleID(rule)
			}
			var methods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				assert.Equal(t, "production", r.URL.Query().Get("dataset"))
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					w.WriteHeader(tc.getStatus)
					_, _ = w.Write([]byte(`{"error":{"message":"lookup rejected"}}`))
					return
				}
				assert.Equal(t, http.MethodPost, r.Method)
				if tc.loseResponse {
					conn, _, err := w.(http.Hijacker).Hijack()
					if assert.NoError(t, err) {
						_ = conn.Close()
					}
					return
				}
				dash0api.SetTelemetryTransformationRuleID(rule, "created-rule")
				_ = json.NewEncoder(w).Encode(rule)
			}))
			t.Cleanup(server.Close)
			raw, err := json.Marshal(rule)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "rule.json")
			require.NoError(t, os.WriteFile(path, raw, 0o600))
			cmd := NewApplyCmd()
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			cmd.SetArgs([]string{"-f", path, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "production"})
			var runErr error
			out := testutil.CaptureStdout(t, func() { runErr = cmd.Execute() })
			server.Close()
			if tc.wantError != "" {
				require.ErrorContains(t, runErr, tc.wantError)
			} else {
				require.NoError(t, runErr)
				assert.Contains(t, out, "created")
			}
			assert.Equal(t, tc.wantMethods, methods)
		})
	}
}

func TestApply_Since_TelemetryTransformationRule(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	raw, err := json.Marshal(transformationInput(t))
	require.NoError(t, err)
	writeFileFixture(t, dir, "rule.yaml", string(raw))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-qm", "add rule")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
	require.NoError(t, os.Remove(filepath.Join(dir, "rule.yaml")))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-qm", "remove rule")
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodDelete, transformationAPIPath+"/dash0-cli-normalize-logs", testutil.MockResponse{StatusCode: http.StatusOK, Validator: func(r *http.Request) error {
		assert.Equal(t, "production", r.URL.Query().Get("dataset"))
		return testutil.RequireHeaders(r)
	}})
	for _, dryRun := range []bool{true, false} {
		cmd := newSinceTestCmd()
		args := []string{"-f", dir, "--since", before, "--force", "--experimental", "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "production"}
		if dryRun {
			args = append(args, "--dry-run")
		}
		cmd.SetArgs(args)
		var runErr error
		out := testutil.CaptureStdout(t, func() { runErr = cmd.Execute() })
		require.NoError(t, runErr)
		assert.Contains(t, out, "Telemetry transformation rule")
		if dryRun {
			assert.Empty(t, server.Requests())
		} else {
			require.Len(t, server.Requests(), 1)
			assert.Contains(t, out, "deleted")
		}
	}
}
