package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dash0hq/dash0-cli/internal/agent0"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/awsmetricscrapeconfigs"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/datasets"
	"github.com/dash0hq/dash0-cli/internal/evals"
	"github.com/dash0hq/dash0-cli/internal/notificationchannels"
	"github.com/dash0hq/dash0-cli/internal/recordingrules"
	"github.com/dash0hq/dash0-cli/internal/samplingrules"
	"github.com/dash0hq/dash0-cli/internal/signaltometricsrules"
	"github.com/dash0hq/dash0-cli/internal/slos"
	"github.com/dash0hq/dash0-cli/internal/spamfilters"
	"github.com/dash0hq/dash0-cli/internal/syntheticchecks"
	"github.com/dash0hq/dash0-cli/internal/teams"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/dash0hq/dash0-cli/internal/timeseriesaggregations"
	"github.com/dash0hq/dash0-cli/internal/views"
)

// routedKind is what the conformance test needs to know about a kind the
// router serves.
type routedKind struct {
	path    string
	fixture string
	// document returns the kind's document with labels as its
	// metadata.labels block, already indented.
	document func(labels string) string
	// orgLevel kinds infer their origin without a dataset.
	orgLevel bool
	// originInPath kinds send the origin only in the path on create.
	originInPath bool
	// envelope is the field a GET response wraps the asset in, or "".
	envelope string
	// putFixture answers a PUT when it differs from fixture.
	putFixture string
	// storedKind overrides the kind of the stored asset.
	storedKind string
	// topLevelLabels kinds keep their labels, and their id, at the top level
	// of the document, as check rules do.
	topLevelLabels bool
	// originIsID kinds store their origin as their id, as check rules do: the
	// labels are the alerts' Prometheus labels and never carry the origin.
	originIsID bool
	// inferKind and inferName override the kind and name the inferred origin
	// is computed from, for kinds whose asset is not named after the document.
	inferKind, inferName string
	// cases replaces routerCases for a kind whose id is required, so the rows
	// without an id cannot apply.
	cases []routerCase
	// unsetFixture answers a GET at a key stored with no origin, for a kind
	// that always exists at its id and is created by its first write.
	unsetFixture string
	// storedID is the dash0.com/id of every stored asset, for a kind whose
	// id cannot differ from the document's, as a dataset's slug.
	storedID string
}

