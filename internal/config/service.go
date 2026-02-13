package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// ConfigDirName is the name of the directory containing Dash0 configuration
	ConfigDirName = ".dash0"
	// ProfilesFileName is the name of the file containing profile configurations
	ProfilesFileName = "profiles.json"
	// ActiveProfileFileName is the name of the file containing the active profile name
	ActiveProfileFileName = "activeProfile"
)

var (
	// ErrNoActiveProfile is returned when there is no active profile
	ErrNoActiveProfile = errors.New("no active profile configured; run 'dash0 config profiles create <name> --api-url <url> --auth-token <token>' to create one")
	// ErrProfileNotFound is returned when a requested profile is not found
	ErrProfileNotFound = errors.New("profile not found")
)

// Service handles configuration operations
type Service struct {
	configDir string
}

// NewService creates a new configuration service
func NewService() (*Service, error) {
	// Check if DASH0_CONFIG_DIR is set for testing
	if configDir := os.Getenv("DASH0_CONFIG_DIR"); configDir != "" {
		return &Service{configDir: configDir}, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ConfigDirName)
	return &Service{configDir: configDir}, nil
}

// GetActiveConfiguration returns the currently active configuration
// Environment variables take precedence over the active profile
func (s *Service) GetActiveConfiguration() (*Configuration, error) {
	envApiUrl := os.Getenv("DASH0_API_URL")
	envAuthToken := os.Getenv("DASH0_AUTH_TOKEN")
	envOtlpUrl := os.Getenv("DASH0_OTLP_URL")
	envDataset := os.Getenv("DASH0_DATASET")

	// If auth token and at least one URL are set via env vars, use them directly without requiring a profile
	if envAuthToken != "" && (envApiUrl != "" || envOtlpUrl != "") {
		return &Configuration{
			ApiUrl:    envApiUrl,
			AuthToken: envAuthToken,
			OtlpUrl:   envOtlpUrl,
			Dataset:   envDataset,
		}, nil
	}

	// Otherwise, start with the active profile
	activeProfile, err := s.GetActiveProfile()
	if err != nil {
		return nil, err
	}

	activeConfiguration := &activeProfile.Configuration

	// Override with env vars if set
	if envApiUrl != "" {
		activeConfiguration.ApiUrl = envApiUrl
	}
	if envAuthToken != "" {
		activeConfiguration.AuthToken = envAuthToken
	}
	if envOtlpUrl != "" {
		activeConfiguration.OtlpUrl = envOtlpUrl
	}
	if envDataset != "" {
		activeConfiguration.Dataset = envDataset
	}

	return activeConfiguration, nil
}

// ResolveConfiguration loads configuration with override handling.
// It loads the active profile, applies environment variable and flag overrides, and validates the result.
// Parameters:
//   - apiUrl: Command-line flag for API URL (empty if not provided)
//   - authToken: Command-line flag for auth token (empty if not provided)
//
// Returns:
//   - Configuration with all overrides applied
//   - Error if configuration couldn't be loaded or is invalid
func ResolveConfiguration(apiUrl, authToken string) (*Configuration, error) {
	return ResolveConfigurationWithOtlp(apiUrl, authToken, "", "")
}

// ResolveConfigurationWithOtlp loads configuration with override handling including OTLP URL and dataset.
// Parameters:
//   - apiUrl: Command-line flag for API URL (empty if not provided)
//   - authToken: Command-line flag for auth token (empty if not provided)
//   - otlpUrl: Command-line flag for OTLP URL (empty if not provided)
//   - dataset: Command-line flag for dataset (empty if not provided)
//
// Returns:
//   - Configuration with all overrides applied
//   - Error if configuration couldn't be loaded or is invalid
func ResolveConfigurationWithOtlp(apiUrl, authToken, otlpUrl, dataset string) (*Configuration, error) {
	// Create result configuration starting with an empty config
	result := &Configuration{}

	// Try to get the active configuration for defaults
	var configErr error
	configService, err := NewService()
	if err == nil {
		// Only attempt to load if service creation was successful
		cfg, err := configService.GetActiveConfiguration()
		if err != nil {
			// Store the error but don't return yet - we might have explicit overrides
			configErr = err
		} else if cfg != nil {
			// Use active configuration as a base
			result.ApiUrl = cfg.ApiUrl
			result.AuthToken = cfg.AuthToken
			result.OtlpUrl = cfg.OtlpUrl
			result.Dataset = cfg.Dataset
		}
	}

	// Override with environment variables (in case GetActiveConfiguration failed
	// before it could apply them, e.g. no active profile but env vars are set)
	if envApiUrl := os.Getenv("DASH0_API_URL"); envApiUrl != "" && result.ApiUrl == "" {
		result.ApiUrl = envApiUrl
	}
	if envAuthToken := os.Getenv("DASH0_AUTH_TOKEN"); envAuthToken != "" && result.AuthToken == "" {
		result.AuthToken = envAuthToken
	}
	if envOtlpUrl := os.Getenv("DASH0_OTLP_URL"); envOtlpUrl != "" && result.OtlpUrl == "" {
		result.OtlpUrl = envOtlpUrl
	}
	if envDataset := os.Getenv("DASH0_DATASET"); envDataset != "" && result.Dataset == "" {
		result.Dataset = envDataset
	}

	// Override with command-line flags if provided (only if non-empty)
	if apiUrl != "" {
		result.ApiUrl = apiUrl
	}

	if authToken != "" {
		result.AuthToken = authToken
	}

	if otlpUrl != "" {
		result.OtlpUrl = otlpUrl
	}

	if dataset != "" {
		result.Dataset = dataset
	}

	// If we had a config error and don't have complete configuration from overrides, return the error
	if configErr != nil && (result.AuthToken == "" || (result.ApiUrl == "" && result.OtlpUrl == "")) {
		return nil, configErr
	}

	// Final validation
	if result.AuthToken == "" {
		return nil, fmt.Errorf("auth-token is required; provide it as a flag or configure a profile")
	}
	if result.ApiUrl == "" && result.OtlpUrl == "" {
		return nil, fmt.Errorf("at least one of api-url or otlp-url is required; provide them as flags or configure a profile")
	}

	return result, nil
}

