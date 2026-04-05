package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-api-client-go/profiles"
)

// setupTestConfigDir creates a temporary directory for testing and returns its path
func setupTestConfigDir(t *testing.T) string {
	t.Helper()

	// Create a temp directory
	tempDir := t.TempDir()

	// Override the config path for testing
	os.Setenv(profiles.EnvConfigDir, tempDir)

	// Cleanup env vars after test
	t.Cleanup(func() {
		os.Unsetenv(profiles.EnvConfigDir)
	})

	return tempDir
}

// createTestProfilesFile creates a test profiles file in the specified directory
func createTestProfilesFile(t *testing.T, configDir string, profs []profiles.Profile) {
	t.Helper()

	// Create profiles file
	profilesFile := profiles.ProfilesFile{Profiles: profs}
	data, err := json.MarshalIndent(profilesFile, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal profiles file: %v", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	profilesFilePath := filepath.Join(configDir, profiles.ProfilesFileName)
	if err := os.WriteFile(profilesFilePath, data, 0644); err != nil {
		t.Fatalf("Failed to write profiles file: %v", err)
	}
}

// setActiveProfile sets the active profile for testing
func setActiveProfile(t *testing.T, configDir, profileName string) {
	t.Helper()

	activeProfilePath := filepath.Join(configDir, profiles.ActiveProfileFileName)
	if err := os.WriteFile(activeProfilePath, []byte(profileName), 0644); err != nil {
		t.Fatalf("Failed to write active profile: %v", err)
	}
}

// TestGetProfiles tests the GetProfiles method
func TestGetProfiles(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []profiles.Profile{
		{
			Name: "test1",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)

	// Create store and test GetProfiles
	store, err := profiles.NewStore()
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	result, err := store.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	// Validate result
	if len(result) != len(testProfiles) {
		t.Errorf("Expected %d profiles, got %d", len(testProfiles), len(result))
	}

	for i, p := range result {
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

// TestGetActiveProfile tests the GetActiveProfile method
func TestGetActiveProfile(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []profiles.Profile{
		{
			Name: "test1",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test2")

	// Create store and test GetActiveProfile
	store, err := profiles.NewStore()
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	profile, err := store.GetActiveProfile()
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

// TestAddProfile tests the AddProfile method
func TestAddProfile(t *testing.T) {
	// Setup test environment
	_ = setupTestConfigDir(t)

	// Create store
	store, err := profiles.NewStore()
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Add a new profile
	newProfile := profiles.Profile{
		Name: "new-profile",
		Configuration: profiles.Configuration{
			ApiUrl:    "https://new.example.com",
			AuthToken: "new-token",
		},
	}

	err = store.AddProfile(newProfile)
	if err != nil {
		t.Fatalf("Failed to add profile: %v", err)
	}

	// Validate result
	result, err := store.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(result))
	}

	if result[0].Name != "new-profile" {
		t.Errorf("Expected profile name new-profile, got %s", result[0].Name)
	}

	// Check if this profile was set as active (it should be, as it's the first one)
	activeProfile, err := store.GetActiveProfile()
	if err != nil {
		t.Fatalf("Failed to get active profile: %v", err)
	}

	if activeProfile.Name != "new-profile" {
		t.Errorf("Expected active profile name new-profile, got %s", activeProfile.Name)
	}
}

// TestRemoveProfile tests the RemoveProfile method
func TestRemoveProfile(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []profiles.Profile{
		{
			Name: "test1",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test2")

	// Create store and remove a profile
	store, err := profiles.NewStore()
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store.RemoveProfile("test2")
	if err != nil {
		t.Fatalf("Failed to remove profile: %v", err)
	}

	// Validate result
	result, err := store.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(result))
	}

	if result[0].Name != "test1" {
		t.Errorf("Expected profile name test1, got %s", result[0].Name)
	}

	// Check if active profile was updated
	activeProfile, err := store.GetActiveProfile()
	if err != nil {
		t.Fatalf("Failed to get active profile: %v", err)
	}

	if activeProfile.Name != "test1" {
		t.Errorf("Expected active profile name test1, got %s", activeProfile.Name)
	}
}

// TestUpdateProfile tests the UpdateProfile method
func TestUpdateProfile(t *testing.T) {
	t.Run("update single field", func(t *testing.T) {
		configDir := setupTestConfigDir(t)
		createTestProfilesFile(t, configDir, []profiles.Profile{
			{Name: "dev", Configuration: profiles.Configuration{ApiUrl: "https://old.example.com", AuthToken: "token1"}},
		})

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		err = store.UpdateProfile("dev", func(cfg *profiles.Configuration) {
			cfg.ApiUrl = "https://new.example.com"
		})
		if err != nil {
			t.Fatalf("Failed to update profile: %v", err)
		}

		result, _ := store.GetProfiles()
		if result[0].Configuration.ApiUrl != "https://new.example.com" {
			t.Errorf("Expected API URL https://new.example.com, got %s", result[0].Configuration.ApiUrl)
		}
		if result[0].Configuration.AuthToken != "token1" {
			t.Errorf("Expected auth token to remain token1, got %s", result[0].Configuration.AuthToken)
		}
	})

	t.Run("update multiple fields", func(t *testing.T) {
		configDir := setupTestConfigDir(t)
		createTestProfilesFile(t, configDir, []profiles.Profile{
			{Name: "dev", Configuration: profiles.Configuration{ApiUrl: "https://old.example.com", AuthToken: "old-token"}},
		})

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		err = store.UpdateProfile("dev", func(cfg *profiles.Configuration) {
			cfg.ApiUrl = "https://new.example.com"
			cfg.AuthToken = "new-token"
			cfg.OtlpUrl = "https://otlp.example.com"
		})
		if err != nil {
			t.Fatalf("Failed to update profile: %v", err)
		}

		result, _ := store.GetProfiles()
		if result[0].Configuration.ApiUrl != "https://new.example.com" {
			t.Errorf("Expected API URL https://new.example.com, got %s", result[0].Configuration.ApiUrl)
		}
		if result[0].Configuration.AuthToken != "new-token" {
			t.Errorf("Expected auth token new-token, got %s", result[0].Configuration.AuthToken)
		}
		if result[0].Configuration.OtlpUrl != "https://otlp.example.com" {
			t.Errorf("Expected OTLP URL https://otlp.example.com, got %s", result[0].Configuration.OtlpUrl)
		}
	})

	t.Run("remove field by setting to empty string", func(t *testing.T) {
		configDir := setupTestConfigDir(t)
		createTestProfilesFile(t, configDir, []profiles.Profile{
			{Name: "dev", Configuration: profiles.Configuration{
				ApiUrl: "https://api.example.com", AuthToken: "token1", OtlpUrl: "https://otlp.example.com",
			}},
		})

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		err = store.UpdateProfile("dev", func(cfg *profiles.Configuration) {
			cfg.OtlpUrl = ""
		})
		if err != nil {
			t.Fatalf("Failed to update profile: %v", err)
		}

		result, _ := store.GetProfiles()
		if result[0].Configuration.OtlpUrl != "" {
			t.Errorf("Expected OTLP URL to be empty, got %s", result[0].Configuration.OtlpUrl)
		}
		if result[0].Configuration.ApiUrl != "https://api.example.com" {
			t.Errorf("Expected API URL to remain unchanged, got %s", result[0].Configuration.ApiUrl)
		}
	})

	t.Run("does not affect other profiles", func(t *testing.T) {
		configDir := setupTestConfigDir(t)
		createTestProfilesFile(t, configDir, []profiles.Profile{
			{Name: "dev", Configuration: profiles.Configuration{ApiUrl: "https://dev.example.com", AuthToken: "dev-token"}},
			{Name: "prod", Configuration: profiles.Configuration{ApiUrl: "https://prod.example.com", AuthToken: "prod-token"}},
		})

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		err = store.UpdateProfile("dev", func(cfg *profiles.Configuration) {
			cfg.ApiUrl = "https://new-dev.example.com"
		})
		if err != nil {
			t.Fatalf("Failed to update profile: %v", err)
		}

		result, _ := store.GetProfiles()
		if result[1].Configuration.ApiUrl != "https://prod.example.com" {
			t.Errorf("Expected prod API URL to remain unchanged, got %s", result[1].Configuration.ApiUrl)
		}
	})

	t.Run("profile not found", func(t *testing.T) {
		configDir := setupTestConfigDir(t)
		createTestProfilesFile(t, configDir, []profiles.Profile{
			{Name: "dev", Configuration: profiles.Configuration{ApiUrl: "https://dev.example.com", AuthToken: "token1"}},
		})

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		err = store.UpdateProfile("nonexistent", func(cfg *profiles.Configuration) {
			cfg.ApiUrl = "https://new.example.com"
		})
		if err == nil {
			t.Fatal("Expected error for nonexistent profile, got nil")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("Expected error to contain profile name, got: %s", err.Error())
		}
	})
}

// TestGetActiveConfiguration tests the GetActiveConfiguration method
func TestGetActiveConfiguration(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Test with environment variables
	t.Run("With environment variables", func(t *testing.T) {
		os.Setenv(profiles.EnvApiUrl, "https://env.example.com")
		os.Setenv(profiles.EnvAuthToken, "env-token")
		defer func() {
			os.Unsetenv(profiles.EnvApiUrl)
			os.Unsetenv(profiles.EnvAuthToken)
		}()

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
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
		os.Unsetenv(profiles.EnvApiUrl)
		os.Unsetenv(profiles.EnvAuthToken)
		os.Unsetenv(profiles.EnvOtlpUrl)

		// Create test profiles
		testProfiles := []profiles.Profile{
			{
				Name: "test1",
				Configuration: profiles.Configuration{
					ApiUrl:    "https://test1.example.com",
					AuthToken: "token1",
				},
			},
		}

		createTestProfilesFile(t, configDir, testProfiles)
		setActiveProfile(t, configDir, "test1")

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
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

	// Test OTLP URL from environment variable
	t.Run("With DASH0_OTLP_URL environment variable", func(t *testing.T) {
		os.Unsetenv(profiles.EnvApiUrl)
		os.Setenv(profiles.EnvAuthToken, "env-token")
		os.Setenv(profiles.EnvOtlpUrl, "https://otlp.example.com")
		defer func() {
			os.Unsetenv(profiles.EnvAuthToken)
			os.Unsetenv(profiles.EnvOtlpUrl)
		}()

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}

		if config.OtlpUrl != "https://otlp.example.com" {
			t.Errorf("Expected OTLP URL https://otlp.example.com, got %s", config.OtlpUrl)
		}
		if config.AuthToken != "env-token" {
			t.Errorf("Expected auth token env-token, got %s", config.AuthToken)
		}
	})

	// Test Dataset from environment variable
	t.Run("With DASH0_DATASET environment variable", func(t *testing.T) {
		os.Setenv(profiles.EnvApiUrl, "https://api.example.com")
		os.Setenv(profiles.EnvAuthToken, "env-token")
		os.Setenv(profiles.EnvDataset, "my-dataset")
		defer func() {
			os.Unsetenv(profiles.EnvApiUrl)
			os.Unsetenv(profiles.EnvAuthToken)
			os.Unsetenv(profiles.EnvDataset)
		}()

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}

		if config.Dataset != "my-dataset" {
			t.Errorf("Expected dataset my-dataset, got %s", config.Dataset)
		}
	})

	// Test Dataset override from env var over profile
	t.Run("Dataset env var overrides profile", func(t *testing.T) {
		os.Unsetenv(profiles.EnvApiUrl)
		os.Unsetenv(profiles.EnvAuthToken)
		os.Unsetenv(profiles.EnvOtlpUrl)
		os.Setenv(profiles.EnvDataset, "env-dataset")
		defer os.Unsetenv(profiles.EnvDataset)

		testProfiles := []profiles.Profile{
			{
				Name: "dataset-test",
				Configuration: profiles.Configuration{
					ApiUrl:    "https://api.example.com",
					AuthToken: "token1",
					Dataset:   "profile-dataset",
				},
			},
		}

		createTestProfilesFile(t, configDir, testProfiles)
		setActiveProfile(t, configDir, "dataset-test")

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}

		if config.Dataset != "env-dataset" {
			t.Errorf("Expected dataset env-dataset, got %s", config.Dataset)
		}
	})

	// Test Dataset from profile (no env var)
	t.Run("Dataset from profile", func(t *testing.T) {
		os.Unsetenv(profiles.EnvApiUrl)
		os.Unsetenv(profiles.EnvAuthToken)
		os.Unsetenv(profiles.EnvOtlpUrl)
		os.Unsetenv(profiles.EnvDataset)

		testProfiles := []profiles.Profile{
			{
				Name: "dataset-profile-test",
				Configuration: profiles.Configuration{
					ApiUrl:    "https://api.example.com",
					AuthToken: "token1",
					Dataset:   "profile-dataset",
				},
			},
		}

		createTestProfilesFile(t, configDir, testProfiles)
		setActiveProfile(t, configDir, "dataset-profile-test")

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}

		if config.Dataset != "profile-dataset" {
			t.Errorf("Expected dataset profile-dataset, got %s", config.Dataset)
		}
	})

	// Test OTLP URL override from env var over profile
	t.Run("OTLP URL env var overrides profile", func(t *testing.T) {
		os.Unsetenv(profiles.EnvApiUrl)
		os.Unsetenv(profiles.EnvAuthToken)
		os.Setenv(profiles.EnvOtlpUrl, "https://otlp-override.example.com")
		defer os.Unsetenv(profiles.EnvOtlpUrl)

		testProfiles := []profiles.Profile{
			{
				Name: "otlp-test",
				Configuration: profiles.Configuration{
					ApiUrl:    "https://api.example.com",
					AuthToken: "token1",
					OtlpUrl:   "https://otlp-profile.example.com",
				},
			},
		}

		createTestProfilesFile(t, configDir, testProfiles)
		setActiveProfile(t, configDir, "otlp-test")

		store, err := profiles.NewStore()
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		config, err := store.GetActiveConfiguration()
		if err != nil {
			t.Fatalf("Failed to get active configuration: %v", err)
		}

		if config.OtlpUrl != "https://otlp-override.example.com" {
			t.Errorf("Expected OTLP URL https://otlp-override.example.com, got %s", config.OtlpUrl)
		}
	})
}

// TestGetProfilesInvalidJSON tests GetProfiles with invalid JSON
func TestGetProfilesInvalidJSON(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create an invalid JSON file
	invalidJSON := []byte(`{invalid json content`)
	profilesFilePath := filepath.Join(configDir, profiles.ProfilesFileName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(profilesFilePath, invalidJSON, 0644); err != nil {
		t.Fatalf("Failed to write invalid profiles file: %v", err)
	}

	// Create store and test GetProfiles
	store, err := profiles.NewStore()
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	_, err = store.GetProfiles()
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}

	// Verify error message contains the file path
	errStr := err.Error()
	if !strings.Contains(errStr, profilesFilePath) {
		t.Errorf("Expected error to contain file path %s, got: %s", profilesFilePath, errStr)
	}
	if !strings.Contains(errStr, "failed to parse profiles file") {
		t.Errorf("Expected error to contain 'failed to parse profiles file', got: %s", errStr)
	}
	if !strings.Contains(errStr, "Hint:") {
		t.Errorf("Expected error to contain hint, got: %s", errStr)
	}
}

// TestResolveConfiguration tests the ResolveConfiguration function
func TestResolveConfiguration(t *testing.T) {
	// Test with environment variables (bypass profile loading)
	t.Run("With environment variables", func(t *testing.T) {
		// Use env vars to bypass profile loading
		os.Setenv(profiles.EnvApiUrl, "https://env.example.com")
		os.Setenv(profiles.EnvAuthToken, "env-token")
		defer func() {
			os.Unsetenv(profiles.EnvApiUrl)
			os.Unsetenv(profiles.EnvAuthToken)
		}()

		config, err := profiles.ResolveConfiguration("", "")
		if err != nil {
			t.Fatalf("Failed to resolve configuration: %v", err)
		}

		if config.ApiUrl != "https://env.example.com" {
			t.Errorf("Expected API URL https://env.example.com, got %s", config.ApiUrl)
		}
		if config.AuthToken != "env-token" {
			t.Errorf("Expected auth token env-token, got %s", config.AuthToken)
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
		os.Setenv(profiles.EnvConfigDir, tempDir)
		defer os.Unsetenv(profiles.EnvConfigDir)

		// Unset environment variables that might interfere
		os.Unsetenv(profiles.EnvApiUrl)
		os.Unsetenv(profiles.EnvAuthToken)

		// Create test profiles with explicit test values
		testProfiles := []profiles.Profile{
			{
				Name: "override-test",
				Configuration: profiles.Configuration{
					ApiUrl:    "https://original.example.com",
					AuthToken: "original-token",
				},
			},
		}

		// Set up profile file and active profile
		profilesFile := profiles.ProfilesFile{Profiles: testProfiles}
		data, _ := json.Marshal(profilesFile)
		_ = os.MkdirAll(tempDir, 0755)
		_ = os.WriteFile(filepath.Join(tempDir, profiles.ProfilesFileName), data, 0644)
		_ = os.WriteFile(filepath.Join(tempDir, profiles.ActiveProfileFileName), []byte("override-test"), 0644)

		// Verify that the active configuration loads correctly
		store, _ := profiles.NewStore()
		origCfg, origErr := store.GetActiveConfiguration()
		if origErr != nil {
			t.Fatalf("Failed to get original config: %v", origErr)
		}
		t.Logf("Original config before resolve: %+v", origCfg)

		// Test with partial override (only API URL)
		resolvedCfg, resolveErr := profiles.ResolveConfiguration("https://override.example.com", "")
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

	// Test error case: no profile, no env vars, no flags
	t.Run("Without configuration", func(t *testing.T) {
		_ = setupTestConfigDir(t)

		_, err := profiles.ResolveConfiguration("", "")
		if err == nil {
			t.Errorf("Expected error for missing configuration, got nil")
		}
	})

	// Test OTLP URL only via env vars (no API URL needed)
	t.Run("With OTLP URL env var only", func(t *testing.T) {
		os.Unsetenv(profiles.EnvApiUrl)
		os.Setenv(profiles.EnvOtlpUrl, "https://otlp.example.com")
		os.Setenv(profiles.EnvAuthToken, "env-token")
		defer func() {
			os.Unsetenv(profiles.EnvOtlpUrl)
			os.Unsetenv(profiles.EnvAuthToken)
		}()

		config, err := profiles.ResolveConfiguration("", "")
		if err != nil {
			t.Fatalf("Expected no error for OTLP-only config, got: %v", err)
		}

		if config.OtlpUrl != "https://otlp.example.com" {
			t.Errorf("Expected OTLP URL https://otlp.example.com, got %s", config.OtlpUrl)
		}
		if config.AuthToken != "env-token" {
			t.Errorf("Expected auth token env-token, got %s", config.AuthToken)
		}
		if config.ApiUrl != "" {
			t.Errorf("Expected empty API URL, got %s", config.ApiUrl)
		}
	})

	// Test dataset resolved from env var
	t.Run("Dataset from env var in ResolveConfiguration", func(t *testing.T) {
		os.Setenv(profiles.EnvApiUrl, "https://api.example.com")
		os.Setenv(profiles.EnvAuthToken, "env-token")
		os.Setenv(profiles.EnvDataset, "env-dataset")
		defer func() {
			os.Unsetenv(profiles.EnvApiUrl)
			os.Unsetenv(profiles.EnvAuthToken)
			os.Unsetenv(profiles.EnvDataset)
		}()

		config, err := profiles.ResolveConfiguration("", "")
		if err != nil {
			t.Fatalf("Failed to resolve configuration: %v", err)
		}

		if config.Dataset != "env-dataset" {
			t.Errorf("Expected dataset env-dataset, got %s", config.Dataset)
		}
	})

	// Test dataset resolved from flag in ResolveConfigurationWithOtlp
	t.Run("Dataset from flag override", func(t *testing.T) {
		os.Setenv(profiles.EnvApiUrl, "https://api.example.com")
		os.Setenv(profiles.EnvAuthToken, "env-token")
		os.Setenv(profiles.EnvDataset, "env-dataset")
		defer func() {
			os.Unsetenv(profiles.EnvApiUrl)
			os.Unsetenv(profiles.EnvAuthToken)
			os.Unsetenv(profiles.EnvDataset)
		}()

		config, err := profiles.ResolveConfigurationWithOtlp("", "", "", "flag-dataset")
		if err != nil {
			t.Fatalf("Failed to resolve configuration: %v", err)
		}

		if config.Dataset != "flag-dataset" {
			t.Errorf("Expected dataset flag-dataset, got %s", config.Dataset)
		}
	})

	// Test ResolveConfigurationWithOtlp with OTLP URL flag
	t.Run("With OTLP URL flag override", func(t *testing.T) {
		os.Setenv(profiles.EnvAuthToken, "env-token")
		os.Unsetenv(profiles.EnvApiUrl)
		os.Unsetenv(profiles.EnvOtlpUrl)
		defer os.Unsetenv(profiles.EnvAuthToken)

		config, err := profiles.ResolveConfigurationWithOtlp("", "", "https://otlp-flag.example.com", "")
		if err != nil {
			t.Fatalf("Expected no error for OTLP flag config, got: %v", err)
		}

		if config.OtlpUrl != "https://otlp-flag.example.com" {
			t.Errorf("Expected OTLP URL https://otlp-flag.example.com, got %s", config.OtlpUrl)
		}
	})
}
