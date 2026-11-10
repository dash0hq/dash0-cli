//go:build integration

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dash0hq/dash0-cli/internal/agent0"
	"github.com/dash0hq/dash0-cli/internal/authtokens"
	"github.com/dash0hq/dash0-cli/internal/awsmetricscrapeconfigs"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/datasets"
	"github.com/dash0hq/dash0-cli/internal/evals"
	"github.com/dash0hq/dash0-cli/internal/notificationchannels"
	"github.com/dash0hq/dash0-cli/internal/recordingrules"
	"github.com/dash0hq/dash0-cli/internal/signaltometricsrules"
	"github.com/dash0hq/dash0-cli/internal/slos"
	"github.com/dash0hq/dash0-cli/internal/spamfilters"
	"github.com/dash0hq/dash0-cli/internal/syntheticchecks"
	"github.com/dash0hq/dash0-cli/internal/teams"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/dash0hq/dash0-cli/internal/timeseriesaggregations"
	"github.com/dash0hq/dash0-cli/internal/views"
)

type updateIdentifiers string

const (
	updateWithNeither updateIdentifiers = "neither"
	updateWithOrigin  updateIdentifiers = "origin"
	updateWithID      updateIdentifiers = "id"
	updateWithBoth    updateIdentifiers = "both"
)

var allUpdateIdentifiers = []updateIdentifiers{updateWithNeither, updateWithOrigin, updateWithID, updateWithBoth}

func (v updateIdentifiers) hasOrigin() bool { return v == updateWithOrigin || v == updateWithBoth }
func (v updateIdentifiers) hasID() bool     { return v == updateWithID || v == updateWithBoth }

// characterizationID is a UUID because AWS integrations and auth tokens
// reject any other id.
const characterizationID = "11111111-1111-4111-8111-111111111111"

// labelBlock renders metadata.labels with the origin "o" and, when idInLabel,
// characterizationID, indented under metadata.
func (v updateIdentifiers) labelBlock(idInLabel bool) string {
	var labels []string
	if v.hasOrigin() {
		labels = append(labels, "    dash0.com/origin: o")
	}
	if v.hasID() && idInLabel {
		labels = append(labels, "    dash0.com/id: "+characterizationID)
	}
	if len(labels) == 0 {
		return ""
	}
	return "  labels:\n" + strings.Join(labels, "\n") + "\n"
}

type updateCommand struct {
	name     string
	newCmd   func() *cobra.Command
	args     []string
	document func(updateIdentifiers) string
}

