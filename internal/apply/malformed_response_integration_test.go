//go:build integration

package apply_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/apply"
	"github.com/dash0hq/dash0-cli/internal/checkrules"
	"github.com/dash0hq/dash0-cli/internal/dashboards"
	"github.com/dash0hq/dash0-cli/internal/notificationchannels"
	"github.com/dash0hq/dash0-cli/internal/recordingrules"
	"github.com/dash0hq/dash0-cli/internal/slos"
	"github.com/dash0hq/dash0-cli/internal/spamfilters"
	"github.com/dash0hq/dash0-cli/internal/syntheticchecks"
	"github.com/dash0hq/dash0-cli/internal/teams"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/dash0hq/dash0-cli/internal/views"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	malformedAssetID = "00000000-0000-0000-0000-000000000001"
	truncatedFixture = "malformed/truncated.json"
	emptyFixture     = "malformed/empty.json"
)

type malformedAssetCase struct {
	fixture, path, asset string
	command              func() *cobra.Command
	summaryList          bool
}

var malformedAssets = []malformedAssetCase{
	{"dashboards", "/api/dashboards", "dashboard", dashboards.NewDashboardsCmd, true},
	{"checkrules", "/api/alerting/check-rules", "check rule", checkrules.NewCheckRulesCmd, true},
	{"views", "/api/views", "view", views.NewViewsCmd, true},
	{"syntheticchecks", "/api/synthetic-checks", "synthetic check", syntheticchecks.NewSyntheticChecksCmd, true},
	{"recordingrules", "/api/recording-rules", "recording rule", recordingrules.NewRecordingRulesCmd, false},
	{"slos", "/api/slos", "SLO", slos.NewSlosCmd, false},
	{"notificationchannels", "/api/notification-channels", "notification channel", notificationchannels.NewNotificationChannelsCmd, false},
	{"spamfilters", "/api/spam-filters", "spam filter", spamfilters.NewSpamFiltersCmd, false},
	{"teams", "/api/teams", "team", teams.NewTeamsCmd, false},
}

var malformedKinds = []string{"type-mismatch", "wrong-envelope", "required-null", "null", "truncated", "empty"}

// Run the same contract through real Cobra commands and the SDK's HTTP parser,
// including iterators and the extra GETs used to export summary-based lists.
func TestMalformedResponse_GetAndList(t *testing.T) {
	for _, asset := range malformedAssets {
		for _, operation := range []string{"get", "list", "list-json", "list-yaml"} {
			if !asset.summaryList && (operation == "list-json" || operation == "list-yaml") {
				continue
			}
			for _, kind := range malformedKinds {
				t.Run(asset.fixture+"/"+operation+"/"+kind, func(t *testing.T) {
					testutil.SetupTestEnv(t)
					server := testutil.NewMockServer(t, testutil.FixturesDir())
					path := asset.path
					args := []string{"list", "-o", "table"}
					isList := operation == "list"
					if operation == "get" {
						path += "/" + malformedAssetID
						args = []string{"get", malformedAssetID}
					} else if !isList {
						server.On(http.MethodGet, asset.path, testutil.MockResponse{
							StatusCode: http.StatusOK, BodyFile: asset.fixture + "/list_success.json", Validator: testutil.RequireHeaders,
						})
						args = []string{"list", "-o", operation[len("list-"):]}
						path += "/"
					}
					response := malformedResponse(t, asset, kind, isList, false)
					if operation == "list-json" || operation == "list-yaml" {
						server.OnPattern(http.MethodGet, regexp.MustCompile("^"+regexp.QuoteMeta(path)+"[^/]+$"), response)
					} else {
						server.On(http.MethodGet, path, response)
					}
					assertMalformedCommand(t, asset.command(), args, server, asset, http.MethodGet, path, kind)
					wantRequests := 1
					if operation == "list-json" || operation == "list-yaml" {
						wantRequests = 2
					}
					assert.Len(t, server.Requests(), wantRequests, "malformed responses must not be retried")
				})
			}
		}
	}
}

func TestMalformedResponse_Apply(t *testing.T) {
	for _, asset := range malformedAssets {
		for _, stage := range []string{"preflight", "create", "update"} {
			for _, kind := range malformedKinds {
				t.Run(asset.fixture+"/"+stage+"/"+kind, func(t *testing.T) {
					testutil.SetupTestEnv(t)
					server := testutil.NewMockServer(t, testutil.FixturesDir())
					input := applyInput(t, asset, stage != "create")
					file := filepath.Join(t.TempDir(), "asset.json")
					data, err := json.Marshal(input)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(file, data, 0600))
					path, method := asset.path+"/"+malformedAssetID, http.MethodGet
					if stage == "create" {
						path, method = asset.path, http.MethodPost
					} else if stage == "update" {
						method = http.MethodPut
						server.On(http.MethodGet, path, testutil.MockResponse{
							StatusCode: http.StatusOK, BodyFile: asset.fixture + "/get_success.json", Validator: testutil.RequireHeaders,
						})
					}
					server.On(method, path, malformedResponse(t, asset, kind, false, stage != "preflight"))
					assertMalformedCommand(t, apply.NewApplyCmd(), []string{"-f", file}, server, asset, method, path, kind)
					wantRequests := 1
					if stage == "update" {
						wantRequests = 2
					}
					assert.Len(t, server.Requests(), wantRequests, "no retries or writes after failed preflight")
				})
			}
		}
	}
}

