package skill

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// whatever it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	return buf.String()
}

func runShowCmd(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := newShowCmd()
	cmd.SetArgs(args)
	var runErr error
	out := captureStdout(t, func() {
		runErr = cmd.Execute()
	})
	return out, runErr
}

func TestShowNoArgumentPrintsSkillMDExactly(t *testing.T) {
	want, err := SkillMD()
	require.NoError(t, err)

	out, err := runShowCmd(t, nil)
	require.NoError(t, err)
	assert.Equal(t, want, out)
}

func TestShowTopicPrintsMatchingReferenceExactly(t *testing.T) {
	want, err := TopicContent("dashboards")
	require.NoError(t, err)

	out, err := runShowCmd(t, []string{"dashboards"})
	require.NoError(t, err)
	assert.Equal(t, want, out)
}

func TestShowEveryTopicSucceeds(t *testing.T) {
	for _, entry := range Manifest {
		t.Run(entry.Topic, func(t *testing.T) {
			out, err := runShowCmd(t, []string{entry.Topic})
			require.NoError(t, err)
			assert.NotEmpty(t, out)
		})
	}
}

func TestShowUnknownTopicListsValidTopics(t *testing.T) {
	_, err := runShowCmd(t, []string{"bogus-topic"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown skill topic "bogus-topic"`)
	for _, name := range TopicNames() {
		assert.Contains(t, err.Error(), name)
	}
}

func TestShowRejectsMoreThanOneArgument(t *testing.T) {
	cmd := newShowCmd()
	cmd.SetArgs([]string{"dashboards", "extra"})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestShowCommandHasNoOutputFlag(t *testing.T) {
	cmd := newShowCmd()
	assert.Nil(t, cmd.Flags().Lookup("output"))
	assert.Nil(t, cmd.Flags().ShorthandLookup("o"))
}
