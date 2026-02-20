package config

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal"
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
		Long: `Display the current active configuration.

Configuration is resolved in the following order (highest priority first):

  1. Environment variables (DASH0_API_URL, DASH0_OTLP_URL, DASH0_AUTH_TOKEN, DASH0_DATASET)
  2. CLI flags (--api-url, --otlp-url, --auth-token, --dataset)
  3. Active profile settings

When an environment variable overrides a profile value, config show annotates the field with "(from <VAR> environment variable)".

The DASH0_CONFIG_DIR environment variable changes the configuration directory (default: ~/.dash0).`,
		Example: `  # Show the active profile and its settings
  dash0 config show

  # See the effect of an environment variable override
  DASH0_API_URL='https://api.example.com' dash0 config show

  # Use a different configuration directory
  DASH0_CONFIG_DIR=/tmp/dash0-test dash0 config show`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configService, err := NewService()
			if err != nil {
				return err
			}

			// Check if environment variables are being used
			envApiUrl := os.Getenv("DASH0_API_URL")
			envAuthToken := os.Getenv("DASH0_AUTH_TOKEN")
			envOtlpUrl := os.Getenv("DASH0_OTLP_URL")
			envDataset := os.Getenv("DASH0_DATASET")

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
			dataset := ""
			if config != nil {
				apiUrl = config.ApiUrl
				authToken = config.AuthToken
				otlpUrl = config.OtlpUrl
				dataset = config.Dataset
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

			datasetDisplay := dataset
			if datasetDisplay == "" {
				datasetDisplay = "default"
			}
			fmt.Printf("Dataset:    %s", datasetDisplay)
			if envDataset != "" {
				fmt.Printf("    (from DASH0_DATASET environment variable)")
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
	var apiUrl, authToken, otlpUrl, dataset string

	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"add"},
		Short:   "Create a new configuration profile",
		Long:    `Create a new named configuration profile with API URL, OTLP URL and auth token`,
		Example: `  # Create a profile with all settings
  dash0 config profiles create dev \
      --api-url https://api.us-west-2.aws.dash0.com \
      --otlp-url https://ingress.us-west-2.aws.dash0.com \
      --auth-token auth_xxx

  # Create a minimal profile (add settings later with update)
  dash0 config profiles create staging --api-url https://api.example.com`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			configService, err := NewService()
			if err != nil {
				return err
			}

			profile := Profile{
				Name: name,
				Configuration: Configuration{
					ApiUrl:    apiUrl,
					AuthToken: authToken,
					OtlpUrl:   otlpUrl,
					Dataset:   dataset,
				},
			}

			if err := configService.AddProfile(profile); err != nil {
				return fmt.Errorf("failed to add profile: %w", err)
			}

			fmt.Printf("Profile '%s' added successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")
	cmd.Flags().StringVar(&otlpUrl, "otlp-url", "", "OTLP endpoint URL for sending telemetry data")
	cmd.Flags().StringVar(&dataset, "dataset", "", "Dataset to operate on")

	return cmd
}

// newUpdateProfileCmd creates a new update profile command
func newUpdateProfileCmd() *cobra.Command {
	var apiUrl, authToken, otlpUrl, dataset string

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a configuration profile",
		Long:  `Update an existing configuration profile. Only the specified flags are changed; unspecified flags are left as-is. Pass an empty string to remove a field.`,
		Example: `  # Update the API URL of a profile
  dash0 config profiles update prod --api-url https://api.us-east-1.aws.dash0.com

  # Add an OTLP URL to an existing profile
  dash0 config profiles update prod --otlp-url https://ingress.us-east-1.aws.dash0.com

  # Remove a field by passing an empty string
  dash0 config profiles update prod --dataset ''`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			apiUrlChanged := cmd.Flags().Changed("api-url")
			authTokenChanged := cmd.Flags().Changed("auth-token")
			otlpUrlChanged := cmd.Flags().Changed("otlp-url")
			datasetChanged := cmd.Flags().Changed("dataset")

			if !apiUrlChanged && !authTokenChanged && !otlpUrlChanged && !datasetChanged {
				return fmt.Errorf("at least one of --api-url, --auth-token, --otlp-url, or --dataset must be specified")
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
				if datasetChanged {
					cfg.Dataset = dataset
				}
			}); err != nil {
				return fmt.Errorf("failed to update profile: %w", err)
			}

			fmt.Printf("Profile '%s' updated successfully\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")
	cmd.Flags().StringVar(&otlpUrl, "otlp-url", "", "OTLP endpoint URL for sending telemetry data")
	cmd.Flags().StringVar(&dataset, "dataset", "", "Dataset to operate on")

	return cmd
}

// newListProfileCmd creates a new list profile command
func newListProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all configuration profiles",
		Long:    `Display a list of all available configuration profiles`,
		Example: `  # List all profiles (active profile marked with *)
  dash0 config profiles list`,
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
			datasetWidth := len("DATASET")
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
				ds := profile.Configuration.Dataset
				if ds == "" {
					ds = "default"
				}
				if len(ds) > datasetWidth {
					datasetWidth = len(ds)
				}
				maskedToken := maskToken(profile.Configuration.AuthToken)
				if len(maskedToken) > authTokenWidth {
					authTokenWidth = len(maskedToken)
				}
			}

			// Print header
			if hasOtlpUrl {
				fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", apiUrlWidth, "API URL", otlpUrlWidth, "OTLP URL", datasetWidth, "DATASET", "AUTH TOKEN")
			} else {
				fmt.Printf("  %-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", apiUrlWidth, "API URL", datasetWidth, "DATASET", "AUTH TOKEN")
			}

			// Print rows
			for _, profile := range profiles {
				marker := " "
				if profile.Name == activeProfileName {
					marker = "*"
				}
				dataset := profile.Configuration.Dataset
				if dataset == "" {
					dataset = "default"
				}
				if hasOtlpUrl {
					fmt.Printf("%s %-*s  %-*s  %-*s  %-*s  %s\n",
						marker,
						nameWidth, profile.Name,
						apiUrlWidth, profile.Configuration.ApiUrl,
						otlpUrlWidth, profile.Configuration.OtlpUrl,
						datasetWidth, dataset,
						maskToken(profile.Configuration.AuthToken))
				} else {
					fmt.Printf("%s %-*s  %-*s  %-*s  %s\n",
						marker,
						nameWidth, profile.Name,
						apiUrlWidth, profile.Configuration.ApiUrl,
						datasetWidth, dataset,
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
		Example: `  # Delete a profile
  dash0 config profiles delete staging`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			configService, err := NewService()
			if err != nil {
				return err
			}

			if err := configService.RemoveProfile(name); err != nil {
				return fmt.Errorf("failed to remove profile: %w", err)
			}

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
		Example: `  # Switch to a different profile
  dash0 config profiles select prod`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			configService, err := NewService()
			if err != nil {
				return err
			}

			if err := configService.SetActiveProfile(name); err != nil {
				return fmt.Errorf("failed to select profile: %w", err)
			}

			fmt.Printf("Profile '%s' is now active\n", name)

			return nil
		},
	}

	return cmd
}
