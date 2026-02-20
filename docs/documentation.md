# Documentation

Command documentation lives in two places:

- @README.md — concise overview with sample invocations (no command outputs).
  When the output is longer than 4 lines, truncate it meaningfully to 4 lines or less.
- @docs/commands.md — full command reference with flags, expected outputs, asset YAML formats, and agent workflows.

When modifying `dash0` in ways that affect the output displayed to users, always validate that the documentation about the commands is correct in both files.

## Attribute keys in examples
When writing filter or query examples in documentation, use real Dash0 attribute keys — not invented ones.
Check the Dash0 API (via the `getAttributeKeys` and `getAttributeValues` MCP tools) to verify that the attribute keys and values used in examples actually exist.
Common log attribute keys include `service.name`, `otel.log.severity.number`, `otel.log.severity.range`, `otel.log.severity.text`, and `otel.log.body`.
The valid values for `otel.log.severity.range` are: TRACE, DEBUG, INFO, WARN, ERROR, FATAL, and UNKNOWN.

## Environment Variables Reference Table
The README contains an "Environment Variables" table listing all supported env vars.
When adding a new environment variable (e.g., for a new config field), add a row to this table.
Keep the table sorted alphabetically by variable name.

## Validation of changes

When modifying `dash0` in ways that affect the outpout displayed to users, always build the tool anew and validate the output.

## Prose rules

Follow these rules when writing or editing prose in this project.

### Line and Paragraph Structure
- **One sentence per line** (semantic line breaks).
  Each sentence starts on its own line; do not wrap mid-sentence.
- Separate paragraphs with a single blank line.
- Keep paragraphs between 2 and 5 lines (sentences).

### Section headers
Seaction headers should be written in sentence case, e.g., "This is an example".

### Links
- Use inline Markdown links: `[visible text](url)`.
- Link the most specific relevant term, not generic phrases like "click here" or "this page."

### Code Blocks
- Fence with triple backticks and a language identifier (e.g., ` ```yaml `).
- Use code blocks to provide illustrative examples.

### Punctuation and Typography
- End sentences with full stops.
- Use the **Oxford comma** (e.g., "error status, latency thresholds, rate limits, and so on").
- Use curly/typographic quotes in prose (`"..."`, `'...'`); straight quotes are fine inside code blocks.
- Write numbers as digits and spell out "percent" (e.g., "10 percent", not "10%" or "ten percent").

### Referencing GitHub Actions in Documentation

Names of GitHub actions are treated as code, e.g., `send-log-event`, and a link is provided to their folder.
