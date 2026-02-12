package config

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal"
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

			// Check if environment variables are being used
			envApiUrl := os.Getenv("DASH0_API_URL")
			envAuthToken := os.Getenv("DASH0_AUTH_TOKEN")
			envOtlpUrl := os.Getenv("DASH0_OTLP_URL")

			activeProfile, err := configService.GetActiveProfile()
			fmt.Printf("Profile:    ")
			if err != nil {
				fmt.Printf("(none)\n")
			} else {
				fmt.Printf("%s\n", activeProfile.Name)
			}

			config, _ := configService.GetActiveConfiguration()

			apiUrl := ""
			authToken := ""
			otlpUrl := ""
			if config != nil {
				apiUrl = config.ApiUrl
				authToken = config.AuthToken
				otlpUrl = config.OtlpUrl
			}

			if apiUrl != "" {
				fmt.Printf("API URL:    %s", apiUrl)
				if envApiUrl != "" {
					fmt.Printf("    (from DASH0_API_URL environment variable)")
				}
			} else {
				fmt.Printf("API URL:    (not set)")
			}
			fmt.Println()

			if otlpUrl != "" {
				fmt.Printf("OTLP URL:   %s", otlpUrl)
				if envOtlpUrl != "" {
					fmt.Printf("    (from DASH0_OTLP_URL environment variable)")
				}
			} else {
				fmt.Printf("OTLP URL:   (not set)")
			}
			fmt.Println()

			if authToken != "" {
				fmt.Printf("Auth Token: %s", maskToken(authToken))
				if envAuthToken != "" {
					fmt.Printf("    (from DASH0_AUTH_TOKEN environment variable)")
				}
			} else {
				fmt.Printf("Auth Token: (not set)")
			}
			fmt.Println()

			return nil
		},
	}
}

// maskToken masks all but the first and last 4 characters of a token
func maskToken(token string) string {
	if len(token) <= 12 {
		return "********"
	}

	return "..." + token[len(token)-7:]
}

// newProfileCmd creates a new profiles command
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profiles",
		Aliases: []string{"profile"},
		Short:   "Manage configuration profiles",
		Long:    `Create, list, delete, and select configuration profiles`,
	}

	// Add subcommands
	cmd.AddCommand(newCreateProfileCmd())
	cmd.AddCommand(newUpdateProfileCmd())
	cmd.AddCommand(newListProfileCmd())
	cmd.AddCommand(newDeleteProfileCmd())
	cmd.AddCommand(newSelectProfileCmd())

	return cmd
}

