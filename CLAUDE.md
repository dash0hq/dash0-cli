# Dash0 CLI Development Guide

This repository provides a CLI utility to perform various tasks related with Dash0, enabling users via terminal,
AI agents and CI/CD workflows to perform tasks like creating, updating and deleting a number of resources in Dash0 like dashboards, alerting rules, views and more.

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
- `/internal/metrics`: Commands and utilities to retrieve metrics from Dash0
- `/internal/api/types`: Contains Type Definitions for request and response bodies for the Dash0 API
- `/internal/log`: Shared logging utilities

Organize code by domain, make interfaces for testability, and follow standard Go package layout.

## Documentation

Available commands are explained in @README.md . The description of what commands do is kept short and to the point. Providing a sample invocation as shell snippet, and when the output is longer than 4 lines, truncate it meaningfully to 4 lines or less. When modifying `dash0` in ways that affect the outpout displayed to users, always validate that the documentation about the commands is correct.

## Validation of changes

When modifying `dash0` in ways that affect the outpout displayed to users, always built the tool anew and validate the output.
