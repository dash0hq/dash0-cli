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

	// Cleanup env vars after test
	t.Cleanup(func() {
		os.Unsetenv("DASH0_CONFIG_DIR")
		os.Unsetenv("DASH0_TEST_MODE")
	})

	return tempDir
}

// createTestProfilesFile creates a test profiles file in the specified directory
func createTestProfilesFile(t *testing.T, configDir string, profiles []Profile) {
	t.Helper()

	// Create profiles file
	profilesFile := ProfilesFile{Profiles: profiles}
	data, err := json.MarshalIndent(profilesFile, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal profiles file: %v", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	profilesFilePath := filepath.Join(configDir, ProfilesFileName)
	if err := os.WriteFile(profilesFilePath, data, 0644); err != nil {
		t.Fatalf("Failed to write profiles file: %v", err)
	}
}

// setActiveProfile sets the active profile for testing
func setActiveProfile(t *testing.T, configDir, profileName string) {
	t.Helper()

	activeProfilePath := filepath.Join(configDir, ActiveProfileFileName)
	if err := os.WriteFile(activeProfilePath, []byte(profileName), 0644); err != nil {
		t.Fatalf("Failed to write active profile: %v", err)
	}
}

// TestServiceGetProfiles tests the GetProfiles method
func TestServiceGetProfiles(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:   "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				ApiUrl:   "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)

	// Create service and test GetProfiles
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	profiles, err := service.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	// Validate result
	if len(profiles) != len(testProfiles) {
		t.Errorf("Expected %d profiles, got %d", len(testProfiles), len(profiles))
	}

	for i, p := range profiles {
		if p.Name != testProfiles[i].Name {
			t.Errorf("Expected profile name %s, got %s", testProfiles[i].Name, p.Name)
		}
		if p.Configuration.ApiUrl != testProfiles[i].Configuration.ApiUrl {
			t.Errorf("Expected API URL %s, got %s", testProfiles[i].Configuration.ApiUrl, p.Configuration.ApiUrl)
		}
		if p.Configuration.AuthToken != testProfiles[i].Configuration.AuthToken {
			t.Errorf("Expected auth token %s, got %s", testProfiles[i].Configuration.AuthToken, p.Configuration.AuthToken)
		}
	}
}

// TestServiceGetActiveProfile tests the GetActiveProfile method
func TestServiceGetActiveProfile(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:   "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				ApiUrl:   "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test2")

	// Create service and test GetActiveProfile
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	profile, err := service.GetActiveProfile()
	if err != nil {
		t.Fatalf("Failed to get active profile: %v", err)
	}

	// Validate result
	if profile.Name != "test2" {
		t.Errorf("Expected active profile name test2, got %s", profile.Name)
	}
	if profile.Configuration.ApiUrl != "https://test2.example.com" {
		t.Errorf("Expected API URL https://test2.example.com, got %s", profile.Configuration.ApiUrl)
	}
	if profile.Configuration.AuthToken != "token2" {
		t.Errorf("Expected auth token token2, got %s", profile.Configuration.AuthToken)
	}
}

// TestServiceAddProfile tests the AddProfile method
func TestServiceAddProfile(t *testing.T) {
	// Setup test environment
	_ = setupTestConfigDir(t)

	// Create service
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Add a new profile
	newProfile := Profile{
		Name: "new-profile",
		Configuration: Configuration{
			ApiUrl:   "https://new.example.com",
			AuthToken: "new-token",
		},
	}

	err = service.AddProfile(newProfile)
	if err != nil {
		t.Fatalf("Failed to add profile: %v", err)
	}

	// Validate result
	profiles, err := service.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if len(profiles) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(profiles))
	}

	if profiles[0].Name != "new-profile" {
		t.Errorf("Expected profile name new-profile, got %s", profiles[0].Name)
	}

	// Check if this profile was set as active (it should be, as it's the first one)
	activeProfile, err := service.GetActiveProfile()
	if err != nil {
		t.Fatalf("Failed to get active profile: %v", err)
	}

	if activeProfile.Name != "new-profile" {
		t.Errorf("Expected active profile name new-profile, got %s", activeProfile.Name)
	}
}

