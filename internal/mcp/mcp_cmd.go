package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0/dash0-cli/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// NewMCPCmd creates a new MCP command
func NewMCPCmd(version string) *cobra.Command {
	var baseURL string
	var authToken string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start an MCP server",
		Long:  `Start a Model Control Protocol (MCP) server that provides tools for AI agents`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get configuration, prioritizing command-line flags
			var cfg *config.Configuration

			// Check if we're in test mode
			testMode := os.Getenv("DASH0_TEST_MODE") == "1"

			// Try to get the active configuration if flags aren't specified
			if !testMode && (baseURL == "" || authToken == "") {
				configService, err := config.NewService()
				if err != nil {
					return fmt.Errorf("failed to initialize config service: %w", err)
				}

				cfg, err = configService.GetActiveConfiguration()
				if err != nil {
					// Only error if we actually need the config values
					if baseURL == "" || authToken == "" {
						return fmt.Errorf("failed to get active configuration: %w", err)
					}
				}
			}

			// Override with command-line flags if provided
			if baseURL == "" && cfg != nil {
				baseURL = cfg.BaseURL
			}

			if authToken == "" && cfg != nil {
				authToken = cfg.AuthToken
			}

			// Final validation (skip in test mode)
			if !testMode && (baseURL == "" || authToken == "") {
				return fmt.Errorf("base-url and auth-token are required; provide them as flags or configure a context")
			}

			// Create a new MCP server with logging
			s := server.NewMCPServer(
				"Dash0",
				version,
				server.WithResourceCapabilities(true, true),
				server.WithLogging(),
			)

			// Add hello world tool
			helloWorldTool := mcp.NewTool("hello_world",
				mcp.WithDescription("A tool that responds with hello world"),
			)

			// Register hello world handler
			s.AddTool(helloWorldTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultText("Hello, World!"), nil
			})

			// Start the server - this will block until the server is stopped
			if err := server.ServeStdio(s); err != nil {
				return fmt.Errorf("failed to start MCP server: %w", err)
			}

			return nil
		},
	}

	// Register flags
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL for the Dash0 API (overrides active context)")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Auth token for the Dash0 API (overrides active context)")

	return cmd
}
