package debuginfo

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func artifactsWithBuildIDs(buildIDs ...string) []sourceMapArtifact {
	artifacts := make([]sourceMapArtifact, len(buildIDs))
	for index, buildID := range buildIDs {
		artifacts[index] = sourceMapArtifact{BuildID: buildID, Script: fmt.Sprintf("script-%d.js", index)}
	}
	return artifacts
}

func reportedBuildIDs(results []uploadResult) []string {
	buildIDs := make([]string, len(results))
	for index, result := range results {
		buildIDs[index] = result.BuildID
	}
	return buildIDs
}

func TestUploadInOrder_UploadsConcurrentlyAndReportsInOrder(t *testing.T) {
	artifacts := artifactsWithBuildIDs("a", "b", "c", "d")
	var mutex sync.Mutex
	inFlight, peak := 0, 0
	upload := func(artifact sourceMapArtifact) (uploadOutcome, string, error) {
		mutex.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mutex.Unlock()
		// Later artifacts finish first, so the report order cannot come from the finish order.
		time.Sleep(map[string]time.Duration{"a": 40, "b": 30, "c": 20, "d": 10}[artifact.BuildID] * time.Millisecond)
		mutex.Lock()
		inFlight--
		mutex.Unlock()
		return outcomeUploaded, "", nil
	}
	var reported []uploadResult

	err := uploadInOrder(artifacts, 4, upload, func(result uploadResult) { reported = append(reported, result) })

	require.NoError(t, err)
	assert.Equal(t, 4, peak)
	assert.Equal(t, []string{"a", "b", "c", "d"}, reportedBuildIDs(reported))
}

func TestUploadInOrder_StartsNoUploadAfterAFailure(t *testing.T) {
	artifacts := artifactsWithBuildIDs("a", "fails", "c", "d")
	var mutex sync.Mutex
	var started []string
	upload := func(artifact sourceMapArtifact) (uploadOutcome, string, error) {
		mutex.Lock()
		started = append(started, artifact.BuildID)
		mutex.Unlock()
		if artifact.BuildID == "fails" {
			return "", "", errors.New("upload failed")
		}
		return outcomeUploaded, "", nil
	}
	var reported []uploadResult

	err := uploadInOrder(artifacts, 1, upload, func(result uploadResult) { reported = append(reported, result) })

	require.EqualError(t, err, "upload failed")
	assert.Equal(t, []string{"a", "fails"}, started)
	assert.Equal(t, []string{"a"}, reportedBuildIDs(reported))
}

func TestUploadInOrder_ReportsUploadsInFlightWhenOneFails(t *testing.T) {
	artifacts := artifactsWithBuildIDs("fails", "slow")
	upload := func(artifact sourceMapArtifact) (uploadOutcome, string, error) {
		if artifact.BuildID == "fails" {
			return "", "", errors.New("upload failed")
		}
		time.Sleep(20 * time.Millisecond)
		return outcomeUploaded, "", nil
	}
	var reported []uploadResult

	err := uploadInOrder(artifacts, 2, upload, func(result uploadResult) { reported = append(reported, result) })

	require.EqualError(t, err, "upload failed")
	assert.Equal(t, []string{"slow"}, reportedBuildIDs(reported))
}

func TestUploadInOrder_UploadsASharedBuildIDOnce(t *testing.T) {
	artifacts := artifactsWithBuildIDs("a", "b", "a")
	var mutex sync.Mutex
	uploads := 0
	upload := func(sourceMapArtifact) (uploadOutcome, string, error) {
		mutex.Lock()
		uploads++
		mutex.Unlock()
		return outcomeUploaded, "", nil
	}
	var reported []uploadResult

	err := uploadInOrder(artifacts, 4, upload, func(result uploadResult) { reported = append(reported, result) })

	require.NoError(t, err)
	assert.Equal(t, 2, uploads)
	require.Len(t, reported, 3)
	assert.Equal(t, outcomeSkipped, reported[2].Status)
	assert.Equal(t, "same source map as script-0.js", reported[2].Reason)
}

func TestUploadInOrder_ReturnsEveryFailureInFlight(t *testing.T) {
	artifacts := artifactsWithBuildIDs("a", "b")
	upload := func(artifact sourceMapArtifact) (uploadOutcome, string, error) {
		return "", "", errors.New("upload of " + artifact.BuildID + " failed")
	}
	var reported []uploadResult

	err := uploadInOrder(artifacts, 2, upload, func(result uploadResult) { reported = append(reported, result) })

	require.EqualError(t, err, "upload of a failed\nupload of b failed")
	assert.Empty(t, reported)
}