// TestServiceRemoveProfile tests the RemoveProfile method
func TestServiceRemoveProfile(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:   "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				ApiUrl:   "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test2")

	// Create service and remove a profile
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	err = service.RemoveProfile("test2")
	if err != nil {
		t.Fatalf("Failed to remove profile: %v", err)
	}

	// Validate result
	profiles, err := service.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if len(profiles) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(profiles))
	}

	if profiles[0].Name != "test1" {
		t.Errorf("Expected profile name test1, got %s", profiles[0].Name)
	}

	// Check if active profile was updated
	activeProfile, err := service.GetActiveProfile()
	if err != nil {
		t.Fatalf("Failed to get active profile: %v", err)
	}

	if activeProfile.Name != "test1" {
		t.Errorf("Expected active profile name test1, got %s", activeProfile.Name)
	}
}

// TestServiceGetActiveConfiguration tests the GetActiveConfiguration method
func TestServiceGetActiveConfiguration(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)
	
	// Test with environment variables
	t.Run("With environment variables", func(t *testing.T) {
		os.Setenv("DASH0_API_URL", "https://env.example.com")
		os.Setenv("DASH0_AUTH_TOKEN", "env-token")
		defer func() {
			os.Unsetenv("DASH0_API_URL")
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
		
		if config.ApiUrl != "https://env.example.com" {
			t.Errorf("Expected API URL https://env.example.com, got %s", config.ApiUrl)
		}
		if config.AuthToken != "env-token" {
			t.Errorf("Expected auth token env-token, got %s", config.AuthToken)
		}
	})
	
	// Test with active profile
	t.Run("With active profile", func(t *testing.T) {
		// Unset environment variables
		os.Unsetenv("DASH0_API_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")

		// Create test profiles
		testProfiles := []Profile{
			{
				Name: "test1",
				Configuration: Configuration{
					ApiUrl:   "https://test1.example.com",
					AuthToken: "token1",
				},
			},
		}

		createTestProfilesFile(t, configDir, testProfiles)
		setActiveProfile(t, configDir, "test1")
		
		service, err := NewService()
		if err != nil {
			t.Fatalf("Failed to create service: %v", err)
		}
		
		config, err := service.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}
		
		if config.ApiUrl != "https://test1.example.com" {
			t.Errorf("Expected API URL https://test1.example.com, got %s", config.ApiUrl)
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
		
		if config.ApiUrl != "https://flag.example.com" {
			t.Errorf("Expected API URL https://flag.example.com, got %s", config.ApiUrl)
		}
		if config.AuthToken != "flag-token" {
			t.Errorf("Expected auth token flag-token, got %s", config.AuthToken)
		}
	})
	
	// Test with active profile and partial override
	t.Run("With active profile and partial override", func(t *testing.T) {
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
		os.Unsetenv("DASH0_API_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")

		// Create test profiles with explicit test values
		testProfiles := []Profile{
			{
				Name: "override-test",
				Configuration: Configuration{
					ApiUrl:   "https://original.example.com",
					AuthToken: "original-token",
				},
			},
		}

		// Set up profile file and active profile
		profilesFile := ProfilesFile{Profiles: testProfiles}
		data, _ := json.Marshal(profilesFile)
		os.MkdirAll(tempDir, 0755)
		os.WriteFile(filepath.Join(tempDir, ProfilesFileName), data, 0644)
		os.WriteFile(filepath.Join(tempDir, ActiveProfileFileName), []byte("override-test"), 0644)
		
		// Verify that the active configuration loads correctly
		svc, _ := NewService()
		origCfg, origErr := svc.GetActiveConfiguration()
		if origErr != nil {
			t.Fatalf("Failed to get original config: %v", origErr)
		}
		t.Logf("Original config before resolve: %+v", origCfg)
		
		// Test with partial override (only API URL)
		resolvedCfg, resolveErr := ResolveConfiguration("https://override.example.com", "")
		if resolveErr != nil {
			t.Fatalf("Failed to resolve configuration: %v", resolveErr)
		}
		t.Logf("Resolved config: %+v", resolvedCfg)
		
		// Test assertions
		if resolvedCfg.ApiUrl != "https://override.example.com" {
			t.Errorf("Expected API URL https://override.example.com, got %s", resolvedCfg.ApiUrl)
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
		if config.ApiUrl != "" {
			t.Errorf("Expected empty API URL, got %s", config.ApiUrl)
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