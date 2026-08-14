package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevoke_SendsClientID(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/oauth/revoke", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseForm())
		gotForm = r.Form
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ok := Revoke(server.URL, "client-abc-123", "dash0_rt_test")
	assert.True(t, ok)
	require.NotNil(t, gotForm)
	assert.Equal(t, "dash0_rt_test", gotForm.Get("token"))
	assert.Equal(t, "refresh_token", gotForm.Get("token_type_hint"))
	assert.Equal(t, "client-abc-123", gotForm.Get("client_id"))
}

func TestRevoke_AcceptsNoContentStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	assert.True(t, Revoke(server.URL, "client-abc-123", "dash0_rt_test"))
}

func TestRevoke_FailureReturnsFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	assert.False(t, Revoke(server.URL, "client-abc-123", "dash0_rt_test"))
}

func TestRevoke_NoOpsOnEmptyArgs(t *testing.T) {
	assert.True(t, Revoke("", "client-abc-123", "dash0_rt_test"), "empty apiURL should no-op")
	assert.True(t, Revoke("https://api.example.com", "client-abc-123", ""), "empty refreshToken should no-op")
}
