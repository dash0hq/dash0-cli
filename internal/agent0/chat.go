package agent0

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type chatFlags struct {
	apiURL    string
	authToken string
	dataset   string
	threadID  string
	verbose   bool
}

func newChatCmd() *cobra.Command {
	var flags chatFlags
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "[experimental] Interactive chat with Agent0",
		Long:  "Start an interactive terminal chat session with Agent0.",
		Example: `  # Start a new conversation
  dash0 -X agent0 chat

  # Resume an existing thread
  dash0 -X agent0 chat --thread <id>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			if agentmode.Enabled {
				return fmt.Errorf(
					"\"agent0 chat\" is interactive and cannot be used in agent mode; use \"agent0 query\" instead",
				)
			}
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf(
					"\"agent0 chat\" requires an interactive terminal; use \"agent0 query\" for non-interactive use",
				)
			}
			return runChat(cmd.Context(), &flags)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVar(&flags.apiURL, "api-url", "", "API endpoint URL")
	cmd.Flags().StringVar(&flags.authToken, "auth-token", "", "Auth token")
	cmd.Flags().StringVar(&flags.dataset, "dataset", "", "Dataset identifier")
	cmd.Flags().StringVar(&flags.threadID, "thread", "", "Resume an existing thread")
	cmd.Flags().BoolVar(&flags.verbose, "verbose", false, "Show tool calls and thinking")

	return cmd
}

func runChat(ctx context.Context, flags *chatFlags) error {
	cfg, err := resolveChatCfg(ctx, flags)
	if err != nil {
		return err
	}

	client := &httpAgent0Client{
		apiURL:    cfg.apiURL,
		authToken: cfg.authToken,
	}

	model := newChatModel(client, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func resolveChatCfg(ctx context.Context, flags *chatFlags) (chatConfig, error) {
	cfg := chatConfig{
		apiURL:       flags.apiURL,
		authToken:    flags.authToken,
		dataset:      flags.dataset,
		threadID:     flags.threadID,
		networkLevel: "trusted_only",
		verbose:      flags.verbose,
	}

	// Fill from profile if not set by flags
	if resolved := profiles.FromContext(ctx); resolved != nil {
		if cfg.apiURL == "" {
			cfg.apiURL = resolved.ApiUrl
		}
		if cfg.authToken == "" {
			cfg.authToken = resolved.AuthToken
		}
		if cfg.dataset == "" {
			cfg.dataset = resolved.Dataset
		}
	}

	if cfg.apiURL == "" {
		return cfg, fmt.Errorf("API URL is required\nHint: set it via --api-url, DASH0_API_URL, or a profile")
	}
	if cfg.authToken == "" {
		return cfg, fmt.Errorf("auth token is required\nHint: set it via --auth-token, DASH0_AUTH_TOKEN, or a profile")
	}

	return cfg, nil
}

// httpAgent0Client implements Agent0Client using the Dash0 API.
type httpAgent0Client struct {
	apiURL    string
	authToken string
}

func (c *httpAgent0Client) InvokeAgent0(ctx context.Context, req *InvokeRequest) (*http.Response, error) {
	body, err := marshalJSON(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/agents/agent0-sdk/invoke", body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("agent0 invoke returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *httpAgent0Client) GetAgent0Thread(ctx context.Context, threadID string) (*ThreadResponse, error) {
	reqBody := struct {
		ThreadID string `json:"threadId"`
	}{ThreadID: threadID}

	body, err := marshalJSON(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/agents/agent0-sdk/thread", body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("thread %q not found", threadID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get thread returned HTTP %d", resp.StatusCode)
	}

	var threadResp ThreadResponse
	if err := decodeJSON(resp.Body, &threadResp); err != nil {
		return nil, fmt.Errorf("failed to parse thread response: %w", err)
	}
	return &threadResp, nil
}

func (c *httpAgent0Client) CancelAgent0(ctx context.Context, threadID string) error {
	reqBody := struct {
		ThreadID string `json:"threadId"`
	}{ThreadID: threadID}

	body, err := marshalJSON(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/agents/agent0-sdk/cancel", body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cancel returned HTTP %d", resp.StatusCode)
	}
	return nil
}
