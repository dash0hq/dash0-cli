//go:build integration

package apply_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
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

func TestMalformedResponse_EscapedIdentifiers(t *testing.T) {
	const identifier = "gitops/prod"
	for _, asset := range malformedAssets {
		t.Run(asset.fixture, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			escapedPath := asset.path + "/" + url.PathEscape(identifier)
			server.On(http.MethodGet, asset.path+"/"+identifier, testutil.MockResponse{
				StatusCode: http.StatusOK, Body: json.RawMessage("null"),
				Validator: func(r *http.Request) error {
					assert.Equal(t, escapedPath, r.URL.EscapedPath())
					return testutil.RequireHeaders(r)
				},
			})
			assertMalformedCommand(t, asset.command(), []string{"get", identifier}, server, asset, http.MethodGet, escapedPath, "null")
			assert.Len(t, server.Requests(), 1)
		})
	}
}

func TestMalformedResponse_ListItemContext(t *testing.T) {
	asset := malformedAssets[0] // dashboards
	for _, value := range []any{123, nil} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, asset.path, testutil.MockResponse{
				StatusCode: http.StatusOK,
				Body:       []any{map[string]any{"id": "valid"}, map[string]any{"id": value}},
				Validator:  testutil.RequireHeaders,
			})
			stdout, err := executeResponseCommand(t, asset.command(), []string{"list", "-o", "table"}, server)
			require.Error(t, err)
			assert.Empty(t, stdout)
			detail := `field "id": expected string, received number`
			if value == nil {
				detail = `required field "id" is missing or null`
			}
			assert.Contains(t, err.Error(), "\n  list item 1:\n    "+detail+"\nHint:")
		})
	}
}

func TestMalformedResponse_ApplyIDPreflightStopsBeforeWrite(t *testing.T) {
	for _, asset := range malformedAssets {
		if asset.fixture != "teams" && asset.fixture != "slos" {
			continue
		}
		for _, kind := range malformedKinds {
			t.Run(asset.fixture+"/"+kind, func(t *testing.T) {
				testutil.SetupTestEnv(t)
				server := testutil.NewMockServer(t, testutil.FixturesDir())
				input := applyInput(t, asset, false)
				input["metadata"].(map[string]any)["labels"] = map[string]any{"dash0.com/id": malformedAssetID}
				data, err := json.Marshal(input)
				require.NoError(t, err)
				file := filepath.Join(t.TempDir(), "asset.json")
				require.NoError(t, os.WriteFile(file, data, 0600))
				path := asset.path + "/" + malformedAssetID
				server.On(http.MethodGet, path, malformedResponse(t, asset, kind, false, false))
				assertMalformedCommand(t, apply.NewApplyCmd(), []string{"-f", file}, server, asset, http.MethodGet, path, kind)
				require.Len(t, server.Requests(), 1, "a failed routing read must not fall back to POST or PUT")
			})
		}
	}
}

