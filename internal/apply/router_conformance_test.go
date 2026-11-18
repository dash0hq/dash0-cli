package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/testutil"
)

// notRoutedYet lists the kinds that do not go through asset.RouteDocument yet.
// Each later change that moves a kind onto the router removes it here.
var notRoutedYet = map[string]string{
	"persesdashboard":        "the Perses conversion drops dash0.com/origin",
	"checkrule":              "creates by POST without an identifier",
	"prometheusrule":         "creates by POST without an identifier",
	"syntheticcheck":         "creates by POST without an identifier",
	"view":                   "creates by POST without an identifier",
	"slo":                    "POSTs when the id is not found",
	"generativeaievaluation": "POSTs when the id is not found",
	"signaltometrics":        "POSTs when the id is not found",
	"sampling":               "POSTs when the id is not found",
	"skill":                  "fails when the id is not found",
	"dataset":                "the slug is the id",
	"awsmetricscrapeconfig":  "the integration id is the id",
}

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
	},
	{
		name:   "id not found, inferred origin stored",
		labels: map[string]string{"dash0.com/id": "i"},
		stored: map[string]string{inferredKey: inferredKey},
		want:   []string{"GET i", "GET " + inferredKey, "PUT " + inferredKey},
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

// TestRouter_EveryKindFollowsTheSameOrder runs the routing rule against the
// mock server for every kind apply accepts. A new kind fails here until it is
// routed or listed in notRoutedYet.
func TestRouter_EveryKindFollowsTheSameOrder(t *testing.T) {
	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			if reason, skip := notRoutedYet[kind]; skip {
				t.Skip("not routed yet: " + reason)
			}
			spec, ok := routedKinds[kind]
			require.True(t, ok, "kind %q is neither routed nor listed in notRoutedYet", kind)
			for _, tc := range routerCases {
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

func runRouterCase(t *testing.T, kind string, spec routedKind, tc routerCase) {
	dataset := "default"
	if spec.orgLevel {
		dataset = ""
	}
	inferred := asset.InferOrigin(kind, "checkout", dataset)
	resolve := func(key string) string { return strings.ReplaceAll(key, inferredKey, inferred) }

	fixture, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), spec.fixture))
	require.NoError(t, err)
	putResponse := fixture
	if spec.putFixture != "" {
		putResponse, err = os.ReadFile(filepath.Join(testutil.FixturesDir(), spec.putFixture))
		require.NoError(t, err)
	}
	storedResponses := make(map[string][]byte)
	stored := make(map[string]string)
	for key, origin := range tc.stored {
		stored[resolve(key)] = origin
		storedResponses[resolve(key)] = storedAsset(t, fixture, spec, origin)
	}

	type request struct {
		line string
		body []byte
	}
	var requests []request
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(func(w http.ResponseWriter, r *http.Request) {
		key, onAsset := strings.CutPrefix(r.URL.Path, spec.path+"/")
		if !onAsset {
			// Reads beside the routing, such as team member lookups.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, request{line: r.Method + " " + key, body: body})
		w.Header().Set("Content-Type", "application/json")
		switch response, found := storedResponses[key]; {
		case r.Method == http.MethodGet && !found:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write(response)
		default:
			_, _ = w.Write(putResponse)
		}
	})

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
	_, err = applyDocument(context.Background(), apiClient, asset.Document{Kind: docKind, Name: name, ID: id, Raw: raw}, nil)

	if tc.wantErr != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.wantErr)
	} else {
		require.NoError(t, err)
	}
	var got []string
	for _, r := range requests {
		got = append(got, r.line)
	}
	var want []string
	for _, line := range tc.want {
		want = append(want, resolve(line))
	}
	require.Equal(t, want, got)

	for _, r := range requests {
		if !strings.HasPrefix(r.line, http.MethodPut) {
			continue
		}
		key := strings.TrimPrefix(r.line, "PUT ")
		bodyOrigin := labelOf(t, r.body, "dash0.com/origin")
		if _, update := stored[key]; update || spec.originInPath {
			assert.Empty(t, bodyOrigin, "an update must not send the origin")
		} else {
			assert.Equal(t, key, bodyOrigin, "a create sends the origin it is created by")
		}
	}
}

// storedAsset returns the fixture as the asset stored with origin.
func storedAsset(t *testing.T, fixture []byte, spec routedKind, origin string) []byte {
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
	metadata := document["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
		metadata["labels"] = labels
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

func labelOf(t *testing.T, body []byte, key string) string {
	t.Helper()
	var document struct {
		Metadata struct {
			Labels map[string]any `json:"labels"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	value, _ := document.Metadata.Labels[key].(string)
	return value
}
