package log

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger is the global logger instance
var Logger zerolog.Logger

// SetupLogger initializes the zerolog logger
func SetupLogger() {
	logLevel := zerolog.InfoLevel
	if os.Getenv("DEBUG") != "" {
		logLevel = zerolog.DebugLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(logLevel)
	
	// Pretty console logging for development
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	
	Logger = zerolog.New(output).With().Timestamp().Caller().Logger()
}