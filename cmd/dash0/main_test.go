package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

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
	_, _ = io.Copy(&buf, r)

	// Verify the command executed without error
	if err != nil {
		t.Errorf("Root command failed: %v", err)
	}

	// Verify the help output contains expected content
	if !bytes.Contains(buf.Bytes(), []byte("Command line interface for interacting with Dash0 services")) {
		t.Errorf("Help output did not contain expected content")
	}
}

// TestFlagValue covers the manual flag scanning used before cobra parses flags.
func TestFlagValue(t *testing.T) {
	cases := []struct {
		name string
		args []string
		flag string
		want string
	}{
		{"not present", []string{"foo", "bar"}, "profile", ""},
		{"space-separated", []string{"--profile", "prod", "cmd"}, "profile", "prod"},
		{"equals form", []string{"--profile=prod", "cmd"}, "profile", "prod"},
		{"value missing at end", []string{"--profile"}, "profile", ""},
		{"empty equals value", []string{"--profile=", "cmd"}, "profile", ""},
		{"stops at --", []string{"--", "--profile", "prod"}, "profile", ""},
		{"does not match prefix only", []string{"--profiled", "prod"}, "profile", ""},
		{"first match wins", []string{"--profile", "first", "--profile", "second"}, "profile", "first"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := flagValue(tc.args, tc.flag)
			if got != tc.want {
				t.Errorf("flagValue(%v, %q) = %q, want %q", tc.args, tc.flag, got, tc.want)
			}
		})
	}
}
