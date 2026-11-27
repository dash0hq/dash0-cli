//go:build integration

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dash0hq/dash0-cli/internal/apply"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/diff"
	"github.com/dash0hq/dash0-cli/internal/testutil"
)

// kindDocument returns a valid document of the kind, carrying the identifier
// the kind upserts by. It reuses the documents of
// TestUpdateKey_Characterization where a command shares the kind.
func kindDocument(kind string) string {
	identifiers := updateWithID
	if kindUpsertsByOrigin[kind] {
		identifiers = updateWithOrigin
	}
	switch kind {
	case "ephemeraldashboard":
		return "kind: EphemeralDashboard\nmetadata:\n  name: checkout\n" + identifiers.labelBlock(false) + "spec:\n  ttl: 1d\n  display:\n    name: checkout\n"
	case "persesdashboard":
		return "apiVersion: perses.dev/v1alpha1\nkind: PersesDashboard\nmetadata:\n  name: checkout\n" + identifiers.labelBlock(true) + "spec:\n  display:\n    name: checkout\n  duration: 5m\n  panels: {}\n"
	case "prometheusrule":
		return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: checkout\n" + identifiers.labelBlock(true) +
			"spec:\n  groups:\n    - name: g\n      rules:\n        - alert: a\n          expr: up == 0\n        - record: r\n          expr: up\n"
	}
	for _, command := range updateCommands {
		if command.name == updateCommandByKind[kind] {
			// The YAML decoder reads a plain n as false, so a name of n reads as "false".
			return strings.ReplaceAll(command.document(identifiers), "name: n\n", "name: checkout\n")
		}
	}
	panic("no characterization document for kind " + kind)
}

var updateCommandByKind = map[string]string{
	"dashboard":              "dashboards",
	"checkrule":              "check-rules",
	"syntheticcheck":         "synthetic-checks",
	"slo":                    "slos",
	"view":                   "views",
	"spamfilter":             "spam-filters v1alpha1",
	"notificationchannel":    "notification-channels",
	"team":                   "teams",
	"timeseriesaggregation":  "time-series-aggregations",
	"dataset":                "datasets",
	"skill":                  "agent0 skills",
	"awsmetricscrapeconfig":  "aws-metric-scrape-configs",
	"generativeaievaluation": "evals",
	"signaltometrics":        "signal-to-metrics-rules",
	"sampling":               "sampling-rules",
}

var kindUpsertsByOrigin = map[string]bool{
	"dashboard": true, "ephemeraldashboard": true, "slo": true, "spamfilter": true, "notificationchannel": true,
	"team": true, "timeseriesaggregation": true, "skill": true, "generativeaievaluation": true,
	"signaltometrics": true, "sampling": true,
}

// changedKindDocument is kindDocument with a different name, or for a scrape
// config, which has none, different namespaces, so applying it over
// kindDocument is an update.
func changedKindDocument(kind string) string {
	document := strings.ReplaceAll(kindDocument(kind), "name: checkout\n", "name: changed\n")
	return strings.Replace(document, "namespaces: []\n", "namespaces:\n    - namespace: AWS/Lambda\n      metrics: [Errors]\n", 1)
}

// invalidKindDocument returns a document of the kind that apply rejects
// before any API call.
func invalidKindDocument(kind string) string {
	switch kind {
	case "dashboard":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dashboard\nmetadata:\n  name: checkout\n  labels:\n    dash0.com/origin: o\n  dash0Extensions:\n    id: i\nspec:\n  display:\n    name: checkout\n"
	case "ephemeraldashboard":
		return "kind: EphemeralDashboard\nmetadata:\n  name: checkout\nspec:\n  display:\n    name: checkout\n"
	case "checkrule":
		return "apiVersion: dash0.com/v1alpha1\nkind: CheckRule\nname: checkout\nexpression: [1]\n"
	case "prometheusrule":
		return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: checkout\nspec:\n  groups:\n    - name: g\n      rules: []\n"
	case "spamfilter":
		return "apiVersion: v9\nkind: Dash0SpamFilter\nmetadata:\n  name: checkout\nspec:\n  contexts: [log]\n  filter: []\n"
	case "dataset":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: checkout\nspec: {}\n"
	case "skill":
		return "kind: Dash0Skill\nmetadata:\n  labels:\n    dash0.com/origin: o\nspec:\n  files: []\n"
	case "awsmetricscrapeconfig":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nmetadata:\n  labels:\n    dash0.com/id: " + characterizationID + "\nspec: {}\n"
	case "generativeaievaluation":
		return "kind: Dash0GenerativeAiEvaluation\nmetadata:\n  name: checkout\n  labels:\n    dash0.com/origin: o\nspec:\n  enabled: true\n"
	case "sampling":
		return "kind: Dash0Sampling\nmetadata:\n  name: checkout\n  labels:\n    dash0.com/origin: o\nspec:\n  conditions:\n    kind: error\n    spec: {}\n"
	}
	// Every other kind has no check of its own, so a spec that does not decode
	// is the error it reports.
	return strings.Replace(kindDocument(kind), "spec:", "spec: [1]\nignored:", 1)
}

// fakeAPI stores what apply writes and serves it back, so a second apply of a
// changed document is an update.
type fakeAPI struct {
	mu     sync.Mutex
	stored map[string][]byte
}

// awsIntegrationPath is the scrape config of an AWS integration that exists:
// every integration has a config, so the API never answers 404 for it.
const awsIntegrationPath = "/api/aws-metric-scrape-configs/" + characterizationID

