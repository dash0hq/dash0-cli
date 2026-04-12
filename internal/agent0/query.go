package agent0

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type queryFlags struct {
	apiURL       string
	authToken    string
	dataset      string
	threadID     string
	output       string
	noStream     bool
	networkLevel string
	verbose      bool
}

func newQueryCmd() *cobra.Command {
	var flags queryFlags
	cmd := &cobra.Command{
		Use:   "query <prompt>",
		Short: "[experimental] Send a query to Agent0",
		Long:  "Send a question to Agent0 and stream the response to stdout.",
		Example: `  # Ask a question
  dash0 -X agent0 query "What services had errors in the last hour?"

  # Continue an existing conversation
  dash0 -X agent0 query "What changed at 14:05?" --thread <id>

  # Read prompt from stdin
  echo "Analyze error patterns" | dash0 -X agent0 query -

  # Get structured JSON output
  dash0 -X agent0 query "service health summary" -o json`,
		Args:         cobra.ExactArgs(1),
		Aliases:      []string{"ask"},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runQuery(cmd.Context(), args[0], &flags)
		},
	}

	cmd.Flags().StringVar(&flags.apiURL, "api-url", "", "API endpoint URL")
	cmd.Flags().StringVar(&flags.authToken, "auth-token", "", "Auth token")
	cmd.Flags().StringVar(&flags.dataset, "dataset", "", "Dataset identifier")
	cmd.Flags().StringVar(&flags.threadID, "thread", "", "Continue an existing thread")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output format: text, json (default: text; json in agent mode)")
	cmd.Flags().BoolVar(&flags.noStream, "no-stream", false, "Wait for complete response before printing")
	cmd.Flags().StringVar(&flags.networkLevel, "network-level", "trusted_only", "Network isolation: no_network, trusted_only, full")
	cmd.Flags().BoolVar(&flags.verbose, "verbose", false, "Show tool calls and thinking")

	return cmd
}

func runQuery(ctx context.Context, prompt string, flags *queryFlags) error {
	// Read from stdin if prompt is "-"
	if prompt == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		prompt = strings.TrimSpace(string(data))
		if prompt == "" {
			return fmt.Errorf("empty prompt from stdin")
		}
	}

	cfg, err := resolveQueryCfg(ctx, flags)
	if err != nil {
		return err
	}

	format := resolveQueryFormat(flags.output)

	client := &httpAgent0Client{
		apiURL:    cfg.apiURL,
		authToken: cfg.authToken,
	}

	req := &InvokeRequest{
		Message:      prompt,
		Dataset:      cfg.dataset,
		ThreadID:     cfg.threadID,
		NetworkLevel: cfg.networkLevel,
	}

	resp, err := client.InvokeAgent0(ctx, req)
	if err != nil {
		return err
	}

	stream := NewSSEStream(resp.Body)
	defer stream.Close()

	var (
		threadID     string
		printedSoFar int    // Bytes of the latest assistant message already printed to stdout
		lastContent  string // Content of the latest assistant message (for final JSON output)
	)

	for {
		event, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("SSE stream error: %w", err)
		}

		if event.IsDone() {
			break
		}

		if event.EventType == "status" {
			continue
		}

		var snapshot InvokeResponse
		if err := json.Unmarshal([]byte(event.Data), &snapshot); err != nil {
			return fmt.Errorf("failed to parse SSE event: %w", err)
		}

		if threadID == "" && snapshot.Thread.ID != "" {
			threadID = snapshot.Thread.ID
		}

		// Find the last assistant message in the snapshot — that's the one being generated.
		latestAssistant := findLastMessageByRole(snapshot.Messages, RoleAssistant)
		if latestAssistant == nil {
			continue
		}

		// Check for errors
		latestError := findLastMessageByRole(snapshot.Messages, RoleError)
		if latestError != nil && latestError.Content != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", latestError.Content)
		}

		// Stream new content
		if format == queryFormatText && !flags.noStream {
			if len(latestAssistant.Content) > printedSoFar {
				newContent := latestAssistant.Content[printedSoFar:]
				fmt.Fprint(os.Stdout, newContent)
				printedSoFar = len(latestAssistant.Content)
			}
		}

		lastContent = latestAssistant.Content
	}

	// Final output
	switch format {
	case queryFormatJSON:
		result := struct {
			ThreadID string `json:"threadId"`
			Query    string `json:"query"`
			Response string `json:"response"`
		}{
			ThreadID: threadID,
			Query:    prompt,
			Response: lastContent,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)

	case queryFormatText:
		if flags.noStream {
			fmt.Fprintln(os.Stdout, lastContent)
		} else {
			fmt.Fprintln(os.Stdout) // Final newline after streamed content
		}
		if threadID != "" {
			fmt.Fprintf(os.Stderr, "Thread: %s\n", threadID)
		}
	}

	return nil
}

type queryFormat int

const (
	queryFormatText queryFormat = iota
	queryFormatJSON
)

func resolveQueryFormat(flag string) queryFormat {
	switch strings.ToLower(flag) {
	case "json":
		return queryFormatJSON
	case "":
		if agentmode.Enabled {
			return queryFormatJSON
		}
		return queryFormatText
	default:
		return queryFormatText
	}
}

func resolveQueryCfg(ctx context.Context, flags *queryFlags) (chatConfig, error) {
	cfg := chatConfig{
		apiURL:       flags.apiURL,
		authToken:    flags.authToken,
		dataset:      flags.dataset,
		threadID:     flags.threadID,
		networkLevel: flags.networkLevel,
		verbose:      flags.verbose,
	}

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

func findLastMessageByRole(messages []Message, role string) *Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return &messages[i]
		}
	}
	return nil
}

