# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

<!-- next version -->

## 1.0.0


### New Components


- `cli`: Expanding the scope of the Dash0 CLI to managing assets (#2)
  Provides commands for managing Dash0 assets including dashboards, check rules,
  synthetic checks, and views. Supports multiple configuration profiles and various
  output formats.
  


### Enhancements


- `config`: Improved error messages with colored output (#)
  Error messages now display "Error:" in red and "Hint:" in cyan for better visibility.
  The error message for invalid profile JSON now includes the actual file path instead
  of a hardcoded value, making it easier to identify and fix configuration issues.
