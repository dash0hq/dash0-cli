//go:build e2e

// Package e2e exercises the real `dash0` binary against a real `git` binary
// inside a container, closing the gap unit tests (in-process) and
// integration tests (real temp git repo, but still in-process against a
// mocked HTTP server) can't: `--since` shells out to `git` rather than using
// a Go git library, so this is the only tier that proves that process
// boundary actually works.
//
// Scoped to `dash0 apply --since` for now; `dash0 diff --since` coverage
// will be added once that command exists.
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

const dash0ImageTag = "dash0-cli-e2e-test:latest"

var buildImageOnce sync.Once
var buildImageErr error

// buildE2EImage cross-compiles a linux binary for the host's container
// architecture and builds the e2e test image from it, once per test binary
// run. The go build step runs on the host (not inside Docker) specifically
// so it can resolve this module's go.mod replace directive (a local sibling
// checkout of dash0-api-client-go) without needing that directory inside
// the Docker build context too.
func buildE2EImage(t *testing.T) {
	t.Helper()
	buildImageOnce.Do(func() {
		buildImageErr = doBuildE2EImage()
	})
	if buildImageErr != nil {
		t.Fatalf("failed to build e2e image: %v", buildImageErr)
	}
}

func doBuildE2EImage() error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to determine test/e2e directory")
	}
	e2eDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(e2eDir))
	binaryPath := filepath.Join(e2eDir, "dash0")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/dash0")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to cross-compile dash0 for linux/%s: %w\n%s", runtime.GOARCH, err, out)
	}
	defer os.Remove(binaryPath)

	dockerCmd := exec.Command("docker", "build", "-t", dash0ImageTag, e2eDir)
	if out, err := dockerCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build e2e Docker image: %w\n%s", err, out)
	}
	return nil
}

// startContainer starts a container from the pre-built e2e image, with
// hostPort (the mock Dash0 API server's port, on the host) reachable from
// inside the container at gitutilHostInternal:hostPort.
func startContainer(ctx context.Context, t *testing.T, hostPort int) testcontainers.Container {
	t.Helper()
	buildE2EImage(t)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:           dash0ImageTag,
			HostAccessPorts: []int{hostPort},
		},
		Started: true,
	}
	container, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("failed to start e2e container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate e2e container: %v", err)
		}
	})
	return container
}
