package debuginfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
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
	// stepErrors holds, per RPC name, the errors that the next calls of that RPC return, in order.
	stepErrors map[string][]error
	// putStatuses holds the HTTP statuses that the next signed URL PUTs answer, in order.
	putStatuses []int
	// putStatusByPath makes every signed URL PUT to that path answer the HTTP status.
	putStatusByPath map[string]int
	// shouldAnswer and initiateAnswer, when set and returning non-nil, replace the answer to the nth call (from 1).
	shouldAnswer   func(call int) *debuginfopb.ShouldInitiateUploadResponse
	initiateAnswer func(call int) *debuginfopb.InitiateUploadResponse

	mutex          sync.Mutex
	calls          map[string]int
	puts           int
	hashes         []string
	putPaths       []string
	markedIDs      []string
	supportsMulti  []bool
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

// step counts a call of the RPC and returns the next injected error for it, if any.
func (service *fakeDebuginfoService) step(name string) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.calls[name]++
	if len(service.stepErrors[name]) == 0 {
		return nil
	}
	err := service.stepErrors[name][0]
	service.stepErrors[name] = service.stepErrors[name][1:]
	return err
}

func (service *fakeDebuginfoService) ShouldInitiateUpload(ctx context.Context, request *debuginfopb.ShouldInitiateUploadRequest) (*debuginfopb.ShouldInitiateUploadResponse, error) {
	service.record(ctx, request.GetType())
	if err := service.step("ShouldInitiateUpload"); err != nil {
		return nil, err
	}
	service.mutex.Lock()
	service.hashes = append(service.hashes, request.GetHash())
	defer service.mutex.Unlock()
	if err := service.failBuildIDs[request.GetBuildId()]; err != nil {
		return nil, err
	}
	if service.shouldAnswer != nil {
		if answer := service.shouldAnswer(service.calls["ShouldInitiateUpload"]); answer != nil {
			return answer, nil
		}
	}
	if _, exists := service.stored[request.GetBuildId()]; exists && !request.GetForce() {
		return &debuginfopb.ShouldInitiateUploadResponse{Reason: "Debuginfo already exists."}, nil
	}
	return &debuginfopb.ShouldInitiateUploadResponse{ShouldInitiateUpload: true}, nil
}

func (service *fakeDebuginfoService) InitiateUpload(ctx context.Context, request *debuginfopb.InitiateUploadRequest) (*debuginfopb.InitiateUploadResponse, error) {
	service.record(ctx, request.GetType())
	if err := service.step("InitiateUpload"); err != nil {
		return nil, err
	}
	service.mutex.Lock()
	service.hashes = append(service.hashes, request.GetHash())
	service.supportsMulti = append(service.supportsMulti, request.GetSupportsMultipleUploads())
	call := service.calls["InitiateUpload"]
	service.mutex.Unlock()
	if service.initiateAnswer != nil {
		if answer := service.initiateAnswer(call); answer != nil {
			return answer, nil
		}
	}
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
	if err := service.step("Upload"); err != nil {
		return err
	}
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
	if err := service.step("MarkUploadFinished"); err != nil {
		return nil, err
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.markedIDs = append(service.markedIDs, request.GetUploadId())
	service.stored[request.GetBuildId()] = service.pending[request.GetBuildId()]
	return &debuginfopb.MarkUploadFinishedResponse{}, nil
}

func startFakeDebuginfoService(t *testing.T, strategy debuginfopb.UploadInstructions_UploadStrategy) (*fakeDebuginfoService, string) {
	t.Helper()
	service := &fakeDebuginfoService{strategy: strategy, stored: map[string][]byte{}, pending: map[string][]byte{}, calls: map[string]int{}}

	signedURLServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.Method != http.MethodPut {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		service.mutex.Lock()
		defer service.mutex.Unlock()
		service.puts++
		service.putPaths = append(service.putPaths, request.URL.Path)
		if code, ok := service.putStatusByPath[request.URL.Path]; ok {
			writer.WriteHeader(code)
			return
		}
		if len(service.putStatuses) > 0 {
			code := service.putStatuses[0]
			service.putStatuses = service.putStatuses[1:]
			if code != http.StatusOK {
				writer.WriteHeader(code)
				return
			}
		}
		service.pending[request.URL.Path[1:]] = body
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

func TestMain(m *testing.M) {
	retryBaseDelay = time.Millisecond
	retryMaxDelay = 5 * time.Millisecond
	m.Run()
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

func uploadArgs(endpoint string) []string {
	return []string{"--type", "source-map", "--base-url", "https://app.example.com/", "--endpoint", endpoint, "--auth-token", testAuthToken}
}

func unavailable(times int) []error {
	errs := make([]error, times)
	for i := range errs {
		errs[i] = status.Error(codes.Unavailable, "try again")
	}
	return errs
}

func TestUpload_RetriesOnlyTheStepThatFailed(t *testing.T) {
	for _, step := range []string{"ShouldInitiateUpload", "InitiateUpload", "Upload", "MarkUploadFinished"} {
		t.Run(step, func(t *testing.T) {
			service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
			service.stepErrors = map[string][]error{step: unavailable(2)}

			output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

			require.NoError(t, err)
			assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
			want := map[string]int{"ShouldInitiateUpload": 1, "InitiateUpload": 1, "Upload": 1, "MarkUploadFinished": 1}
			want[step] = 3
			assert.Equal(t, want, service.calls)
		})
	}
}

func TestUpload_NonRetryableCodeFailsOnFirstAttempt(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
	service.stepErrors = map[string][]error{"InitiateUpload": {status.Error(codes.InvalidArgument, "bad build ID")}}

	_, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad build ID (InvalidArgument)")
	assert.Equal(t, 1, service.calls["InitiateUpload"])
}

func TestUpload_GivesUpAfterMaxRetriesAndNamesTheFile(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
	service.stepErrors = map[string][]error{"MarkUploadFinished": unavailable(10)}
	dir := writeBuildOutput(t)

	_, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), dir)...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(dir, "assets/app.js"))
	assert.Contains(t, err.Error(), "failed to finish the upload: try again (Unavailable)")
	// The default of --max-retries is 3: one attempt and three retries.
	assert.Equal(t, 4, service.calls["MarkUploadFinished"])
}

func TestUpload_RetriesRetryableGRPCCodes(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted} {
		t.Run(code.String(), func(t *testing.T) {
			service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
			service.stepErrors = map[string][]error{"ShouldInitiateUpload": {status.Error(code, "busy")}}

			output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

			require.NoError(t, err)
			assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
			assert.Equal(t, 2, service.calls["ShouldInitiateUpload"])
		})
	}
}

