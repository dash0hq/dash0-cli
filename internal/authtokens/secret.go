package authtokens

import (
	"fmt"
	"io"
	"log/slog"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

const redacted = "[REDACTED]"

// secret holds an auth token's secret. Every way of printing, encoding or
// logging it yields "[REDACTED]"; only reveal returns the value, and only the
// two delivery paths (an interactive terminal and --secret-file) call it.
type secret struct{ value string }

func (held secret) reveal() string { return held.value }
func (held secret) empty() bool    { return held.value == "" }

func (secret) String() string               { return redacted }
func (secret) GoString() string             { return redacted }
func (secret) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, redacted) }
func (secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
func (secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }
func (secret) MarshalYAML() (any, error)    { return redacted, nil }
func (secret) LogValue() slog.Value         { return slog.StringValue(redacted) }

// takeSecret moves spec.token out of an API response, so nothing that later
// prints, diffs or encodes the response can carry the plaintext.
func takeSecret(token *dash0api.AuthTokenResponseDefinition) secret {
	if token == nil || token.Spec.Token == nil {
		return secret{}
	}
	extracted := secret{value: *token.Spec.Token}
	token.Spec.Token = nil
	return extracted
}
