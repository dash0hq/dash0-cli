package debuginfo

import (
	"fmt"
	"testing"
	"time"

	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	"github.com/stretchr/testify/require"
)

// BenchmarkUpload_100SourceMaps measures the wall clock of one upload run against a service that answers each call
// after a fixed delay, the shape of a CI job that uploads a build to Dash0 over the network.
func BenchmarkUpload_100SourceMaps(b *testing.B) {
	service, endpoint := startFakeDebuginfoService(b, debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC)
	service.latency = 20 * time.Millisecond
	dir := b.TempDir()
	for index := range 100 {
		writeFile(b, dir, fmt.Sprintf("assets/chunk-%03d.js", index), fmt.Sprintf("console.log(%d)", index))
		writeFile(b, dir, fmt.Sprintf("assets/chunk-%03d.js.map", index), `{"version":3,"sources":["chunk.ts"],"mappings":""}`)
	}

	for _, concurrency := range []int{1, 4, 8, 16, 32} {
		b.Run(fmt.Sprintf("concurrency=%d", concurrency), func(b *testing.B) {
			args := append([]string{dir, "--force", "--concurrency", fmt.Sprint(concurrency)}, uploadArgs(endpoint)...)
			for b.Loop() {
				_, err := runDebuginfoCmd(b, args...)
				require.NoError(b, err)
			}
		})
	}
}
