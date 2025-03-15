package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestNewMCPCmd(t *testing.T) {
	cmd := NewMCPCmd("test-version")
	
	// Verify command basic properties
	if cmd.Use != "mcp" {
		t.Errorf("Expected Use to be 'mcp', got %s", cmd.Use)
	}
	
	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	
	if cmd.Long == "" {
		t.Error("Long description should not be empty")
	}
	
	// Verify flags
	flags := cmd.Flags()
	
	baseURLFlag := flags.Lookup("base-url")
	if baseURLFlag == nil {
		t.Error("base-url flag is missing")
	} else {
		if baseURLFlag.Usage == "" {
			t.Error("base-url flag should have usage information")
		}
	}
	
	authTokenFlag := flags.Lookup("auth-token")
	if authTokenFlag == nil {
		t.Error("auth-token flag is missing")
	} else {
		if authTokenFlag.Usage == "" {
			t.Error("auth-token flag should have usage information")
		}
	}
}

func TestMCPCommandExecution(t *testing.T) {
	// Set test mode to bypass validation
	os.Setenv("DASH0_TEST_MODE", "1")
	defer os.Unsetenv("DASH0_TEST_MODE")

	// Create a new MCP command
	cmd := NewMCPCmd("test-version")

	// Set up stdin/stdout capture
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	
	// Create pipes for stdin and stdout
	r, w, _ := os.Pipe()
	os.Stdin = r
	
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	// Create a channel to signal command completion
	done := make(chan struct{})
	
	// Execute command in a goroutine
	go func() {
		defer close(done)
		// Execute the command
		if err := cmd.Execute(); err != nil {
			t.Logf("Command execution error: %v", err)
		}
	}()

	// Allow time for the server to start
	time.Sleep(100 * time.Millisecond)

	// Prepare MCP request
	requestMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}
	requestJSON, err := json.Marshal(requestMsg)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	requestJSON = append(requestJSON, '\n')

	// Write the request to stdin
	_, err = w.Write(requestJSON)
	if err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}

	// Allow time for the server to process the request
	time.Sleep(100 * time.Millisecond)

	// Buffer to capture stdout
	var buf bytes.Buffer
	
	// Create a channel to signal reading completion
	readDone := make(chan struct{})
	
	// Read the response from stdout
	go func() {
		defer close(readDone)
		
		data := make([]byte, 8192)
		n, err := outR.Read(data)
		if err != nil {
			t.Logf("Error reading from stdout: %v", err)
			return
		}
		
		buf.Write(data[:n])
	}()

	// Wait for the read to complete with a timeout
	select {
	case <-readDone:
		// Read completed
	case <-time.After(500 * time.Millisecond):
		t.Log("Timeout waiting for server response")
	}

	// Restore original stdin and stdout
	w.Close()
	outW.Close()
	os.Stdin = oldStdin
	os.Stdout = oldStdout

	// Process the response
	response := buf.String()
	t.Logf("Raw response from server: %q", response)
	
	// Find and extract the JSON response (the server might output other text)
	jsonStart := strings.Index(response, "{")
	if jsonStart == -1 {
		t.Fatalf("No JSON found in response")
	}
	
	jsonResponse := response[jsonStart:]
	jsonEnd := strings.LastIndex(jsonResponse, "}") + 1
	if jsonEnd <= 0 {
		t.Fatalf("Incomplete JSON in response")
	}
	
	jsonResponse = jsonResponse[:jsonEnd]
	t.Logf("Extracted JSON: %s", jsonResponse)

	// Parse the JSON
	var responseObj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonResponse), &responseObj); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	// Verify the response structure
	if id, ok := responseObj["id"]; !ok || fmt.Sprintf("%v", id) != "1" {
		t.Errorf("Expected id 1, got %v", responseObj["id"])
	}

	if jsonrpc, ok := responseObj["jsonrpc"]; !ok || jsonrpc != "2.0" {
		t.Errorf("Expected jsonrpc 2.0, got %v", responseObj["jsonrpc"])
	}

	// Handle both success and error cases
	if result, ok := responseObj["result"].(map[string]interface{}); ok {
		// Success case - we have a result
		tools, ok := result["tools"].([]interface{})
		if !ok {
			t.Fatalf("Expected tools to be an array, got %T", result["tools"])
		}

		// Check that hello_world tool is included
		foundHelloWorld := false
		for _, tool := range tools {
			toolMap, ok := tool.(map[string]interface{})
			if !ok {
				continue
			}
			
			if name, ok := toolMap["name"].(string); ok && name == "hello_world" {
				foundHelloWorld = true
				
				// Verify tool has a description
				if desc, ok := toolMap["description"].(string); !ok || desc == "" {
					t.Error("hello_world tool is missing a description")
				}
				
				break
			}
		}

		if !foundHelloWorld {
			t.Error("hello_world tool not found in server response")
		}
	} else if errObj, ok := responseObj["error"].(map[string]interface{}); ok {
		// Error case - log the error
		t.Fatalf("Server returned an error: %v", errObj["message"])
	} else {
		t.Fatalf("Response doesn't contain result or error field")
	}
}

// Helper utility for command execution in tests
func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}