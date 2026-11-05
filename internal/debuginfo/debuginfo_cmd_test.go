package debuginfo

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testAuthToken = "auth_test_token"

type fakeDebuginfoService struct {
	debuginfopb.UnimplementedDebuginfoServiceServer

	strategy  debuginfopb.UploadInstructions_UploadStrategy
	signedURL string
	// failBuildIDs makes ShouldInitiateUpload fail with that error for the build ID.
	failBuildIDs map[string]error
	// existingInInitiate makes InitiateUpload answer AlreadyExists, as when another upload won the race.
	existingInInitiate bool

	mutex          sync.Mutex
	stored         map[string][]byte
	pending        map[string][]byte
	authorizations []string
	types          []debuginfopb.DebuginfoType
}

func (service *fakeDebuginfoService) record(ctx context.Context, debuginfoType debuginfopb.DebuginfoType) {
	incoming, _ := metadata.FromIncomingContext(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.authorizations = append(service.authorizations, incoming.Get("authorization")...)
	service.types = append(service.types, debuginfoType)
}

func (service *fakeDebuginfoService) ShouldInitiateUpload(ctx context.Context, request *debuginfopb.ShouldInitiateUploadRequest) (*debuginfopb.ShouldInitiateUploadResponse, error) {
	service.record(ctx, request.GetType())
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if err := service.failBuildIDs[request.GetBuildId()]; err != nil {
		return nil, err
	}
	if _, exists := service.stored[request.GetBuildId()]; exists && !request.GetForce() {
		return &debuginfopb.ShouldInitiateUploadResponse{Reason: "Debuginfo already exists."}, nil
	}
	return &debuginfopb.ShouldInitiateUploadResponse{ShouldInitiateUpload: true}, nil
}

func (service *fakeDebuginfoService) InitiateUpload(ctx context.Context, request *debuginfopb.InitiateUploadRequest) (*debuginfopb.InitiateUploadResponse, error) {
	service.record(ctx, request.GetType())
	if service.existingInInitiate {
		return nil, status.Error(codes.AlreadyExists, "already uploaded by another client")
	}
	return &debuginfopb.InitiateUploadResponse{UploadInstructions: &debuginfopb.UploadInstructions{
		BuildId:        request.GetBuildId(),
		UploadId:       "upload-" + request.GetBuildId(),
		UploadStrategy: service.strategy,
		SignedUrl:      service.signedURL + "/" + request.GetBuildId(),
		Type:           request.GetType(),
	}}, nil
}

func (service *fakeDebuginfoService) Upload(stream debuginfopb.DebuginfoService_UploadServer) error {
	header, err := stream.Recv()
	if err != nil {
		return err
	}
	service.record(stream.Context(), header.GetInfo().GetType())
	var body []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		body = append(body, chunk.GetChunkData()...)
	}
	service.mutex.Lock()
	service.pending[header.GetInfo().GetBuildId()] = body
	service.mutex.Unlock()
	return stream.SendAndClose(&debuginfopb.UploadResponse{BuildId: header.GetInfo().GetBuildId(), Size: uint64(len(body))})
}

func (service *fakeDebuginfoService) MarkUploadFinished(ctx context.Context, request *debuginfopb.MarkUploadFinishedRequest) (*debuginfopb.MarkUploadFinishedResponse, error) {
	service.record(ctx, request.GetType())
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.stored[request.GetBuildId()] = service.pending[request.GetBuildId()]
	return &debuginfopb.MarkUploadFinishedResponse{}, nil
}

func startFakeDebuginfoService(t *testing.T, strategy debuginfopb.UploadInstructions_UploadStrategy) (*fakeDebuginfoService, string) {
	t.Helper()
	service := &fakeDebuginfoService{strategy: strategy, stored: map[string][]byte{}, pending: map[string][]byte{}}

	signedURLServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.Method != http.MethodPut {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		service.mutex.Lock()
		service.pending[request.URL.Path[1:]] = body
		service.mutex.Unlock()
	}))
	t.Cleanup(signedURLServer.Close)
	service.signedURL = signedURLServer.URL

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	debuginfopb.RegisterDebuginfoServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return service, "http://" + listener.Addr().String()
}

func newTestRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewDebuginfoCmd())
	return root
}

func runDebuginfoCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	testutil.SetupTestEnv(t)
	cmd := newTestRootCmd()
	cmd.SetArgs(append([]string{"-X", "debuginfo", "upload"}, args...))
	var err error
	output := testutil.CaptureStdout(t, func() { err = cmd.Execute() })
	return output, err
}

func writeBuildOutput(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "assets/app.js", "console.log('app')")
	writeFile(t, dir, "assets/app.js.map", `{"version":3,"sources":["app.ts"],"mappings":""}`)
	return dir
}