var routedKinds = map[string]routedKind{
	"dashboard": {
		path: "/api/dashboards", fixture: "dashboards/get_success.json",
		document: func(labels string) string {
			return "kind: Dashboard\nmetadata:\n  name: checkout\n" + labels + "spec:\n  display:\n    name: checkout\n"
		},
	},
	"ephemeraldashboard": {
		path: "/api/dashboards", fixture: "dashboards/get_success.json", storedKind: "EphemeralDashboard",
		document: func(labels string) string {
			return "kind: EphemeralDashboard\nmetadata:\n  name: checkout\n" + labels + "spec:\n  ttl: 1d\n  display:\n    name: checkout\n"
		},
	},
	"team": {
		path: "/api/teams", fixture: "teams/get_success.json", orgLevel: true,
		envelope: "team", putFixture: "teams/upsert_echo.json",
		document: func(labels string) string {
			return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Team\nmetadata:\n  name: checkout\n" + labels + "spec:\n  display:\n    name: checkout\n"
		},
	},
	"notificationchannel": {
		path: "/api/notification-channels", fixture: "notificationchannels/get_success.json", orgLevel: true,
		document: func(labels string) string {
			return "apiVersion: dash0.com/v1alpha1\nkind: Dash0NotificationChannel\nmetadata:\n  name: checkout\n" + labels + "spec:\n  type: slack\n"
		},
	},
	"timeseriesaggregation": {
		path: "/api/time-series-aggregations", fixture: "timeseriesaggregations/get_success.json", originInPath: true,
		document: func(labels string) string {
			return "kind: Dash0TimeSeriesAggregation\nmetadata:\n  name: checkout\n" + labels + "spec: {}\n"
		},
	},
	"slo": {
		path: "/api/slos", fixture: "slos/get_success.json", originInPath: true,
		document: func(labels string) string {
			return "apiVersion: openslo.com/v1\nkind: SLO\nmetadata:\n  name: checkout\n" + labels + "spec:\n  service: checkout\n"
		},
	},
	"generativeaievaluation": {
		path: "/api/generative-ai-evaluations", fixture: "evals/get_success.json",
		document: func(labels string) string {
			return "kind: Dash0GenerativeAiEvaluation\nmetadata:\n  name: checkout\n" + labels +
				"spec:\n  enabled: true\n  judge:\n    model: standard\n    output:\n      kind: boolean\n    userPrompt: judge\n" +
				"  match:\n    filters:\n      - key: gen_ai.operation.name\n        operator: is_set\n  sample:\n    rate: 0.5\n"
		},
	},
	"signaltometrics": {
		path: "/api/signal-to-metrics", fixture: "signaltometricsrules/get_success.json", originInPath: true,
		document: func(labels string) string {
			return "kind: Dash0SignalToMetrics\nmetadata:\n  name: checkout\n" + labels + "spec:\n  enabled: true\n  match:\n    signal: logs\n"
		},
	},
	"sampling": {
		path: "/api/sampling-rules", fixture: "samplingrules/get_success.json", originInPath: true,
		document: func(labels string) string {
			return "kind: Dash0Sampling\nmetadata:\n  name: checkout\n" + labels + "spec:\n  display:\n    name: Shown in the UI\n  enabled: false\n  conditions:\n    kind: error\n    spec: {}\n"
		},
	},
	"skill": {
		// The inferred origin comes from the SKILL.md frontmatter name, not metadata.name.
		path: "/api/agent0/custom-skills", fixture: "agent0skills/get_success.json", orgLevel: true,
		document: func(labels string) string {
			return "kind: Dash0Skill\nmetadata:\n  name: not-the-skill-name\n" + labels +
				"spec:\n  files:\n    - relativePath: SKILL.md\n      content: \"---\\nname: checkout\\n---\\nbody\\n\"\n"
		},
	},
	"view": {
		path: "/api/views", fixture: "views/get_success.json",
		document: func(labels string) string {
			return "kind: Dash0View\nmetadata:\n  name: checkout\n" + labels + "spec:\n  type: logs\n  display:\n    name: checkout\n"
		},
	},
	"syntheticcheck": {
		path: "/api/synthetic-checks", fixture: "syntheticchecks/get_success.json",
		document: func(labels string) string {
			return "kind: Dash0SyntheticCheck\nmetadata:\n  name: checkout\n" + labels + "spec:\n  display:\n    name: checkout\n  enabled: false\n"
		},
	},
	"checkrule": {
		// The id is the top-level id field; the labels sit at the top level.
		path: "/api/alerting/check-rules", fixture: "checkrules/get_success.json", topLevelLabels: true, originIsID: true,
		document: func(labels string) string {
			return "kind: CheckRule\nname: checkout\nexpression: up == 0\n" + topLevel(labels)
		},
	},
	"persesdashboard": {
		// The conversion names the dashboard after its title, but the inferred
		// origin keeps the CRD's metadata.name.
		path: "/api/dashboards", fixture: "dashboards/get_success.json", inferKind: "dashboard",
		document: func(labels string) string {
			return "apiVersion: perses.dev/v1alpha1\nkind: PersesDashboard\nmetadata:\n  name: checkout\n" + labels + "spec:\n  display:\n    name: Checkout title\n  duration: 5m\n"
		},
	},
	"prometheusrule": {
		// One check rule per alert, named "<group> - <alert>"; its inferred
		// origin comes from the CRD's metadata.name and that composed name.
		path: "/api/alerting/check-rules", fixture: "checkrules/get_success.json", topLevelLabels: true, originIsID: true,
		inferName: "checkout\x00group - HighErrorRate",
		document: func(labels string) string {
			return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: checkout\n" + labels +
				"spec:\n  groups:\n    - name: group\n      rules:\n        - alert: HighErrorRate\n          expr: up == 0\n"
		},
	},
	"dataset": {
		// The slug is the id and is required. The inferred origin hashes it in
		// place of metadata.name.
		path: "/api/datasets", fixture: "datasets/get_success.json", orgLevel: true, inferName: "i", storedID: "i",
		cases: datasetRouterCases,
		document: func(labels string) string {
			return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: checkout\n" + labels + "spec: {}\n"
		},
	},
	"awsmetricscrapeconfig": {
		// The integration id is the only key. Every integration has a config,
		// so a 404 is an unknown integration, which the CLI cannot create.
		path: "/api/aws-metric-scrape-configs", fixture: "awsmetricscrapeconfigs/get_configured.json", orgLevel: true,
		inferName: awsIntegrationID, unsetFixture: "awsmetricscrapeconfigs/get_unconfigured.json",
		cases: awsMetricScrapeConfigRouterCases,
		document: func(labels string) string {
			return "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nmetadata:\n" + labels + "spec:\n  namespaces: []\n"
		},
	},
	"spamfilter": {
		path: "/api/spam-filters", fixture: "spamfilters/get_success.json",
		document: func(labels string) string {
			return "apiVersion: v1alpha1\nkind: Dash0SpamFilter\nmetadata:\n  name: checkout\n" + labels + "spec:\n  contexts: [log]\n  filter: []\n"
		},
	},
}

