package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// setupTestConfigDir creates a temporary directory for testing and returns its path
func setupTestConfigDir(t *testing.T) string {
	t.Helper()
	
	// Create a temp directory
	tempDir := t.TempDir()
	
	// Override the config path for testing
	os.Setenv("DASH0_CONFIG_DIR", tempDir)
	
	// Enable test mode to bypass some validations
	os.Setenv("DASH0_TEST_MODE", "1")
	
	return tempDir
}

// createTestContextsFile creates a test contexts file in the specified directory
func createTestContextsFile(t *testing.T, configDir string, contexts []Context) {
	t.Helper()
	
	// Create contexts file
	contextsFile := ContextsFile{Contexts: contexts}
	data, err := json.MarshalIndent(contextsFile, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal contexts file: %v", err)
	}
	
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	
	contextsFilePath := filepath.Join(configDir, ContextsFileName)
	if err := os.WriteFile(contextsFilePath, data, 0644); err != nil {
		t.Fatalf("Failed to write contexts file: %v", err)
	}
}

// setActiveContext sets the active context for testing
func setActiveContext(t *testing.T, configDir, contextName string) {
	t.Helper()
	
	activeContextPath := filepath.Join(configDir, ActiveContextFileName)
	if err := os.WriteFile(activeContextPath, []byte(contextName), 0644); err != nil {
		t.Fatalf("Failed to write active context: %v", err)
	}
}

