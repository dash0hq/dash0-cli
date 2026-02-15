package config

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)


// executeCommand is a helper function that executes a command and returns its output
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	// Redirect stdout
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Reset args for testing
	root.SetArgs(args)

	// Execute the command
	err = root.Execute()

	// Restore stdout
	w.Close()
	os.Stdout = stdout

	// Read output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)

	return buf.String(), err
}

// TestShowCmd tests the show command
func TestShowCmd(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test1")

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute show command
	output, err := executeCommand(rootCmd, "config", "show")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify output contains expected data (note: spacing is "API URL:    " with alignment)
	if !bytes.Contains([]byte(output), []byte("https://test1.example.com")) {
		t.Errorf("Expected output to contain https://test1.example.com, got: %s", output)
	}
}

// TestShowCmdNoProfile tests the show command when no profile is configured
func TestShowCmdNoProfile(t *testing.T) {
	// Setup test environment with no profiles
	_ = setupTestConfigDir(t)

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute show command — should not error
	output, err := executeCommand(rootCmd, "config", "show")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !bytes.Contains([]byte(output), []byte("Profile:    (none)")) {
		t.Errorf("Expected output to contain 'Profile:    (none)', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("API URL:    (not set)")) {
		t.Errorf("Expected output to contain 'API URL:    (not set)', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("OTLP URL:   (not set)")) {
		t.Errorf("Expected output to contain 'OTLP URL:   (not set)', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("Auth Token: (not set)")) {
		t.Errorf("Expected output to contain 'Auth Token: (not set)', got: %s", output)
	}
}

// TestShowCmdWithDataset tests the show command displays dataset
func TestShowCmdWithDataset(t *testing.T) {
	configDir := setupTestConfigDir(t)

	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
				Dataset:   "my-dataset",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test1")

	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	output, err := executeCommand(rootCmd, "config", "show")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !bytes.Contains([]byte(output), []byte("Dataset:    my-dataset")) {
		t.Errorf("Expected output to contain 'Dataset:    my-dataset', got: %s", output)
	}
}

// TestShowCmdDatasetDefault tests the show command displays 'default' when dataset is empty
func TestShowCmdDatasetDefault(t *testing.T) {
	configDir := setupTestConfigDir(t)

	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test1")

	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	output, err := executeCommand(rootCmd, "config", "show")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !bytes.Contains([]byte(output), []byte("Dataset:    default")) {
		t.Errorf("Expected output to contain 'Dataset:    default', got: %s", output)
	}
}

// TestShowCmdDatasetEnvVar tests the show command with DASH0_DATASET env var
func TestShowCmdDatasetEnvVar(t *testing.T) {
	configDir := setupTestConfigDir(t)

	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test1")

	os.Setenv("DASH0_DATASET", "env-dataset")
	defer os.Unsetenv("DASH0_DATASET")

	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	output, err := executeCommand(rootCmd, "config", "show")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !bytes.Contains([]byte(output), []byte("Dataset:    env-dataset")) {
		t.Errorf("Expected output to contain 'Dataset:    env-dataset', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("(from DASH0_DATASET environment variable)")) {
		t.Errorf("Expected output to contain env var annotation, got: %s", output)
	}
}

// TestCreateProfileCmdPartialFields tests that profile creation succeeds with any combination of fields
func TestCreateProfileCmdPartialFields(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected Configuration
	}{
		{
			name: "auth-token only",
			args: []string{"--auth-token", "token1"},
			expected: Configuration{
				AuthToken: "token1",
			},
		},
		{
			name: "api-url only",
			args: []string{"--api-url", "https://api.example.com"},
			expected: Configuration{
				ApiUrl: "https://api.example.com",
			},
		},
		{
			name: "otlp-url only",
			args: []string{"--otlp-url", "https://otlp.example.com"},
			expected: Configuration{
				OtlpUrl: "https://otlp.example.com",
			},
		},
		{
			name: "dataset only",
			args: []string{"--dataset", "my-dataset"},
			expected: Configuration{
				Dataset: "my-dataset",
			},
		},
		{
			name: "auth-token and api-url",
			args: []string{"--auth-token", "token1", "--api-url", "https://api.example.com"},
			expected: Configuration{
				AuthToken: "token1",
				ApiUrl:    "https://api.example.com",
			},
		},
		{
			name: "auth-token and otlp-url",
			args: []string{"--auth-token", "token1", "--otlp-url", "https://otlp.example.com"},
			expected: Configuration{
				AuthToken: "token1",
				OtlpUrl:   "https://otlp.example.com",
			},
		},
		{
			name: "all fields",
			args: []string{"--auth-token", "token1", "--api-url", "https://api.example.com", "--otlp-url", "https://otlp.example.com", "--dataset", "prod"},
			expected: Configuration{
				AuthToken: "token1",
				ApiUrl:    "https://api.example.com",
				OtlpUrl:   "https://otlp.example.com",
				Dataset:   "prod",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = setupTestConfigDir(t)

			rootCmd := &cobra.Command{Use: "dash0"}
			configCmd := NewConfigCmd()
			rootCmd.AddCommand(configCmd)

			args := append([]string{"config", "profiles", "create", "test-profile"}, tc.args...)
			output, err := executeCommand(rootCmd, args...)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !bytes.Contains([]byte(output), []byte("Profile 'test-profile' added successfully")) {
				t.Errorf("Expected success message, got: %s", output)
			}

			service, err := NewService()
			if err != nil {
				t.Fatalf("Failed to create service: %v", err)
			}

			profiles, err := service.GetProfiles()
			if err != nil {
				t.Fatalf("Failed to get profiles: %v", err)
			}

			if len(profiles) != 1 {
				t.Fatalf("Expected 1 profile, got %d", len(profiles))
			}

			cfg := profiles[0].Configuration
			if cfg.ApiUrl != tc.expected.ApiUrl {
				t.Errorf("Expected ApiUrl %q, got %q", tc.expected.ApiUrl, cfg.ApiUrl)
			}
			if cfg.AuthToken != tc.expected.AuthToken {
				t.Errorf("Expected AuthToken %q, got %q", tc.expected.AuthToken, cfg.AuthToken)
			}
			if cfg.OtlpUrl != tc.expected.OtlpUrl {
				t.Errorf("Expected OtlpUrl %q, got %q", tc.expected.OtlpUrl, cfg.OtlpUrl)
			}
			if cfg.Dataset != tc.expected.Dataset {
				t.Errorf("Expected Dataset %q, got %q", tc.expected.Dataset, cfg.Dataset)
			}
		})
	}
}

// TestCreateProfileCmdWithDataset tests profile creation with --dataset
func TestCreateProfileCmdWithDataset(t *testing.T) {
	_ = setupTestConfigDir(t)

	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	_, err := executeCommand(rootCmd, "config", "profiles", "create", "new-profile",
		"--api-url", "https://new.example.com", "--auth-token", "new-token", "--dataset", "my-dataset")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	profiles, err := service.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if len(profiles) != 1 {
		t.Fatalf("Expected 1 profile, got %d", len(profiles))
	}

	if profiles[0].Configuration.Dataset != "my-dataset" {
		t.Errorf("Expected dataset my-dataset, got %s", profiles[0].Configuration.Dataset)
	}
}

// TestUpdateProfileCmdWithDataset tests profile update with --dataset
func TestUpdateProfileCmdWithDataset(t *testing.T) {
	configDir := setupTestConfigDir(t)

	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)

	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	_, err := executeCommand(rootCmd, "config", "profiles", "update", "test1", "--dataset", "updated-dataset")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	profiles, err := service.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if profiles[0].Configuration.Dataset != "updated-dataset" {
		t.Errorf("Expected dataset updated-dataset, got %s", profiles[0].Configuration.Dataset)
	}

	// Verify other fields were not changed
	if profiles[0].Configuration.ApiUrl != "https://test1.example.com" {
		t.Errorf("Expected API URL to remain unchanged, got %s", profiles[0].Configuration.ApiUrl)
	}
}

// TestListProfileCmd tests the list profile command
func TestListProfileCmd(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				ApiUrl:    "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test2")

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute list command
	output, err := executeCommand(rootCmd, "config", "profiles", "list")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify output contains expected data
	if !bytes.Contains([]byte(output), []byte("* test2")) {
		t.Errorf("Expected output to contain active profile marker (* test2), got: %s", output)
	}

	if !bytes.Contains([]byte(output), []byte("test1")) {
		t.Errorf("Expected output to contain test1, got: %s", output)
	}
}

// TestCreateProfileCmd tests the create profile command
func TestCreateProfileCmd(t *testing.T) {
	// Setup test environment
	_ = setupTestConfigDir(t)

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute create command
	output, err := executeCommand(rootCmd, "config", "profiles", "create", "new-profile", "--api-url", "https://new.example.com", "--auth-token", "new-token")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Profile 'new-profile' added successfully")) {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}

	// Verify profile was added
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	profiles, err := service.GetProfiles()
	if err != nil {
		t.Fatalf("Failed to get profiles: %v", err)
	}

	if len(profiles) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(profiles))
		return // Avoid index out of range error
	}

	if profiles[0].Name != "new-profile" {
		t.Errorf("Expected profile name new-profile, got %s", profiles[0].Name)
	}

	if profiles[0].Configuration.ApiUrl != "https://new.example.com" {
		t.Errorf("Expected API URL https://new.example.com, got %s", profiles[0].Configuration.ApiUrl)
	}
}