// routedVariants are other schema versions of a routed kind that apply imports
// through their own code path, keyed by the kind they belong to.
var routedVariants = map[string]map[string]routedKind{
	"prometheusrule": {
		"recording": {
			// A CRD's recording rules are one asset, inferred from the CRD's
			// metadata.name.
			path: "/api/recording-rules", fixture: "recordingrules/get_success.json", inferKind: "recordingrule",
			document: func(labels string) string {
				return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: checkout\n" + labels +
					"spec:\n  groups:\n    - name: group\n      rules:\n        - record: instance:up:sum\n          expr: sum(up)\n"
			},
		},
	},
	"spamfilter": {
		"v1alpha2": {
			path: "/api/spam-filters", fixture: "spamfilters/get_success_v1alpha2.json",
			document: func(labels string) string {
				return "apiVersion: v1alpha2\nkind: Dash0SpamFilter\nmetadata:\n  name: checkout\n" + labels + "spec:\n  context: log\n  filter: []\n"
			},
		},
	},
}

// routerCase is one row of the routing rule every kind follows.
type routerCase struct {
	name   string
	labels map[string]string
	// stored maps a key to the origin the asset stored at that key has.
	stored map[string]string
	// want is the request sequence as "METHOD key"; inferred stands for the
	// document's inferred origin.
	want    []string
	wantErr string
	// idMiss rows write at the origin after the id is not found.
	idMiss bool
	// createOrigin is the origin a create at the id sends, for a kind whose
	// create keeps the id as its key.
	createOrigin string
}

const inferredKey = "<inferred>"

var routerCases = []routerCase{
	{
		name:   "origin only",
		labels: map[string]string{"dash0.com/origin": "o"},
		want:   []string{"GET o", "PUT o"},
	},
	{
		name:   "origin only, stored",
		labels: map[string]string{"dash0.com/origin": "o"},
		stored: map[string]string{"o": "o"},
		want:   []string{"GET o", "PUT o"},
	},
	{
		name:   "id hit",
		labels: map[string]string{"dash0.com/id": "i", "dash0.com/origin": "o"},
		stored: map[string]string{"i": "o"},
		want:   []string{"GET i", "PUT i"},
	},
	{
		name:   "id not found",
		labels: map[string]string{"dash0.com/id": "i"},
		want:   []string{"GET i", "GET " + inferredKey, "PUT " + inferredKey},
		idMiss: true,
	},
	{
		name:   "id not found, inferred origin stored",
		labels: map[string]string{"dash0.com/id": "i"},
		stored: map[string]string{inferredKey: inferredKey},
		want:   []string{"GET i", "GET " + inferredKey, "PUT " + inferredKey},
		idMiss: true,
	},
	{
		name:    "id with a differing stored origin",
		labels:  map[string]string{"dash0.com/id": "i", "dash0.com/origin": "o"},
		stored:  map[string]string{"i": "other"},
		want:    []string{"GET i"},
		wantErr: `has origin "other" in Dash0`,
	},
	{
		name: "no identifier",
		want: []string{"GET " + inferredKey, "PUT " + inferredKey},
	},
}