func TestUpload_TypeSelection(t *testing.T) {
	dir := writeBuildOutput(t)
	cases := map[string]string{
		"":           "--type is required",
		"pdb":        `unknown --type "pdb"`,
		"debuginfo":  "--type debuginfo is not supported yet",
		"executable": "--type executable is not supported yet",
		"sources":    "--type sources is not supported yet",
	}
	for debuginfoType, want := range cases {
		_, err := runDebuginfoCmd(t, "--type", debuginfoType, dir, "--base-url", "https://app.example.com/", "--dry-run")
		require.Error(t, err, debuginfoType)
		assert.Contains(t, err.Error(), want, debuginfoType)
	}
}

func TestUpload_RequiresExperimentalFlag(t *testing.T) {
	testutil.SetupTestEnv(t)
	cmd := newTestRootCmd()
	cmd.SetArgs([]string{"debuginfo", "upload", "--type", "source-map", writeBuildOutput(t), "--base-url", "https://app.example.com/", "--dry-run"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

func TestUpload_SingleScriptFollowsSourceMappingURLComment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.js", "x\n//# sourceMappingURL=other.js.map\n")
	writeFile(t, dir, "other.js.map", "{}")

	output, err := runDebuginfoCmd(t, "--type", "source-map", filepath.Join(dir, "app.js"), "--script-url", "https://app.example.com/app.js", "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, output, filepath.Join(dir, "other.js.map"))
}

func TestUpload_DryRunListsBuildIDs(t *testing.T) {
	output, err := runDebuginfoCmd(t, "--type", "source-map", writeBuildOutput(t), "--base-url", "https://app.example.com/", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, output, "Dry run: 1 source map found")
	assert.Contains(t, output, "app.example.com/assets/app.js <- ")
}

func TestUpload_SourceMaps(t *testing.T) {
	strategies := []debuginfopb.UploadInstructions_UploadStrategy{
		debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL,
		debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC,
	}
	for _, strategy := range strategies {
		t.Run(strategy.String(), func(t *testing.T) {
			service, endpoint := startFakeDebuginfoService(t, strategy)
			dir := writeBuildOutput(t)
			args := []string{"--type", "source-map", dir, "--base-url", "https://app.example.com/", "--endpoint", endpoint, "--auth-token", testAuthToken}

			output, err := runDebuginfoCmd(t, args...)
			require.NoError(t, err)
			assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
			assert.Equal(t,
				encodeSourceMapBundle([]byte("console.log('app')"), []byte(`{"version":3,"sources":["app.ts"],"mappings":""}`)),
				service.stored["app.example.com/assets/app.js"])
			for _, authorization := range service.authorizations {
				assert.Equal(t, "Bearer "+testAuthToken, authorization)
			}
			for _, debuginfoType := range service.types {
				assert.Equal(t, debuginfopb.DebuginfoType_DEBUGINFO_TYPE_SOURCE_MAP, debuginfoType)
			}

			output, err = runDebuginfoCmd(t, args...)
			require.NoError(t, err)
			assert.Contains(t, output, `Source map "app.example.com/assets/app.js" skipped: Debuginfo already exists.`)

			output, err = runDebuginfoCmd(t, append(args, "--force")...)
			require.NoError(t, err)
			assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
		})
	}
}

func TestUpload_DryRunRejectsInvalidSourceMap(t *testing.T) {
	dir := writeBuildOutput(t)
	writeFile(t, dir, "assets/broken.js", "broken")
	writeFile(t, dir, "assets/broken.js.map", "not json")

	_, err := runDebuginfoCmd(t, "--type", "source-map", dir, "--base-url", "https://app.example.com/", "--dry-run")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.js.map is not valid JSON")
}

func TestUpload_PermissionErrorHintsAtIngestDebuginfo(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
	service.failBuildIDs = map[string]error{"app.example.com/assets/app.js": status.Error(codes.PermissionDenied, "missing permission")}

	_, err := runDebuginfoCmd(t, "--type", "source-map", writeBuildOutput(t), "--base-url", "https://app.example.com/", "--endpoint", endpoint, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must grant the ingest:debuginfo permission")
}

func TestUpload_AlreadyExistsOnInitiateIsSkipped(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
	service.existingInInitiate = true

	output, err := runDebuginfoCmd(t, "--type", "source-map", writeBuildOutput(t), "--base-url", "https://app.example.com/", "--endpoint", endpoint, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, output, `Source map "app.example.com/assets/app.js" skipped: already uploaded by another client`)
}

func TestUpload_AgentModePrintsResultsUploadedBeforeAFailure(t *testing.T) {
	previous := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = previous })
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
	service.failBuildIDs = map[string]error{"app.example.com/assets/later.js": status.Error(codes.Unavailable, "down")}
	dir := writeBuildOutput(t)
	writeFile(t, dir, "assets/later.js", "later")
	writeFile(t, dir, "assets/later.js.map", "{}")

	output, err := runDebuginfoCmd(t, "--type", "source-map", dir, "--base-url", "https://app.example.com/", "--endpoint", endpoint, "--auth-token", testAuthToken)

	require.Error(t, err)
	var results []uploadResult
	require.NoError(t, json.Unmarshal([]byte(output), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "app.example.com/assets/app.js", results[0].BuildID)
	assert.Equal(t, outcomeUploaded, results[0].Status)
}
