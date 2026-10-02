package client

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responseRoundTripper func(*http.Request) (*http.Response, error)

func (f responseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type trackedResponseBody struct {
	io.Reader
	closed bool
}

func (b *trackedResponseBody) Close() error { b.closed = true; return nil }

func TestAssetResponseTransport(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, contentType, body string
		status                                int
		wantError, validated                  bool
	}{
		{"valid", http.MethodGet, "/api/views/id", "application/json", `{"kind":"Dash0View","metadata":{"name":"test"},"spec":{},"futureField":42}`, http.StatusOK, false, true},
		{"empty list", http.MethodGet, "/api/views", "application/json", `[]`, http.StatusOK, false, true},
		{"optional null", http.MethodGet, "/api/views", "application/json", `[{"id":"id","type":"logs","name":null}]`, http.StatusOK, false, true},
		{"missing metadata", http.MethodGet, "/api/views/id", "application/json", `{"kind":"Dash0View","spec":{}}`, http.StatusOK, true, true},
		{"null spec", http.MethodGet, "/api/views/id", "application/json", `{"kind":"Dash0View","metadata":{"name":"test"},"spec":null}`, http.StatusOK, true, true},
		{"null list item", http.MethodGet, "/api/views", "application/json", `[null]`, http.StatusOK, true, true},
		{"wrong content type", http.MethodGet, "/api/views", "text/plain", `[]`, http.StatusOK, true, true},
		{"created response", http.MethodPost, "/api/views", "application/json", `{"metadata":`, http.StatusCreated, true, true},
		{"HTTP error unchanged", http.MethodGet, "/api/views/id", "application/json", `not JSON`, http.StatusBadRequest, false, false},
		{"delete unchanged", http.MethodDelete, "/api/views/id", "application/json", ``, http.StatusOK, false, false},
		{"other endpoint unchanged", http.MethodPost, "/api/logs", "application/json", `not JSON`, http.StatusOK, false, false},
		{"team membership unchanged", http.MethodPost, "/api/teams/id/members", "application/json", `{}`, http.StatusOK, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := &trackedResponseBody{Reader: strings.NewReader(tc.body)}
			transport := &assetResponseTransport{base: responseRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {tc.contentType}}, Body: original}, nil
			})}
			req, err := http.NewRequest(tc.method, "https://example.test"+tc.path+"?dataset=private", nil)
			require.NoError(t, err)
			resp, err := transport.RoundTrip(req)
			require.NoError(t, err, "decoding errors must not trigger SDK transport retries")
			assert.Equal(t, tc.validated, original.closed)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if tc.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.method+" "+tc.path)
				assert.NotContains(t, err.Error(), "private")
				var malformed *malformedResponseError
				assert.ErrorAs(t, err, &malformed)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(body))
			}
		})
	}
}

func TestMalformedResponseError_PreservesCauseAndField(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test/api/views/id", nil)
	require.NoError(t, err)
	cause := assetResponseSchema(req).validate([]byte(`{"kind":"Dash0View","metadata":{"name":123},"spec":{}}`))
	require.Error(t, cause)
	wrapped := &malformedResponseError{asset: "view", method: req.Method, path: req.URL.Path, cause: cause}
	var typeErr *json.UnmarshalTypeError
	require.True(t, errors.As(wrapped, &typeErr))
	assert.Contains(t, wrapped.Error(), `field "metadata.name": expected string, received number`)
	assert.Contains(t, wrapped.Error(), "Hint:")
}
