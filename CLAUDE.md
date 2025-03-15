# Dash0 CLI Development Guide

## Commands
- Build: `go build -o dash0 ./cmd/dash0`
- Test all: `go test -v ./...`
- Test specific: `go test -v ./internal/config -run TestServiceAddContext`
- Run locally: `./dash0 [command]`

## Code Style
- Use Go 1.24+ features
- Format with `gofmt`
- Add unit tests for all new functionality
- Use zerolog for structured logging
- Error handling: wrap errors with descriptive messages using `fmt.Errorf("... %w", err)`
- Naming: use camelCase for variable names and PascalCase for exported functions/types
- Set `DASH0_TEST_MODE=1` when writing tests that need to bypass validation

## Project Structure
- `/cmd/dash0`: Main entrypoint
- `/internal/config`: Configuration management
- `/pkg/log`: Shared logging utilities

Organize code by domain, make interfaces for testability, and follow standard Go package layout.
