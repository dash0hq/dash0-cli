package diff

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func viewDoc(filePath, id string) asset.Document {
	return asset.Document{Kind: "View", ID: id, FilePath: filePath, Raw: []byte("kind: View\nmetadata:\n  name: v\n  labels:\n    dash0.com/id: " + id + "\n")}
}

func updatePlan(displayKind, name, key string, before, proposed any) docPlan {
	return docPlan{displayKind: displayKind, name: name, plan: asset.ImportPlan{Key: key, Action: asset.ActionUpdated, Before: before, Proposed: proposed}}
}

func TestBuildPlan_Create(t *testing.T) {
	planned := []plannedDoc{{
		doc:   viewDoc("view.yaml", ""),
		plans: []docPlan{{displayKind: "View", name: "my-view", plan: asset.ImportPlan{Action: asset.ActionCreated, Proposed: &dash0api.ViewDefinition{}}}},
	}}

	plan, err := buildPlan(planned, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"view.yaml"}, plan.Files())
	changes := plan.Changes("view.yaml")
	require.Len(t, changes, 1)
	assert.Equal(t, "create", changes[0].Op)
	assert.Equal(t, "my-view", changes[0].Name)
	assert.Equal(t, 1, plan.PendingCount())
}

// apply restores a soft-deleted asset, so diff reports it as a create.
func TestBuildPlan_RestoreIsCreate(t *testing.T) {
	planned := []plannedDoc{{
		doc:   viewDoc("view.yaml", "view-id"),
		plans: []docPlan{{displayKind: "View", name: "v", plan: asset.ImportPlan{Key: "view-id", Action: asset.ActionRestored, Before: &dash0api.ViewDefinition{}, Proposed: &dash0api.ViewDefinition{}}}},
	}}

	plan, err := buildPlan(planned, nil)
	require.NoError(t, err)
	assert.Equal(t, "create", plan.Changes("view.yaml")[0].Op)
}

func TestBuildPlan_UpdateWithRealChange(t *testing.T) {
	before := &dash0api.ViewDefinition{}
	before.Metadata.Name = "old-name"
	after := &dash0api.ViewDefinition{}
	after.Metadata.Name = "new-name"
	planned := []plannedDoc{{doc: viewDoc("view.yaml", "view-id"), plans: []docPlan{updatePlan("View", "new-name", "view-id", before, after)}}}

	plan, err := buildPlan(planned, nil)
	require.NoError(t, err)
	change := plan.Changes("view.yaml")[0]
	assert.Equal(t, "update", change.Op)
	assert.Contains(t, change.Diff, "+  name: new-name")
	assert.Equal(t, 1, plan.PendingCount())
}

// The two values differ only in the order of a set-like slice, so the semantic
// comparison decides "no change", not pointer identity.
func TestBuildPlan_UpdateWithNoChange(t *testing.T) {
	before := &dash0api.SyntheticCheckDefinition{}
	before.Metadata.Name = "same-name"
	before.Spec.Schedule.Locations = []string{"de-frankfurt", "us-oregon"}
	after := &dash0api.SyntheticCheckDefinition{}
	after.Metadata.Name = "same-name"
	after.Spec.Schedule.Locations = []string{"us-oregon", "de-frankfurt"}
	planned := []plannedDoc{{doc: viewDoc("check.yaml", "check-id"), plans: []docPlan{updatePlan("SyntheticCheck", "same-name", "check-id", before, after)}}}

	plan, err := buildPlan(planned, nil)
	require.NoError(t, err)
	change := plan.Changes("check.yaml")[0]
	assert.Equal(t, "update", change.Op)
	assert.Empty(t, change.Diff)
	assert.Equal(t, 0, plan.PendingCount(), "an unchanged update must not count as a pending difference")
}

// A PrometheusRule CRD with one alert and a recording rule gives both assets
// the CRD's id, so each update must keep its own diff.
func TestBuildPlan_SharedIDKeepsEachDiff(t *testing.T) {
	ruleBefore := &dash0api.PrometheusAlertRule{Name: "g - A", Expression: "up == 0"}
	ruleAfter := &dash0api.PrometheusAlertRule{Name: "g - A", Expression: "up == 1"}
	recording := &dash0api.RecordingRule{}
	recording.Metadata.Name = "mixed"
	planned := []plannedDoc{{
		doc: asset.Document{Kind: "PrometheusRule", FilePath: "rules.yaml"},
		plans: []docPlan{
			updatePlan("CheckRule", "g - A", "shared-id", ruleBefore, ruleAfter),
			updatePlan("RecordingRule", "mixed", "shared-id", recording, recording),
		},
	}}

	plan, err := buildPlan(planned, nil)
	require.NoError(t, err)
	byKind := map[string]string{}
	for _, change := range plan.Changes("rules.yaml") {
		byKind[change.Kind] = change.Diff
	}
	assert.Contains(t, byKind["CheckRule"], "+expression: up == 1")
	assert.Empty(t, byKind["RecordingRule"])
}

