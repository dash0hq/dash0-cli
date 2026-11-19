package asset

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

func TestPreferredKey(t *testing.T) {
	const both = "metadata: {labels: {dash0.com/origin: the-origin, dash0.com/id: the-id}}"
	const idOnly = "metadata: {labels: {dash0.com/id: the-id}}"
	tests := []struct {
		name       string
		definition dash0api.AssetMetadata
		document   string
		want       string
	}{
		{"origin wins when the kind upserts by both", &dash0api.SloDefinition{}, both, "the-origin"},
		{"id when the document has no origin", &dash0api.SloDefinition{}, idOnly, "the-id"},
		{"id-only kind ignores the origin label", &dash0api.ViewDefinition{}, both, "the-id"},
		{"origin-only kind ignores the id", &dash0api.NotificationChannelDefinition{}, idOnly, ""},
		{"no identifier", &dash0api.SloDefinition{}, "metadata: {}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := dash0yaml.Unmarshal([]byte(tt.document), tt.definition); err != nil {
				t.Fatal(err)
			}
			if got := PreferredKey(tt.definition); got != tt.want {
				t.Errorf("PreferredKey = %q, want %q", got, tt.want)
			}
		})
	}
}