// A failed response does not roll back the server's successful write. A manual
// retry without an ID creates another asset; a pinned ID replaces the same one.
func TestMalformedResponse_RepeatedApplyAfterWrite(t *testing.T) {
	asset := malformedAssets[2] // views
	for _, withID := range []bool{false, true} {
		t.Run(fmt.Sprintf("withID=%t", withID), func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			data, err := json.Marshal(applyInput(t, asset, withID))
			require.NoError(t, err)
			file := filepath.Join(t.TempDir(), "view.json")
			require.NoError(t, os.WriteFile(file, data, 0600))
			path, method, status := asset.path, http.MethodPost, http.StatusCreated
			if withID {
				path, method, status = asset.path+"/"+malformedAssetID, http.MethodPut, http.StatusOK
				server.On(http.MethodGet, path, testutil.MockResponse{StatusCode: http.StatusNotFound, Validator: testutil.RequireHeaders})
			}
			stored := make(map[string]dash0api.ViewDefinition)
			writes := 0
			var mu sync.Mutex
			storeWrite := func(r *http.Request) error {
				if err := testutil.RequireHeaders(r); err != nil {
					return err
				}
				var view dash0api.ViewDefinition
				if err := json.Unmarshal(server.LastRequest().Body, &view); err != nil {
					return err
				}
				mu.Lock()
				defer mu.Unlock()
				id := malformedAssetID
				if !withID {
					id = fmt.Sprintf("generated-%d", writes)
				}
				stored[id] = view
				writes++
				return nil
			}
			server.On(method, path, testutil.MockResponse{StatusCode: status, Body: json.RawMessage("null"), Validator: storeWrite})
			assertMalformedCommand(t, apply.NewApplyCmd(), []string{"-f", file}, server, asset, method, path, "null")
			mu.Lock()
			assert.Equal(t, 1, writes, "the successful write must not be automatically replayed")
			assert.Len(t, stored, 1)
			firstView := stored[malformedAssetID]
			mu.Unlock()
			if withID {
				server.On(http.MethodGet, path, testutil.MockResponse{StatusCode: http.StatusOK, Body: firstView, Validator: testutil.RequireHeaders})
			}
			response := validWriteResponse(t, asset)
			response.StatusCode, response.Validator = status, storeWrite
			server.On(method, path, response)
			_, err = executeResponseCommand(t, apply.NewApplyCmd(), []string{"-f", file}, server)
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, 2, writes)
			if withID {
				assert.Len(t, stored, 1)
			} else {
				assert.Len(t, stored, 2)
			}
		})
	}
}

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
		for _, stage := range []string{"preflight", "create", "create-201", "update"} {
			for _, kind := range malformedKinds {
				t.Run(asset.fixture+"/"+stage+"/"+kind, func(t *testing.T) {
					testutil.SetupTestEnv(t)
					server := testutil.NewMockServer(t, testutil.FixturesDir())
					create := strings.HasPrefix(stage, "create")
					input := applyInput(t, asset, !create)
					file := filepath.Join(t.TempDir(), "asset.json")
					data, err := json.Marshal(input)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(file, data, 0600))
					path, method := asset.path+"/"+malformedAssetID, http.MethodGet
					if create {
						path, method = asset.path, http.MethodPost
					} else if stage == "update" {
						method = http.MethodPut
						server.On(http.MethodGet, path, testutil.MockResponse{
							StatusCode: http.StatusOK, BodyFile: asset.fixture + "/get_success.json", Validator: testutil.RequireHeaders,
						})
					}
					response := malformedResponse(t, asset, kind, false, stage != "preflight")
					if stage == "create-201" {
						response.StatusCode = http.StatusCreated
					}
					server.On(method, path, response)
					if stage == "preflight" && asset.fixture != "slos" {
						// These reads only supply the before-state for reporting. Preserve
						// the existing PUT path so a document can repair an unreadable asset.
						server.On(http.MethodPut, path, validWriteResponse(t, asset))
						_, err := executeResponseCommand(t, apply.NewApplyCmd(), []string{"-f", file}, server)
						require.NoError(t, err)
						requests := server.Requests()
						require.Len(t, requests, 2)
						assert.Equal(t, http.MethodGet, requests[0].Method)
						assert.Equal(t, http.MethodPut, requests[1].Method)
						return
					}
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
	stdout, err := executeResponseCommand(t, cmd, args, server)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid API response for "+asset.asset)
	assert.Contains(t, err.Error(), method+" "+path)
	assert.Contains(t, err.Error(), "Hint: Update the CLI")
	if method == http.MethodPost || method == http.MethodPut {
		assert.Contains(t, err.Error(), "write may already have been applied")
		assert.Contains(t, err.Error(), "Check the asset in Dash0 before retrying")
	} else {
		assert.NotContains(t, err.Error(), "write may already have been applied")
	}
	if method == http.MethodPost {
		assert.Contains(t, err.Error(), "can create a duplicate")
	}
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

func executeResponseCommand(t *testing.T, cmd *cobra.Command, args []string, server *testutil.MockServer) (string, error) {
	t.Helper()
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.PersistentFlags().Bool("experimental", true, "enable experimental commands")
	cmd.SetArgs(append(args, "--api-url", server.URL, "--auth-token", "auth_test-token-12345"))
	var err error
	var stdout string
	require.NotPanics(t, func() {
		stdout = testutil.CaptureStdout(t, func() { err = cmd.Execute() })
	})
	return stdout, err
}

func validWriteResponse(t *testing.T, asset malformedAssetCase) testutil.MockResponse {
	t.Helper()
	body := readResponseFixture(t, asset, false)
	if asset.fixture == "teams" {
		body = body.(map[string]any)["team"]
		// Membership resolution is unrelated to response decoding.
		body.(map[string]any)["spec"].(map[string]any)["members"] = []any{}
	}
	return testutil.MockResponse{StatusCode: http.StatusOK, Body: body, Validator: testutil.RequireHeaders}
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