func newFakeAPI(t *testing.T) *testutil.MockServer {
	t.Helper()
	fake := &fakeAPI{stored: map[string][]byte{
		awsIntegrationPath: []byte(`{"apiVersion":"dash0.com/v1alpha1","kind":"Dash0AwsMetricScrapeConfig","metadata":{"labels":{"dash0.com/id":"` + characterizationID + `"}},"spec":{"namespaces":[]}}`),
	}}
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(fake.serve)
	return server
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		stored, ok := f.stored[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/teams/") {
			stored = []byte(`{"team":` + string(stored) + `,"members":[]}`)
		}
		_, _ = w.Write(stored)
	case http.MethodPut:
		if r.URL.Path == awsIntegrationPath {
			body = withCreatedAtAnnotation(body)
		}
		f.stored[r.URL.Path] = body
		_, _ = w.Write(body)
	case http.MethodPost:
		for _, key := range writtenKeys(body) {
			f.stored[r.URL.Path+"/"+key] = body
		}
		_, _ = w.Write(body)
	case http.MethodDelete:
		_, _ = w.Write([]byte(`{}`))
	}
}

// withCreatedAtAnnotation adds the annotation the API sets on a scrape config
// once one is written, which is how the CLI tells a set config from none.
func withCreatedAtAnnotation(body []byte) []byte {
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		return body
	}
	metadata, _ := config["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
		config["metadata"] = metadata
	}
	metadata["annotations"] = map[string]string{"dash0.com/created-at": "2026-01-01T00:00:00Z"}
	annotated, err := json.Marshal(config)
	if err != nil {
		return body
	}
	return annotated
}