// datasetRouterCases are the routerCases for a dataset, whose slug is its id:
// the rows with an id are the same, and a document without one fails
// validation before any request.
var datasetRouterCases = append(slices.DeleteFunc(slices.Clone(routerCases), func(tc routerCase) bool {
	return tc.labels["dash0.com/id"] == ""
}),
	routerCase{
		name:   "id and origin, id not found",
		labels: map[string]string{"dash0.com/id": "i", "dash0.com/origin": "o"},
		want:   []string{"GET i", "GET o", "PUT o"},
	},
	routerCase{
		name:    "origin only",
		labels:  map[string]string{"dash0.com/origin": "o"},
		wantErr: "requires a slug",
	},
	routerCase{
		name:    "no identifier",
		wantErr: "requires a slug",
	},
)

const awsIntegrationID = "27755ed0-0009-422d-8c11-19c6dd72df98"

// awsMetricScrapeConfigRouterCases are the own rule of the one kind with no
// create by origin: an AWS metric scrape config is written at its
// integration id, and its first write is the create that sends the origin.
var awsMetricScrapeConfigRouterCases = []routerCase{
	{
		name:   "id hit",
		labels: map[string]string{"dash0.com/id": awsIntegrationID, "dash0.com/origin": "o"},
		stored: map[string]string{awsIntegrationID: "o"},
		want:   []string{"GET " + awsIntegrationID, "PUT " + awsIntegrationID},
	},
	{
		name:   "id hit, no origin in the document",
		labels: map[string]string{"dash0.com/id": awsIntegrationID},
		stored: map[string]string{awsIntegrationID: "o"},
		want:   []string{"GET " + awsIntegrationID, "PUT " + awsIntegrationID},
	},
	{
		name:         "first config, origin in the document",
		labels:       map[string]string{"dash0.com/id": awsIntegrationID, "dash0.com/origin": "o"},
		stored:       map[string]string{awsIntegrationID: ""},
		want:         []string{"GET " + awsIntegrationID, "PUT " + awsIntegrationID},
		createOrigin: "o",
	},
	{
		name:         "first config, no origin in the document",
		labels:       map[string]string{"dash0.com/id": awsIntegrationID},
		stored:       map[string]string{awsIntegrationID: ""},
		want:         []string{"GET " + awsIntegrationID, "PUT " + awsIntegrationID},
		createOrigin: inferredKey,
	},
	{
		name:    "id with a differing stored origin",
		labels:  map[string]string{"dash0.com/id": awsIntegrationID, "dash0.com/origin": "o"},
		stored:  map[string]string{awsIntegrationID: "other"},
		want:    []string{"GET " + awsIntegrationID},
		wantErr: `has origin "other" in Dash0`,
	},
	{
		name:    "integration not found",
		labels:  map[string]string{"dash0.com/id": awsIntegrationID},
		want:    []string{"GET " + awsIntegrationID},
		wantErr: "not found",
	},
	{
		name:    "no identifier",
		labels:  map[string]string{"dash0.com/origin": "o"},
		wantErr: `requires metadata.labels["dash0.com/id"]`,
	},
}

// TestRouter_EveryKindFollowsTheSameOrder runs the routing rule against the
// mock server for every kind apply accepts. A new kind fails here until it is
// routed.
func TestRouter_EveryKindFollowsTheSameOrder(t *testing.T) {
	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			spec, ok := routedKinds[kind]
			require.True(t, ok, "kind %q is not routed", kind)
			cases := routerCases
			if spec.cases != nil {
				cases = spec.cases
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					runRouterCase(t, kind, spec, tc)
				})
			}
			for version, variant := range routedVariants[kind] {
				for _, tc := range routerCases {
					t.Run(version+"/"+tc.name, func(t *testing.T) {
						runRouterCase(t, kind, variant, tc)
					})
				}
			}
		})
	}
}

// keepsOriginOnUpdateAtOrigin lists the kinds whose update at the stored origin
// sends that origin: without it, Dash0 leaves the origin out of a later read by
// id, which the stored-origin check relies on. An update at the id cannot send
// it, because the API rejects a body origin that differs from the path.
// A PrometheusRule's recording rules keep it too; its alerts are check rules,
// which never send it.
var keepsOriginOnUpdateAtOrigin = map[string]bool{"view": true, "syntheticcheck": true, "prometheusrule": true}

