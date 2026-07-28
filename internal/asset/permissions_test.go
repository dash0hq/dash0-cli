package asset

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string { return &s }

func TestSortViewPermissions_SortsLexicographically(t *testing.T) {
	view := &dash0api.ViewDefinition{
		Spec: dash0api.ViewSpec{
			Permissions: &[]dash0api.ViewPermission{
				{Role: strPtr("basic_member"), Actions: []dash0api.ViewAction{dash0api.ViewsRead}},
				{TeamId: strPtr("team_123"), Actions: []dash0api.ViewAction{dash0api.ViewsRead}},
				{Role: strPtr("admin"), Actions: []dash0api.ViewAction{dash0api.ViewsWrite}},
				{UserId: strPtr("alice@example.com"), Actions: []dash0api.ViewAction{dash0api.ViewsRead}},
			},
		},
	}

	SortViewPermissions(view)

	perms := *view.Spec.Permissions
	assert.Equal(t, "admin", *perms[0].Role)
	assert.Equal(t, "basic_member", *perms[1].Role)
	assert.Equal(t, "team_123", *perms[2].TeamId)
	assert.Equal(t, "alice@example.com", *perms[3].UserId)
}

func TestSortViewPermissions_StableOrderRegardlessOfInputOrder(t *testing.T) {
	buildAndSort := func(order []dash0api.ViewPermission) []dash0api.ViewPermission {
		view := &dash0api.ViewDefinition{Spec: dash0api.ViewSpec{Permissions: &order}}
		SortViewPermissions(view)
		return *view.Spec.Permissions
	}

	a := dash0api.ViewPermission{Role: strPtr("admin")}
	b := dash0api.ViewPermission{Role: strPtr("basic_member")}
	c := dash0api.ViewPermission{TeamId: strPtr("team_123")}

	sortedOne := buildAndSort([]dash0api.ViewPermission{a, b, c})
	sortedTwo := buildAndSort([]dash0api.ViewPermission{c, a, b})
	sortedThree := buildAndSort([]dash0api.ViewPermission{b, c, a})

	assert.Equal(t, sortedOne, sortedTwo)
	assert.Equal(t, sortedOne, sortedThree)
}

func TestSortViewPermissions_NilSafe(t *testing.T) {
	assert.NotPanics(t, func() { SortViewPermissions(nil) })

	view := &dash0api.ViewDefinition{}
	assert.NotPanics(t, func() { SortViewPermissions(view) })
	assert.Nil(t, view.Spec.Permissions)
}

func TestSortSyntheticCheckPermissions_SortsLexicographically(t *testing.T) {
	check := &dash0api.SyntheticCheckDefinition{
		Spec: dash0api.SyntheticCheckSpec{
			Permissions: &[]dash0api.SyntheticCheckPermission{
				{Role: strPtr("basic_member")},
				{TeamId: strPtr("team_123")},
				{Role: strPtr("admin")},
				{UserId: strPtr("alice@example.com")},
			},
		},
	}

	SortSyntheticCheckPermissions(check)

	perms := *check.Spec.Permissions
	assert.Equal(t, "admin", *perms[0].Role)
	assert.Equal(t, "basic_member", *perms[1].Role)
	assert.Equal(t, "team_123", *perms[2].TeamId)
	assert.Equal(t, "alice@example.com", *perms[3].UserId)
}

func TestSortSyntheticCheckPermissions_NilSafe(t *testing.T) {
	assert.NotPanics(t, func() { SortSyntheticCheckPermissions(nil) })

	check := &dash0api.SyntheticCheckDefinition{}
	assert.NotPanics(t, func() { SortSyntheticCheckPermissions(check) })
	assert.Nil(t, check.Spec.Permissions)
}
