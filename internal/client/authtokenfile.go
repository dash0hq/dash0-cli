package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ApplyAuthTokenFile reads the token named by the global --auth-token-file flag
// and sets it as the value of the command's own --auth-token flag, so every
// command resolves it exactly like a token passed with --auth-token. Commands
// without an --auth-token flag do not talk to Dash0 and never read the file.
func ApplyAuthTokenFile(cmd *cobra.Command) error {
	path, _ := cmd.Flags().GetString("auth-token-file")
	authToken := cmd.Flags().Lookup("auth-token")
	if path == "" || authToken == nil {
		return nil
	}
	if authToken.Changed {
		return fmt.Errorf("--auth-token and --auth-token-file are mutually exclusive")
	}
	// Past flag validation: an unreadable file is a runtime error, not a
	// usage error.
	cmd.SilenceUsage = true
	token, err := readAuthTokenFile(path)
	if err != nil {
		return err
	}
	return cmd.Flags().Set("auth-token", token)
}

// readAuthTokenFile reads an auth token from a file that contains only the
// token, as written by Kubernetes secrets, Docker secrets, or systemd
// credentials. Surrounding whitespace (such as a trailing newline) is removed.
func readAuthTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read --auth-token-file:\n  %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("--auth-token-file %q is empty", path)
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", fmt.Errorf("--auth-token-file %q must contain only the auth token, but it contains more than one line", path)
	}
	return token, nil
}