// newCreateProfileCmd creates a new create profile command
func newCreateProfileCmd() *cobra.Command {
	var apiUrl, authToken, otlpUrl string

	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"add"},
		Short:   "Create a new configuration profile",
		Long:    `Create a new named configuration profile with API URL, OTLP URL and auth token`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			configService, err := NewService()
			if err != nil {
				return err
			}

			if authToken == "" {
				return fmt.Errorf("auth-token is required")
			}

			if apiUrl == "" && otlpUrl == "" {
				return fmt.Errorf("at least one of api-url or otlp-url is required")
			}

			profile := Profile{
				Name: name,
				Configuration: Configuration{
					ApiUrl:    apiUrl,
					AuthToken: authToken,
					OtlpUrl:   otlpUrl,
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

	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")
	cmd.Flags().StringVar(&otlpUrl, "otlp-url", "", "OTLP endpoint URL for sending telemetry data")

	return cmd
}

// newUpdateProfileCmd creates a new update profile command
func newUpdateProfileCmd() *cobra.Command {
	var apiUrl, authToken, otlpUrl string

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a configuration profile",
		Long:  `Update an existing configuration profile. Only the specified flags are changed; unspecified flags are left as-is. Pass an empty string to remove a field.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			apiUrlChanged := cmd.Flags().Changed("api-url")
			authTokenChanged := cmd.Flags().Changed("auth-token")
			otlpUrlChanged := cmd.Flags().Changed("otlp-url")

			if !apiUrlChanged && !authTokenChanged && !otlpUrlChanged {
				return fmt.Errorf("at least one of --api-url, --auth-token, or --otlp-url must be specified")
			}

			configService, err := NewService()
			if err != nil {
				return err
			}

			if err := configService.UpdateProfile(name, func(cfg *Configuration) {
				if apiUrlChanged {
					cfg.ApiUrl = apiUrl
				}
				if authTokenChanged {
					cfg.AuthToken = authToken
				}
				if otlpUrlChanged {
					cfg.OtlpUrl = otlpUrl
				}
			}); err != nil {
				return fmt.Errorf("failed to update profile: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Profile updated successfully")
			fmt.Printf("Profile '%s' updated successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")
	cmd.Flags().StringVar(&otlpUrl, "otlp-url", "", "OTLP endpoint URL for sending telemetry data")

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

			// Check if any profile has an OTLP URL configured
			hasOtlpUrl := false
			for _, profile := range profiles {
				if profile.Configuration.OtlpUrl != "" {
					hasOtlpUrl = true
					break
				}
			}

			// Calculate column widths (including 2 chars for active marker "* ")
			nameWidth := len(internal.HEADER_NAME)
			apiUrlWidth := len("API URL")
			otlpUrlWidth := len("OTLP URL")
			authTokenWidth := len("AUTH TOKEN")

			for _, profile := range profiles {
				if len(profile.Name) > nameWidth {
					nameWidth = len(profile.Name)
				}
				if len(profile.Configuration.ApiUrl) > apiUrlWidth {
					apiUrlWidth = len(profile.Configuration.ApiUrl)
				}
				if len(profile.Configuration.OtlpUrl) > otlpUrlWidth {
					otlpUrlWidth = len(profile.Configuration.OtlpUrl)
				}
				maskedToken := maskToken(profile.Configuration.AuthToken)
				if len(maskedToken) > authTokenWidth {
					authTokenWidth = len(maskedToken)
				}
			}

			// Print header
			if hasOtlpUrl {
				fmt.Printf("  %-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", apiUrlWidth, "API URL", otlpUrlWidth, "OTLP URL", "AUTH TOKEN")
			} else {
				fmt.Printf("  %-*s  %-*s  %s\n", nameWidth, "NAME", apiUrlWidth, "API URL", "AUTH TOKEN")
			}

			// Print rows
			for _, profile := range profiles {
				marker := " "
				if profile.Name == activeProfileName {
					marker = "*"
				}
				if hasOtlpUrl {
					fmt.Printf("%s %-*s  %-*s  %-*s  %s\n",
						marker,
						nameWidth, profile.Name,
						apiUrlWidth, profile.Configuration.ApiUrl,
						otlpUrlWidth, profile.Configuration.OtlpUrl,
						maskToken(profile.Configuration.AuthToken))
				} else {
					fmt.Printf("%s %-*s  %-*s  %s\n",
						marker,
						nameWidth, profile.Name,
						apiUrlWidth, profile.Configuration.ApiUrl,
						maskToken(profile.Configuration.AuthToken))
				}
			}

			return nil
		},
	}

	return cmd
}

// newDeleteProfileCmd creates a new delete profile command
func newDeleteProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"remove"},
		Short:   "Delete a configuration profile",
		Long:    `Delete a named configuration profile`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			configService, err := NewService()
			if err != nil {
				return err
			}

			if err := configService.RemoveProfile(name); err != nil {
				return fmt.Errorf("failed to remove profile: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Profile deleted successfully")
			fmt.Printf("Profile '%s' deleted successfully\n", name)

			return nil
		},
	}

	return cmd
}

// newSelectProfileCmd creates a new select profile command
func newSelectProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "select <name>",
		Short: "Select a configuration profile",
		Long:  `Set the active configuration profile`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			configService, err := NewService()
			if err != nil {
				return err
			}

			if err := configService.SetActiveProfile(name); err != nil {
				return fmt.Errorf("failed to select profile: %w", err)
			}

			log.Logger.Info().Str("name", name).Msg("Profile selected successfully")
			fmt.Printf("Profile '%s' is now active\n", name)

			return nil
		},
	}

	return cmd
}
