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

// TestResolveConfiguration tests the ResolveConfiguration function
func TestResolveConfiguration(t *testing.T) {
	// Test with command-line flags
	t.Run("With command-line flags", func(t *testing.T) {
		// Setup test environment
		_ = setupTestConfigDir(t)
		
		config, err := ResolveConfiguration("https://flag.example.com", "flag-token")
		if err != nil {
			t.Fatalf("Failed to resolve configuration: %v", err)
		}
		
		if config.BaseURL != "https://flag.example.com" {
			t.Errorf("Expected base URL https://flag.example.com, got %s", config.BaseURL)
		}
		if config.AuthToken != "flag-token" {
			t.Errorf("Expected auth token flag-token, got %s", config.AuthToken)
		}
	})
	
	// Test with active context and partial override
	t.Run("With active context and partial override", func(t *testing.T) {
		// Create a completely new temporary directory for this test
		tempDir, err := os.MkdirTemp("", "resolve-config-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Set environment variables for this test only
		os.Setenv("DASH0_CONFIG_DIR", tempDir)
		defer os.Unsetenv("DASH0_CONFIG_DIR")
		
		// Enable test mode
		os.Setenv("DASH0_TEST_MODE", "1")
		defer os.Unsetenv("DASH0_TEST_MODE")
		
		// Unset environment variables that might interfere
		os.Unsetenv("DASH0_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")
		
		// Create test contexts with explicit test values
		testContexts := []Context{
			{
				Name: "override-test",
				Configuration: Configuration{
					BaseURL:   "https://original.example.com",
					AuthToken: "original-token",
				},
			},
		}
		
		// Set up context file and active context
		contextsFile := ContextsFile{Contexts: testContexts}
		data, _ := json.Marshal(contextsFile)
		os.MkdirAll(tempDir, 0755)
		os.WriteFile(filepath.Join(tempDir, ContextsFileName), data, 0644)
		os.WriteFile(filepath.Join(tempDir, ActiveContextFileName), []byte("override-test"), 0644)
		
		// Verify that the active configuration loads correctly
		svc, _ := NewService()
		origCfg, origErr := svc.GetActiveConfiguration()
		if origErr != nil {
			t.Fatalf("Failed to get original config: %v", origErr)
		}
		t.Logf("Original config before resolve: %+v", origCfg)
		
		// Test with partial override (only base URL)
		resolvedCfg, resolveErr := ResolveConfiguration("https://override.example.com", "")
		if resolveErr != nil {
			t.Fatalf("Failed to resolve configuration: %v", resolveErr)
		}
		t.Logf("Resolved config: %+v", resolvedCfg)
		
		// Test assertions
		if resolvedCfg.BaseURL != "https://override.example.com" {
			t.Errorf("Expected base URL https://override.example.com, got %s", resolvedCfg.BaseURL)
		}
		if resolvedCfg.AuthToken != "original-token" {
			t.Errorf("Expected auth token original-token, got %s", resolvedCfg.AuthToken)
		}
	})
	
	// Test test mode
	t.Run("In test mode without configuration", func(t *testing.T) {
		// Setup test environment without active context
		_ = setupTestConfigDir(t)
		
		// Test in test mode (should bypass validation)
		os.Setenv("DASH0_TEST_MODE", "1")
		defer os.Unsetenv("DASH0_TEST_MODE")
		
		config, err := ResolveConfiguration("", "")
		if err != nil {
			t.Fatalf("Failed to resolve configuration: %v", err)
		}
		
		// Empty values are allowed in test mode
		if config.BaseURL != "" {
			t.Errorf("Expected empty base URL, got %s", config.BaseURL)
		}
		if config.AuthToken != "" {
			t.Errorf("Expected empty auth token, got %s", config.AuthToken)
		}
	})
	
	// Test error case (no test mode, no config)
	t.Run("Without test mode and configuration", func(t *testing.T) {
		// Setup test environment without active context
		_ = setupTestConfigDir(t)
		
		// Disable test mode
		os.Unsetenv("DASH0_TEST_MODE")
		
		// Should fail validation
		_, err := ResolveConfiguration("", "")
		if err == nil {
			t.Errorf("Expected error for missing configuration, got nil")
		}
	})
}