// writtenKeys returns every identifier a created asset can be read back by.
func writtenKeys(body []byte) []string {
	var document struct {
		ID       string `json:"id"`
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			Slug string `json:"slug"`
		} `json:"spec"`
	}
	_ = json.Unmarshal(body, &document)
	var keys []string
	for _, key := range []string{document.ID, document.Metadata.Labels["dash0.com/id"], document.Metadata.Labels["dash0.com/origin"], document.Spec.Slug} {
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// cliRun is what one CLI invocation printed and sent.
type cliRun struct {
	stdout   string
	stderr   string
	err      string
	requests []string
}

// runCLI runs dash0 with args against server. Each placeholder replaces its
// value in the output, so the result does not depend on a temporary directory
// or a commit SHA.
func runCLI(t *testing.T, server *testutil.MockServer, placeholders map[string]string, agentMode bool, args ...string) cliRun {
	t.Helper()
	withAgentMode(t, agentMode)

	requestsBefore := len(server.Requests())
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(apply.NewApplyCmd(), diff.NewDiffCmd())
	root.SetArgs(append(append([]string{"-X"}, args...), "--api-url", server.URL, "--auth-token", "auth_test_token", "--dataset", "ds"))

	var run cliRun
	var err error
	run.stderr = testutil.CaptureStderr(t, func() {
		run.stdout = testutil.CaptureStdout(t, func() { err = root.Execute() })
	})
	if err != nil {
		run.err = err.Error()
	}
	for _, request := range server.Requests()[requestsBefore:] {
		line := request.Method + " " + request.Path
		if request.Query != "" {
			line += "?" + request.Query
		}
		run.requests = append(run.requests, line)
	}
	for placeholder, value := range placeholders {
		run.stdout = strings.ReplaceAll(run.stdout, value, placeholder)
		run.stderr = strings.ReplaceAll(run.stderr, value, placeholder)
		run.err = strings.ReplaceAll(run.err, value, placeholder)
	}
	return run
}

func writeAssetFile(t *testing.T, dir, content string) string {
	t.Helper()
	file := filepath.Join(dir, "asset.yaml")
	require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
	return file
}

// assertCharacterization compares got with the row of want for the kind the
// running subtest is named after, so a kind without a row fails.
func assertCharacterization[T any](t *testing.T, want map[string]T, got T) {
	t.Helper()
	kind := filepath.Base(t.Name())
	expected, ok := want[kind]
	if !ok {
		t.Fatalf("no characterization for kind %q; add one: %#v", kind, got)
	}
	assert.Equal(t, expected, got)
}

// sinceCharacterization is what apply --since and diff --since do for the
// documents of one kind that were removed since the ref.
type sinceCharacterization struct {
	dryRun     cliRun
	dryRunJSON cliRun
	diff       cliRun
	apply      cliRun
}

// TestApplySince_KindCharacterization pins, for every kind, which deletions
// apply --since plans, the warnings it prints, and the DELETE requests it
// sends. Change this table only together with a user-facing change.
func TestApplySince_KindCharacterization(t *testing.T) {
	testutil.SetupTestEnv(t)
	repoDir, ref := testutil.BuildGitScenario(t, "every-kind-deletion")
	placeholders := map[string]string{"$REF": ref}

	want := map[string]sinceCharacterization{
		"dashboard": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Dashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Dashboard \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Dashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Dashboard \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Dashboard \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Dashboard \"checkout\" (o) deleted\n",
				requests: []string{
					"DELETE /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/dashboards/o?dataset=ds",
				},
			},
		},
		"ephemeraldashboard": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Ephemeral dashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Ephemeral dashboard \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Ephemeral dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Ephemeral dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Ephemeral dashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Ephemeral dashboard \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Ephemeral dashboard \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Ephemeral dashboard \"checkout\" (o) deleted\n",
				requests: []string{
					"DELETE /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/dashboards/o?dataset=ds",
				},
			},
		},
		"checkrule": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Check rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Check rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Check rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "Check rule \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n",
				requests: []string{
					"DELETE /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
		},
		"syntheticcheck": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Synthetic check \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Synthetic check\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Synthetic check \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "Synthetic check \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/synthetic-checks/dash0-cli_f2debf26c52f473e7a4b1816c6f299ca?dataset=ds",
				},
			},
		},
		"slo": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete SLO \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete SLO \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"SLO\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"SLO\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete SLO \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete SLO \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "SLO \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"SLO \"checkout\" (o) deleted\n",
				stderr: "warning: SLO \"checkout\" (11111111-1111-4111-8111-111111111111) was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but an SLO that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on SLOs you manage with --since\n",
				requests: []string{
					"GET /api/slos/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/slos/dash0-cli_2b7c28a0ab687522b9ed77c33344bc6f?dataset=ds",
					"DELETE /api/slos/o?dataset=ds",
				},
			},
		},
		"view": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete View \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"View\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete View \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "View \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/views/dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870?dataset=ds",
				},
			},
		},
		"prometheusrule": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete PrometheusRule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"PrometheusRule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete PrometheusRule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "PrometheusRule \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n",
				requests: []string{
					"DELETE /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/recording-rules/dash0-cli_dd2229d9c437266800ab5a5d1a8dd019?dataset=ds",
				},
			},
		},
		"persesdashboard": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete PersesDashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"PersesDashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete PersesDashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "PersesDashboard \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/dashboards/dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23?dataset=ds",
				},
			},
		},
		"spamfilter": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Spam filter \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Spam filter \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Spam filter\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Spam filter\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Spam filter \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Spam filter \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Spam filter \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Spam filter \"checkout\" (o) deleted\n",
				stderr: "warning: spam filter \"checkout\" (11111111-1111-4111-8111-111111111111) was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a spam filter that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on spam filters you manage with --since\n",
				requests: []string{
					"GET /api/spam-filters/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/spam-filters/dash0-cli_8ecbbb9380552fc47bafd305cbaccb0e?dataset=ds",
					"DELETE /api/spam-filters/o?dataset=ds",
				},
			},
		},
		"notificationchannel": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  origin.yaml\n" +
					"    * Delete Notification channel \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Notification channel\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "origin.yaml\n" +
					"  Delete Notification channel \"checkout\" (o)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "Notification channel \"checkout\" (o) deleted\n",
				requests: []string{
					"DELETE /api/notification-channels/o",
				},
			},
		},
		"team": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Team \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Team \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Team\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Team\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Team \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Team \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Team \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Team \"checkout\" (o) deleted\n",
				stderr: "warning: team \"checkout\" (11111111-1111-4111-8111-111111111111) was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a team that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on teams you manage with --since\n",
				requests: []string{
					"GET /api/teams/11111111-1111-4111-8111-111111111111",
					"DELETE /api/teams/dash0-cli_bcd2e925dfd133e960739b9c63643445",
					"DELETE /api/teams/o",
				},
			},
		},
		"timeseriesaggregation": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  origin.yaml\n" +
					"    * Delete Time series aggregation \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Time series aggregation\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "origin.yaml\n" +
					"  Delete Time series aggregation \"checkout\" (o)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "Time series aggregation \"checkout\" (o) deleted\n",
				requests: []string{
					"DELETE /api/time-series-aggregations/o?dataset=ds",
				},
			},
		},
		"dataset": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Dataset \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Dataset\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Dataset \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "Dataset \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n",
				requests: []string{
					"DELETE /api/datasets/11111111-1111-4111-8111-111111111111",
				},
			},
		},
		"skill": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Agent0 skill \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Agent0 skill \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Agent0 skill\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Agent0 skill\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Agent0 skill \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Agent0 skill \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Agent0 skill \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Agent0 skill \"checkout\" (o) deleted\n",
				requests: []string{
					"GET /api/agent0/custom-skills/11111111-1111-4111-8111-111111111111",
					"DELETE /api/agent0/custom-skills/dash0-cli_95637659fc967c84d5d13302af42475c",
					"DELETE /api/agent0/custom-skills/o",
				},
			},
		},
		"awsmetricscrapeconfig": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 1 deletion pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete AWS metric scrape config \"<name>\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"AWS metric scrape config\",\n" +
					"        \"name\": \"\\u003cname\\u003e\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete AWS metric scrape config \"<name>\" (11111111-1111-4111-8111-111111111111)\n",
				err: "1 difference pending",
			},
			apply: cliRun{
				stdout: "AWS metric scrape config \"<name>\" (11111111-1111-4111-8111-111111111111) cleared\n",
				requests: []string{
					"DELETE /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
		},
		"generativeaievaluation": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Eval \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Eval \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Eval\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Eval\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Eval \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Eval \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Eval \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Eval \"checkout\" (o) deleted\n",
				stderr: "warning: eval \"checkout\" (11111111-1111-4111-8111-111111111111) was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but an eval that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on evals you manage with --since\n",
				requests: []string{
					"GET /api/generative-ai-evaluations/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/generative-ai-evaluations/dash0-cli_a066261debeec3968b41f4a0d21806cd?dataset=ds",
					"DELETE /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
		},
		"signaltometrics": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Signal-to-metrics rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Signal-to-metrics rule \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Signal-to-metrics rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Signal-to-metrics rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Signal-to-metrics rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Signal-to-metrics rule \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Signal-to-metrics rule \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Signal-to-metrics rule \"checkout\" (o) deleted\n",
				stderr: "warning: signal-to-metrics rule \"checkout\" (11111111-1111-4111-8111-111111111111) was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a signal-to-metrics rule that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on signal-to-metrics rules you manage with --since\n",
				requests: []string{
					"GET /api/signal-to-metrics/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/signal-to-metrics/dash0-cli_6a41e519ef6e238e992ee15aef91eeb4?dataset=ds",
					"DELETE /api/signal-to-metrics/o?dataset=ds",
				},
			},
		},
		"sampling": {
			dryRun: cliRun{
				stdout: "Dry run: 0 documents from 0 files validated; 2 deletions pending due to --since '$REF'\n" +
					"  id.yaml\n" +
					"    * Delete Sampling rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"  origin.yaml\n" +
					"    * Delete Sampling rule \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"id.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Sampling rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  },\n" +
					"  {\n" +
					"    \"path\": \"origin.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"delete\",\n" +
					"        \"kind\": \"Sampling rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\",\n" +
					"        \"since\": \"$REF\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diff: cliRun{
				stdout: "id.yaml\n" +
					"  Delete Sampling rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n" +
					"origin.yaml\n" +
					"  Delete Sampling rule \"checkout\" (o)\n",
				err: "2 differences pending",
			},
			apply: cliRun{
				stdout: "Sampling rule \"checkout\" (11111111-1111-4111-8111-111111111111) deleted\n" +
					"Sampling rule \"checkout\" (o) deleted\n",
				stderr: "warning: sampling rule \"checkout\" (11111111-1111-4111-8111-111111111111) was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a sampling rule that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on sampling rules you manage with --since\n",
				requests: []string{
					"GET /api/sampling-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"DELETE /api/sampling-rules/dash0-cli_e284652e8ae253e0ae52087d4ea1dee1?dataset=ds",
					"DELETE /api/sampling-rules/o?dataset=ds",
				},
			},
		},
	}

	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			target := filepath.Join(repoDir, kind)
			server := newFakeAPI(t)
			got := sinceCharacterization{
				dryRun:     runCLI(t, server, placeholders, false, "apply", "-f", target, "--since", ref, "--dry-run"),
				dryRunJSON: runCLI(t, server, placeholders, true, "apply", "-f", target, "--since", ref, "--dry-run"),
				diff:       runCLI(t, server, placeholders, false, "diff", "-f", target, "--since", ref),
				apply:      runCLI(t, server, placeholders, false, "apply", "-f", target, "--since", ref, "--force"),
			}
			assertCharacterization(t, want, got)
		})
	}
}

