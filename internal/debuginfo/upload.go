package debuginfo

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/dash0hq/dash0-cli/internal/version"
	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	"github.com/parca-dev/parca/pkg/hash"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// The Dash0 debuginfo service shares the host of the OTLP gRPC ingress.
const debuginfoGRPCPort = "4317"

const uploadChunkSize = 512 * 1024

// ponytail: one timeout for the whole PUT; size it from the bundle if multi-hundred-MB maps show up.
var signedURLClient = &http.Client{Timeout: 5 * time.Minute}

// Backoff before retry n (from 0) is a random duration in [d/2, d], with d = retryBaseDelay * 2^n capped at
// retryMaxDelay. A variable, so that tests can shorten it.
var (
	retryBaseDelay = 500 * time.Millisecond
	retryMaxDelay  = 5 * time.Second
)

// debuginfoTarget keeps the port of raw only when it came from --endpoint, because the port of an
// OTLP URL is the port of the OTLP ingress, not of the debuginfo service.
func debuginfoTarget(raw string, explicitEndpoint bool) (string, credentials.TransportCredentials, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", nil, fmt.Errorf("invalid debuginfo endpoint %q: expected a URL like https://ingress.eu-west-1.aws.dash0.com:4317", raw)
	}
	port := debuginfoGRPCPort
	if explicitEndpoint && parsed.Port() != "" {
		port = parsed.Port()
	}
	target := net.JoinHostPort(parsed.Hostname(), port)
	switch parsed.Scheme {
	case "https":
		return target, credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}), nil
	case "http":
		return target, insecure.NewCredentials(), nil
	default:
		return "", nil, fmt.Errorf("invalid debuginfo endpoint %q: the scheme must be http or https", raw)
	}
}

type uploader struct {
	client     debuginfopb.DebuginfoServiceClient
	authToken  string
	force      bool
	maxRetries int
}

func dialUploader(target string, transport credentials.TransportCredentials, authToken string, force bool, maxRetries int) (*uploader, func() error, error) {
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(transport),
		grpc.WithUserAgent(version.UserAgent()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to the debuginfo service at %s:\n  %w", target, err)
	}
	return &uploader{
		client:     debuginfopb.NewDebuginfoServiceClient(conn),
		authToken:  authToken,
		force:      force,
		maxRetries: maxRetries,
	}, conn.Close, nil
}

type uploadOutcome string

const (
	outcomePending  uploadOutcome = "pending"
	outcomeUploaded uploadOutcome = "uploaded"
	outcomeSkipped  uploadOutcome = "skipped"
)

// upload retries each step on its own, not the whole flow: once InitiateUpload succeeds, the service marks the
// build ID as uploading, and a new ShouldInitiateUpload answers "upload in progress" until that upload is stale.
func (u *uploader) upload(ctx context.Context, buildID string, debuginfoType debuginfopb.DebuginfoType, body []byte) (uploadOutcome, string, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+u.authToken)
	bodyHash, err := hash.Reader(bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	// The build ID of a source map is a script URL, not a hash, but the hash type keeps the service
	// from asking public debuginfod servers about it.
	buildIDType := debuginfopb.BuildIDType_BUILD_ID_TYPE_HASH

	var should *debuginfopb.ShouldInitiateUploadResponse
	err = u.retry(ctx, "check whether to upload "+buildID, func() (err error) {
		should, err = u.client.ShouldInitiateUpload(ctx, &debuginfopb.ShouldInitiateUploadRequest{
			BuildId: buildID, Hash: bodyHash, Force: u.force, Type: debuginfoType, BuildIdType: buildIDType,
		})
		return err
	})
	if err != nil {
		return "", "", rpcError("check whether to upload", err)
	}
	if !should.GetShouldInitiateUpload() {
		return outcomeSkipped, should.GetReason(), nil
	}

	var initiated *debuginfopb.InitiateUploadResponse
	err = u.retry(ctx, "start the upload of "+buildID, func() (err error) {
		initiated, err = u.client.InitiateUpload(ctx, &debuginfopb.InitiateUploadRequest{
			BuildId: buildID, Size: int64(len(body)), Hash: bodyHash, Force: u.force, Type: debuginfoType, BuildIdType: buildIDType,
		})
		return err
	})
	if status.Code(err) == codes.AlreadyExists {
		return outcomeSkipped, status.Convert(err).Message(), nil
	}
	if err != nil {
		return "", "", rpcError("start the upload", err)
	}
	instructions := initiated.GetUploadInstructions()

	switch instructions.GetUploadStrategy() {
	case debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL:
		err = u.retry(ctx, "upload "+buildID, func() error { return putSignedURL(ctx, instructions.GetSignedUrl(), body) })
	case debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC:
		err = u.retry(ctx, "upload "+buildID, func() error { return u.stream(ctx, instructions, body) })
		if err != nil {
			err = rpcError("stream the upload", err)
		}
	default:
		err = fmt.Errorf("the debuginfo service asked for upload strategy %s, which this CLI does not support", instructions.GetUploadStrategy())
	}
	if err != nil {
		return "", "", err
	}

	// Repeating MarkUploadFinished is safe: the service checks the upload ID and sets the state to uploaded again.
	err = u.retry(ctx, "finish the upload of "+buildID, func() error {
		_, err := u.client.MarkUploadFinished(ctx, &debuginfopb.MarkUploadFinishedRequest{
			BuildId: buildID, UploadId: instructions.GetUploadId(), Type: debuginfoType,
		})
		return err
	})
	if err != nil {
		return "", "", rpcError("finish the upload", err)
	}
	return outcomeUploaded, "", nil
}