func runRouterCase(t *testing.T, kind string, spec routedKind, tc routerCase) {
	inferred := inferredOrigin(kind, spec)
	resolve := func(key string) string { return strings.ReplaceAll(key, inferredKey, inferred) }

	fixture, putResponse := routedFixtures(t, spec)
	storedResponses := make(map[string][]byte)
	stored := make(map[string]string)
	if spec.originIsID && tc.idMiss {
		t.Skip("an id of this kind that is not found is written at the id; see TestRouter_CheckRuleOriginIsItsID")
	}
	for key, origin := range tc.stored {
		if spec.originIsID && origin != key {
			t.Skip("the stored origin of this kind is its id; see TestRouter_CheckRuleOriginIsItsID")
		}
		if origin == "" && spec.unsetFixture != "" {
			storedResponses[resolve(key)] = readFixture(t, spec.unsetFixture)
			continue
		}
		stored[resolve(key)] = origin
		storedResponses[resolve(key)] = storedAsset(t, fixture, spec, resolve(key), origin)
	}
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	requests := serveRouted(server, spec, storedResponses, putResponse)

	var labels string
	if len(tc.labels) > 0 {
		labels = "  labels:\n"
		for _, key := range []string{"dash0.com/id", "dash0.com/origin"} {
			if value, ok := tc.labels[key]; ok {
				labels += fmt.Sprintf("    %s: %s\n", key, value)
			}
		}
	}
	raw := []byte(spec.document(labels))
	docKind, name, id, err := asset.ParseDocumentHeader(raw)
	require.NoError(t, err)

	apiClient, err := dash0api.NewClient(dash0api.WithApiUrl(server.URL), dash0api.WithAuthToken("auth_test-token-12345"), dash0api.WithMaxRetries(0))
	require.NoError(t, err)
	_, err = asset.ApplyDocument(context.Background(), apiClient, asset.Document{Kind: docKind, Name: name, ID: id, Raw: raw}, nil)

	if tc.wantErr != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.wantErr)
	} else {
		require.NoError(t, err)
	}
	var got []string
	for _, r := range *requests {
		got = append(got, r.line)
	}
	var want []string
	for _, line := range tc.want {
		want = append(want, resolve(line))
	}
	require.Equal(t, want, got)

	for _, r := range *requests {
		if !strings.HasPrefix(r.line, http.MethodPut) {
			continue
		}
		key := strings.TrimPrefix(r.line, "PUT ")
		bodyOrigin := labelOf(t, r.body, spec, "dash0.com/origin")
		storedOrigin, update := stored[key]
		switch {
		case spec.originIsID:
			assert.Empty(t, bodyOrigin, "the path carries the origin, and the labels belong to the alerts")
		case update && storedOrigin == key && keepsOriginOnUpdateAtOrigin[kind]:
			assert.Equal(t, key, bodyOrigin, "an update at the stored origin keeps it, so a read by id still shows it")
		case update || spec.originInPath:
			assert.Empty(t, bodyOrigin, "an update must not send the origin")
		case tc.createOrigin != "":
			assert.Equal(t, resolve(tc.createOrigin), bodyOrigin, "a create sends the origin it is created by")
		default:
			assert.Equal(t, key, bodyOrigin, "a create sends the origin it is created by")
		}
	}
}

type routedUpdateCommand struct {
	newCmd func() *cobra.Command
	args   []string
}

// routedUpdateCommands is the `update -f` command of each kind in routedKinds.
var routedUpdateCommands = map[string]routedUpdateCommand{
	"dashboard":              {dashboards.NewDashboardsCmd, []string{"dashboards", "update"}},
	"ephemeraldashboard":     {dashboards.NewDashboardsCmd, []string{"dashboards", "update"}},
	"team":                   {teams.NewTeamsCmd, []string{"-X", "teams", "update"}},
	"notificationchannel":    {notificationchannels.NewNotificationChannelsCmd, []string{"-X", "notification-channels", "update"}},
	"timeseriesaggregation":  {timeseriesaggregations.NewTimeSeriesAggregationsCmd, []string{"time-series-aggregations", "update"}},
	"spamfilter":             {spamfilters.NewSpamFiltersCmd, []string{"spam-filters", "update"}},
	"slo":                    {slos.NewSlosCmd, []string{"slos", "update"}},
	"generativeaievaluation": {evals.NewEvalsCmd, []string{"-X", "evals", "update"}},
	"signaltometrics":        {signaltometricsrules.NewSignalToMetricsRulesCmd, []string{"signal-to-metrics-rules", "update"}},
	"sampling":               {samplingrules.NewSamplingRulesCmd, []string{"sampling-rules", "update"}},
	"skill":                  {agent0.NewAgent0Cmd, []string{"-X", "agent0", "skills", "update"}},
	"view":                   {views.NewViewsCmd, []string{"views", "update"}},
	"syntheticcheck":         {syntheticchecks.NewSyntheticChecksCmd, []string{"synthetic-checks", "update"}},
	"checkrule":              {checkrules.NewCheckRulesCmd, []string{"check-rules", "update"}},
	"persesdashboard":        {dashboards.NewDashboardsCmd, []string{"dashboards", "update"}},
	"prometheusrule":         {checkrules.NewCheckRulesCmd, []string{"check-rules", "update"}},
	"dataset":                {datasets.NewDatasetsCmd, []string{"-X", "datasets", "update"}},
	"awsmetricscrapeconfig":  {awsmetricscrapeconfigs.NewAwsMetricScrapeConfigsCmd, []string{"-X", "aws-metric-scrape-configs", "update"}},
	// A variant whose writes go to another endpoint has its own command.
	"prometheusrule/recording": {recordingrules.NewRecordingRulesCmd, []string{"recording-rules", "update"}},
}

