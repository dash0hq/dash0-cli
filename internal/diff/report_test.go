package diff

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func viewDoc(filePath, id string) asset.Document {
	return asset.Document{Kind: "View", ID: id, FilePath: filePath, Raw: []byte("kind: View\nmetadata:\n  name: v\n  labels:\n    dash0.com/id: " + id + "\n")}
}

func TestBuildRows_Create(t *testing.T) {
	planned := []plannedDoc{
		{
			doc: viewDoc("view.yaml", ""),
			plans: []docPlan{
				{displayKind: "View", name: "my-view", id: "", before: nil, after: &dash0api.ViewDefinition{}},
			},
		},
	}

	rowsByFile, files, err := buildRows(planned, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"view.yaml"}, files)
	require.Len(t, rowsByFile["view.yaml"], 1)
	row := rowsByFile["view.yaml"][0]
	assert.Equal(t, "create", row.op)
	assert.Equal(t, "my-view", row.name)
	assert.Equal(t, 1, pendingCount(rowsByFile))
}

func TestBuildRows_UpdateWithRealChange(t *testing.T) {
	before := &dash0api.ViewDefinition{}
	before.Metadata.Name = "old-name"
	after := &dash0api.ViewDefinition{}
	after.Metadata.Name = "new-name"

	planned := []plannedDoc{
		{
			doc: viewDoc("view.yaml", "view-id"),
			plans: []docPlan{
				{displayKind: "View", name: "new-name", id: "view-id", before: before, after: after},
			},
		},
	}

	rowsByFile, _, err := buildRows(planned, nil)
	require.NoError(t, err)
	row := rowsByFile["view.yaml"][0]
	assert.Equal(t, "update", row.op)
	assert.True(t, row.changed)
	assert.Equal(t, 1, pendingCount(rowsByFile))
}

// TestBuildRows_UpdateWithNoChange uses a realistic near-miss -- two
// distinct *ViewDefinition values whose spec.filter entries are in a
// different order -- rather than the same pointer on both sides, so this
// proves the semantic comparison engine (asset.HasDifference, backed by
// dash0yaml.Equivalent's order-independent slice comparison) is what
// decides "no change" here, not incidental pointer/struct identity.
func TestBuildRows_UpdateWithNoChange(t *testing.T) {
	before := &dash0api.ViewDefinition{}
	before.Metadata.Name = "same-name"
	before.Spec.Filter = &dash0api.FilterCriteria{
		{Key: "service.name", Operator: "is_set"},
		{Key: "severity", Operator: "is_set"},
	}
	after := &dash0api.ViewDefinition{}
	after.Metadata.Name = "same-name"
	after.Spec.Filter = &dash0api.FilterCriteria{
		{Key: "severity", Operator: "is_set"},
		{Key: "service.name", Operator: "is_set"},
	}

	planned := []plannedDoc{
		{
			doc: viewDoc("view.yaml", "view-id"),
			plans: []docPlan{
				{displayKind: "View", name: "same-name", id: "view-id", before: before, after: after},
			},
		},
	}

	rowsByFile, _, err := buildRows(planned, nil)
	require.NoError(t, err)
	row := rowsByFile["view.yaml"][0]
	assert.Equal(t, "update", row.op)
	assert.False(t, row.changed)
	assert.Equal(t, 0, pendingCount(rowsByFile), "an unchanged update must not count as a pending difference")
}

func TestBuildRows_Deletion(t *testing.T) {
	sincePlan := &gitutil.SincePlan{
		Plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "dashboard", Identifier: "gone-id", Path: "removed.yaml"},
			},
		},
		Names: map[string]string{"removed.yaml": "Old Dashboard"},
	}

	rowsByFile, files, err := buildRows(nil, sincePlan)
	require.NoError(t, err)
	require.Equal(t, []string{"removed.yaml"}, files)
	row := rowsByFile["removed.yaml"][0]
	assert.Equal(t, "delete", row.op)
	assert.Equal(t, "Old Dashboard", row.name)
	assert.Equal(t, 1, pendingCount(rowsByFile))
}

func TestBuildRows_MixedCreateAndDeleteInSameFile(t *testing.T) {
	planned := []plannedDoc{
		{
			doc: viewDoc("assets.yaml", ""),
			plans: []docPlan{
				{displayKind: "View", name: "survivor", id: "", before: nil, after: &dash0api.ViewDefinition{}},
			},
		},
	}
	sincePlan := &gitutil.SincePlan{
		Plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "checkrule", Identifier: "gone-id", Path: "assets.yaml#1"},
			},
		},
		Names: map[string]string{"assets.yaml#1": "Removed Rule"},
	}

	rowsByFile, files, err := buildRows(planned, sincePlan)
	require.NoError(t, err)
	require.Equal(t, []string{"assets.yaml"}, files, "the create and the deletion from the same file must merge into one entry")
	require.Len(t, rowsByFile["assets.yaml"], 2)
	assert.Equal(t, 2, pendingCount(rowsByFile))
}