// TestApplyValidation_KindCharacterization pins, for every kind, the error
// apply reports for an invalid document, before it sends any request.
func TestApplyValidation_KindCharacterization(t *testing.T) {
	testutil.SetupTestEnv(t)

	want := map[string]string{
		"dashboard": "validation failed with 1 error:\n" +
			"  document 1: metadata.labels[\"dash0.com/origin\"] (\"o\") and metadata.dash0Extensions.id (\"i\") must be equal; set only metadata.labels[\"dash0.com/origin\"], and move \"i\" to metadata.labels[\"dash0.com/id\"] if it is the id of an existing dashboard",
		"ephemeraldashboard": "validation failed with 1 error:\n" +
			"  document 1: an EphemeralDashboard requires spec.ttl, a duration such as 1d or 12h",
		"checkrule": "validation failed with 1 error:\n" +
			"  failed to detect document kind: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .expression of type string",
		"syntheticcheck": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.SyntheticCheckSpec",
		"slo": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.SloSpec",
		"view": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.ViewSpec",
		"prometheusrule": "validation failed with 1 error:\n" +
			"  document 1: PrometheusRule contains no alerting or recording rules",
		"persesdashboard": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type map[string]interface {}",
		"spamfilter": "validation failed with 1 error:\n" +
			"  document 1: unsupported spam filter apiVersion \"v9\" (supported: \"v1alpha1\", \"v1alpha2\")",
		"notificationchannel": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.NotificationChannelSpec",
		"team": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.TeamSpec",
		"timeseriesaggregation": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.TimeSeriesAggregationSpec",
		"dataset": "validation failed with 1 error:\n" +
			"  document 1: dataset definition requires a slug in spec.slug or metadata.labels[\"dash0.com/id\"]",
		"skill": "validation failed with 1 error:\n" +
			"  document 1: Agent0 skill definition requires a SKILL.md file in spec.files",
		"awsmetricscrapeconfig": "validation failed with 1 error:\n" +
			"  document 1: spec.namespaces is required; set it to [] to stop all scraping for the integration",
		"generativeaievaluation": "validation failed with 1 error:\n" +
			"  document 1: eval definition requires spec.judge.model",
		"signaltometrics": "validation failed with 1 error:\n" +
			"  failed to decode document: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go struct field .spec of type dash0.SignalToMetricsSpec",
		"sampling": "validation failed with 1 error:\n" +
			"  document 1: sampling rule definition requires spec.enabled\n" +
			"Hint: set spec.enabled to true or false; without it the rule would be created disabled",
	}

	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			file := writeAssetFile(t, dir, invalidKindDocument(kind))
			server := newFakeAPI(t)
			run := runCLI(t, server, map[string]string{"$DIR": dir}, false, "apply", "-f", file)
			assert.Empty(t, run.requests)
			assertCharacterization(t, want, run.err)
		})
	}
}

// outputCharacterization is what apply, apply --dry-run, and diff print for
// one document of a kind, first as a create and then as an update.
type outputCharacterization struct {
	dryRun         cliRun
	dryRunJSON     cliRun
	diffCreate     cliRun
	diffCreateJSON cliRun
	applyCreate    cliRun
	diffUpdate     cliRun
	diffUpdateJSON cliRun
	applyUpdate    cliRun
}