// TestRouter_UpdateNeverSendsTheOrigin checks that `update -f` sends the body
// apply sends on an update, which never carries the origin.
func TestRouter_UpdateNeverSendsTheOrigin(t *testing.T) {
	for kind, spec := range routedKinds {
		command, ok := routedUpdateCommands[kind]
		require.True(t, ok, "routed kind %q has no update command", kind)
		specs := map[string]routedKind{kind: spec}
		for version, variant := range routedVariants[kind] {
			specs[kind+"/"+version] = variant
		}
		for name, spec := range specs {
			command := command
			if variantCommand, ok := routedUpdateCommands[name]; ok {
				command = variantCommand
			}
			t.Run(name, func(t *testing.T) {
				testutil.SetupTestEnv(t)
				fixture, putResponse := routedFixtures(t, spec)
				server := testutil.NewMockServer(t, testutil.FixturesDir())
				// A kind whose id is required cannot update by its origin alone.
				key, labels := "o", "  labels:\n    dash0.com/origin: o\n"
				if spec.cases != nil {
					key = updateTestID(kind)
					labels = "  labels:\n    dash0.com/id: " + key + "\n    dash0.com/origin: o\n"
				}
				requests := serveRouted(server, spec, map[string][]byte{key: storedAsset(t, fixture, spec, key, "o")}, putResponse)

				err := executeUpdate(t, command, spec.document(labels), server.URL)
				require.NoError(t, err)

				require.NotEmpty(t, *requests)
				last := (*requests)[len(*requests)-1]
				require.Equal(t, "PUT "+key, last.line)
				bodyOrigin := labelOf(t, last.body, spec, "dash0.com/origin")
				if keepsOriginOnUpdateAtOrigin[kind] && !spec.originIsID {
					assert.Equal(t, "o", bodyOrigin, "an update at the stored origin keeps it, so a read by id still shows it")
				} else {
					assert.Empty(t, bodyOrigin, "an update must not send the origin")
				}
			})
		}
	}
}

// TestRouter_UpdateFailsWhenTheStoredOriginDiffers checks that `update -f`, with
// and without the id as argument, fails before the write when the asset at the
// file's id has another origin, as apply does.
func TestRouter_UpdateFailsWhenTheStoredOriginDiffers(t *testing.T) {
	for kind, spec := range routedKinds {
		command, ok := routedUpdateCommands[kind]
		require.True(t, ok, "routed kind %q has no update command", kind)
		id := updateTestID(kind)
		for name, args := range map[string][]string{"without argument": nil, "with the id as argument": {id}} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				testutil.SetupTestEnv(t)
				fixture, putResponse := routedFixtures(t, spec)
				server := testutil.NewMockServer(t, testutil.FixturesDir())
				requests := serveRouted(server, spec, map[string][]byte{id: storedAsset(t, fixture, spec, id, "other")}, putResponse)

				err := executeUpdate(t, command, spec.document("  labels:\n    dash0.com/id: "+id+"\n    dash0.com/origin: o\n"), server.URL, args...)

				require.Error(t, err)
				assert.Contains(t, err.Error(), "the CLI never changes a stored origin")
				for _, r := range *requests {
					assert.True(t, strings.HasPrefix(r.line, http.MethodGet), "unexpected write %q", r.line)
				}
			})
		}
	}
}