// retry calls call until it succeeds, fails with an error that a retry cannot fix, or used up u.maxRetries.
func (u *uploader) retry(ctx context.Context, what string, call func() error) error {
	for attempt := 0; ; attempt++ {
		err := call()
		if err == nil || attempt >= u.maxRetries || !isRetryable(err) {
			return err
		}
		delay := min(retryBaseDelay<<attempt, retryMaxDelay)
		delay = delay/2 + rand.N(delay/2+1)
		fmt.Fprintf(os.Stderr, "Retrying to %s in %s (retry %d of %d): %s\n", what, delay.Round(time.Millisecond), attempt+1, u.maxRetries, retryReason(err))
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
	}
}

// transientError marks a signed URL failure that a retry can fix.
type transientError struct{ error }

func (e transientError) Unwrap() error { return e.error }

func isRetryable(err error) bool {
	if errors.As(err, &transientError{}) {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

func retryReason(err error) string {
	if rpcStatus, ok := status.FromError(err); ok {
		return fmt.Sprintf("%s (%s)", rpcStatus.Message(), rpcStatus.Code())
	}
	return err.Error()
}

func putSignedURL(ctx context.Context, signedURL string, body []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid signed upload URL from the debuginfo service:\n  %w", err)
	}
	response, err := signedURLClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return transientError{fmt.Errorf("failed to upload to the signed URL:\n  %w", err)}
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		return nil
	}
	err = fmt.Errorf("upload to the signed URL failed with HTTP status %s", response.Status)
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		return transientError{err}
	}
	return err
}

// stream returns the gRPC error as is, so that retry can read its code.
func (u *uploader) stream(ctx context.Context, instructions *debuginfopb.UploadInstructions, body []byte) error {
	uploadStream, err := u.client.Upload(ctx)
	if err != nil {
		return err
	}
	sendErr := uploadStream.Send(&debuginfopb.UploadRequest{Data: &debuginfopb.UploadRequest_Info{Info: &debuginfopb.UploadInfo{
		BuildId: instructions.GetBuildId(), UploadId: instructions.GetUploadId(), Type: instructions.GetType(),
	}}})
	for offset := 0; sendErr == nil && offset < len(body); offset += uploadChunkSize {
		end := min(offset+uploadChunkSize, len(body))
		sendErr = uploadStream.Send(&debuginfopb.UploadRequest{Data: &debuginfopb.UploadRequest_ChunkData{ChunkData: body[offset:end]}})
	}
	// Send returns io.EOF when the server ends the stream; the server's status comes from CloseAndRecv.
	if _, err := uploadStream.CloseAndRecv(); err != nil {
		return err
	}
	if sendErr != nil && !errors.Is(sendErr, io.EOF) {
		return sendErr
	}
	return nil
}

func rpcError(action string, err error) error {
	rpcStatus := status.Convert(err)
	switch rpcStatus.Code() {
	case codes.Unauthenticated, codes.PermissionDenied:
		return fmt.Errorf("failed to %s: %s\nHint: the auth token must grant the ingest:debuginfo permission; an ingest token works", action, rpcStatus.Message())
	default:
		return fmt.Errorf("failed to %s: %s (%s)", action, rpcStatus.Message(), rpcStatus.Code())
	}
}
