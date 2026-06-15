package confirmation

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
)

// reader is the input source for confirmation prompts. Tests override this to
// supply canned responses without touching os.Stdin.
var reader io.Reader = os.Stdin

// SetReaderForTest overrides the confirmation input reader, returning a
// function that restores the previous reader. It is intended for use only
// from test code in other packages that need to simulate user input.
func SetReaderForTest(r io.Reader) (restore func()) {
	prev := reader
	reader = r
	return func() { reader = prev }
}

// ConfirmDestructiveOperation prompts the user for confirmation before a
// destructive operation. It returns true if the operation should proceed.
// The prompt is skipped (returns true) when force is set or agent mode is
// active.
//
// The read honors ctx cancellation — a Ctrl-C (which the root process wires
// into ctx via signal.NotifyContext) unblocks the prompt with
// context.Canceled. Without this, the SIGINT handler would intercept the
// signal but the bufio read would keep blocking, leaving the user stuck
// until SIGKILL.
//
// On cancellation the stdin-reader goroutine is leaked (it stays blocked
// on os.Stdin). That is acceptable: cancellation only happens on the way
// to process exit, and Go's runtime tears the goroutine down with the
// process.
func ConfirmDestructiveOperation(ctx context.Context, prompt string, force bool) (bool, error) {
	if force || agentmode.Enabled {
		return true, nil
	}

	fmt.Print(prompt)

	type readResult struct {
		line string
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		r := bufio.NewReader(reader)
		line, err := r.ReadString('\n')
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		// Newline so the cancellation message starts on its own line
		// instead of after the prompt.
		fmt.Println()
		return false, ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			if errors.Is(res.err, io.EOF) {
				return false, fmt.Errorf("confirmation aborted: stdin closed")
			}
			return false, fmt.Errorf("failed to read response: %w", res.err)
		}
		response := strings.TrimSpace(strings.ToLower(res.line))
		return response == "y" || response == "yes", nil
	}
}
