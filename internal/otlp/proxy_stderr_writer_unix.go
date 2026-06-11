//go:build !windows

package otlp

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// InstallWidthRefresh subscribes to SIGWINCH on Unix and writes to ch every
// time the terminal is resized. The writer can use this to redraw the
// stats line immediately on resize instead of waiting for the next 1-
// second stats tick.
//
// Cleans up the signal handler when ctx is done. Non-blocking sends on ch:
// if the writer hasn't drained the previous resize event, the new one is
// dropped — the next tick's normal width refresh will pick up the latest
// size anyway.
func InstallWidthRefresh(ctx context.Context, ch chan<- struct{}) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(sig)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sig:
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
}