// updateTestID is the id the update tests store an asset at. An AWS
// integration id must be a UUID.
func updateTestID(kind string) string {
	if kind == "awsmetricscrapeconfig" {
		return awsIntegrationID
	}
	return "i"
}

// executeUpdate runs command with document as its -f file, against apiURL.
func executeUpdate(t *testing.T, command routedUpdateCommand, document, apiURL string, args ...string) error {
	t.Helper()
	file := filepath.Join(t.TempDir(), "asset.yaml")
	require.NoError(t, os.WriteFile(file, []byte(document), 0o600))
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(command.newCmd())
	cmdArgs := append(append(append([]string{}, command.args...), args...), "-f", file, "--api-url", apiURL, "--auth-token", "auth_test-token-12345")
	root.SetArgs(cmdArgs)
	var err error
	testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() { err = root.Execute() })
	})
	return err
}

// inferredOrigin is the origin the CLI infers for a document of kind named checkout.
func inferredOrigin(kind string, spec routedKind) string {
	dataset := "default"
	if spec.orgLevel {
		dataset = ""
	}
	name := "checkout"
	if spec.inferKind != "" {
		kind = spec.inferKind
	}
	if spec.inferName != "" {
		name = spec.inferName
	}
	return asset.InferOrigin(kind, name, dataset)
}

// topLevel turns a metadata.labels block into the top-level id and labels of
// a check rule.
func topLevel(labels string) string {
	var out string
	for _, line := range strings.Split(strings.TrimSuffix(labels, "\n"), "\n") {
		if id, ok := strings.CutPrefix(line, "    dash0.com/id: "); ok {
			out = "id: " + id + "\n" + out
		} else if line != "" {
			out += strings.TrimPrefix(line, "  ") + "\n"
		}
	}
	return out
}

// routedFixtures returns the stored-asset fixture of spec and the body that answers a write.
func routedFixtures(t *testing.T, spec routedKind) (fixture, writeResponse []byte) {
	t.Helper()
	fixture = readFixture(t, spec.fixture)
	writeResponse = fixture
	if spec.putFixture != "" {
		writeResponse = readFixture(t, spec.putFixture)
	}
	return fixture, writeResponse
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), name))
	require.NoError(t, err)
	return fixture
}

type routedRequest struct {
	line string
	body []byte
}

