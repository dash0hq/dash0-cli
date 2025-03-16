# Dash0 CLI Development Guide

## Commands
- Build: `make build`
- Test all: `make test`
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
- `/internal/mcp/tools`: Contains MCP tool implementations. The tools are registered via `tools.go`
- `/internal/metrics`: Commands and utilities to retrieve metrics from Dash0
- `/internal/api/types`: Contains Type Definitions for request and response bodies for the Dash0 API
- `/internal/log`: Shared logging utilities

Organize code by domain, make interfaces for testability, and follow standard Go package layout.
