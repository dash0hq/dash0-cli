package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A new kind must decide whether `apply --since` warns before it deletes an
// id-only document of that kind, instead of silently getting no warning.
func TestIDOnlyDeleteWarning_EveryValidKindDecides(t *testing.T) {
	var warningKinds []string
	for _, kind := range ValidKinds {
		_, needed, decided := idOnlyDeleteWarningOf(kind)
		assert.Truef(t, decided, "kind %q has no case in idOnlyDeleteWarningOf", kind)
		if needed {
			warningKinds = append(warningKinds, kind)
			assert.NotEmptyf(t, IDOnlyDeleteWarning(kind, `"name" (id)`), "kind %q", kind)
		}
	}
	assert.ElementsMatch(t, []string{"slo", "spamfilter", "team", "generativeaievaluation", "signaltometrics", "sampling"}, warningKinds)
}

func TestIdentifiedByIDOnly(t *testing.T) {
	idOnly := []byte("kind: Dash0Team\nmetadata:\n  labels:\n    dash0.com/id: team-1\n")
	withOrigin := []byte("kind: Dash0Team\nmetadata:\n  labels:\n    dash0.com/id: team-1\n    dash0.com/origin: backend\n")

	got, err := IdentifiedByIDOnly("team", idOnly)
	assert.NoError(t, err)
	assert.True(t, got)

	got, err = IdentifiedByIDOnly("team", withOrigin)
	assert.NoError(t, err)
	assert.False(t, got)

	got, err = IdentifiedByIDOnly("view", idOnly)
	assert.NoError(t, err)
	assert.False(t, got, "a kind without the warning is never reported")

	_, err = IdentifiedByIDOnly("team", []byte("metadata: ["))
	assert.ErrorContains(t, err, "failed to determine team identifier source")
}