// GetActiveProfile returns the currently active profile
func (s *Service) GetActiveProfile() (*Profile, error) {
	activeProfileName, err := s.getActiveProfileName()
	if err != nil {
		return nil, err
	}

	profiles, err := s.GetProfiles()
	if err != nil {
		return nil, err
	}

	for _, profile := range profiles {
		if profile.Name == activeProfileName {
			return &profile, nil
		}
	}

	return nil, ErrProfileNotFound
}

// GetProfiles returns all available profiles
func (s *Service) GetProfiles() ([]Profile, error) {
	profilesFilePath := filepath.Join(s.configDir, ProfilesFileName)
	if _, err := os.Stat(profilesFilePath); os.IsNotExist(err) {
		return []Profile{}, nil
	}

	data, err := os.ReadFile(profilesFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read profiles file: %w", err)
	}

	var profilesFile ProfilesFile
	if err := json.Unmarshal(data, &profilesFile); err != nil {
		return nil, fmt.Errorf("failed to parse profiles file %s: %w\nHint: delete the file and reconfigure with 'dash0 config profiles create'", profilesFilePath, err)
	}

	return profilesFile.Profiles, nil
}

// AddProfile adds a new profile to the configuration
func (s *Service) AddProfile(profile Profile) error {
	profiles, err := s.GetProfiles()
	if err != nil {
		return err
	}

	// Check if profile already exists
	for i, existing := range profiles {
		if existing.Name == profile.Name {
			// Replace existing profile
			profiles[i] = profile
			return s.saveProfiles(profiles)
		}
	}

	// Add new profile
	profiles = append(profiles, profile)

	err = s.saveProfiles(profiles)
	if err != nil {
		return err
	}

	// If this is the first profile, make it active
	if len(profiles) == 1 {
		if err := s.SetActiveProfile(profile.Name); err != nil {
			return err
		}
	}

	return nil
}

// UpdateProfile finds a profile by name and applies the updateFn to its configuration, then saves.
// Returns ErrProfileNotFound if no profile with the given name exists.
func (s *Service) UpdateProfile(name string, updateFn func(*Configuration)) error {
	profiles, err := s.GetProfiles()
	if err != nil {
		return err
	}

	found := false
	for i, profile := range profiles {
		if profile.Name == name {
			updateFn(&profiles[i].Configuration)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("profile %q not found", name)
	}

	return s.saveProfiles(profiles)
}

// RemoveProfile removes a profile from the configuration
func (s *Service) RemoveProfile(profileName string) error {
	profiles, err := s.GetProfiles()
	if err != nil {
		return err
	}

	var found bool
	var newProfiles []Profile
	for _, profile := range profiles {
		if profile.Name != profileName {
			newProfiles = append(newProfiles, profile)
		} else {
			found = true
		}
	}

	if !found {
		return ErrProfileNotFound
	}

	// Check if removing the active profile
	activeProfileName, err := s.getActiveProfileName()
	if err == nil && activeProfileName == profileName {
		// Set a new active profile if possible, otherwise clear it
		if len(newProfiles) > 0 {
			if err := s.SetActiveProfile(newProfiles[0].Name); err != nil {
				return err
			}
		} else {
			if err := os.Remove(filepath.Join(s.configDir, ActiveProfileFileName)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove active profile file: %w", err)
			}
		}
	}

	return s.saveProfiles(newProfiles)
}

// SetActiveProfile sets the active profile
func (s *Service) SetActiveProfile(profileName string) error {
	// Verify the profile exists
	profiles, err := s.GetProfiles()
	if err != nil {
		return err
	}

	var found bool
	for _, profile := range profiles {
		if profile.Name == profileName {
			found = true
			break
		}
	}

	if !found {
		return ErrProfileNotFound
	}

	// Ensure config directory exists
	if err := os.MkdirAll(s.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write the active profile name
	activeProfilePath := filepath.Join(s.configDir, ActiveProfileFileName)
	if err := os.WriteFile(activeProfilePath, []byte(profileName), 0644); err != nil {
		return fmt.Errorf("failed to write active profile: %w", err)
	}

	return nil
}

// getActiveProfileName returns the name of the active profile
func (s *Service) getActiveProfileName() (string, error) {
	activeProfilePath := filepath.Join(s.configDir, ActiveProfileFileName)
	if _, err := os.Stat(activeProfilePath); os.IsNotExist(err) {
		return "", ErrNoActiveProfile
	}

	data, err := os.ReadFile(activeProfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read active profile: %w", err)
	}

	activeProfileName := string(data)
	if activeProfileName == "" {
		return "", ErrNoActiveProfile
	}

	return activeProfileName, nil
}

// saveProfiles saves the profiles to the configuration file
func (s *Service) saveProfiles(profiles []Profile) error {
	// Ensure config directory exists
	if err := os.MkdirAll(s.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	profilesFile := ProfilesFile{Profiles: profiles}
	data, err := json.MarshalIndent(profilesFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profiles: %w", err)
	}

	profilesFilePath := filepath.Join(s.configDir, ProfilesFileName)
	if err := os.WriteFile(profilesFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write profiles file: %w", err)
	}

	return nil
}
