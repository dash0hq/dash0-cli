# Maintaining the dash0-cli Agent Skill

This document covers the repository-side maintenance of the dash0-cli Agent Skill, distributed via `dash0 skill install` / `dash0 skill show` (`internal/skill/`).
For the user-facing command documentation, see the [Agent tooling commands](commands.md#agent-tooling-commands) section of `docs/commands.md`.

## How the bundle is assembled

The skill bundle has two kinds of content:

- **`internal/skill/content/SKILL.md`** is hand-curated.
  It carries the frontmatter (`name`, `description`), the command taxonomy, prerequisites, global flags, and mechanics genuinely shared across many commands (the asset CRUD list/get/create/update/delete pattern, ID/origin/upsert semantics, filter syntax, custom columns, precision mode), plus the "Common workflows for AI agents" content and a topic index.
  Edit it directly; nothing regenerates it.
- **`internal/skill/content/references/*.md`** — one file per top-level `dash0` command (`dashboards`, `logs`, `teams`, and so on; 19 files as of this writing) — is generated from `docs/commands.md` by `internal/skill/gen`.
  Never hand-edit these; edits are overwritten on the next `make skill-bundle`.

Topics are named after the actual `dash0 <command>` an agent would run, not the internal taxonomy category `docs/commands.md` groups them under — an agent that wants to work with dashboards runs `dash0 skill show dashboards`, not `dash0 skill show asset-crud`.
See the [command taxonomy](commands.md#command-taxonomy) for how the categories map to commands.

## Why a generator instead of a mechanical split

`docs/commands.md` documents some patterns once across several commands rather than once per command — the "Asset CRUD commands" section explains the shared list/get/create/update/delete pattern a single time ("The examples below use `dashboards`, but the same patterns apply to every asset type"), and "Query commands" documents filter syntax, custom columns, and precision mode once across five commands.
A blind per-heading split would either duplicate that shared material into every relevant topic file (going stale independently in each copy) or leave per-command topic files with no self-contained content at all.

Instead, `internal/skill/gen/main.go` keeps a hand-maintained `topicSpec` per topic, naming exactly which `docs/commands.md` sections and labeled YAML examples it draws from, plus an optional hand-authored `extraNote` for behavior that isn't cleanly extractable from a single heading (e.g. `dashboards` also accepting PersesDashboard CRD files).
Genuinely shared mechanics live once in `SKILL.md` instead of being duplicated per topic.

## The generator also strips flag tables

Every extracted section has its Markdown flag table (`| Flag | ... |`) replaced with a pointer to `dash0 --agent-mode <command> --help`, which is backed by `internal/help.PrintJSONHelp` and is always exactly current.
This is deliberate (see the [output format exception](code-style.md#output-format-default) in `docs/code-style.md`): flags are the most volatile part of any command's documentation, and duplicating them into the skill bundle would be the single biggest source of drift.
The generator also rewrites the one non-portable link in `docs/commands.md` (`documentation.md#code-blocks`) to an absolute GitHub URL, using the same find/replace pair as `.github/workflows/sync-docs/transformations.yaml`.

## When to regenerate

Run `make skill-bundle` whenever `docs/commands.md` changes in a way that affects a topic's extracted sections, or whenever a new command or asset kind is added (see step 10 of `docs/adding-commands.md`).
It writes to three locations from one generation step:

1. `internal/skill/content/references/*.md` — embedded into the `dash0` binary via `//go:embed`, served by `dash0 skill install`/`dash0 skill show`.
2. `.claude/skills/dash0-cli/` (repo root) and `.agents/skills/dash0-cli/` (repo root) — checked-in copies of the same content, so `npx skills add dash0hq/dash0-cli` and `gh skill install dash0hq/dash0-cli` can discover the skill directly from this repository for hosts `dash0 skill install` doesn't yet auto-detect, without a separate registry-submission step.
   These also mean anyone using Claude Code or Codex to work on the dash0-cli source itself gets the skill automatically.

`make skill-validate` (part of `make lint`) regenerates into a scratch directory and diffs it against all three checked-in locations, failing the build if anything is stale.

## Adding a new topic

1. Add a `ManifestEntry` to `Manifest` in `internal/skill/bundle.go` (topic slug, `references/<topic>.md`, one-line description).
2. Add a matching `topicSpec` to `topics` in `internal/skill/gen/main.go`: the `docs/commands.md` heading title(s) to extract (`sections`), or, for a new asset kind, `includeQuickRef: true` plus the `assetYAMLLabels` to pull from "Asset YAML formats".
3. Run `make skill-bundle` and inspect the generated file.
4. Add the topic to `SKILL.md`'s topic index table.
5. Run `make skill-validate` to confirm everything is in sync.