func TestUpload_SignedURLRetriesTemporaryHTTPStatuses(t *testing.T) {
	for _, code := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
			service.putStatuses = []int{code, http.StatusOK}

			output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

			require.NoError(t, err)
			assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
			assert.Equal(t, 2, service.puts)
		})
	}
}

func TestUpload_SignedURLDoesNotRetryForbidden(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.putStatuses = []int{http.StatusForbidden, http.StatusOK}

	_, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP status 403 Forbidden")
	assert.Equal(t, 1, service.puts)
}

func TestRetry_StopsWhenContextIsCancelledDuringBackoff(t *testing.T) {
	previous := retryBaseDelay
	retryBaseDelay, retryMaxDelay = time.Minute, time.Minute
	t.Cleanup(func() { retryBaseDelay, retryMaxDelay = previous, 5*time.Millisecond })
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	calls := 0

	started := time.Now()
	err := (&uploader{maxRetries: 3}).retry(ctx, "test", func() error {
		calls++
		return status.Error(codes.Unavailable, "down")
	})

	assert.Less(t, time.Since(started), 5*time.Second)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, calls)
}

func TestUpload_SendsTheXXHashOfTheBundle(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)

	_, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.NoError(t, err)
	bundle := encodeSourceMapBundle([]byte("console.log('app')"), []byte(`{"version":3,"sources":["app.ts"],"mappings":""}`))
	want := fmt.Sprintf("%016x", xxhash.Sum64(bundle))
	assert.Equal(t, []string{want, want}, service.hashes)
}

const testBuildID = "app.example.com/assets/app.js"

// signedTarget is an upload target that PUTs to path on the fake signed URL server.
func (service *fakeDebuginfoService) signedTarget(uploadID, path string) *debuginfopb.UploadInstructions {
	return &debuginfopb.UploadInstructions{
		BuildId:        testBuildID,
		UploadId:       uploadID,
		UploadStrategy: debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL,
		SignedUrl:      service.signedURL + path,
		Type:           debuginfopb.DebuginfoType_DEBUGINFO_TYPE_SOURCE_MAP,
	}
}

func temporaryNo(call int) *debuginfopb.ShouldInitiateUploadResponse {
	return &debuginfopb.ShouldInitiateUploadResponse{Reason: "upload in progress", RetryRequired: true}
}

func TestUpload_LegacyUploadInstructionsStillUpload(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.NoError(t, err)
	assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
	assert.Equal(t, []string{"/app.example.com/assets/app.js"}, service.putPaths)
	assert.Equal(t, []string{"upload-app.example.com/assets/app.js"}, service.markedIDs)
}

