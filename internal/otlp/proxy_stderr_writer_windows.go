//go:build windows

package otlp

import "context"

// InstallWidthRefresh is a no-op on Windows for v1; terminal resize
// handling will land in a follow-up session. Until then, the stats line
// uses the width snapshot taken at each render tick — the worst case is a
// stats line at the wrong width for up to one second after a window
// resize.
//
// `syscall.SIGWINCH` does not exist on Windows so the Unix subscriber
// pattern doesn't compile here. The body is being addressed in a separate
// session per KTD13; the file split is in place now so the Unix path
// builds.
func InstallWidthRefresh(_ context.Context, _ chan<- struct{}) {
	// No-op. See file comment.
}
