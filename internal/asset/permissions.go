package asset

import (
	"sort"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// permissionSortKey returns a deterministic sort key for a single permission
// entry, built from whichever of role/team/user is set. Exactly one of the
// three is expected to be present per entry; the prefixes mirror the
// `dash0.com/sharing` annotation format ("role:...", "team:...", "user:...").
func permissionSortKey(role, teamId, userId *string) string {
	switch {
	case role != nil:
		return "role:" + *role
	case teamId != nil:
		return "team:" + *teamId
	case userId != nil:
		return "user:" + *userId
	default:
		return ""
	}
}

// SortViewPermissions sorts a view's spec.permissions lexicographically.
// The server does not guarantee a stable order for this field (see issue
// #231), so the CLI normalizes it before display or diffing to avoid
// spurious reordering noise across repeated get/apply cycles.
func SortViewPermissions(view *dash0api.ViewDefinition) {
	if view == nil || view.Spec.Permissions == nil {
		return
	}
	perms := *view.Spec.Permissions
	sort.SliceStable(perms, func(i, j int) bool {
		return permissionSortKey(perms[i].Role, perms[i].TeamId, perms[i].UserId) <
			permissionSortKey(perms[j].Role, perms[j].TeamId, perms[j].UserId)
	})
}

// SortSyntheticCheckPermissions is the SyntheticCheck equivalent of
// SortViewPermissions.
func SortSyntheticCheckPermissions(check *dash0api.SyntheticCheckDefinition) {
	if check == nil || check.Spec.Permissions == nil {
		return
	}
	perms := *check.Spec.Permissions
	sort.SliceStable(perms, func(i, j int) bool {
		return permissionSortKey(perms[i].Role, perms[i].TeamId, perms[i].UserId) <
			permissionSortKey(perms[j].Role, perms[j].TeamId, perms[j].UserId)
	})
}