func TestUpload_UploadsToEveryTarget(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.initiateAnswer = func(int) *debuginfopb.InitiateUploadResponse {
		return &debuginfopb.InitiateUploadResponse{UploadTargets: []*debuginfopb.UploadInstructions{
			service.signedTarget("token-a", "/backend-a"), service.signedTarget("token-b", "/backend-b"),
		}}
	}

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.NoError(t, err)
	assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
	assert.Equal(t, []string{"/backend-a", "/backend-b"}, service.putPaths)
	assert.Equal(t, []string{"token-a", "token-b"}, service.markedIDs)
}

func TestUpload_InitiateAdvertisesMultipleUploads(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)

	_, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.NoError(t, err)
	assert.Equal(t, []bool{true}, service.supportsMulti)
}

func TestUpload_RejectsResponseWithBothShapes(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.initiateAnswer = func(int) *debuginfopb.InitiateUploadResponse {
		return &debuginfopb.InitiateUploadResponse{
			UploadInstructions: service.signedTarget("legacy", "/legacy"),
			UploadTargets:      []*debuginfopb.UploadInstructions{service.signedTarget("token-a", "/backend-a")},
		}
	}

	_, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "both upload_instructions and upload_targets")
	assert.Empty(t, service.putPaths)
}

func TestUpload_ChecksAgainWhenTheNoIsTemporary(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.shouldAnswer = func(call int) *debuginfopb.ShouldInitiateUploadResponse {
		if call == 1 {
			return temporaryNo(call)
		}
		return nil
	}

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.NoError(t, err)
	assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
	assert.Equal(t, 2, service.calls["ShouldInitiateUpload"])
}

func TestUpload_FailsWhenTheNoStaysTemporary(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.shouldAnswer = temporaryNo
	dir := writeBuildOutput(t)

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), dir)...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(dir, "assets/app.js"))
	assert.Contains(t, err.Error(), "delivery is incomplete")
	assert.NotContains(t, output, "skipped")
	// The default of --max-retries is 3: one check and three more.
	assert.Equal(t, 4, service.calls["ShouldInitiateUpload"])
}

func TestUpload_NegotiatesAgainAfterInitiateRetryRequired(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.initiateAnswer = func(int) *debuginfopb.InitiateUploadResponse {
		return &debuginfopb.InitiateUploadResponse{
			UploadTargets: []*debuginfopb.UploadInstructions{service.signedTarget("token-a", "/backend-a")}, RetryRequired: true,
		}
	}
	service.shouldAnswer = func(call int) *debuginfopb.ShouldInitiateUploadResponse {
		if call == 2 {
			return &debuginfopb.ShouldInitiateUploadResponse{Reason: "Debuginfo already exists."}
		}
		return nil
	}

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), writeBuildOutput(t))...)

	require.NoError(t, err)
	assert.Contains(t, output, `Source map "app.example.com/assets/app.js" uploaded`)
	assert.Equal(t, 2, service.calls["ShouldInitiateUpload"])
	assert.Equal(t, []string{"token-a"}, service.markedIDs)
}

func TestUpload_FailsWhenInitiateStaysRetryRequired(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.initiateAnswer = func(int) *debuginfopb.InitiateUploadResponse {
		return &debuginfopb.InitiateUploadResponse{
			UploadTargets: []*debuginfopb.UploadInstructions{service.signedTarget("token-a", "/backend-a")}, RetryRequired: true,
		}
	}
	service.shouldAnswer = func(int) *debuginfopb.ShouldInitiateUploadResponse {
		return &debuginfopb.ShouldInitiateUploadResponse{ShouldInitiateUpload: true}
	}
	dir := writeBuildOutput(t)

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), dir)...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(dir, "assets/app.js"))
	assert.Contains(t, err.Error(), "delivery is incomplete")
	assert.NotContains(t, output, "uploaded")
	assert.Equal(t, 4, service.calls["InitiateUpload"])
}

func TestUpload_FailsWhenOneTargetFails(t *testing.T) {
	service, endpoint := startFakeDebuginfoService(t, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL)
	service.initiateAnswer = func(int) *debuginfopb.InitiateUploadResponse {
		return &debuginfopb.InitiateUploadResponse{UploadTargets: []*debuginfopb.UploadInstructions{
			service.signedTarget("token-a", "/backend-a"), service.signedTarget("token-b", "/backend-b"),
		}}
	}
	service.putStatusByPath = map[string]int{"/backend-b": http.StatusForbidden}
	dir := writeBuildOutput(t)

	output, err := runDebuginfoCmd(t, append(uploadArgs(endpoint), dir)...)

	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(dir, "assets/app.js"))
	assert.Contains(t, err.Error(), "upload target 2 of 2")
	assert.NotContains(t, output, "uploaded")
	assert.Equal(t, []string{"token-a"}, service.markedIDs)
}
