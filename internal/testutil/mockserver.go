// Package testutil provides testing utilities for the Dash0 CLI.
package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"
)

// Fixture paths relative to the fixtures directory.
const (
	// Dashboards fixtures
	FixtureDashboardsListSuccess   = "dashboards/list_success.json"
	FixtureDashboardsListEmpty     = "dashboards/list_empty.json"
	FixtureDashboardsGetSuccess    = "dashboards/get_success.json"
	FixtureDashboardsImportSuccess = "dashboards/import_success.json"
	FixtureDashboardsNotFound      = "dashboards/error_not_found.json"
	FixtureDashboardsUnauthorized  = "dashboards/error_unauthorized.json"

	// Check rules fixtures
	FixtureCheckRulesListSuccess   = "checkrules/list_success.json"
	FixtureCheckRulesListEmpty     = "checkrules/list_empty.json"
	FixtureCheckRulesGetSuccess    = "checkrules/get_success.json"
	FixtureCheckRulesImportSuccess = "checkrules/import_success.json"
	FixtureCheckRulesNotFound      = "checkrules/error_not_found.json"

	// Views fixtures
	FixtureViewsListSuccess   = "views/list_success.json"
	FixtureViewsListEmpty     = "views/list_empty.json"
	FixtureViewsGetSuccess    = "views/get_success.json"
	FixtureViewsImportSuccess = "views/import_success.json"
	FixtureViewsNotFound      = "views/error_not_found.json"

	// Logs fixtures
	FixtureLogsQuerySuccess = "logs/query_success.json"
	FixtureLogsQueryEmpty   = "logs/query_empty.json"

	// Synthetic checks fixtures
	FixtureSyntheticChecksListSuccess   = "syntheticchecks/list_success.json"
	FixtureSyntheticChecksListEmpty     = "syntheticchecks/list_empty.json"
	FixtureSyntheticChecksGetSuccess    = "syntheticchecks/get_success.json"
	FixtureSyntheticChecksImportSuccess = "syntheticchecks/import_success.json"
	FixtureSyntheticChecksNotFound      = "syntheticchecks/error_not_found.json"
)

// FixturesDir returns the absolute path to the fixtures directory.
func FixturesDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "fixtures")
}

// MockResponse defines how the mock server should respond to a request.
type MockResponse struct {
	// StatusCode is the HTTP status code to return.
	StatusCode int
	// BodyFile is the path to a fixture file containing the response body.
	// If empty, Body is used instead.
	BodyFile string
	// Body is the response body to return if BodyFile is not set.
	Body interface{}
	// Validator is an optional function to validate the incoming request.
	// If it returns an error, the mock server returns a 400 Bad Request.
	Validator func(r *http.Request) error
}

// RequireAuthHeader is a validator that checks for the presence of an Authorization header.
func RequireAuthHeader(r *http.Request) error {
	if r.Header.Get("Authorization") == "" {
		return fmt.Errorf("missing Authorization header")
	}
	return nil
}

// routeKey uniquely identifies an HTTP route.
type routeKey struct {
	Method string
	Path   string
}

// patternRoute matches requests by regex pattern.
type patternRoute struct {
	Method   string
	Pattern  *regexp.Regexp
	Response MockResponse
}

// RecordedRequest stores information about a received request.
type RecordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   []byte
	Header http.Header
}

// MockServer is a test HTTP server that serves responses from fixtures.
type MockServer struct {
	*httptest.Server
	t              *testing.T
	mu             sync.RWMutex
	routes         map[routeKey]MockResponse
	patternRoutes  []patternRoute
	requests       []RecordedRequest
	fixturesDir    string
	defaultHandler http.HandlerFunc
}

// NewMockServer creates a new mock server for testing.
// The fixturesDir parameter specifies the base directory for fixture files.
func NewMockServer(t *testing.T, fixturesDir string) *MockServer {
	t.Helper()

	m := &MockServer{
		t:           t,
		routes:      make(map[routeKey]MockResponse),
		fixturesDir: fixturesDir,
	}

	m.Server = httptest.NewServer(http.HandlerFunc(m.handleRequest))
	t.Cleanup(func() {
		m.Server.Close()
	})

	return m
}

// On registers a mock response for a specific method and path.
func (m *MockServer) On(method, path string, resp MockResponse) *MockServer {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.routes[routeKey{Method: method, Path: path}] = resp
	return m
}