// TestServiceGetContexts tests the GetContexts method
func TestServiceGetContexts(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)
	
	// Create test contexts
	testContexts := []Context{
		{
			Name: "test1",
			Configuration: Configuration{
				BaseURL:   "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				BaseURL:   "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}
	
	createTestContextsFile(t, configDir, testContexts)
	
	// Create service and test GetContexts
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	contexts, err := service.GetContexts()
	if err != nil {
		t.Fatalf("Failed to get contexts: %v", err)
	}
	
	// Validate result
	if len(contexts) != len(testContexts) {
		t.Errorf("Expected %d contexts, got %d", len(testContexts), len(contexts))
	}
	
	for i, ctx := range contexts {
		if ctx.Name != testContexts[i].Name {
			t.Errorf("Expected context name %s, got %s", testContexts[i].Name, ctx.Name)
		}
		if ctx.Configuration.BaseURL != testContexts[i].Configuration.BaseURL {
			t.Errorf("Expected base URL %s, got %s", testContexts[i].Configuration.BaseURL, ctx.Configuration.BaseURL)
		}
		if ctx.Configuration.AuthToken != testContexts[i].Configuration.AuthToken {
			t.Errorf("Expected auth token %s, got %s", testContexts[i].Configuration.AuthToken, ctx.Configuration.AuthToken)
		}
	}
}

// TestServiceGetActiveContext tests the GetActiveContext method
func TestServiceGetActiveContext(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)
	
	// Create test contexts
	testContexts := []Context{
		{
			Name: "test1",
			Configuration: Configuration{
				BaseURL:   "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				BaseURL:   "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}
	
	createTestContextsFile(t, configDir, testContexts)
	setActiveContext(t, configDir, "test2")
	
	// Create service and test GetActiveContext
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	context, err := service.GetActiveContext()
	if err != nil {
		t.Fatalf("Failed to get active context: %v", err)
	}
	
	// Validate result
	if context.Name != "test2" {
		t.Errorf("Expected active context name test2, got %s", context.Name)
	}
	if context.Configuration.BaseURL != "https://test2.example.com" {
		t.Errorf("Expected base URL https://test2.example.com, got %s", context.Configuration.BaseURL)
	}
	if context.Configuration.AuthToken != "token2" {
		t.Errorf("Expected auth token token2, got %s", context.Configuration.AuthToken)
	}
}

// TestServiceAddContext tests the AddContext method
func TestServiceAddContext(t *testing.T) {
	// Setup test environment
	_ = setupTestConfigDir(t)
	
	// Create service
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Add a new context
	newContext := Context{
		Name: "new-context",
		Configuration: Configuration{
			BaseURL:   "https://new.example.com",
			AuthToken: "new-token",
		},
	}
	
	err = service.AddContext(newContext)
	if err != nil {
		t.Fatalf("Failed to add context: %v", err)
	}
	
	// Validate result
	contexts, err := service.GetContexts()
	if err != nil {
		t.Fatalf("Failed to get contexts: %v", err)
	}
	
	if len(contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(contexts))
	}
	
	if contexts[0].Name != "new-context" {
		t.Errorf("Expected context name new-context, got %s", contexts[0].Name)
	}
	
	// Check if this context was set as active (it should be, as it's the first one)
	activeContext, err := service.GetActiveContext()
	if err != nil {
		t.Fatalf("Failed to get active context: %v", err)
	}
	
	if activeContext.Name != "new-context" {
		t.Errorf("Expected active context name new-context, got %s", activeContext.Name)
	}
}

// TestServiceRemoveContext tests the RemoveContext method
func TestServiceRemoveContext(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)
	
	// Create test contexts
	testContexts := []Context{
		{
			Name: "test1",
			Configuration: Configuration{
				BaseURL:   "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				BaseURL:   "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}
	
	createTestContextsFile(t, configDir, testContexts)
	setActiveContext(t, configDir, "test2")
	
	// Create service and remove a context
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	err = service.RemoveContext("test2")
	if err != nil {
		t.Fatalf("Failed to remove context: %v", err)
	}
	
	// Validate result
	contexts, err := service.GetContexts()
	if err != nil {
		t.Fatalf("Failed to get contexts: %v", err)
	}
	
	if len(contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(contexts))
	}
	
	if contexts[0].Name != "test1" {
		t.Errorf("Expected context name test1, got %s", contexts[0].Name)
	}
	
	// Check if active context was updated
	activeContext, err := service.GetActiveContext()
	if err != nil {
		t.Fatalf("Failed to get active context: %v", err)
	}
	
	if activeContext.Name != "test1" {
		t.Errorf("Expected active context name test1, got %s", activeContext.Name)
	}
}

// TestServiceGetActiveConfiguration tests the GetActiveConfiguration method
func TestServiceGetActiveConfiguration(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)
	
	// Test with environment variables
	t.Run("With environment variables", func(t *testing.T) {
		os.Setenv("DASH0_URL", "https://env.example.com")
		os.Setenv("DASH0_AUTH_TOKEN", "env-token")
		defer func() {
			os.Unsetenv("DASH0_URL")
			os.Unsetenv("DASH0_AUTH_TOKEN")
		}()
		
		service, err := NewService()
		if err != nil {
			t.Fatalf("Failed to create service: %v", err)
		}
		
		config, err := service.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}
		
		if config.BaseURL != "https://env.example.com" {
			t.Errorf("Expected base URL https://env.example.com, got %s", config.BaseURL)
		}
		if config.AuthToken != "env-token" {
			t.Errorf("Expected auth token env-token, got %s", config.AuthToken)
		}
	})
	
	// Test with active context
	t.Run("With active context", func(t *testing.T) {
		// Unset environment variables
		os.Unsetenv("DASH0_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")
		
		// Create test contexts
		testContexts := []Context{
			{
				Name: "test1",
				Configuration: Configuration{
					BaseURL:   "https://test1.example.com",
					AuthToken: "token1",
				},
			},
		}
		
		createTestContextsFile(t, configDir, testContexts)
		setActiveContext(t, configDir, "test1")
		
		service, err := NewService()
		if err != nil {
			t.Fatalf("Failed to create service: %v", err)
		}
		
		config, err := service.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}
		
		if config.BaseURL != "https://test1.example.com" {
			t.Errorf("Expected base URL https://test1.example.com, got %s", config.BaseURL)
		}
		if config.AuthToken != "token1" {
			t.Errorf("Expected auth token token1, got %s", config.AuthToken)
		}
	})
}