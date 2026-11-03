package members

import (
	"slices"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			member := &dash0api.MemberDefinition{Metadata: dash0api.MemberMetadata{Labels: tt.labels}}
			assert.Equal(t, tt.want, MemberValues(member, "")["role"])
		})
	}
}

func TestMemberTable_LongRoles(t *testing.T) {
	for _, columns := range [][]string{nil, {"role", "id"}} {
		for _, skipHeader := range []bool{false, true} {
			cols, err := ResolveMemberListColumns(columns)
			require.NoError(t, err)
			originalColumns := slices.Clone(cols)
			var items []*dash0api.MemberDefinition
			for _, role := range []string{"admin", "organization_admin", ""} {
				items = append(items, &dash0api.MemberDefinition{
					Metadata: dash0api.MemberMetadata{Labels: &dash0api.MemberLabels{
						Dash0Comid: dash0api.Ptr("member-id"), Dash0Comrole: dash0api.Ptr(role),
					}},
				})
			}
			output := testutil.CaptureStdout(t, func() {
				err = RenderMembersTable(items, cols, skipHeader, "")
			})
			require.NoError(t, err)
			assert.Equal(t, originalColumns, cols, "rendering must not change caller-owned columns")
			assert.Contains(t, output, "organization_admin")
			lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
			if !skipHeader {
				lines = lines[1:]
			}
			require.Len(t, lines, 3)
			idColumn := strings.Index(lines[0], "member-id")
			for _, line := range lines[1:] {
				assert.Equal(t, idColumn, strings.Index(line, "member-id"), "ID columns must stay aligned")
			}
		}
	}
}
