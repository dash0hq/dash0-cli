package authtokens

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

const canary = "auth_CANARY_unit"

func TestSecret_RedactedEverywhereButReveal(t *testing.T) {
	value := secret{value: canary}
	wrapped := struct{ Secret secret }{value}

	rendered := []string{
		fmt.Sprint(value), fmt.Sprintf("%v %+v %#v %s %q %x", value, value, value, value, value, value),
		fmt.Sprintf("%+v %#v", wrapped, wrapped), fmt.Sprintf("%v", &value),
	}
	for _, encode := range []func(any) ([]byte, error){json.Marshal, sigsyaml.Marshal, yamlv3.Marshal} {
		data, err := encode(wrapped)
		require.NoError(t, err)
		rendered = append(rendered, string(data))
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("token", "secret", value, "wrapped", wrapped)
	slog.New(slog.NewTextHandler(&logs, nil)).Info("token", "secret", value)
	rendered = append(rendered, logs.String())

	for _, text := range rendered {
		assert.NotContains(t, text, canary)
		assert.Contains(t, text, redacted)
	}
	assert.Equal(t, canary, value.reveal())
}

func TestTakeSecret_MovesTokenOutOfResponse(t *testing.T) {
	token := canary
	response := &dash0api.AuthTokenResponseDefinition{Spec: dash0api.AuthTokenResponseSpec{Token: &token}}

	value := takeSecret(response)

	assert.Equal(t, canary, value.reveal())
	assert.Nil(t, response.Spec.Token)
	data, err := json.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(data), canary)
	assert.True(t, takeSecret(response).empty())
}