// OnPattern registers a mock response for requests matching a regex pattern.
func (m *MockServer) OnPattern(method string, pattern *regexp.Regexp, resp MockResponse) *MockServer {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.patternRoutes = append(m.patternRoutes, patternRoute{
		Method:   method,
		Pattern:  pattern,
		Response: resp,
	})
	return m
}

// OnDefault sets a default handler for unmatched requests.
func (m *MockServer) OnDefault(handler http.HandlerFunc) *MockServer {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.defaultHandler = handler
	return m
}

// Requests returns all recorded requests.
func (m *MockServer) Requests() []RecordedRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]RecordedRequest{}, m.requests...)
}

// LastRequest returns the most recent request, or nil if no requests were made.
func (m *MockServer) LastRequest() *RecordedRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.requests) == 0 {
		return nil
	}
	req := m.requests[len(m.requests)-1]
	return &req
}

// Reset clears all recorded requests.
func (m *MockServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests = nil
}

func (m *MockServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Record the request
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   body,
		Header: r.Header.Clone(),
	})
	m.mu.Unlock()

	// Find matching route
	resp, found := m.findRoute(r)
	if !found {
		if m.defaultHandler != nil {
			m.defaultHandler(w, r)
			return
		}
		m.t.Logf("MockServer: no route matched for %s %s", r.Method, r.URL.Path)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Validate request if validator is provided
	if resp.Validator != nil {
		if err := resp.Validator(r); err != nil {
			m.t.Logf("MockServer: request validation failed: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Get response body
	var responseBody []byte
	var err error

	if resp.BodyFile != "" {
		responseBody, err = m.loadFixture(resp.BodyFile)
		if err != nil {
			m.t.Logf("MockServer: failed to load fixture %s: %v", resp.BodyFile, err)
			http.Error(w, "fixture not found", http.StatusInternalServerError)
			return
		}
	} else if resp.Body != nil {
		responseBody, err = json.Marshal(resp.Body)
		if err != nil {
			m.t.Logf("MockServer: failed to marshal response body: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if resp.StatusCode != 0 {
		w.WriteHeader(resp.StatusCode)
	}
	if responseBody != nil {
		w.Write(responseBody)
	}
}

func (m *MockServer) findRoute(r *http.Request) (MockResponse, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check exact routes first
	key := routeKey{Method: r.Method, Path: r.URL.Path}
	if resp, ok := m.routes[key]; ok {
		return resp, true
	}

	// Check pattern routes
	for _, pr := range m.patternRoutes {
		if pr.Method == r.Method && pr.Pattern.MatchString(r.URL.Path) {
			return pr.Response, true
		}
	}

	return MockResponse{}, false
}

func (m *MockServer) loadFixture(filename string) ([]byte, error) {
	path := filepath.Join(m.fixturesDir, filename)
	return os.ReadFile(path)
}

// --- Convenience methods for common API routes ---

// WithDashboardsList sets up the mock server to return a list of dashboards.
func (m *MockServer) WithDashboardsList(fixture string) *MockServer {
	return m.On(http.MethodGet, "/api/dashboards", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithDashboardsGet sets up the mock server to return a dashboard by ID.
func (m *MockServer) WithDashboardsGet(fixture string) *MockServer {
	return m.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/dashboards/[^/]+$`), MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithDashboardsCreate sets up the mock server to accept dashboard creation.
func (m *MockServer) WithDashboardsCreate(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/dashboards", MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixture,
	})
}

// WithDashboardsUpdate sets up the mock server to accept dashboard updates.
func (m *MockServer) WithDashboardsUpdate(fixture string) *MockServer {
	return m.OnPattern(http.MethodPut, regexp.MustCompile(`^/api/dashboards/[^/]+$`), MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithDashboardsDelete sets up the mock server to accept dashboard deletion.
func (m *MockServer) WithDashboardsDelete() *MockServer {
	return m.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/dashboards/[^/]+$`), MockResponse{
		StatusCode: http.StatusNoContent,
	})
}

// WithCheckRulesList sets up the mock server to return a list of check rules.
func (m *MockServer) WithCheckRulesList(fixture string) *MockServer {
	return m.On(http.MethodGet, "/api/alerting/check-rules", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithCheckRulesGet sets up the mock server to return a check rule by ID.
func (m *MockServer) WithCheckRulesGet(fixture string) *MockServer {
	return m.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/alerting/check-rules/[^/]+$`), MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithCheckRulesCreate sets up the mock server to accept check rule creation.
func (m *MockServer) WithCheckRulesCreate(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/alerting/check-rules", MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixture,
	})
}

// WithCheckRulesDelete sets up the mock server to accept check rule deletion.
func (m *MockServer) WithCheckRulesDelete() *MockServer {
	return m.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/alerting/check-rules/[^/]+$`), MockResponse{
		StatusCode: http.StatusNoContent,
	})
}

// WithViewsList sets up the mock server to return a list of views.
func (m *MockServer) WithViewsList(fixture string) *MockServer {
	return m.On(http.MethodGet, "/api/views", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithViewsGet sets up the mock server to return a view by ID.
func (m *MockServer) WithViewsGet(fixture string) *MockServer {
	return m.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/views/[^/]+$`), MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithViewsCreate sets up the mock server to accept view creation.
func (m *MockServer) WithViewsCreate(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/views", MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixture,
	})
}

// WithViewsDelete sets up the mock server to accept view deletion.
func (m *MockServer) WithViewsDelete() *MockServer {
	return m.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/views/[^/]+$`), MockResponse{
		StatusCode: http.StatusNoContent,
	})
}

// WithSyntheticChecksList sets up the mock server to return a list of synthetic checks.
func (m *MockServer) WithSyntheticChecksList(fixture string) *MockServer {
	return m.On(http.MethodGet, "/api/synthetic-checks", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithSyntheticChecksGet sets up the mock server to return a synthetic check by ID.
func (m *MockServer) WithSyntheticChecksGet(fixture string) *MockServer {
	return m.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/synthetic-checks/[^/]+$`), MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
	})
}

// WithSyntheticChecksCreate sets up the mock server to accept synthetic check creation.
func (m *MockServer) WithSyntheticChecksCreate(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/synthetic-checks", MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixture,
	})
}

// WithSyntheticChecksDelete sets up the mock server to accept synthetic check deletion.
func (m *MockServer) WithSyntheticChecksDelete() *MockServer {
	return m.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/synthetic-checks/[^/]+$`), MockResponse{
		StatusCode: http.StatusNoContent,
	})
}

// --- Import endpoint helpers ---

// WithDashboardImport sets up the mock server to accept dashboard imports.
func (m *MockServer) WithDashboardImport(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/import/dashboard", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
		Validator:  RequireAuthHeader,
	})
}

// WithCheckRuleImport sets up the mock server to accept check rule imports.
func (m *MockServer) WithCheckRuleImport(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/import/check-rule", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
		Validator:  RequireAuthHeader,
	})
}

// WithViewImport sets up the mock server to accept view imports.
func (m *MockServer) WithViewImport(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/import/view", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
		Validator:  RequireAuthHeader,
	})
}