// TestDeleteProfileCmd tests the delete profile command
func TestDeleteProfileCmd(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				ApiUrl:    "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test1")

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute delete command
	output, err := executeCommand(rootCmd, "config", "profiles", "delete", "test2")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Profile 'test2' deleted successfully")) {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}

	// Verify profile was removed
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

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
}

// TestSelectProfileCmd tests the select profile command
func TestSelectProfileCmd(t *testing.T) {
	// Setup test environment
	configDir := setupTestConfigDir(t)

	// Create test profiles
	testProfiles := []Profile{
		{
			Name: "test1",
			Configuration: Configuration{
				ApiUrl:    "https://test1.example.com",
				AuthToken: "token1",
			},
		},
		{
			Name: "test2",
			Configuration: Configuration{
				ApiUrl:    "https://test2.example.com",
				AuthToken: "token2",
			},
		},
	}

	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "test1")

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute select command
	output, err := executeCommand(rootCmd, "config", "profiles", "select", "test2")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Profile 'test2' is now active")) {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}

	// Verify active profile was updated
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	activeProfile, err := service.GetActiveProfile()
	if err != nil {
		t.Fatalf("Failed to get active profile: %v", err)
	}

	if activeProfile.Name != "test2" {
		t.Errorf("Expected active profile name test2, got %s", activeProfile.Name)
	}
}
