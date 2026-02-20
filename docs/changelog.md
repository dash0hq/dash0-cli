# Changelog

This project uses [chloggen](https://github.com/open-telemetry/opentelemetry-go-build-tools/tree/main/chloggen) to manage changelog entries.
Each user-facing change gets its own YAML file in `.chloggen/`.

## Creating a Changelog Entry
1. Run `make chlog-new` — this creates `.chloggen/<branch-name>.yaml` from the template
2. Fill in the fields:
   - `change_type`: one of `breaking`, `deprecation`, `new_component`, `enhancement`, `bug_fix`
   - `component`: area of concern (e.g., `dashboards`, `config`, `apply`, `logs`)
   - `note`: brief description of the change; wrap in quotes if it starts with a backtick
   - `issues`: list of related issue or PR numbers, e.g., `[28]`
   - `subtext`: (optional) additional detail, use `|` for multiline
3. Run `make chlog-validate` to verify the entry is well-formed
4. Run `make chlog-preview` to see how it will render in `CHANGELOG.md`
5. Commit the `.chloggen/<branch-name>.yaml` file with the rest of the changes

## When to Skip
If a change doesn't affect end users (refactoring, CI changes, etc.), prefix the PR title with `chore` or add the "Skip Changelog" label instead of creating an entry.

## Reference
- Template: `.chloggen/TEMPLATE.yaml`
- Config: `.chloggen/config.yaml`
- Render template: `.chloggen/summary.tmpl`
- See existing entries (e.g., `.chloggen/logs_create.yaml`) for examples
