package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestNeedsConfig(t *testing.T) {
	// Build a command tree that mirrors the real CLI:
	// dash0 -> config -> profiles -> create
	root := &cobra.Command{Use: "dash0"}
	configCmd := &cobra.Command{Use: "config"}
	profilesCmd := &cobra.Command{Use: "profiles"}
	createCmd := &cobra.Command{Use: "create"}
	showCmd := &cobra.Command{Use: "show"}
	dashboardsCmd := &cobra.Command{Use: "dashboards"}
	listCmd := &cobra.Command{Use: "list"}

	root.AddCommand(configCmd, dashboardsCmd)
	configCmd.AddCommand(profilesCmd, showCmd)
	profilesCmd.AddCommand(createCmd)
	dashboardsCmd.AddCommand(listCmd)

	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"nil command", nil, true},
		{"root command", root, false},
		{"help command", &cobra.Command{Use: "help"}, false},
		{"version command", &cobra.Command{Use: "version"}, false},
		{"completion command", &cobra.Command{Use: "completion"}, false},
		{"config command (direct child of root)", configCmd, false},
		{"config show (child of config)", showCmd, false},
		{"config profiles (child of config)", profilesCmd, false},
		{"config profiles create (grandchild of config)", createCmd, false},
		{"dashboards list (non-config command)", listCmd, true},
		{"dashboards (non-config command)", dashboardsCmd, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsConfig(tt.cmd)
			if got != tt.want {
				t.Errorf("needsConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNeedsConfigIgnoresNonRootConfig verifies that a command named "config"
// that is not a direct child of root is not treated as the config command
func TestNeedsConfigIgnoresNonRootConfig(t *testing.T) {
	root := &cobra.Command{Use: "dash0"}
	other := &cobra.Command{Use: "other"}
	nestedConfig := &cobra.Command{Use: "config"}
	sub := &cobra.Command{Use: "sub"}

	root.AddCommand(other)
	other.AddCommand(nestedConfig)
	nestedConfig.AddCommand(sub)

	if !needsConfig(sub) {
		t.Error("needsConfig() should return true for a 'config' command that is not a direct child of root")
	}
	if !needsConfig(nestedConfig) {
		t.Error("needsConfig() should return true for a 'config' command whose parent is not 'dash0'")
	}
}

// TestRootCommandExecution tests the root command execution
func TestRootCommandExecution(t *testing.T) {
	// Save the original stdout
	stdout := os.Stdout
	
	// Create a pipe to capture output
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	// Execute the root command
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	
	// Close the write end of the pipe
	w.Close()
	os.Stdout = stdout
	
	// Read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	
	// Verify the command executed without error
	if err != nil {
		t.Errorf("Root command failed: %v", err)
	}
	
	// Verify the help output contains expected content
	if !bytes.Contains(buf.Bytes(), []byte("Command line interface for interacting with Dash0 services")) {
		t.Errorf("Help output did not contain expected content")
	}
}