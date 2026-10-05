package members

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestMemberValues_Role(t *testing.T) {
	for _, tt := range []struct {
		name   string
		labels *dash0api.MemberLabels
		want   string
	}{
		{name: "no labels"},
		{name: "missing role", labels: &dash0api.MemberLabels{}},
		{name: "admin", labels: &dash0api.MemberLabels{Dash0Comrole: dash0api.Ptr("admin")}, want: "admin"},
		{name: "basic member", labels: &dash0api.MemberLabels{Dash0Comrole: dash0api.Ptr("basic_member")}, want: "basic_member"},
		{name: "future role", labels: &dash0api.MemberLabels{Dash0Comrole: dash0api.Ptr("future_role")}, want: "future_role"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			member := &dash0api.MemberDefinition{Metadata: dash0api.MemberMetadata{Labels: tt.labels}}
			assert.Equal(t, tt.want, MemberValues(member, "")["role"])
		})
	}
}