// serveRouted answers a GET under spec.path with the stored asset at that key,
// 500 when the stored asset is nil, or 404, and any write with writeResponse. It records every request under spec.path.
func serveRouted(server *testutil.MockServer, spec routedKind, stored map[string][]byte, writeResponse []byte) *[]routedRequest {
	var requests []routedRequest
	server.OnDefault(func(w http.ResponseWriter, r *http.Request) {
		key, onAsset := strings.CutPrefix(r.URL.Path, spec.path+"/")
		if !onAsset {
			// Reads beside the routing, such as team member lookups.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, routedRequest{line: r.Method + " " + key, body: body})
		w.Header().Set("Content-Type", "application/json")
		switch response, found := stored[key]; {
		case r.Method == http.MethodGet && !found:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
		case r.Method == http.MethodGet && response == nil:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":500,"message":"internal error"}}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write(response)
		default:
			_, _ = w.Write(writeResponse)
		}
	})
	return &requests
}

// storedAsset returns the fixture as the asset stored with origin.
func storedAsset(t *testing.T, fixture []byte, spec routedKind, key, origin string) []byte {
	t.Helper()
	var response map[string]any
	require.NoError(t, json.Unmarshal(fixture, &response))
	document := response
	if spec.envelope != "" {
		document = response[spec.envelope].(map[string]any)
	}
	if spec.storedKind != "" {
		document["kind"] = spec.storedKind
		document["spec"].(map[string]any)["ttl"] = "1d"
	}
	if spec.originIsID {
		document["id"] = key
		out, err := json.Marshal(response)
		require.NoError(t, err)
		return out
	}
	parent := document
	if !spec.topLevelLabels {
		parent = document["metadata"].(map[string]any)
	}
	labels, _ := parent["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
		parent["labels"] = labels
	}
	if spec.storedID != "" {
		labels["dash0.com/id"] = spec.storedID
	}
	if origin == "" {
		delete(labels, "dash0.com/origin")
	} else {
		labels["dash0.com/origin"] = origin
	}
	out, err := json.Marshal(response)
	require.NoError(t, err)
	return out
}

func labelOf(t *testing.T, body []byte, spec routedKind, key string) string {
	t.Helper()
	var document struct {
		Labels   map[string]any `json:"labels"`
		Metadata struct {
			Labels map[string]any `json:"labels"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	labels := document.Metadata.Labels
	if spec.topLevelLabels {
		labels = document.Labels
	}
	value, _ := labels[key].(string)
	return value
}

// TestRouter_MultiAlertPrometheusRuleRoutesEachAlertByItsDerivedKey covers a
// CRD with two alerts, whose id and origin each alert derives for itself.
func TestRouter_MultiAlertPrometheusRuleRoutesEachAlertByItsDerivedKey(t *testing.T) {
	spec := routedKinds["prometheusrule"]
	spec.document = func(labels string) string {
		return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: checkout\n" + labels +
			"spec:\n  groups:\n    - name: group\n      rules:\n" +
			"        - alert: HighErrorRate\n          expr: up == 0\n" +
			"        - alert: DiskFull\n          expr: up == 1\n"
	}
	derive := func(key, alert string) string { return asset.DeriveAlertCheckRuleID(key, "group - "+alert) }
	cases := []routerCase{
		{
			name:   "origin only",
			labels: map[string]string{"dash0.com/origin": "o"},
			want: []string{
				"GET " + derive("o", "HighErrorRate"), "PUT " + derive("o", "HighErrorRate"),
				"GET " + derive("o", "DiskFull"), "PUT " + derive("o", "DiskFull"),
			},
		},
		{
			name:   "id not found",
			labels: map[string]string{"dash0.com/id": "i"},
			want: []string{
				"GET " + derive("i", "HighErrorRate"), "PUT " + derive("i", "HighErrorRate"),
				"GET " + derive("i", "DiskFull"), "PUT " + derive("i", "DiskFull"),
			},
		},
		{
			name:   "id hit",
			labels: map[string]string{"dash0.com/id": "i"},
			stored: map[string]string{
				derive("i", "HighErrorRate"): derive("i", "HighErrorRate"),
				derive("i", "DiskFull"):      derive("i", "DiskFull"),
			},
			want: []string{
				"GET " + derive("i", "HighErrorRate"), "PUT " + derive("i", "HighErrorRate"),
				"GET " + derive("i", "DiskFull"), "PUT " + derive("i", "DiskFull"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runRouterCase(t, "prometheusrule", spec, tc)
		})
	}
}

// TestRouter_CheckRuleOriginIsItsID covers check rules, whose id is their
// origin: Dash0 creates a check rule with its origin as its id, and a read
// returns no origin label. So an id that is not found is written at the id,
// which keeps the id the file names.
func TestRouter_CheckRuleOriginIsItsID(t *testing.T) {
	cases := []routerCase{
		{
			name:   "id not found",
			labels: map[string]string{"dash0.com/id": "i"},
			want:   []string{"GET i", "PUT i"},
		},
		{
			name:    "id not found with another origin",
			labels:  map[string]string{"dash0.com/id": "i", "dash0.com/origin": "o"},
			wantErr: `sets id "i" and dash0.com/origin "o"`,
		},
		{
			name:   "id hit without an origin",
			labels: map[string]string{"dash0.com/id": "i"},
			stored: map[string]string{"i": "i"},
			want:   []string{"GET i", "PUT i"},
		},
		{
			name:   "id hit with the id as origin",
			labels: map[string]string{"dash0.com/id": "i", "dash0.com/origin": "i"},
			stored: map[string]string{"i": "i"},
			want:   []string{"GET i", "PUT i"},
		},
		{
			name:    "id hit with another origin",
			labels:  map[string]string{"dash0.com/id": "i", "dash0.com/origin": "o"},
			stored:  map[string]string{"i": "i"},
			wantErr: `sets id "i" and dash0.com/origin "o"`,
		},
	}
	for _, kind := range []string{"checkrule", "prometheusrule"} {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				runRouterCase(t, kind, routedKinds[kind], tc)
			})
		}
	}
}