var updateCommands = []updateCommand{
	{"dashboards", dashboards.NewDashboardsCmd, []string{"dashboards", "update"}, func(v updateIdentifiers) string {
		extensions := ""
		if v.hasID() {
			extensions = "  dash0Extensions:\n    id: " + characterizationID + "\n"
		}
		return "apiVersion: dash0.com/v1alpha1\nkind: Dashboard\nmetadata:\n  name: n\n" + v.labelBlock(false) + extensions + "spec:\n  display:\n    name: n\n"
	}},
	{"check-rules", checkrules.NewCheckRulesCmd, []string{"check-rules", "update"}, func(v updateIdentifiers) string {
		id := ""
		if v.hasID() {
			id = "id: " + characterizationID + "\n"
		}
		return "apiVersion: dash0.com/v1alpha1\nkind: CheckRule\n" + id + "name: n\nexpression: up == 0\nmetadata:\n" + v.labelBlock(false)
	}},
	{"synthetic-checks", syntheticchecks.NewSyntheticChecksCmd, []string{"synthetic-checks", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: SyntheticCheck\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec: {}\n"
	}},
	{"slos", slos.NewSlosCmd, []string{"slos", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: openslo.com/v1\nkind: SLO\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec: {}\n"
	}},
	{"views", views.NewViewsCmd, []string{"views", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec: {}\n"
	}},
	{"recording-rules", recordingrules.NewRecordingRulesCmd, []string{"recording-rules", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: n\n" + v.labelBlock(true) +
			"spec:\n  groups:\n    - name: g\n      rules:\n        - record: r\n          expr: up\n"
	}},
	{"notification-channels", notificationchannels.NewNotificationChannelsCmd, []string{"-X", "notification-channels", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0NotificationChannel\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec:\n  type: slack\n"
	}},
	{"spam-filters v1alpha1", spamfilters.NewSpamFiltersCmd, []string{"-X", "spam-filters", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: v1alpha1\nkind: Dash0SpamFilter\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec:\n  contexts: [log]\n  filter: []\n"
	}},
	{"spam-filters v1alpha2", spamfilters.NewSpamFiltersCmd, []string{"-X", "spam-filters", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: v1alpha2\nkind: Dash0SpamFilter\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec:\n  context: log\n  filter: []\n"
	}},
	{"teams", teams.NewTeamsCmd, []string{"-X", "teams", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Team\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec:\n  display:\n    name: n\n"
	}},
	{"time-series-aggregations", timeseriesaggregations.NewTimeSeriesAggregationsCmd, []string{"time-series-aggregations", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0TimeSeriesAggregation\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec: {}\n"
	}},
	{"datasets", datasets.NewDatasetsCmd, []string{"-X", "datasets", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec: {}\n"
	}},
	{"agent0 skills", agent0.NewAgent0Cmd, []string{"-X", "agent0", "skills", "update"}, func(v updateIdentifiers) string {
		return "kind: Dash0Skill\nmetadata:\n" + v.labelBlock(true) +
			"spec:\n  files:\n    - relativePath: SKILL.md\n      content: |\n        ---\n        name: n\n        description: d\n        ---\n"
	}},
	{"aws-metric-scrape-configs", awsmetricscrapeconfigs.NewAwsMetricScrapeConfigsCmd, []string{"-X", "aws-metric-scrape-configs", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nmetadata:\n" + v.labelBlock(true) + "spec:\n  namespaces: []\n"
	}},
	{"evals", evals.NewEvalsCmd, []string{"-X", "evals", "update"}, func(v updateIdentifiers) string {
		return "kind: Dash0GenerativeAiEvaluation\nmetadata:\n  name: n\n" + v.labelBlock(true) +
			"spec:\n  display:\n    name: n\n  enabled: true\n  judge:\n    model: standard\n    output:\n      kind: boolean\n    systemPrompt: s\n    userPrompt: u\n  match:\n    filters:\n      - key: gen_ai.operation.name\n        operator: is_set\n  sample:\n    rate: 0.25\n"
	}},
	{"signal-to-metrics-rules", signaltometricsrules.NewSignalToMetricsRulesCmd, []string{"signal-to-metrics-rules", "update"}, func(v updateIdentifiers) string {
		return "kind: Dash0SignalToMetrics\nmetadata:\n  name: n\n" + v.labelBlock(true) +
			"spec:\n  display:\n    name: n\n  enabled: true\n  match:\n    signal: logs\n    filters: []\n  output:\n    name: m\n    interval: 1m\n"
	}},
	{"auth-tokens", authtokens.NewAuthTokensCmd, []string{"-X", "auth-tokens", "update"}, func(v updateIdentifiers) string {
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0AuthToken\nmetadata:\n  name: n\n" + v.labelBlock(true) + "spec:\n  actions:\n    - ingest:*\n"
	}},
}

// runUpdateAgainstNotFound runs `update -f` without an argument against an API
// that answers every request with 404, and returns the first request it sent,
// or the error it failed with before sending any.
func runUpdateAgainstNotFound(t *testing.T, command updateCommand, v updateIdentifiers) string {
	t.Helper()
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	file := filepath.Join(t.TempDir(), "asset.yaml")
	require.NoError(t, os.WriteFile(file, []byte(command.document(v)), 0o600))

	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(command.newCmd())
	root.SetArgs(append(command.args, "-f", file, "--api-url", server.URL, "--auth-token", "auth_test_token"))

	var err error
	testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() { err = root.Execute() })
	})

	if requests := server.Requests(); len(requests) > 0 {
		return requests[0].Method + " " + requests[0].Path
	}
	require.Error(t, err)
	return "error: " + strings.SplitN(err.Error(), "\n", 2)[0]
}