func TestBuildPlan_Deletion(t *testing.T) {
	sincePlan := &gitutil.SincePlan{
		Plan:  gitutil.DeletionPlan{ByIdentifier: []gitutil.Deletion{{Kind: "dashboard", Identifier: "gone-id", Path: "removed.yaml"}}},
		Names: map[string]string{"removed.yaml": "Old Dashboard"},
	}

	plan, err := buildPlan(nil, sincePlan)
	require.NoError(t, err)
	require.Equal(t, []string{"removed.yaml"}, plan.Files())
	change := plan.Changes("removed.yaml")[0]
	assert.Equal(t, "delete", change.Op)
	assert.Equal(t, "Old Dashboard", change.Name)
	assert.Equal(t, 1, plan.PendingCount())
}

// apply --since skips deleting a check rule that -f still declares, so diff
// must not report that deletion either.
func TestBuildPlan_DeclaredCheckRuleIsNotDeleted(t *testing.T) {
	sincePlan := &gitutil.SincePlan{
		Plan:                 gitutil.DeletionPlan{ByIdentifier: []gitutil.Deletion{{Kind: "checkrule", Identifier: "still-declared", Path: "old.yaml"}}},
		DeclaredCheckRuleIDs: map[string]bool{"still-declared": true},
	}

	plan, err := buildPlan(nil, sincePlan)
	require.NoError(t, err)
	assert.Empty(t, plan.Files())
	assert.Equal(t, 0, plan.PendingCount())
}

func TestBuildPlan_MixedCreateAndDeleteInSameFile(t *testing.T) {
	planned := []plannedDoc{{
		doc:   viewDoc("assets.yaml", ""),
		plans: []docPlan{{displayKind: "View", name: "survivor", plan: asset.ImportPlan{Action: asset.ActionCreated, Proposed: &dash0api.ViewDefinition{}}}},
	}}
	sincePlan := &gitutil.SincePlan{
		Plan:  gitutil.DeletionPlan{ByIdentifier: []gitutil.Deletion{{Kind: "checkrule", Identifier: "gone-id", Path: "assets.yaml#1"}}},
		Names: map[string]string{"assets.yaml#1": "Removed Rule"},
	}

	plan, err := buildPlan(planned, sincePlan)
	require.NoError(t, err)
	require.Equal(t, []string{"assets.yaml"}, plan.Files(), "the create and the deletion from the same file must merge into one entry")
	require.Len(t, plan.Changes("assets.yaml"), 2)
	assert.Equal(t, 2, plan.PendingCount())
}

// A single-file target has no file grouping, so the text lists every change
// in the same identifier order as the JSON, even when the deletion is keyed by
// its path in git.
func TestRenderReportText_SingleFileMixesDeletionsInIdentifierOrder(t *testing.T) {
	planned := []plannedDoc{{
		doc:   viewDoc("/abs/assets.yaml", "b-view"),
		plans: []docPlan{{displayKind: "View", name: "survivor", plan: asset.ImportPlan{Key: "b-view", Action: asset.ActionCreated, Proposed: &dash0api.ViewDefinition{}}}},
	}}
	sincePlan := &gitutil.SincePlan{
		Plan:  gitutil.DeletionPlan{ByIdentifier: []gitutil.Deletion{{Kind: "view", Identifier: "a-view", Path: "assets.yaml#1"}}},
		Names: map[string]string{"assets.yaml#1": "removed"},
	}
	plan, err := buildPlan(planned, sincePlan)
	require.NoError(t, err)

	output := testutil.CaptureStdout(t, func() {
		require.NoError(t, renderReportText(plan, false, plan.PendingCount()))
	})

	assert.Equal(t, "Delete View \"removed\" (a-view)\nCreate View \"survivor\" (b-view)\n", output)
}
