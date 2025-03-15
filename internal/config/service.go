package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dash0/dash0-cli/pkg/log"
)

const (
	// ConfigDirName is the name of the directory containing Dash0 configuration
	ConfigDirName = ".dash0"
	// ContextsFileName is the name of the file containing context configurations
	ContextsFileName = "contexts.json"
	// ActiveContextFileName is the name of the file containing the active context name
	ActiveContextFileName = "activeContext"
)

var (
	// ErrNoActiveContext is returned when there is no active context
	ErrNoActiveContext = errors.New("no active context configured")
	// ErrContextNotFound is returned when a requested context is not found
	ErrContextNotFound = errors.New("context not found")
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
func (s *Service) GetActiveConfiguration() (*Configuration, error) {
	// Check environment variables first
	baseURL := os.Getenv("DASH0_URL")
	authToken := os.Getenv("DASH0_AUTH_TOKEN")

	if baseURL != "" && authToken != "" {
		log.Logger.Debug().Msg("Using configuration from environment variables")
		return &Configuration{
			BaseURL:   baseURL,
			AuthToken: authToken,
		}, nil
	}

	// Otherwise, try to get configuration from the active context
	activeContext, err := s.GetActiveContext()
	if err != nil {
		return nil, err
	}

	return &activeContext.Configuration, nil
}

// ResolveConfiguration loads configuration with override handling
// It handles test mode detection, configuration loading, and validation of base URL and auth token
// Parameters:
//   - baseURL: Command-line flag for base URL (empty if not provided)
//   - authToken: Command-line flag for auth token (empty if not provided)
//
// Returns:
//   - Configuration with all overrides applied
//   - Error if configuration couldn't be loaded or is invalid outside of test mode
func ResolveConfiguration(baseURL, authToken string) (*Configuration, error) {
	// Create result configuration starting with an empty config
	result := &Configuration{}

	// Try to get the active configuration for defaults
	configService, err := NewService()
	if err == nil {
		// Only attempt to load if service creation was successful
		cfg, err := configService.GetActiveConfiguration()
		if err == nil && cfg != nil {
			// Use active configuration as a base
			result.BaseURL = cfg.BaseURL
			result.AuthToken = cfg.AuthToken
		}
	}

	// Override with command-line flags if provided (only if non-empty)
	if baseURL != "" {
		result.BaseURL = baseURL
	}

	if authToken != "" {
		result.AuthToken = authToken
	}

	// Final validation (skip in test mode)
	testMode := os.Getenv("DASH0_TEST_MODE") == "1"
	if !testMode && (result.BaseURL == "" || result.AuthToken == "") {
		return nil, fmt.Errorf("base-url and auth-token are required; provide them as flags or configure a context")
	}

	return result, nil
}

// GetActiveContext returns the currently active context
func (s *Service) GetActiveContext() (*Context, error) {
	activeContextName, err := s.getActiveContextName()
	if err != nil {
		return nil, err
	}

	contexts, err := s.GetContexts()
	if err != nil {
		return nil, err
	}

	for _, context := range contexts {
		if context.Name == activeContextName {
			return &context, nil
		}
	}

	return nil, ErrContextNotFound
}

// GetContexts returns all available contexts
func (s *Service) GetContexts() ([]Context, error) {
	contextsFilePath := filepath.Join(s.configDir, ContextsFileName)
	if _, err := os.Stat(contextsFilePath); os.IsNotExist(err) {
		return []Context{}, nil
	}

	data, err := os.ReadFile(contextsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read contexts file: %w", err)
	}

	var contextsFile ContextsFile
	if err := json.Unmarshal(data, &contextsFile); err != nil {
		return nil, fmt.Errorf("failed to parse contexts file: %w", err)
	}

	return contextsFile.Contexts, nil
}

// AddContext adds a new context to the configuration
func (s *Service) AddContext(context Context) error {
	contexts, err := s.GetContexts()
	if err != nil {
		return err
	}

	// Check if context already exists
	for i, existing := range contexts {
		if existing.Name == context.Name {
			// Replace existing context
			contexts[i] = context
			return s.saveContexts(contexts)
		}
	}

	// Add new context
	contexts = append(contexts, context)

	err = s.saveContexts(contexts)
	if err != nil {
		return err
	}

	// If this is the first context, make it active
	if len(contexts) == 1 {
		if err := s.SetActiveContext(context.Name); err != nil {
			return err
		}
	}

	return nil
}

// RemoveContext removes a context from the configuration
func (s *Service) RemoveContext(contextName string) error {
	contexts, err := s.GetContexts()
	if err != nil {
		return err
	}

	var found bool
	var newContexts []Context
	for _, context := range contexts {
		if context.Name != contextName {
			newContexts = append(newContexts, context)
		} else {
			found = true
		}
	}

	if !found {
		return ErrContextNotFound
	}

	// Check if removing the active context
	activeContextName, err := s.getActiveContextName()
	if err == nil && activeContextName == contextName {
		// Set a new active context if possible, otherwise clear it
		if len(newContexts) > 0 {
			if err := s.SetActiveContext(newContexts[0].Name); err != nil {
				return err
			}
		} else {
			if err := os.Remove(filepath.Join(s.configDir, ActiveContextFileName)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove active context file: %w", err)
			}
		}
	}

	return s.saveContexts(newContexts)
}

// SetActiveContext sets the active context
func (s *Service) SetActiveContext(contextName string) error {
	// During testing we may need to bypass context validation
	if os.Getenv("DASH0_TEST_MODE") != "1" {
		// Verify the context exists
		contexts, err := s.GetContexts()
		if err != nil {
			return err
		}

		var found bool
		for _, context := range contexts {
			if context.Name == contextName {
				found = true
				break
			}
		}

		if !found {
			return ErrContextNotFound
		}
	}

	// Ensure config directory exists
	if err := os.MkdirAll(s.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write the active context name
	activeContextPath := filepath.Join(s.configDir, ActiveContextFileName)
	if err := os.WriteFile(activeContextPath, []byte(contextName), 0644); err != nil {
		return fmt.Errorf("failed to write active context: %w", err)
	}

	return nil
}

// getActiveContextName returns the name of the active context
func (s *Service) getActiveContextName() (string, error) {
	activeContextPath := filepath.Join(s.configDir, ActiveContextFileName)
	if _, err := os.Stat(activeContextPath); os.IsNotExist(err) {
		return "", ErrNoActiveContext
	}

	data, err := os.ReadFile(activeContextPath)
	if err != nil {
		return "", fmt.Errorf("failed to read active context: %w", err)
	}

	activeContextName := string(data)
	if activeContextName == "" {
		return "", ErrNoActiveContext
	}

	return activeContextName, nil
}

// saveContexts saves the contexts to the configuration file
func (s *Service) saveContexts(contexts []Context) error {
	// Ensure config directory exists
	if err := os.MkdirAll(s.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	contextsFile := ContextsFile{Contexts: contexts}
	data, err := json.MarshalIndent(contextsFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal contexts: %w", err)
	}

	contextsFilePath := filepath.Join(s.configDir, ContextsFileName)
	if err := os.WriteFile(contextsFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write contexts file: %w", err)
	}

	return nil
}