// TestUpdateKey_Characterization pins which asset `update -f` addresses for
// every combination of identifiers in the file, so moving the key choice into
// one rule cannot change it. Change this table only together with a
// user-facing change.
func TestUpdateKey_Characterization(t *testing.T) {
	const tsaMissingOrigin = "Dash0TimeSeriesAggregation requires metadata.labels[\"dash0.com/origin\"]: the Dash0 API rejects a time series aggregation without an origin, and the origin is the key `apply` upserts by"
	want := map[string]map[updateIdentifiers]string{
		"dashboards": {
			updateWithNeither: "error: no dashboard id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no dashboard id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/dashboards/" + characterizationID,
			updateWithBoth:    "GET /api/dashboards/" + characterizationID,
		},
		"check-rules": {
			updateWithNeither: "error: no check rule id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no check rule id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/alerting/check-rules/" + characterizationID,
			updateWithBoth:    "GET /api/alerting/check-rules/" + characterizationID,
		},
		"synthetic-checks": {
			updateWithNeither: "error: no synthetic check id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no synthetic check id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/synthetic-checks/" + characterizationID,
			updateWithBoth:    "GET /api/synthetic-checks/" + characterizationID,
		},
		// update addresses the id first, while apply addresses the origin first: #370.
		"slos": {
			updateWithNeither: "error: no SLO ID provided as argument, and the file contains neither an ID nor a dash0.com/origin label",
			updateWithOrigin:  "GET /api/slos/o",
			updateWithID:      "GET /api/slos/" + characterizationID,
			updateWithBoth:    "GET /api/slos/" + characterizationID,
		},
		"views": {
			updateWithNeither: "error: no view id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no view id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/views/" + characterizationID,
			updateWithBoth:    "GET /api/views/" + characterizationID,
		},
		"recording-rules": {
			updateWithNeither: "error: no recording rule id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no recording rule id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/recording-rules/" + characterizationID,
			updateWithBoth:    "GET /api/recording-rules/" + characterizationID,
		},
		"notification-channels": {
			updateWithNeither: "error: no notification channel id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no notification channel id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/notification-channels/" + characterizationID,
			updateWithBoth:    "GET /api/notification-channels/" + characterizationID,
		},
		"spam-filters v1alpha1": {
			updateWithNeither: "error: no spam filter origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/spam-filters/o",
			updateWithID:      "GET /api/spam-filters/" + characterizationID,
			updateWithBoth:    "GET /api/spam-filters/o",
		},
		"spam-filters v1alpha2": {
			updateWithNeither: "error: no spam filter origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/spam-filters/o",
			updateWithID:      "GET /api/spam-filters/" + characterizationID,
			updateWithBoth:    "GET /api/spam-filters/o",
		},
		"teams": {
			updateWithNeither: "error: no team origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/teams/o",
			updateWithID:      "GET /api/teams/" + characterizationID,
			updateWithBoth:    "GET /api/teams/o",
		},
		"time-series-aggregations": {
			updateWithNeither: "error: " + tsaMissingOrigin,
			updateWithOrigin:  "GET /api/time-series-aggregations/o",
			updateWithID:      "error: " + tsaMissingOrigin,
			updateWithBoth:    "GET /api/time-series-aggregations/o",
		},
		"datasets": {
			updateWithNeither: "error: no dataset id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "GET /api/datasets/o",
			updateWithID:      "GET /api/datasets/" + characterizationID,
			updateWithBoth:    "GET /api/datasets/o",
		},
		"agent0 skills": {
			updateWithNeither: "error: no Agent0 skill origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/agent0/custom-skills/o",
			updateWithID:      "GET /api/agent0/custom-skills/" + characterizationID,
			updateWithBoth:    "GET /api/agent0/custom-skills/o",
		},
		"aws-metric-scrape-configs": {
			updateWithNeither: "error: no AWS integration id given: pass one as an argument, or set the id in the file",
			updateWithOrigin:  "error: no AWS integration id given: pass one as an argument, or set the id in the file",
			updateWithID:      "GET /api/aws-metric-scrape-configs/" + characterizationID,
			updateWithBoth:    "GET /api/aws-metric-scrape-configs/" + characterizationID,
		},
		"evals": {
			updateWithNeither: "error: no eval origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/generative-ai-evaluations/o",
			updateWithID:      "GET /api/generative-ai-evaluations/" + characterizationID,
			updateWithBoth:    "GET /api/generative-ai-evaluations/o",
		},
		"signal-to-metrics-rules": {
			updateWithNeither: "error: no signal-to-metrics rule origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/signal-to-metrics/o",
			updateWithID:      "GET /api/signal-to-metrics/" + characterizationID,
			updateWithBoth:    "GET /api/signal-to-metrics/o",
		},
		"auth-tokens": {
			updateWithNeither: "error: no auth token origin or id given: pass one as an argument, or set metadata.labels[\"dash0.com/origin\"] in the file",
			updateWithOrigin:  "GET /api/auth-tokens/o",
			updateWithID:      "GET /api/auth-tokens/" + characterizationID,
			updateWithBoth:    "GET /api/auth-tokens/o",
		},
	}

	for _, command := range updateCommands {
		for _, v := range allUpdateIdentifiers {
			t.Run(command.name+"/"+string(v), func(t *testing.T) {
				got := runUpdateAgainstNotFound(t, command, v)
				expected, ok := want[command.name][v]
				require.True(t, ok, "no characterization for this command; add one")
				assert.Equal(t, expected, got)
			})
		}
	}
}
