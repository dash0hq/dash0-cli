package log

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
)

// TestSetupLogger tests the logger initialization with default settings
func TestSetupLogger(t *testing.T) {
	// Reset global logger before test
	Logger = zerolog.Logger{}
	
	// Capture current state
	prevDebugEnv := os.Getenv("DEBUG")
	
	// Ensure DEBUG is not set
	os.Unsetenv("DEBUG")
	defer func() {
		// Restore DEBUG environment variable
		if prevDebugEnv != "" {
			os.Setenv("DEBUG", prevDebugEnv)
		}
	}()
	
	// Call the function to test
	SetupLogger()
	
	// Verify global level is set to Info by default
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("Expected global log level to be InfoLevel, got %v", zerolog.GlobalLevel())
	}
	
	// Just verify the logger exists (since we can't compare zerolog.Logger directly)
	// The logger initialization is considered successful if we reach this point
}

// TestSetupLoggerWithDebug tests the logger initialization with DEBUG environment variable
func TestSetupLoggerWithDebug(t *testing.T) {
	// Reset global logger before test
	Logger = zerolog.Logger{}
	
	// Capture current state
	prevDebugEnv := os.Getenv("DEBUG")
	
	// Set DEBUG environment variable
	os.Setenv("DEBUG", "true")
	defer func() {
		// Restore DEBUG environment variable
		if prevDebugEnv != "" {
			os.Setenv("DEBUG", prevDebugEnv)
		} else {
			os.Unsetenv("DEBUG")
		}
	}()
	
	// Call the function to test
	SetupLogger()
	
	// Verify global level is set to Debug when DEBUG is set
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("Expected global log level to be DebugLevel, got %v", zerolog.GlobalLevel())
	}
	
	// Just verify the logger exists (since we can't compare zerolog.Logger directly)
	// The logger initialization is considered successful if we reach this point
}