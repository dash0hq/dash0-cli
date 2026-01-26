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
	output, err := executeCommand(rootCmd, "config", "profile", "list")
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

// TestAddProfileCmd tests the add profile command
func TestAddProfileCmd(t *testing.T) {
	// Setup test environment
	_ = setupTestConfigDir(t)

	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)

	// Execute add command
	output, err := executeCommand(rootCmd, "config", "profile", "add", "new-profile", "--api-url", "https://new.example.com", "--auth-token", "new-token")
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

// TestRemoveProfileCmd tests the remove profile command
func TestRemoveProfileCmd(t *testing.T) {
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

	// Execute remove command
	output, err := executeCommand(rootCmd, "config", "profile", "remove", "test2")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Profile 'test2' removed successfully")) {
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
	output, err := executeCommand(rootCmd, "config", "profile", "select", "test2")
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
