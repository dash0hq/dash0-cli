package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dashboardYAML = `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`

const checkRuleYAML = `apiVersion: dash0.com/v1alpha1
kind: CheckRule
id: b2c3d4e5-6789-01bc-def0-234567890abc
name: High Error Rate
expression: up == 0
`

const dashboardNoIdentifierYAML = `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: no-id-dashboard
spec:
  display:
    name: No ID Dashboard
`

const prometheusRuleYAML = `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-rules
  labels:
    dash0.com/id: shared-id
spec:
  groups:
    - name: group-a
      rules:
        - alert: HighErrorRate
          expr: errors > 0
        - alert: DiskFull
          expr: disk > 0
`

const configMapYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: unrelated-configmap
data:
  foo: bar
`

// TestBuildSnapshotFromRef_UnrecognizedKindIsTolerated is a regression test
// for a bug where a document whose kind Dash0 doesn't recognize (e.g. a
// stray Kubernetes ConfigMap sitting in a scanned scope, unrelated to any
// Dash0 asset) hard-failed the whole snapshot build via
// dash0yaml.ExtractIdentifier's "unsupported kind" error, masking whatever
// real Dash0 deletions --since should have detected in the same scope.
func TestBuildSnapshotFromRef_UnrecognizedKindIsTolerated(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", dashboardYAML)
	writeFile(t, repo.Dir, "configmap.yaml", configMapYAML)
	commitAll(t, repo.Dir, "add assets")

	snap, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	assert.Equal(t, "dashboard.yaml", snap.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"}])
	assert.NotContains(t, snap.NoIdentifier, "configmap.yaml", "an unrecognized kind must be silently ignored, not tracked as a no-identifier document")
	assert.Len(t, snap.Identifiers, 1)
}

func TestBuildSnapshotFromRef_BasicIdentifiers(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", dashboardYAML)
	writeFile(t, repo.Dir, "checkrule.yaml", checkRuleYAML)
	commitAll(t, repo.Dir, "add assets")

	snap, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	assert.Equal(t, "dashboard.yaml", snap.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"}])
	assert.Equal(t, "checkrule.yaml", snap.Identifiers[IdentifierKey{Kind: "checkrule", Identifier: "b2c3d4e5-6789-01bc-def0-234567890abc"}])
	assert.True(t, snap.Paths["dashboard.yaml"])
	assert.True(t, snap.Paths["checkrule.yaml"])
	assert.Empty(t, snap.NoIdentifier)
}

func TestBuildSnapshotFromRef_NoIdentifierTracked(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", dashboardNoIdentifierYAML)
	commitAll(t, repo.Dir, "add asset")

	snap, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	require.Contains(t, snap.NoIdentifier, "dashboard.yaml")
	assert.Equal(t, NoIdentifierDoc{Kind: "dashboard", FilePath: "dashboard.yaml"}, snap.NoIdentifier["dashboard.yaml"])
	assert.Empty(t, snap.Identifiers)
}

func TestBuildSnapshotFromRef_MultiDocument(t *testing.T) {
	repo := testRepo(t)
	multiDoc := dashboardYAML + "---\n" + checkRuleYAML
	writeFile(t, repo.Dir, "combined.yaml", multiDoc)
	commitAll(t, repo.Dir, "add combined")

	snap, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	assert.Equal(t, "combined.yaml", snap.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"}])
	assert.Equal(t, "combined.yaml#1", snap.Identifiers[IdentifierKey{Kind: "checkrule", Identifier: "b2c3d4e5-6789-01bc-def0-234567890abc"}])
}

func TestBuildSnapshotFromRef_PrometheusRuleAlerts(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "rules.yaml", prometheusRuleYAML)
	commitAll(t, repo.Dir, "add rules")

	snap, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	require.Contains(t, snap.PrometheusAlertsByIdentifier, "shared-id")
	assert.Len(t, snap.PrometheusAlertsByIdentifier["shared-id"], 2)
}

func TestBuildSnapshotFromDisk_MatchesGitSide(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", dashboardYAML)
	writeFile(t, repo.Dir, ".hidden/skip.yaml", dashboardYAML)
	writeFile(t, repo.Dir, "notes.txt", "not yaml")

	snap, err := BuildSnapshotFromDisk(context.Background(), repo.Dir, repo.Dir)
	require.NoError(t, err)

	assert.Contains(t, snap.Identifiers, IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"})
	assert.True(t, snap.Paths["dashboard.yaml"])
	assert.NotContains(t, snap.Paths, ".hidden/skip.yaml")
	assert.Len(t, snap.Identifiers, 1)
}

func TestBuildSnapshotFromDisk_SingleFileScope(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", dashboardYAML)
	writeFile(t, repo.Dir, "checkrule.yaml", checkRuleYAML)

	snap, err := BuildSnapshotFromDisk(context.Background(), repo.Dir+"/dashboard.yaml", repo.Dir)
	require.NoError(t, err)

	assert.Len(t, snap.Identifiers, 1)
	assert.Contains(t, snap.Identifiers, IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"})
	assert.Equal(t, "dashboard.yaml", snap.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"}])
}

// TestBuildSnapshotFromDisk_SingleFileScopeIgnoresExtension is a regression
// test for a bug where a single-file -f target without a .yaml/.yml
// extension (e.g. -f config.json) was silently excluded from the disk side
// of a --since scan, even though apply's own single-file create/update path
// (readMultiDocumentYAML) has no extension check at all and would read the
// exact same file just fine.
func TestBuildSnapshotFromDisk_SingleFileScopeIgnoresExtension(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "config.json", dashboardYAML)

	snap, err := BuildSnapshotFromDisk(context.Background(), repo.Dir+"/config.json", repo.Dir)
	require.NoError(t, err)

	assert.Len(t, snap.Identifiers, 1)
	assert.Contains(t, snap.Identifiers, IdentifierKey{Kind: "dashboard", Identifier: "a1b2c3d4-5678-90ab-cdef-1234567890ab"}, "a single-file scope must be scanned regardless of its extension")
}

func TestBuildSnapshotFromDisk_PathsAlignWithRepoRootWhenScopeIsSubdirectory(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "sub/dashboard.yaml", dashboardYAML)

	snap, err := BuildSnapshotFromDisk(context.Background(), repo.Dir+"/sub", repo.Dir)
	require.NoError(t, err)

	assert.True(t, snap.Paths["sub/dashboard.yaml"], "disk-side paths must be repo-root-relative, matching the git ls-tree side, even when scope is a subdirectory")
}