// WithSyntheticCheckImport sets up the mock server to accept synthetic check imports.
func (m *MockServer) WithSyntheticCheckImport(fixture string) *MockServer {
	return m.On(http.MethodPost, "/api/import/synthetic-check", MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixture,
		Validator:  RequireAuthHeader,
	})
}

// WithNotFound sets up any unmatched route to return 404.
func (m *MockServer) WithNotFound(fixture string) *MockServer {
	return m.OnDefault(func(w http.ResponseWriter, r *http.Request) {
		body, err := m.loadFixture(fixture)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write(body)
	})
}

// WithUnauthorized sets up the mock server to return 401 Unauthorized.
func (m *MockServer) WithUnauthorized(fixture string) *MockServer {
	return m.OnDefault(func(w http.ResponseWriter, r *http.Request) {
		body, err := m.loadFixture(fixture)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(body)
	})
}

// SetupTestEnv sets environment variables for testing and returns a cleanup function.
func SetupTestEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
}

// CaptureStdout captures stdout during the execution of the given function.
// It returns the captured output as a string.
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()

	// Save original stdout
	oldStdout := os.Stdout

	// Create a pipe
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Redirect stdout to the pipe
	os.Stdout = w

	// Run the function
	fn()

	// Close writer and restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	return buf.String()
}
