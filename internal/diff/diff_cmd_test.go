package diff

import (
	"context"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"

	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withExperimentalFlag registers a local --experimental/-X bool flag on cmd,
// standing in for the persistent flag main.go registers on the real root
// command. Standalone tests that construct NewDiffCmd() directly (with no
// root parent) need this for experimental.RequireExperimental's
// cmd.Flags().GetBool("experimental") lookup to succeed instead of silently
// treating the flag as unregistered (=> always disabled).
func withExperimentalFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("experimental", "X", false, "Enable experimental features")
}

func TestDiff_RequiresExperimentalFlag(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "does-not-need-to-exist.yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
	assert.Contains(t, err.Error(), "--experimental")
}

func TestDiff_RequiresFile(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file is required")
}

func TestDiff_RejectsSinceWithStdin(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "-", "--since", "HEAD~1", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stdin")
}

func TestDiff_RejectsUnexpectedArguments(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "assets.yaml", "extra-arg", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

func TestPendingDifferencesError_Error(t *testing.T) {
	assert.Equal(t, "1 difference pending", (&PendingDifferencesError{Count: 1}).Error())
	assert.Equal(t, "3 differences pending", (&PendingDifferencesError{Count: 3}).Error())
}

// Every kind apply accepts must have a planDocument case, or diff fails with
// "unsupported kind" on a document apply would write. The document does not
// parse, so no case reaches the API.
func TestPlanDocument_PlansEveryKindApplyAccepts(t *testing.T) {
	for _, kind := range asset.ValidKinds {
		t.Run(kind, func(t *testing.T) {
			_, err := planDocument(context.Background(), nil, asset.Document{Kind: kind, Raw: []byte("{")}, nil, false)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "unsupported kind")
		})
	}
}

// unmergeableDatasetClient serves a stored dataset that cannot be marshaled
// to JSON, so merging the document onto it for the preview fails.
type unmergeableDatasetClient struct {
	dash0api.Client
}

func (unmergeableDatasetClient) GetDataset(context.Context, string) (*dash0api.DatasetDefinition, error) {
	outOfRange := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	return &dash0api.DatasetDefinition{
		ApiVersion: asset.DatasetAPIVersion,
		Metadata: dash0api.DatasetMetadata{
			Name:        "Prod",
			Annotations: &dash0api.DatasetAnnotations{Dash0ComcreatedAt: &outOfRange},
		},
	}, nil
}

// The preview of a dataset update is the document merged onto the stored
// dataset, so a merge that fails aborts the preview instead of showing a
// wrong diff.
func TestPlanDocument_DatasetUpdateFailsWhenThePreviewMergeFails(t *testing.T) {
	raw := []byte("apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: Prod\n  labels:\n    dash0.com/id: prod\n")
	_, err := planDocument(context.Background(), unmergeableDatasetClient{}, asset.Document{Kind: "Dash0Dataset", Raw: raw}, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "year outside of range")
}
