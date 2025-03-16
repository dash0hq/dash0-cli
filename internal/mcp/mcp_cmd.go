package mcp

import (
	"fmt"
	"github.com/dash0hq/dash0-cli/internal/mcp/tools"

	"github.com/dash0hq/dash0-cli/internal/config"
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
			// Resolve configuration with overrides
			cfg, err := config.ResolveConfiguration(baseURL, authToken)
			if err != nil {
				return err
			}

			// Use resolved configuration values
			baseURL = cfg.BaseURL
			authToken = cfg.AuthToken

			// Create a new MCP server with logging
			s := server.NewMCPServer(
				"Dash0",
				version,
				server.WithResourceCapabilities(true, true),
				server.WithLogging(),
			)

			tools.AddTools(s)

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