func assertMalformedCommand(t *testing.T, cmd *cobra.Command, args []string, server *testutil.MockServer, asset malformedAssetCase, method, path, kind string) {
	t.Helper()
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.PersistentFlags().Bool("experimental", true, "enable experimental commands")
	cmd.SetArgs(append(args, "--api-url", server.URL, "--auth-token", "auth_test-token-12345"))
	var err error
	var stdout string
	require.NotPanics(t, func() {
		stdout = testutil.CaptureStdout(t, func() { err = cmd.Execute() })
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid API response for "+asset.asset)
	assert.Contains(t, err.Error(), method+" "+path)
	assert.Contains(t, err.Error(), "Hint: Update the CLI")
	switch kind {
	case "type-mismatch":
		assert.Contains(t, err.Error(), "expected string, received number")
		if method == http.MethodGet && path == asset.path && (asset.summaryList || asset.fixture == "teams") {
			assert.Contains(t, err.Error(), `field "id"`)
		} else if asset.fixture == "checkrules" {
			assert.Contains(t, err.Error(), `field "name"`)
		} else {
			assert.Contains(t, err.Error(), "metadata.name")
		}
	case "required-null":
		assert.Contains(t, err.Error(), "missing or null")
	case "truncated":
		assert.Contains(t, err.Error(), "invalid JSON at byte")
	}
	assert.Empty(t, stdout, "a malformed response must not produce success output")
}

func readResponseFixture(t *testing.T, asset malformedAssetCase, list bool) any {
	t.Helper()
	name := "get_success.json"
	if list {
		name = "list_success.json"
	}
	data, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), asset.fixture, name))
	require.NoError(t, err)
	var body any
	require.NoError(t, json.Unmarshal(data, &body))
	return body
}

func malformedResponse(t *testing.T, asset malformedAssetCase, kind string, list, write bool) testutil.MockResponse {
	t.Helper()
	response := testutil.MockResponse{StatusCode: http.StatusOK, Validator: testutil.RequireHeaders}
	switch kind {
	case "truncated":
		response.BodyFile = truncatedFixture
	case "empty":
		response.BodyFile = emptyFixture
	case "null":
		response.Body = json.RawMessage("null")
	case "wrong-envelope":
		response.Body = map[string]any{"unexpected": []any{}}
	default:
		body := readResponseFixture(t, asset, list)
		var item map[string]any
		if list {
			items := body
			if asset.fixture == "spamfilters" {
				items = body.(map[string]any)["spamFilters"]
			}
			item = items.([]any)[0].(map[string]any)
		} else {
			item = body.(map[string]any)
			if asset.fixture == "teams" {
				item = item["team"].(map[string]any)
				if write {
					body = item
				}
			}
		}
		key := "name"
		if list && (asset.summaryList || asset.fixture == "teams") {
			key = "id"
		} else if asset.fixture != "checkrules" {
			item = item["metadata"].(map[string]any)
		}
		if kind == "required-null" {
			item[key] = nil
		} else {
			item[key] = 123
		}
		response.Body = body
	}
	return response
}

func applyInput(t *testing.T, asset malformedAssetCase, withID bool) map[string]any {
	t.Helper()
	input := readResponseFixture(t, asset, false).(map[string]any)
	if asset.fixture == "teams" {
		input = input["team"].(map[string]any)
	}
	if asset.fixture == "checkrules" {
		input["kind"] = "CheckRule"
		delete(input, "id")
		if withID {
			input["id"] = malformedAssetID
		}
		return input
	}
	metadata := input["metadata"].(map[string]any)
	delete(metadata, "labels")
	delete(metadata, "dash0Extensions")
	if withID {
		switch asset.fixture {
		case "dashboards":
			metadata["dash0Extensions"] = map[string]any{"id": malformedAssetID}
		case "notificationchannels", "teams", "slos", "spamfilters":
			metadata["labels"] = map[string]any{"dash0.com/origin": malformedAssetID}
		default:
			metadata["labels"] = map[string]any{"dash0.com/id": malformedAssetID}
		}
	}
	return input
}
