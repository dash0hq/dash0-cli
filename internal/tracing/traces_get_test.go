package tracing

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTracesGetCmd() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	tracesCmd := NewTracesCmd()
	root.AddCommand(tracesCmd)
	var getCmd *cobra.Command
	for _, c := range tracesCmd.Commands() {
		if c.Name() == "get" {
			getCmd = c
			break
		}
	}
	return root, getCmd
}

func TestGetRequiresExperimentalFlag(t *testing.T) {
	root, _ := newTracesGetCmd()
	root.SetArgs([]string{"traces", "get", "0af7651916cd43dd8448eb211c80319c"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
}

func TestGetRequiresTraceID(t *testing.T) {
	root, _ := newTracesGetCmd()
	root.SetArgs([]string{"-X", "traces", "get"})
	err := root.Execute()
	require.Error(t, err)
}

func TestGetValidatesTraceIDLength(t *testing.T) {
	root, _ := newTracesGetCmd()
	root.SetArgs([]string{"-X", "traces", "get", "too-short"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 hex characters")
}

func TestGetValidatesTraceIDHex(t *testing.T) {
	root, _ := newTracesGetCmd()
	root.SetArgs([]string{"-X", "traces", "get", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "valid hex")
}

func TestGetSkipHeaderFlag(t *testing.T) {
	_, getCmd := newTracesGetCmd()
	flag := getCmd.Flags().Lookup("skip-header")
	require.NotNil(t, flag, "--skip-header flag should be registered on traces get")
	assert.Equal(t, "false", flag.DefValue)
}

func TestGetColumnFlag(t *testing.T) {
	_, getCmd := newTracesGetCmd()
	flag := getCmd.Flags().Lookup("column")
	require.NotNil(t, flag, "--column flag should be registered on traces get")
	assert.Equal(t, "[]", flag.DefValue)
}

func TestGetFollowSpanLinksFlag(t *testing.T) {
	_, getCmd := newTracesGetCmd()
	flag := getCmd.Flags().Lookup("follow-span-links")
	require.NotNil(t, flag)
	assert.Equal(t, "1h", flag.NoOptDefVal)
}

func TestBuildTree(t *testing.T) {
	spans := []flatTraceSpan{
		{spanID: "child2", parentID: "root", name: "child2"},
		{spanID: "root", parentID: "", name: "root"},
		{spanID: "child1", parentID: "root", name: "child1"},
		{spanID: "grandchild", parentID: "child1", name: "grandchild"},
	}

	ordered := buildTree(spans)
	require.Len(t, ordered, 4)

	assert.Equal(t, "root", ordered[0].name)
	assert.Equal(t, "child2", ordered[1].name)
	assert.Equal(t, "child1", ordered[2].name)
	assert.Equal(t, "grandchild", ordered[3].name)
}

func TestBuildTreeOrphanedSpans(t *testing.T) {
	spans := []flatTraceSpan{
		{spanID: "a", parentID: "missing", name: "orphan"},
		{spanID: "b", parentID: "", name: "root"},
	}

	ordered := buildTree(spans)
	require.Len(t, ordered, 2)
	// Orphan becomes root-level since parent is not in set
	assert.Equal(t, "orphan", ordered[0].name)
}

func TestExtractLinkedTraceIDs(t *testing.T) {
	svc := "test-service"
	linkTraceID := []byte{0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd}
	linkSpanID := []byte{0xaa, 0xbb, 0x11, 0x22, 0x33, 0x44, 0x00, 0x11}

	resourceSpans := []dash0api.ResourceSpans{
		{
			Resource: dash0api.Resource{
				Attributes: []dash0api.KeyValue{
					{Key: "service.name", Value: dash0api.AnyValue{StringValue: &svc}},
				},
			},
			ScopeSpans: []dash0api.ScopeSpans{
				{
					Spans: []dash0api.Span{
						{
							TraceId: []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xaa, 0xbb, 0xcc, 0xdd, 0xaa, 0xbb, 0xcc, 0xdd, 0xaa, 0xbb, 0xcc, 0xdd},
							SpanId:  []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
							Name:    "test",
							Links: []dash0api.SpanLink{
								{TraceId: linkTraceID, SpanId: linkSpanID},
							},
						},
					},
				},
			},
		},
	}

	seen := map[string]bool{"aabbccddaabbccddaabbccddaabbccdd": true}
	newIDs := extractLinkedTraceIDs(resourceSpans, seen)
	require.Len(t, newIDs, 1)
	assert.Equal(t, "eeff00112233445566778899aabbccdd", newIDs[0])
}

func TestExtractLinkedTraceIDsSkipsSeen(t *testing.T) {
	linkTraceID := []byte{0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd}

	resourceSpans := []dash0api.ResourceSpans{
		{
			Resource: dash0api.Resource{},
			ScopeSpans: []dash0api.ScopeSpans{
				{
					Spans: []dash0api.Span{
						{
							TraceId: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
							SpanId:  []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
							Name:    "test",
							Links:   []dash0api.SpanLink{{TraceId: linkTraceID, SpanId: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}},
						},
					},
				},
			},
		},
	}

	// Already seen
	seen := map[string]bool{"eeff00112233445566778899aabbccdd": true}
	newIDs := extractLinkedTraceIDs(resourceSpans, seen)
	assert.Empty(t, newIDs)
}
