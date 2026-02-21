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
