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
	cmd.AddCommand(newProfileCmd())

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

			fmt.Printf("API URL: %s\n", config.ApiUrl)
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

// newProfileCmd creates a new profile command
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
		Long:  `Add, list, remove, and select configuration profiles`,
	}

	// Add subcommands
	cmd.AddCommand(newAddProfileCmd())
	cmd.AddCommand(newListProfileCmd())
	cmd.AddCommand(newRemoveProfileCmd())
	cmd.AddCommand(newSelectProfileCmd())

	return cmd
}

// newAddProfileCmd creates a new add profile command
func newAddProfileCmd() *cobra.Command {
	var apiUrl, authToken, name string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new configuration profile",
		Long:  `Add a new named configuration profile with API URL and auth token`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			if name == "" || apiUrl == "" || authToken == "" {
				return fmt.Errorf("name, api-url, and auth-token are required")
			}

			profile := Profile{
				Name: name,
				Configuration: Configuration{
					ApiUrl:    apiUrl,
					AuthToken: authToken,
				},
			}

			if err := configService.AddProfile(profile); err != nil {
				return fmt.Errorf("failed to add profile: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Profile added successfully")
			fmt.Printf("Profile '%s' added successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the profile")
	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")

	return cmd
}

// newListProfileCmd creates a new list profile command
func newListProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all configuration profiles",
		Long:    `Display a list of all available configuration profiles`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			profiles, err := configService.GetProfiles()
			if err != nil {
				return fmt.Errorf("failed to get profiles: %w", err)
			}

			if len(profiles) == 0 {
				fmt.Println("No profiles configured")
				return nil
			}

			activeProfileName := ""
			activeProfile, err := configService.GetActiveProfile()
			if err == nil && activeProfile != nil {
				activeProfileName = activeProfile.Name
			}

			fmt.Println("Available profiles:")
			for _, profile := range profiles {
				if profile.Name == activeProfileName {
					fmt.Printf("* %s\n", profile.Name)
				} else {
					fmt.Printf("  %s\n", profile.Name)
				}
			}

			return nil
		},
	}

	return cmd
}

// newRemoveProfileCmd creates a new remove profile command
func newRemoveProfileCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a configuration profile",
		Long:  `Remove a named configuration profile`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("name is required")
			}

			if err := configService.RemoveProfile(name); err != nil {
				return fmt.Errorf("failed to remove profile: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Profile removed successfully")
			fmt.Printf("Profile '%s' removed successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the profile to remove")

	return cmd
}

// newSelectProfileCmd creates a new select profile command
func newSelectProfileCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "select",
		Short: "Select a configuration profile",
		Long:  `Set the active configuration profile`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			if name == "" {
				return fmt.Errorf("name is required")
			}

			if err := configService.SetActiveProfile(name); err != nil {
				return fmt.Errorf("failed to select profile: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Profile selected successfully")
			fmt.Printf("Profile '%s' is now active\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the profile to select")

	return cmd
}
