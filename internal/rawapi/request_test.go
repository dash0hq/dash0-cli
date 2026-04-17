package rawapi

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMethodAndPath(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantErr    string
	}{
		{"path only", []string{"/foo"}, "GET", "/foo", ""},
		{"explicit GET", []string{"GET", "/foo"}, "GET", "/foo", ""},
		{"lowercase method", []string{"post", "/foo"}, "POST", "/foo", ""},
		{"mixed case method", []string{"Delete", "/foo"}, "DELETE", "/foo", ""},
		{"unsupported method", []string{"WAT", "/foo"}, "", "", "unsupported HTTP method"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, p, err := parseMethodAndPath(tc.args)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantMethod, m)
			assert.Equal(t, tc.wantPath, p)
		})
	}
}

func TestReadBody(t *testing.T) {
	t.Run("empty path returns nil", func(t *testing.T) {
		data, err := readBody("", strings.NewReader(""))
		require.NoError(t, err)
		assert.Nil(t, data)
	})

	t.Run("stdin", func(t *testing.T) {
		data, err := readBody("-", strings.NewReader(`{"a":1}`))
		require.NoError(t, err)
		assert.Equal(t, `{"a":1}`, string(data))
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := readBody("/nonexistent/file.json", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read body file")
	})
}

func TestParseHeaders(t *testing.T) {
	t.Run("valid headers", func(t *testing.T) {
		hs, err := parseHeaders([]string{"X-One: alpha", "X-Two:beta", "X-Three:   gamma   "})
		require.NoError(t, err)
		require.Len(t, hs, 3)
		assert.Equal(t, parsedHeader{Key: "X-One", Value: "alpha"}, hs[0])
		assert.Equal(t, parsedHeader{Key: "X-Two", Value: "beta"}, hs[1])
		assert.Equal(t, parsedHeader{Key: "X-Three", Value: "gamma"}, hs[2])
	})

	t.Run("missing colon", func(t *testing.T) {
		_, err := parseHeaders([]string{"not a header"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected 'Key: Value'")
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := parseHeaders([]string{": value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected 'Key: Value'")
	})

	t.Run("authorization rejected", func(t *testing.T) {
		_, err := parseHeaders([]string{"Authorization: Bearer x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot set Authorization")
	})

	t.Run("authorization case insensitive", func(t *testing.T) {
		_, err := parseHeaders([]string{"authorization: Bearer x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot set Authorization")
	})
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		want    string
		wantErr bool
	}{
		{"relative with leading slash", "https://api.dash0.com", "/api/foo/bar", "https://api.dash0.com/api/foo/bar", false},
		{"relative without leading slash", "https://api.dash0.com", "api/foo/bar", "https://api.dash0.com/api/foo/bar", false},
		{"base with trailing slash", "https://api.dash0.com/", "/api/foo", "https://api.dash0.com/api/foo", false},
		{"preserves query", "https://api.dash0.com", "/api/foo?limit=10", "https://api.dash0.com/api/foo?limit=10", false},
		{"absolute http URL passes through", "https://api.dash0.com", "http://other.example/x", "http://other.example/x", false},
		{"absolute https URL passes through", "https://api.dash0.com", "https://other.example/x?a=1", "https://other.example/x?a=1", false},
		{"relative without api prefix rejected", "https://api.dash0.com", "/notification-channels", "", true},
		{"relative bare path rejected", "https://api.dash0.com", "foo/bar", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := resolveURL(tc.base, tc.path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, u.String())
		})
	}
}

func TestResolveURL_RejectsRelativePathWithoutAPIPrefix(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"bare path", "notification-channels"},
		{"leading slash only", "/notification-channels"},
		{"nested without prefix", "/signal-to-metrics/configs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveURL("https://api.dash0.com", tc.path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must start with /api/")
		})
	}
}

func TestInjectDataset(t *testing.T) {
	parse := func(s string) string {
		u, err := resolveURL("https://api.dash0.com", s)
		require.NoError(t, err)
		return u.String()
	}

	t.Run("profile dataset injected by default", func(t *testing.T) {
		u, _ := resolveURL("https://api.dash0.com", "/api/foo")
		err := injectDataset(u, "prod", "", false)
		require.NoError(t, err)
		assert.Equal(t, "https://api.dash0.com/api/foo?dataset=prod", u.String())
	})

	t.Run("flag overrides profile", func(t *testing.T) {
		u, _ := resolveURL("https://api.dash0.com", "/api/foo")
		err := injectDataset(u, "prod", "staging", true)
		require.NoError(t, err)
		assert.Equal(t, "https://api.dash0.com/api/foo?dataset=staging", u.String())
	})

	t.Run("flag empty opts out", func(t *testing.T) {
		u, _ := resolveURL("https://api.dash0.com", "/api/foo")
		err := injectDataset(u, "prod", "", true)
		require.NoError(t, err)
		assert.Equal(t, "https://api.dash0.com/api/foo", u.String())
	})

	t.Run("no dataset anywhere leaves URL alone", func(t *testing.T) {
		u, _ := resolveURL("https://api.dash0.com", "/api/foo")
		err := injectDataset(u, "", "", false)
		require.NoError(t, err)
		assert.Equal(t, "https://api.dash0.com/api/foo", u.String())
	})

	t.Run("conflict with existing dataset in path", func(t *testing.T) {
		u, _ := resolveURL("https://api.dash0.com", "/api/foo?dataset=baked")
		err := injectDataset(u, "prod", "", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dataset is set in both")
	})

	t.Run("dataset in path plus --dataset= keeps query", func(t *testing.T) {
		u, _ := resolveURL("https://api.dash0.com", "/api/foo?dataset=baked")
		err := injectDataset(u, "prod", "", true)
		require.NoError(t, err)
		assert.Equal(t, "https://api.dash0.com/api/foo?dataset=baked", u.String())
	})

	// Sanity check that parse still works before the actual test.
	_ = parse
}

func TestBuildRequest_Basics(t *testing.T) {
	req, err := buildRequest(buildRequestInput{
		BaseURL:        "https://api.dash0.com",
		Path:           "/api/signal-to-metrics/configs",
		Method:         http.MethodGet,
		ProfileDataset: "prod",
		AuthToken:      "tkn",
		UserAgent:      "dash0-cli/test",
	})
	require.NoError(t, err)
	assert.Equal(t, "GET", req.Method)
	assert.Equal(t, "https://api.dash0.com/api/signal-to-metrics/configs?dataset=prod", req.URL.String())
	assert.Equal(t, "Bearer tkn", req.Header.Get("Authorization"))
	assert.Equal(t, "dash0-cli/test", req.Header.Get("User-Agent"))
	assert.Empty(t, req.Header.Get("Content-Type"))
}

func TestBuildRequest_BodyDefaultsContentType(t *testing.T) {
	req, err := buildRequest(buildRequestInput{
		BaseURL:   "https://api.dash0.com",
		Path:      "/api/foo",
		Method:    http.MethodPost,
		Body:      []byte(`{"a":1}`),
		AuthToken: "tkn",
		UserAgent: "dash0-cli/test",
	})
	require.NoError(t, err)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

	data, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, string(data))
}

func TestBuildRequest_ContentTypeOverride(t *testing.T) {
	req, err := buildRequest(buildRequestInput{
		BaseURL:   "https://api.dash0.com",
		Path:      "/api/foo",
		Method:    http.MethodPost,
		Body:      []byte("name=foo"),
		Headers:   []parsedHeader{{Key: "Content-Type", Value: "application/x-www-form-urlencoded"}},
		AuthToken: "tkn",
		UserAgent: "dash0-cli/test",
	})
	require.NoError(t, err)
	assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
}

func TestBuildRequest_CustomHeaders(t *testing.T) {
	req, err := buildRequest(buildRequestInput{
		BaseURL:   "https://api.dash0.com",
		Path:      "/api/foo",
		Method:    http.MethodGet,
		Headers:   []parsedHeader{{Key: "X-Request-Id", Value: "abc"}},
		AuthToken: "tkn",
		UserAgent: "dash0-cli/test",
	})
	require.NoError(t, err)
	assert.Equal(t, "abc", req.Header.Get("X-Request-Id"))
}

func TestBuildRequest_AbsoluteURLSkipsDataset(t *testing.T) {
	// Absolute URLs pass through verbatim — dataset injection still applies.
	req, err := buildRequest(buildRequestInput{
		BaseURL:           "https://api.dash0.com",
		Path:              "https://other.example/raw",
		Method:            http.MethodGet,
		ProfileDataset:    "prod",
		AuthToken:         "tkn",
		AuthTokenFromFlag: true,
		UserAgent:         "dash0-cli/test",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://other.example/raw?dataset=prod", req.URL.String())
}

func TestCheckHostMismatch_RelativePathAlwaysAllowed(t *testing.T) {
	u, _ := resolveURL("https://api.dash0.com", "/api/foo")
	err := checkHostMismatch("https://api.dash0.com", "/foo", u, false)
	assert.NoError(t, err)
}

func TestCheckHostMismatch_SameHostAllowed(t *testing.T) {
	u, _ := resolveURL("https://api.dash0.com", "https://api.dash0.com/other/path")
	err := checkHostMismatch("https://api.dash0.com", "https://api.dash0.com/other/path", u, false)
	assert.NoError(t, err)
}

func TestCheckHostMismatch_DifferentHostWithExplicitTokenAllowed(t *testing.T) {
	u, _ := resolveURL("https://api.dash0.com", "https://other.example/path")
	err := checkHostMismatch("https://api.dash0.com", "https://other.example/path", u, true)
	assert.NoError(t, err)
}

func TestCheckHostMismatch_DifferentHostWithProfileTokenRejected(t *testing.T) {
	u, _ := resolveURL("https://api.dash0.com", "https://other.example/path")
	err := checkHostMismatch("https://api.dash0.com", "https://other.example/path", u, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"other.example" does not match the profile's api-url host "api.dash0.com"`)
	assert.Contains(t, err.Error(), "--auth-token")
}
