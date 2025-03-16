package config

import (
	"fmt"

	"github.com/dash0hq/dash0-cli/internal/log"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates a new config command
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Dash0 CLI configuration",
		Long:  `View and manage Dash0 CLI configuration settings`,
	}

	// Add subcommands
	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newContextCmd())

	return cmd
}

// newShowCmd creates a new show command
func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long:  `Display the current active configuration`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			config, err := configService.GetActiveConfiguration()
			if err != nil {
				return fmt.Errorf("failed to get active configuration: %w", err)
			}

			fmt.Printf("Base URL: %s\n", config.BaseURL)
			fmt.Printf("Auth Token: %s\n", maskToken(config.AuthToken))

			return nil
		},
	}
}

// maskToken masks all but the first and last 4 characters of a token
func maskToken(token string) string {
	if len(token) <= 8 {
		return "********"
	}

	return token[:4] + "..." + token[len(token)-4:]
}

// newContextCmd creates a new context command
func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage configuration contexts",
		Long:  `Add, list, remove, and select configuration contexts`,
	}

	// Add subcommands
	cmd.AddCommand(newAddContextCmd())
	cmd.AddCommand(newListContextCmd())
	cmd.AddCommand(newRemoveContextCmd())
	cmd.AddCommand(newSelectContextCmd())

	return cmd
}

// newAddContextCmd creates a new add context command
func newAddContextCmd() *cobra.Command {
	var baseURL, authToken, name string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new configuration context",
		Long:  `Add a new named configuration context with base URL and auth token`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			if name == "" || baseURL == "" || authToken == "" {
				return fmt.Errorf("name, base-url, and auth-token are required")
			}

			context := Context{
				Name: name,
				Configuration: Configuration{
					BaseURL:   baseURL,
					AuthToken: authToken,
				},
			}

			if err := configService.AddContext(context); err != nil {
				return fmt.Errorf("failed to add context: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Context added successfully")
			fmt.Printf("Context '%s' added successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the context")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")

	return cmd
}

// newListContextCmd creates a new list context command
func newListContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all configuration contexts",
		Long:    `Display a list of all available configuration contexts`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			contexts, err := configService.GetContexts()
			if err != nil {
				return fmt.Errorf("failed to get contexts: %w", err)
			}

			if len(contexts) == 0 {
				fmt.Println("No contexts configured")
				return nil
			}

			activeContextName := ""
			activeContext, err := configService.GetActiveContext()
			if err == nil && activeContext != nil {
				activeContextName = activeContext.Name
			}

			fmt.Println("Available contexts:")
			for _, context := range contexts {
				if context.Name == activeContextName {
					fmt.Printf("* %s\n", context.Name)
				} else {
					fmt.Printf("  %s\n", context.Name)
				}
			}

			return nil
		},
	}

	return cmd
}

// newRemoveContextCmd creates a new remove context command
func newRemoveContextCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a configuration context",
		Long:  `Remove a named configuration context`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("name is required")
			}

			if err := configService.RemoveContext(name); err != nil {
				return fmt.Errorf("failed to remove context: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Context removed successfully")
			fmt.Printf("Context '%s' removed successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the context to remove")

	return cmd
}

// newSelectContextCmd creates a new select context command
func newSelectContextCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "select",
		Short: "Select a configuration context",
		Long:  `Set the active configuration context`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("name is required")
			}

			if err := configService.SetActiveContext(name); err != nil {
				return fmt.Errorf("failed to select context: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Context selected successfully")
			fmt.Printf("Context '%s' is now active\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the context to select")

	return cmd
}
