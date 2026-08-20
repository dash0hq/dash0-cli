package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dash0hq/dash0-api-client-go/profiles"
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

	ok := Revoke(RevokeRequest{APIURL: server.URL, ClientID: "client-abc-123", RefreshToken: "dash0_rt_test"})
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

	assert.True(t, Revoke(RevokeRequest{APIURL: server.URL, ClientID: "client-abc-123", RefreshToken: "dash0_rt_test"}))
}

func TestRevoke_FailureReturnsFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	assert.False(t, Revoke(RevokeRequest{APIURL: server.URL, ClientID: "client-abc-123", RefreshToken: "dash0_rt_test"}))
}

func TestRevoke_NoOpsOnEmptyArgs(t *testing.T) {
	assert.True(t, Revoke(RevokeRequest{ClientID: "client-abc-123", RefreshToken: "dash0_rt_test"}), "empty apiURL should no-op")
	assert.True(t, Revoke(RevokeRequest{APIURL: "https://api.example.com", ClientID: "client-abc-123"}), "empty refreshToken should no-op")
}

func TestRevoke_OmitsEmptyClientID(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.Form
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	assert.True(t, Revoke(RevokeRequest{APIURL: server.URL, RefreshToken: "dash0_rt_test"}))
	require.NotNil(t, gotForm)
	_, present := gotForm["client_id"]
	assert.False(t, present, "empty client_id must be omitted, not sent as an empty parameter")
}

func TestRevoke_FallsBackToDCRCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DASH0_CONFIG_DIR", dir)

	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.Form
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store, err := profiles.NewOAuthClientStore()
	require.NoError(t, err)
	require.NoError(t, store.Put(server.URL, profiles.OAuthClientRecord{
		ClientID:    "cached-from-dcr",
		RedirectURI: "http://localhost/cb",
	}))

	assert.True(t, Revoke(RevokeRequest{APIURL: server.URL, RefreshToken: "dash0_rt_test"}))
	require.NotNil(t, gotForm)
	assert.Equal(t, "cached-from-dcr", gotForm.Get("client_id"))
}
