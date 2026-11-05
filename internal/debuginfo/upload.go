package debuginfo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/dash0hq/dash0-cli/internal/version"
	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
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
	client    debuginfopb.DebuginfoServiceClient
	authToken string
	force     bool
}

func dialUploader(target string, transport credentials.TransportCredentials, authToken string, force bool) (*uploader, func() error, error) {
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(transport),
		grpc.WithUserAgent(version.UserAgent()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to the debuginfo service at %s:\n  %w", target, err)
	}
	return &uploader{
		client:    debuginfopb.NewDebuginfoServiceClient(conn),
		authToken: authToken,
		force:     force,
	}, conn.Close, nil
}

type uploadOutcome string

const (
	outcomePending  uploadOutcome = "pending"
	outcomeUploaded uploadOutcome = "uploaded"
	outcomeSkipped  uploadOutcome = "skipped"
)

func (u *uploader) upload(ctx context.Context, buildID string, debuginfoType debuginfopb.DebuginfoType, body []byte) (uploadOutcome, string, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+u.authToken)
	digest := sha256.Sum256(body)
	hash := hex.EncodeToString(digest[:])
	// The build ID of a source map is a script URL, not a hash, but the hash type keeps the service
	// from asking public debuginfod servers about it.
	buildIDType := debuginfopb.BuildIDType_BUILD_ID_TYPE_HASH

	should, err := u.client.ShouldInitiateUpload(ctx, &debuginfopb.ShouldInitiateUploadRequest{
		BuildId: buildID, Hash: hash, Force: u.force, Type: debuginfoType, BuildIdType: buildIDType,
	})
	if err != nil {
		return "", "", rpcError("check whether to upload", err)
	}
	if !should.GetShouldInitiateUpload() {
		return outcomeSkipped, should.GetReason(), nil
	}

	initiated, err := u.client.InitiateUpload(ctx, &debuginfopb.InitiateUploadRequest{
		BuildId: buildID, Size: int64(len(body)), Hash: hash, Force: u.force, Type: debuginfoType, BuildIdType: buildIDType,
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
		if err := u.putSignedURL(ctx, instructions.GetSignedUrl(), body); err != nil {
			return "", "", err
		}
	case debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC:
		if err := u.stream(ctx, instructions, body); err != nil {
			return "", "", err
		}
	default:
		return "", "", fmt.Errorf("the debuginfo service asked for upload strategy %s, which this CLI does not support", instructions.GetUploadStrategy())
	}

	if _, err := u.client.MarkUploadFinished(ctx, &debuginfopb.MarkUploadFinishedRequest{
		BuildId: buildID, UploadId: instructions.GetUploadId(), Type: debuginfoType,
	}); err != nil {
		return "", "", rpcError("finish the upload", err)
	}
	return outcomeUploaded, "", nil
}

func (u *uploader) putSignedURL(ctx context.Context, signedURL string, body []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid signed upload URL from the debuginfo service:\n  %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("failed to upload to the signed URL:\n  %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("upload to the signed URL failed with HTTP status %s", response.Status)
	}
	return nil
}

func (u *uploader) stream(ctx context.Context, instructions *debuginfopb.UploadInstructions, body []byte) error {
	uploadStream, err := u.client.Upload(ctx)
	if err != nil {
		return rpcError("open the upload stream", err)
	}
	if err := uploadStream.Send(&debuginfopb.UploadRequest{Data: &debuginfopb.UploadRequest_Info{Info: &debuginfopb.UploadInfo{
		BuildId: instructions.GetBuildId(), UploadId: instructions.GetUploadId(), Type: instructions.GetType(),
	}}}); err != nil {
		return rpcError("send the upload header", err)
	}
	for offset := 0; offset < len(body); offset += uploadChunkSize {
		end := min(offset+uploadChunkSize, len(body))
		if err := uploadStream.Send(&debuginfopb.UploadRequest{Data: &debuginfopb.UploadRequest_ChunkData{ChunkData: body[offset:end]}}); err != nil {
			break // the real error surfaces from CloseAndRecv
		}
	}
	if _, err := uploadStream.CloseAndRecv(); err != nil {
		return rpcError("stream the upload", err)
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
