package agent0

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type threadsGetFlags struct {
	apiURL           string
	authToken        string
	output           string
	includeToolCalls bool
}

func newThreadsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "threads",
		Short: "[experimental] Manage Agent0 conversation threads",
		Long:  "Commands for managing Agent0 conversation threads.",
	}

	cmd.AddCommand(newThreadsGetCmd())

	return cmd
}

func newThreadsGetCmd() *cobra.Command {
	var flags threadsGetFlags
	cmd := &cobra.Command{
		Use:   "get <thread-id>",
		Short: "[experimental] Get a conversation thread",
		Long:  "Retrieve a conversation thread with its full message history.",
		Example: `  # Get a thread
  dash0 -X agent0 threads get <id>

  # Get a thread as JSON
  dash0 -X agent0 threads get <id> -o json

  # Include tool calls in output
  dash0 -X agent0 threads get <id> --include-tool-calls`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runThreadsGet(cmd.Context(), args[0], &flags)
		},
	}

	cmd.Flags().StringVar(&flags.apiURL, "api-url", "", "API endpoint URL")
	cmd.Flags().StringVar(&flags.authToken, "auth-token", "", "Auth token")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output format: table, json, yaml (default: table; json in agent mode)")
	cmd.Flags().BoolVar(&flags.includeToolCalls, "include-tool-calls", false, "Include tool call details in output")

	return cmd
}

func runThreadsGet(ctx context.Context, threadID string, flags *threadsGetFlags) error {
	apiURL, authToken, err := resolveThreadsCfg(ctx, flags.apiURL, flags.authToken)
	if err != nil {
		return err
	}

	client := &httpAgent0Client{
		apiURL:    apiURL,
		authToken: authToken,
	}

	resp, err := client.GetAgent0Thread(ctx, threadID)
	if err != nil {
		return err
	}

	format := resolveThreadsGetFormat(flags.output)

	switch format {
	case threadsGetFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)

	case threadsGetFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		return enc.Encode(resp)

	case threadsGetFormatTable:
		return renderThreadTranscript(resp, flags.includeToolCalls)
	}

	return nil
}

func renderThreadTranscript(resp *ThreadResponse, includeToolCalls bool) error {
	fmt.Printf("Thread: %s\n", resp.Thread.ID)
	if resp.Thread.Name != "" {
		fmt.Printf("Title:  %s\n", resp.Thread.Name)
	}
	fmt.Printf("Created: %s\n", resp.Thread.CreatedAt.Format("2006-01-02 15:04"))
	fmt.Println()

	renderer := newMarkdownRenderer(80)

	for _, msg := range resp.Messages {
		switch msg.Role {
		case RoleHuman:
			ts := ""
			if msg.StartedAt != nil {
				ts = msg.StartedAt.Format("15:04")
			}
			fmt.Printf("--- User (%s) ---\n", ts)
			fmt.Println(msg.Content)
			fmt.Println()

		case RoleAssistant:
			ts := ""
			if msg.StartedAt != nil {
				ts = msg.StartedAt.Format("15:04")
			}
			fmt.Printf("--- Agent0 (%s) ---\n", ts)
			fmt.Println(renderMarkdown(renderer, msg.Content))
			fmt.Println()

		case RoleError:
			fmt.Printf("--- Error ---\n")
			fmt.Printf("Error: %s\n", msg.Content)
			fmt.Println()

		case RoleTool:
			if includeToolCalls {
				fmt.Printf("--- Tool ---\n")
				fmt.Println(msg.Content)
				fmt.Println()
			}

		case RoleThinking:
			if includeToolCalls {
				fmt.Printf("--- Thinking ---\n")
				fmt.Println(msg.Content)
				fmt.Println()
			}
		}
	}

	return nil
}

type threadsGetFormat int

const (
	threadsGetFormatTable threadsGetFormat = iota
	threadsGetFormatJSON
	threadsGetFormatYAML
)

func resolveThreadsGetFormat(flag string) threadsGetFormat {
	switch strings.ToLower(flag) {
	case "json":
		return threadsGetFormatJSON
	case "yaml":
		return threadsGetFormatYAML
	case "":
		if agentmode.Enabled {
			return threadsGetFormatJSON
		}
		return threadsGetFormatTable
	default:
		return threadsGetFormatTable
	}
}

func resolveThreadsCfg(ctx context.Context, apiURL, authToken string) (string, string, error) {
	if resolved := profiles.FromContext(ctx); resolved != nil {
		if apiURL == "" {
			apiURL = resolved.ApiUrl
		}
		if authToken == "" {
			authToken = resolved.AuthToken
		}
	}

	if apiURL == "" {
		return "", "", fmt.Errorf("API URL is required\nHint: set it via --api-url, DASH0_API_URL, or a profile")
	}
	if authToken == "" {
		return "", "", fmt.Errorf("auth token is required\nHint: set it via --auth-token, DASH0_AUTH_TOKEN, or a profile")
	}

	return apiURL, authToken, nil
}