// TestApplyAndDiffOutput_KindCharacterization pins, for every kind, the
// output of apply, apply --dry-run, and diff, in text and in agent mode.
// Change this table only together with a user-facing change.
func TestApplyAndDiffOutput_KindCharacterization(t *testing.T) {
	testutil.SetupTestEnv(t)

	want := map[string]outputCharacterization{
		"dashboard": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Dashboard \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Dashboard \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Dashboard \"checkout\" (o) created\n",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
					"PUT /api/dashboards/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Dashboard (before)\n" +
					"+++ Dashboard (after)\n" +
					"@@ -1,7 +1,7 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/origin: o\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Dashboard\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Dashboard (before)\n" +
					"+++ Dashboard (after)\n" +
					"@@ -1,5 +1,5 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
					"PUT /api/dashboards/o?dataset=ds",
				},
			},
		},
		"ephemeraldashboard": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Ephemeral dashboard \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Ephemeral dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Ephemeral dashboard \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Ephemeral dashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Ephemeral dashboard \"checkout\" (o) created\n",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
					"PUT /api/dashboards/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Ephemeral dashboard (before)\n" +
					"+++ Ephemeral dashboard (after)\n" +
					"@@ -1,8 +1,8 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/origin: o\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   ttl: 1d\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Ephemeral dashboard\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Ephemeral dashboard (before)\n" +
					"+++ Ephemeral dashboard (after)\n" +
					"@@ -1,6 +1,6 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   ttl: 1d\n",
				requests: []string{
					"GET /api/dashboards/o?dataset=ds",
					"PUT /api/dashboards/o?dataset=ds",
				},
			},
		},
		"checkrule": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Check rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Check rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Check rule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Check rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Check rule \"checkout\" (11111111-1111-4111-8111-111111111111) created\n",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"PUT /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Check rule (before)\n" +
					"+++ Check rule (after)\n" +
					"@@ -1,3 +1,3 @@\n" +
					" expression: up == 0\n" +
					" id: 11111111-1111-4111-8111-111111111111\n" +
					"-name: checkout\n" +
					"+name: changed\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Check rule\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Check rule (before)\n" +
					"+++ Check rule (after)\n" +
					"@@ -1,3 +1,3 @@\n" +
					" expression: up == 0\n" +
					" id: 11111111-1111-4111-8111-111111111111\n" +
					"-name: checkout\n" +
					"+name: changed\n",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"PUT /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
				},
			},
		},
		"syntheticcheck": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Synthetic check \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Synthetic check\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Synthetic check \"checkout\" (dash0-cli_f2debf26c52f473e7a4b1816c6f299ca)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/synthetic-checks/dash0-cli_f2debf26c52f473e7a4b1816c6f299ca?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Synthetic check\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"dash0-cli_f2debf26c52f473e7a4b1816c6f299ca\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/synthetic-checks/dash0-cli_f2debf26c52f473e7a4b1816c6f299ca?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Synthetic check \"checkout\" created\n",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/synthetic-checks/dash0-cli_f2debf26c52f473e7a4b1816c6f299ca?dataset=ds",
					"PUT /api/synthetic-checks/dash0-cli_f2debf26c52f473e7a4b1816c6f299ca?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "Create Synthetic check \"changed\" (dash0-cli_7e713b04dcf6adaf5b182c232147a957)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/synthetic-checks/dash0-cli_7e713b04dcf6adaf5b182c232147a957?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Synthetic check\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"dash0-cli_7e713b04dcf6adaf5b182c232147a957\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/synthetic-checks/dash0-cli_7e713b04dcf6adaf5b182c232147a957?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "Synthetic check \"changed\" created\n",
				requests: []string{
					"GET /api/synthetic-checks/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/synthetic-checks/dash0-cli_7e713b04dcf6adaf5b182c232147a957?dataset=ds",
					"PUT /api/synthetic-checks/dash0-cli_7e713b04dcf6adaf5b182c232147a957?dataset=ds",
				},
			},
		},
		"slo": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply SLO \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"SLO\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create SLO \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/slos/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"SLO\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/slos/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "SLO \"checkout\" created\n",
				requests: []string{
					"GET /api/slos/o?dataset=ds",
					"PUT /api/slos/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- SLO (before)\n" +
					"+++ SLO (after)\n" +
					"@@ -1,2 +1,2 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/slos/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"SLO\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/slos/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- SLO (before)\n" +
					"+++ SLO (after)\n" +
					"@@ -1,2 +1,2 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n",
				requests: []string{
					"GET /api/slos/o?dataset=ds",
					"PUT /api/slos/o?dataset=ds",
				},
			},
		},
		"view": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply View \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"View\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create View \"checkout\" (dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/views/dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"View\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/views/dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "View \"checkout\" created\n",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/views/dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870?dataset=ds",
					"PUT /api/views/dash0-cli_7440a9a04e8812130e9c4dcbc5a3c870?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "Create View \"changed\" (dash0-cli_9d4aa7878d5e59c395d08ea224b08c5b)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/views/dash0-cli_9d4aa7878d5e59c395d08ea224b08c5b?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"View\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"dash0-cli_9d4aa7878d5e59c395d08ea224b08c5b\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/views/dash0-cli_9d4aa7878d5e59c395d08ea224b08c5b?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "View \"changed\" created\n",
				requests: []string{
					"GET /api/views/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/views/dash0-cli_9d4aa7878d5e59c395d08ea224b08c5b?dataset=ds",
					"PUT /api/views/dash0-cli_9d4aa7878d5e59c395d08ea224b08c5b?dataset=ds",
				},
			},
		},
		"prometheusrule": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply PrometheusRule \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"PrometheusRule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Check rule \"g - a\" (11111111-1111-4111-8111-111111111111)\n" +
					"Create Recording rule \"checkout\" (dash0-cli_dd2229d9c437266800ab5a5d1a8dd019)\n",
				err: "2 differences pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/dash0-cli_dd2229d9c437266800ab5a5d1a8dd019?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Check rule\",\n" +
					"        \"name\": \"g - a\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      },\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Recording rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"dash0-cli_dd2229d9c437266800ab5a5d1a8dd019\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "2 differences pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/dash0-cli_dd2229d9c437266800ab5a5d1a8dd019?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Check rule \"g - a\" (11111111-1111-4111-8111-111111111111) created\n" +
					"Recording rule \"checkout\" created\n",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"PUT /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/dash0-cli_dd2229d9c437266800ab5a5d1a8dd019?dataset=ds",
					"PUT /api/recording-rules/dash0-cli_dd2229d9c437266800ab5a5d1a8dd019?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "Check rule \"g - a\": no changes\n" +
					"Create Recording rule \"changed\" (dash0-cli_b1efe9aa12ad303de0a22e0d23a5a226)\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/dash0-cli_b1efe9aa12ad303de0a22e0d23a5a226?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Recording rule\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"dash0-cli_b1efe9aa12ad303de0a22e0d23a5a226\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/dash0-cli_b1efe9aa12ad303de0a22e0d23a5a226?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "Check rule \"g - a\": no changes\n" +
					"Recording rule \"changed\" created\n",
				requests: []string{
					"GET /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"PUT /api/alerting/check-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/recording-rules/dash0-cli_b1efe9aa12ad303de0a22e0d23a5a226?dataset=ds",
					"PUT /api/recording-rules/dash0-cli_b1efe9aa12ad303de0a22e0d23a5a226?dataset=ds",
				},
			},
		},
		"persesdashboard": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply PersesDashboard \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"PersesDashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Dashboard \"checkout\" (dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/dashboards/dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"PersesDashboard\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/dashboards/dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Dashboard \"checkout\" (dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23) created\n",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/dashboards/dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23?dataset=ds",
					"PUT /api/dashboards/dash0-cli_dc6fd4345ef58576eb19431fc1ccfa23?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "Create Dashboard \"changed\" (dash0-cli_af4e084e9ed9269c89119b966223e9d8)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/dashboards/dash0-cli_af4e084e9ed9269c89119b966223e9d8?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"PersesDashboard\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"dash0-cli_af4e084e9ed9269c89119b966223e9d8\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/dashboards/dash0-cli_af4e084e9ed9269c89119b966223e9d8?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "Dashboard \"changed\" (dash0-cli_af4e084e9ed9269c89119b966223e9d8) created\n",
				requests: []string{
					"GET /api/dashboards/11111111-1111-4111-8111-111111111111?dataset=ds",
					"GET /api/dashboards/dash0-cli_af4e084e9ed9269c89119b966223e9d8?dataset=ds",
					"PUT /api/dashboards/dash0-cli_af4e084e9ed9269c89119b966223e9d8?dataset=ds",
				},
			},
		},
		"spamfilter": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Spam filter \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Spam filter\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Spam filter \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/spam-filters/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Spam filter\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/spam-filters/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Spam filter \"checkout\" created\n",
				requests: []string{
					"GET /api/spam-filters/o?dataset=ds",
					"PUT /api/spam-filters/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Spam filter (before)\n" +
					"+++ Spam filter (after)\n" +
					"@@ -1,7 +1,7 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/origin: o\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   contexts:\n" +
					"   - log\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/spam-filters/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Spam filter\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/spam-filters/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Spam filter (before)\n" +
					"+++ Spam filter (after)\n" +
					"@@ -1,5 +1,5 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   contexts:\n" +
					"   - log\n",
				requests: []string{
					"GET /api/spam-filters/o?dataset=ds",
					"PUT /api/spam-filters/o?dataset=ds",
				},
			},
		},
		"notificationchannel": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Notification channel \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Notification channel\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Notification channel \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/notification-channels/o",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Notification channel\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/notification-channels/o",
				},
			},
			applyCreate: cliRun{
				stdout: "Notification channel \"checkout\" created\n",
				requests: []string{
					"GET /api/notification-channels/o",
					"PUT /api/notification-channels/o",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Notification channel (before)\n" +
					"+++ Notification channel (after)\n" +
					"@@ -1,6 +1,6 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/origin: o\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   type: slack\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/notification-channels/o",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Notification channel\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/notification-channels/o",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Notification channel (before)\n" +
					"+++ Notification channel (after)\n" +
					"@@ -1,4 +1,4 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   type: slack\n",
				requests: []string{
					"GET /api/notification-channels/o",
					"PUT /api/notification-channels/o",
				},
			},
		},
		"team": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Team \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Team\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Team \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/teams/o",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Team\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/teams/o",
				},
			},
			applyCreate: cliRun{
				stdout: "Team \"checkout\" created\n",
				requests: []string{
					"GET /api/teams/o",
					"PUT /api/teams/o",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Team (before)\n" +
					"+++ Team (after)\n" +
					"@@ -1,7 +1,7 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/origin: o\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/teams/o",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Team\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/teams/o",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Team (before)\n" +
					"+++ Team (after)\n" +
					"@@ -1,5 +1,5 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n",
				requests: []string{
					"GET /api/teams/o",
					"PUT /api/teams/o",
				},
			},
		},
		"timeseriesaggregation": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Time series aggregation \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Time series aggregation\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Time series aggregation \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/time-series-aggregations/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Time series aggregation\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/time-series-aggregations/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Time series aggregation \"checkout\" created\n",
				requests: []string{
					"GET /api/time-series-aggregations/o?dataset=ds",
					"PUT /api/time-series-aggregations/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Time series aggregation (before)\n" +
					"+++ Time series aggregation (after)\n" +
					"@@ -1,4 +1,4 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   enabled: false\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/time-series-aggregations/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Time series aggregation\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/time-series-aggregations/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Time series aggregation (before)\n" +
					"+++ Time series aggregation (after)\n" +
					"@@ -1,4 +1,4 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   enabled: false\n",
				requests: []string{
					"GET /api/time-series-aggregations/o?dataset=ds",
					"PUT /api/time-series-aggregations/o?dataset=ds",
				},
			},
		},
		"dataset": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Dataset \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Dataset\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Dataset \"checkout\" (11111111-1111-4111-8111-111111111111)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/datasets/11111111-1111-4111-8111-111111111111",
					"GET /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Dataset\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/datasets/11111111-1111-4111-8111-111111111111",
					"GET /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
				},
			},
			applyCreate: cliRun{
				stdout: "Dataset \"checkout\" (11111111-1111-4111-8111-111111111111) created\n",
				requests: []string{
					"GET /api/datasets/11111111-1111-4111-8111-111111111111",
					"GET /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
					"PUT /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Dataset (before)\n" +
					"+++ Dataset (after)\n" +
					"@@ -2,4 +2,4 @@\n" +
					"   labels:\n" +
					"     dash0.com/id: 11111111-1111-4111-8111-111111111111\n" +
					"     dash0.com/origin: dash0-cli_6607c9b6b409c12240c46a45369b2c8c\n" +
					"-  name: checkout\n" +
					"+  name: changed\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/datasets/11111111-1111-4111-8111-111111111111",
					"GET /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Dataset\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/datasets/11111111-1111-4111-8111-111111111111",
					"GET /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Dataset (before)\n" +
					"+++ Dataset (after)\n" +
					"@@ -1,4 +1,4 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/id: 11111111-1111-4111-8111-111111111111\n" +
					"-  name: checkout\n" +
					"+  name: changed\n",
				requests: []string{
					"GET /api/datasets/11111111-1111-4111-8111-111111111111",
					"GET /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
					"PUT /api/datasets/dash0-cli_6607c9b6b409c12240c46a45369b2c8c",
				},
			},
		},
		"skill": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Agent0 skill \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Agent0 skill\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Agent0 skill \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/agent0/custom-skills/o",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Agent0 skill\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/agent0/custom-skills/o",
				},
			},
			applyCreate: cliRun{
				stdout: "Agent0 skill \"checkout\" (o) created\n",
				requests: []string{
					"GET /api/agent0/custom-skills/o",
					"PUT /api/agent0/custom-skills/o",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Agent0 skill (before)\n" +
					"+++ Agent0 skill (after)\n" +
					"@@ -5,7 +5,7 @@\n" +
					"   files:\n" +
					"   - content: |\n" +
					"       ---\n" +
					"-      name: checkout\n" +
					"+      name: changed\n" +
					"       description: d\n" +
					"       ---\n" +
					"     relativePath: SKILL.md\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/agent0/custom-skills/o",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Agent0 skill\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/agent0/custom-skills/o",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Agent0 skill (before)\n" +
					"+++ Agent0 skill (after)\n" +
					"@@ -1,11 +1,8 @@\n" +
					"-metadata:\n" +
					"-  labels:\n" +
					"-    dash0.com/origin: o\n" +
					" spec:\n" +
					"   files:\n" +
					"   - content: |\n" +
					"       ---\n" +
					"-      name: checkout\n" +
					"+      name: changed\n" +
					"       description: d\n" +
					"       ---\n" +
					"     relativePath: SKILL.md\n",
				requests: []string{
					"GET /api/agent0/custom-skills/o",
					"PUT /api/agent0/custom-skills/o",
				},
			},
		},
		"awsmetricscrapeconfig": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply AWS metric scrape config (11111111-1111-4111-8111-111111111111)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"AWS metric scrape config\",\n" +
					"        \"name\": \"\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create AWS metric scrape config (11111111-1111-4111-8111-111111111111)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"AWS metric scrape config\",\n" +
					"        \"name\": \"\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
			applyCreate: cliRun{
				stdout: "AWS metric scrape config (11111111-1111-4111-8111-111111111111) created\n",
				requests: []string{
					"GET /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
					"PUT /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- AWS metric scrape config (before)\n" +
					"+++ AWS metric scrape config (after)\n" +
					"@@ -1 +1,5 @@\n" +
					"-{}\n" +
					"+spec:\n" +
					"+  namespaces:\n" +
					"+  - metrics:\n" +
					"+    - Errors\n" +
					"+    namespace: AWS/Lambda\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"AWS metric scrape config\",\n" +
					"        \"name\": \"\",\n" +
					"        \"originOrId\": \"11111111-1111-4111-8111-111111111111\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- AWS metric scrape config (before)\n" +
					"+++ AWS metric scrape config (after)\n" +
					"@@ -1 +1,5 @@\n" +
					"-{}\n" +
					"+spec:\n" +
					"+  namespaces:\n" +
					"+  - metrics:\n" +
					"+    - Errors\n" +
					"+    namespace: AWS/Lambda\n",
				requests: []string{
					"GET /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
					"PUT /api/aws-metric-scrape-configs/11111111-1111-4111-8111-111111111111",
				},
			},
		},
		"generativeaievaluation": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Eval \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Eval\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Eval \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Eval\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Eval \"checkout\" created\n",
				requests: []string{
					"GET /api/generative-ai-evaluations/o?dataset=ds",
					"PUT /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Eval (before)\n" +
					"+++ Eval (after)\n" +
					"@@ -1,10 +1,10 @@\n" +
					" metadata:\n" +
					"   labels:\n" +
					"     dash0.com/origin: o\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   enabled: true\n" +
					"   judge:\n" +
					"     model: standard\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Eval\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Eval (before)\n" +
					"+++ Eval (after)\n" +
					"@@ -1,8 +1,8 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   enabled: true\n" +
					"   judge:\n" +
					"     model: standard\n",
				requests: []string{
					"GET /api/generative-ai-evaluations/o?dataset=ds",
					"PUT /api/generative-ai-evaluations/o?dataset=ds",
				},
			},
		},
		"signaltometrics": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Signal-to-metrics rule \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Signal-to-metrics rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Signal-to-metrics rule \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/signal-to-metrics/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Signal-to-metrics rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/signal-to-metrics/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Signal-to-metrics rule \"checkout\" created\n",
				requests: []string{
					"GET /api/signal-to-metrics/o?dataset=ds",
					"PUT /api/signal-to-metrics/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Signal-to-metrics rule (before)\n" +
					"+++ Signal-to-metrics rule (after)\n" +
					"@@ -1,8 +1,8 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   enabled: true\n" +
					"   match:\n" +
					"     signal: logs\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/signal-to-metrics/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Signal-to-metrics rule\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/signal-to-metrics/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Signal-to-metrics rule (before)\n" +
					"+++ Signal-to-metrics rule (after)\n" +
					"@@ -1,8 +1,8 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   enabled: true\n" +
					"   match:\n" +
					"     signal: logs\n",
				requests: []string{
					"GET /api/signal-to-metrics/o?dataset=ds",
					"PUT /api/signal-to-metrics/o?dataset=ds",
				},
			},
		},
		"sampling": {
			dryRun: cliRun{
				stdout: "Dry run: 1 document validated\n" +
					"  * Apply Sampling rule \"checkout\" (o)\n",
			},
			dryRunJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"apply\",\n" +
					"        \"kind\": \"Sampling rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
			},
			diffCreate: cliRun{
				stdout: "Create Sampling rule \"checkout\" (o)\n",
				err:    "1 difference pending",
				requests: []string{
					"GET /api/sampling-rules/o?dataset=ds",
				},
			},
			diffCreateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"create\",\n" +
					"        \"kind\": \"Sampling rule\",\n" +
					"        \"name\": \"checkout\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/sampling-rules/o?dataset=ds",
				},
			},
			applyCreate: cliRun{
				stdout: "Sampling rule \"checkout\" created\n",
				requests: []string{
					"GET /api/sampling-rules/o?dataset=ds",
					"PUT /api/sampling-rules/o?dataset=ds",
				},
			},
			diffUpdate: cliRun{
				stdout: "--- Sampling rule (before)\n" +
					"+++ Sampling rule (after)\n" +
					"@@ -1,8 +1,8 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   conditions:\n" +
					"     kind: error\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   enabled: false\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/sampling-rules/o?dataset=ds",
				},
			},
			diffUpdateJSON: cliRun{
				stdout: "[\n" +
					"  {\n" +
					"    \"path\": \"$DIR/asset.yaml\",\n" +
					"    \"changes\": [\n" +
					"      {\n" +
					"        \"op\": \"update\",\n" +
					"        \"kind\": \"Sampling rule\",\n" +
					"        \"name\": \"changed\",\n" +
					"        \"originOrId\": \"o\"\n" +
					"      }\n" +
					"    ]\n" +
					"  }\n" +
					"]\n",
				err: "1 difference pending",
				requests: []string{
					"GET /api/sampling-rules/o?dataset=ds",
				},
			},
			applyUpdate: cliRun{
				stdout: "--- Sampling rule (before)\n" +
					"+++ Sampling rule (after)\n" +
					"@@ -1,8 +1,8 @@\n" +
					" metadata:\n" +
					"-  name: checkout\n" +
					"+  name: changed\n" +
					" spec:\n" +
					"   conditions:\n" +
					"     kind: error\n" +
					"   display:\n" +
					"-    name: checkout\n" +
					"+    name: changed\n" +
					"   enabled: false\n",
				requests: []string{
					"GET /api/sampling-rules/o?dataset=ds",
					"PUT /api/sampling-rules/o?dataset=ds",
				},
			},
		},
	}

	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			placeholders := map[string]string{"$DIR": dir}
			server := newFakeAPI(t)
			file := writeAssetFile(t, dir, kindDocument(kind))
			var got outputCharacterization
			got.dryRun = runCLI(t, server, placeholders, false, "apply", "-f", file, "--dry-run")
			got.dryRunJSON = runCLI(t, server, placeholders, true, "apply", "-f", file, "--dry-run")
			got.diffCreate = runCLI(t, server, placeholders, false, "diff", "-f", file)
			got.diffCreateJSON = runCLI(t, server, placeholders, true, "diff", "-f", file)
			got.applyCreate = runCLI(t, server, placeholders, false, "apply", "-f", file)
			writeAssetFile(t, dir, changedKindDocument(kind))
			got.diffUpdate = runCLI(t, server, placeholders, false, "diff", "-f", file)
			got.diffUpdateJSON = runCLI(t, server, placeholders, true, "diff", "-f", file)
			got.applyUpdate = runCLI(t, server, placeholders, false, "apply", "-f", file)
			assertCharacterization(t, want, got)
		})
	}
}
