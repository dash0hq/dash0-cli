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
	
	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)
	
	// Execute show command
	output, err := executeCommand(rootCmd, "config", "show")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Verify output contains expected data
	if !bytes.Contains([]byte(output), []byte("Base URL: https://test1.example.com")) {
		t.Errorf("Expected output to contain Base URL: https://test1.example.com, got: %s", output)
	}
}

// TestListContextCmd tests the list context command
func TestListContextCmd(t *testing.T) {
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
	
	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)
	
	// Execute list command
	output, err := executeCommand(rootCmd, "config", "context", "list")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Verify output contains expected data
	if !bytes.Contains([]byte(output), []byte("* test2")) {
		t.Errorf("Expected output to contain active context marker (* test2), got: %s", output)
	}
	
	if !bytes.Contains([]byte(output), []byte("test1")) {
		t.Errorf("Expected output to contain test1, got: %s", output)
	}
}

// TestAddContextCmd tests the add context command
func TestAddContextCmd(t *testing.T) {
	// Setup test environment
	_ = setupTestConfigDir(t)
	
	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)
	
	// Execute add command
	output, err := executeCommand(rootCmd, "config", "context", "add", "--name", "new-context", "--base-url", "https://new.example.com", "--auth-token", "new-token")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Context 'new-context' added successfully")) {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}
	
	// Verify context was added
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	contexts, err := service.GetContexts()
	if err != nil {
		t.Fatalf("Failed to get contexts: %v", err)
	}
	
	if len(contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(contexts))
		return // Avoid index out of range error
	}
	
	if contexts[0].Name != "new-context" {
		t.Errorf("Expected context name new-context, got %s", contexts[0].Name)
	}
	
	if contexts[0].Configuration.BaseURL != "https://new.example.com" {
		t.Errorf("Expected base URL https://new.example.com, got %s", contexts[0].Configuration.BaseURL)
	}
}

// TestRemoveContextCmd tests the remove context command
func TestRemoveContextCmd(t *testing.T) {
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
	setActiveContext(t, configDir, "test1")
	
	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)
	
	// Execute remove command
	output, err := executeCommand(rootCmd, "config", "context", "remove", "--name", "test2")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Context 'test2' removed successfully")) {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}
	
	// Verify context was removed
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
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
}

// TestSelectContextCmd tests the select context command
func TestSelectContextCmd(t *testing.T) {
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
	setActiveContext(t, configDir, "test1")
	
	// Create root command and add config command
	rootCmd := &cobra.Command{Use: "dash0"}
	configCmd := NewConfigCmd()
	rootCmd.AddCommand(configCmd)
	
	// Execute select command
	output, err := executeCommand(rootCmd, "config", "context", "select", "--name", "test2")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Verify output contains success message
	if !bytes.Contains([]byte(output), []byte("Context 'test2' is now active")) {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}
	
	// Verify active context was updated
	service, err := NewService()
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	activeContext, err := service.GetActiveContext()
	if err != nil {
		t.Fatalf("Failed to get active context: %v", err)
	}
	
	if activeContext.Name != "test2" {
		t.Errorf("Expected active context name test2, got %s", activeContext.Name)
	}
}