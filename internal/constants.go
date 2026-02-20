package internal

const HEADER_ID = "ID"
const HEADER_DATASET = "DATASET"
const HEADER_NAME = "NAME"
const HEADER_ORIGIN = "ORIGIN"
const HEADER_URL = "URL"

// DEFAULT_ORIGIN is the value used to tag assets managed by the Dash0 CLI.
const DEFAULT_ORIGIN = "dash0-cli"

// CONFIG_HINT is appended to Long descriptions of commands that accept
// --api-url, --auth-token, --dataset, or --otlp-url overrides.
const CONFIG_HINT = " Run 'dash0 config show --help' for details on how authentication and connection settings are resolved from profiles, flags, and environment variables."
