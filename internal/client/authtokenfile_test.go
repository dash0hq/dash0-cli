package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// newAuthTokenCmd returns a command with the global --auth-token-file flag
// and, if withAuthToken is set, a per-command --auth-token flag.
func newAuthTokenCmd(withAuthToken bool, args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("auth-token-file", "", "")
	if withAuthToken {
		cmd.Flags().String("auth-token", "", "")
	}
	_ = cmd.Flags().Parse(args)
	return cmd
}

func TestApplyAuthTokenFile_SetsAuthTokenFlag(t *testing.T) {
	cmd := newAuthTokenCmd(true, "--auth-token-file", writeTokenFile(t, " auth_file\r\n"))
	require.NoError(t, ApplyAuthTokenFile(cmd))
	token, _ := cmd.Flags().GetString("auth-token")
	assert.Equal(t, "auth_file", token)
}

func TestApplyAuthTokenFile_NotReadWithoutAuthTokenFlag(t *testing.T) {
	cmd := newAuthTokenCmd(false, "--auth-token-file", filepath.Join(t.TempDir(), "missing"))
	assert.NoError(t, ApplyAuthTokenFile(cmd))
}

func TestApplyAuthTokenFile_Errors(t *testing.T) {
	tests := map[string]struct {
		content string // "" means the file does not exist
		extra   []string
		wantErr string
	}{
		"mutually exclusive": {content: "auth_file", extra: []string{"--auth-token", "auth_flag"}, wantErr: "mutually exclusive"},
		"missing file":       {wantErr: "failed to read --auth-token-file"},
		"empty file":         {content: " \n", wantErr: "is empty"},
		"multiple lines":     {content: "auth_a\nauth_b\n", wantErr: "must contain only the auth token"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing")
			if tt.content != "" {
				path = writeTokenFile(t, tt.content)
			}
			cmd := newAuthTokenCmd(true, append([]string{"--auth-token-file", path}, tt.extra...)...)
			err := ApplyAuthTokenFile(cmd)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewRawHTTPConfig_FlagTokenOverridesStaticProfile(t *testing.T) {
	ctx := profiles.WithConfiguration(context.Background(), &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_profile",
	})

	cfg, err := NewRawHTTPConfig(ctx, "", "auth_flag")
	require.NoError(t, err)
	assert.Equal(t, "auth_flag", cfg.AuthToken)
}
