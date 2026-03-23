package confirmation

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
)

// reader is the input source for confirmation prompts. Tests override this to
// supply canned responses without touching os.Stdin.
var reader io.Reader = os.Stdin

// ConfirmDestructiveOperation prompts the user for confirmation before a
// destructive operation. It returns true if the operation should proceed.
// The prompt is skipped (returns true) when force is set or agent mode is
// active.
func ConfirmDestructiveOperation(prompt string, force bool) (bool, error) {
	if force || agentmode.Enabled {
		return true, nil
	}

	fmt.Print(prompt)
	r := bufio.NewReader(reader)
	response, err := r.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}
