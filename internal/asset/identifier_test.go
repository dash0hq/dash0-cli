package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractIdentifier_ReadsOnlyTheKindsUpsertField is a regression test for
// silent no-op deletions. ExtractIdentifier used to fall back across fields,
// so a View carrying only dash0.com/origin reported that origin as its
// identifier -- but ImportView upserts by dash0.com/id alone and the CLI
// strips origin outbound, so nothing live ever had it. --since then recorded
// the asset, deleted by an identifier that matches nothing, got a 404, and
// printed "was already deleted" while the asset stayed live. Returning ""
// instead routes it to the NoIdentifier hard-fail written for this case.
//
// Only Dash0Team and Dash0SpamFilter genuinely accept either field.
func TestExtractIdentifier_ReadsOnlyTheKindsUpsertField(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want string
	}{
		{"View ignores origin", "kind: View\nmetadata:\n  labels:\n    dash0.com/origin: o\n", ""},
		{"SyntheticCheck ignores origin", "kind: SyntheticCheck\nmetadata:\n  labels:\n    dash0.com/origin: o\n", ""},
		{"PersesDashboard ignores origin", "kind: PersesDashboard\nmetadata:\n  labels:\n    dash0.com/origin: o\n", ""},
		{"PrometheusRule ignores origin", "kind: PrometheusRule\nmetadata:\n  labels:\n    dash0.com/origin: o\n", ""},
		{"Dashboard ignores the id label", "kind: Dashboard\nmetadata:\n  labels:\n    dash0.com/id: i\n", ""},
		{"CheckRule ignores the id label", "kind: CheckRule\nmetadata:\n  labels:\n    dash0.com/id: i\n", ""},
		{"Dash0NotificationChannel ignores id", "kind: Dash0NotificationChannel\nmetadata:\n  labels:\n    dash0.com/id: i\n", ""},

		{"Dash0Team falls back to id", "kind: Dash0Team\nmetadata:\n  labels:\n    dash0.com/id: i\n", "i"},
		{"Dash0SpamFilter falls back to id", "kind: Dash0SpamFilter\nmetadata:\n  labels:\n    dash0.com/id: i\n", "i"},
		{"Dash0Team prefers origin", "kind: Dash0Team\nmetadata:\n  labels:\n    dash0.com/id: i\n    dash0.com/origin: o\n", "o"},

		{"View reads the id label", "kind: View\nmetadata:\n  labels:\n    dash0.com/id: i\n", "i"},
		{"Dashboard reads dash0Extensions.id", "kind: Dashboard\nmetadata:\n  dash0Extensions:\n    id: i\n", "i"},
		{"CheckRule reads the top-level id", "kind: CheckRule\nid: i\n", "i"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractIdentifier([]byte(tc.doc))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
