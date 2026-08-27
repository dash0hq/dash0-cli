## ADDED Requirements

### Requirement: `--since` requires `--experimental`
`--since` SHALL be gated behind `--experimental`/`-X`, using a new flag-level gate (`experimental.RequireExperimentalFlag(cmd, "since")`) rather than the existing whole-command gate — `apply` itself is not experimental and its create/update behavior is unaffected. The gate SHALL only fire when `--since` is actually passed; `apply` invocations that don't use `--since` SHALL NOT require `-X`, regardless of any other flag (including `--force` or `--dry-run`) being present. `--force` requires no gate of its own — it has no effect unless combined with `--since`, so it is covered transitively.

#### Scenario: `--since` without `--experimental` fails
- **GIVEN** `--since <ref>` is passed without `--experimental`/`-X`
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the command fails before any git or API operation, with an error naming `--since` specifically and pointing at `--experimental`/`-X`

#### Scenario: `--since` with `--experimental` proceeds normally
- **GIVEN** `--since <ref>` is passed together with `--experimental`/`-X`
- **WHEN** `dash0 apply -f <dir> --since <ref> --experimental` runs
- **THEN** the command proceeds exactly as specified by the rest of this document — the gate does not otherwise alter behavior

#### Scenario: `apply` without `--since` needs no gate
- **GIVEN** an ordinary `apply` invocation that does not pass `--since`
- **WHEN** `dash0 apply -f <dir>` runs (with or without `--force`/`--dry-run`)
- **THEN** the command runs exactly as it does today, with no `--experimental` requirement

Every other scenario in this document that exercises `--since` assumes `--experimental`/`-X` is already passed alongside it — the gate is this requirement's concern alone; the remaining requirements describe `--since`'s functional behavior once that precondition holds, and do not repeat it in every `WHEN` clause.

### Requirement: Deletion-aware sync via `--since`
`dash0 apply -f <file|directory>` SHALL accept an optional `--since <ref>` flag, where `<ref>` is any revision expression git itself accepts — a branch name, a tag, a commit SHA, or a relative expression like `HEAD~<n>`. When present, in addition to its existing create/update behavior, `apply` SHALL delete every asset whose identifier (id or origin, depending on kind) was present in the scanned scope at `<ref>` and is no longer present in the scanned scope's current disk contents — the same files `apply`'s existing create/update path already reads, not a second git-object read of the current commit. An uncommitted local deletion is therefore visible to `--since` the same way it's already visible to plain `apply` today. Detection SHALL operate on individual asset identifiers extracted from every document across the scanned scope, not on whole-file presence: a multi-document YAML file (documents separated by `---`) can lose one identifier while the file itself is unchanged or still exists, and that asset SHALL still be detected as deleted. `PrometheusRule` CRD identity is CRD-level for identifier-based detection, matching the identity model `apply` already uses for create/update (every alerting/recording rule converted from one CRD shares that CRD's single identifier). Removing an individual alerting rule from a CRD that still exists SHALL still be detected — by comparing the parsed rule list (group name + alert name) between `<ref>` and the current commit — and SHALL be deleted by resolving the removed alert's check rule by name (`<group name> - <alert name>`) rather than by the CRD's shared identifier, since that identifier cannot distinguish between alerts within the same CRD. Removing an individual recording rule from a CRD that still exists is not a deletion: recording rules are never individually named or created as separate resources, so the reapplied CRD's normal update already reflects the removal. When `--since` is omitted, `apply` SHALL behave exactly as it does today (create/update only, no deletion detection). Detection SHALL compare only the identifier set at `<ref>` against the identifier set at the current commit, not scan intermediate commits: an asset added and then removed again entirely within that range SHALL be ignored — neither created, updated, nor deleted — since it is absent at both comparison points.

#### Scenario: File removed from the directory since the given ref
- **GIVEN** a directory containing a file at git ref `<ref>` whose content yields one or more asset identifiers (e.g. a single-document file, a multi-document YAML file, or a `PrometheusRule` CRD with several rules)
- **AND** that file has since been deleted from the directory
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** every asset whose identifier came from that file is deleted from Dash0, subject to the confirmation requirement below

#### Scenario: `--since` omitted preserves today's behavior
- **GIVEN** a directory with files added, modified, and removed since some earlier commit
- **WHEN** `dash0 apply -f <dir>` runs without `--since`
- **THEN** only creates and updates are performed; no asset is deleted

#### Scenario: Relative ref like `HEAD~1` is accepted
- **GIVEN** a directory containing a file at `HEAD~1` that has since been deleted
- **WHEN** `dash0 apply -f <dir> --since HEAD~1` runs
- **THEN** the corresponding asset is deleted from Dash0, the same as passing a branch name, tag, or commit SHA — `--since` does not special-case relative expressions

#### Scenario: Document removed from a surviving multi-document file
- **GIVEN** a single multi-document YAML file that still exists after the change
- **AND** one of its documents, present at `<ref>`, has since been removed from that file
- **WHEN** `dash0 apply -f <file> --since <ref>` runs
- **THEN** the asset corresponding to the removed document is deleted from Dash0, even though the file itself still exists

#### Scenario: Alerting rule removed from a surviving PrometheusRule CRD
- **GIVEN** a `PrometheusRule` CRD file that still exists after the change
- **AND** one of its alerting rules, present at `<ref>`, has since been removed from that file, while other rules remain
- **WHEN** `dash0 apply -f <file> --since <ref>` runs
- **THEN** the check rule named `<group name> - <alert name>` for the removed alert is resolved by name and deleted, even though the CRD's shared identifier is still present and its remaining rules are updated as usual

#### Scenario: Recording rule removed from a surviving PrometheusRule CRD is a plain update, not a deletion
- **GIVEN** a `PrometheusRule` CRD file that still exists after the change
- **AND** one of its recording rules, present at `<ref>`, has since been removed from that file, while other rules remain
- **WHEN** `dash0 apply -f <file> --since <ref>` runs
- **THEN** no deletion is performed; the CRD's normal update already reflects the removal, since recording rules are never individually named or created as separate resources

#### Scenario: Asset added and removed between `<ref>` and HEAD
- **GIVEN** an asset's identifier does not exist at `<ref>`, was added in a later commit, and was removed again before the current commit
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** that asset is neither created, updated, nor deleted — it is ignored, since it is absent at both `<ref>` and the current commit

### Requirement: Deletion covers every asset kind `apply` supports
Deletion detection SHALL dispatch to the same per-kind `delete` operation `apply` already uses for that kind's create/update path, covering all kinds `apply` supports: `Dashboard`/`PersesDashboard`, `CheckRule`, `SyntheticCheck`, `View`, `Dash0SpamFilter`, `Dash0NotificationChannel`, `Dash0Team`, and `PrometheusRule` (recording and alerting rules).

#### Scenario: Mixed PrometheusRule CRD deletion
- **GIVEN** a deleted file that was a `PrometheusRule` CRD containing multiple alerting rules and recording rules
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** both the corresponding check rules and the corresponding recording rules are deleted

#### Scenario: Multiple alerting rules in one PrometheusRule CRD
- **GIVEN** a deleted file that was a `PrometheusRule` CRD containing multiple alerting rules
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** all the corresponding check rules are deleted

#### Scenario: Multiple recording rules in one PrometheusRule CRD
- **GIVEN** a deleted file that was a `PrometheusRule` CRD containing multiple recording rules
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** all the corresponding recording rules are deleted

### Requirement: Deletion does not verify the asset's live state against git history
`--since` SHALL delete an asset solely because its identifier is present in the scanned scope at `<ref>` and absent at the current commit. It SHALL NOT compare the asset's current state in Dash0 against the content git last recorded for that identifier before deleting it; there is no drift check. This matches every other destructive command in this CLI, none of which verify server state before deleting.

#### Scenario: Asset modified out-of-band since last recorded in git
- **GIVEN** an asset's file was removed from the directory since `<ref>`
- **AND** the asset's current state in Dash0 differs from what was last recorded in git for that identifier (e.g. it was edited via the Dash0 UI after the last commit that touched it)
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the asset is deleted anyway, subject only to the confirmation requirement below — no comparison against its live state is performed

### Requirement: `apply` gains a `--force` flag; deletions require confirmation like any other destructive command
`dash0 apply` SHALL accept a new `--force` flag — `apply` has none today, unlike every per-kind `<kind> delete` command. Each deletion triggered by `--since` SHALL go through the same confirmation prompt every `<kind> delete` command already uses, skipped only when `--force` is passed or agent-mode is active. No separate confirmation mechanism is introduced for this feature. Declining an individual deletion's prompt SHALL skip only that asset — the rest of the `--since` run (other creates, updates, and deletions) continues. A run containing at least one declined deletion SHALL cause `apply` to exit with a non-zero status once it completes, since the desired end state (that asset gone) was not reached.

#### Scenario: Interactive run without --force
- **GIVEN** `--since` detects one or more assets to delete
- **AND** `--force` is not passed and agent-mode is not active
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the user is prompted to confirm each deletion individually before it happens

#### Scenario: Non-interactive run with --force or agent-mode
- **GIVEN** `--since` detects one or more assets to delete
- **AND** `--force` is passed (or agent-mode is active)
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** each deletion proceeds without a prompt

#### Scenario: Deletion prompt declined
- **GIVEN** `--since` detects one or more assets to delete, in an interactive run without `--force`
- **WHEN** the user declines the confirmation prompt for one of them
- **THEN** that asset is not deleted, the rest of the run continues, and the command exits with a non-zero status once complete

### Requirement: Identity is by id/origin, never by local file path
An asset whose backing file still exists anywhere under the target directory (e.g. it was renamed or moved to a different subdirectory) SHALL NOT be deleted, even though git reports its old path as removed. Matching SHALL use the asset's `id`/`origin` label read from file content, not its file path. The target directory is a local discovery scope only and has no relationship to an asset's `dash0.com/folder-path` annotation or its placement in Dash0.

#### Scenario: File renamed within the directory
- **GIVEN** an asset's file is moved from one subdirectory to another within the same target directory, with its id/origin unchanged
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the asset is not deleted, and is updated in place if its content changed

### Requirement: A deleted asset with no identifier fails the whole `--since` run
When a deleted document's content at `<ref>` carries no stable identifier (id or origin, depending on kind), `apply` SHALL fail the entire `--since` run before creating, updating, or deleting anything — the same failure mode as an unresolvable ref. It SHALL NOT skip that one deletion with only a warning and continue: a silently-skipped orphan is exactly the easy-to-miss failure this feature exists to close.

#### Scenario: Deleted document never had an id
- **GIVEN** a deleted document whose content at `<ref>` has no `dash0.com/id` (or equivalent origin) set
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the command exits with an error before creating, updating, or deleting anything, naming the offending document and the missing identifier

### Requirement: An unresolvable `--since` ref fails the whole command
When `<ref>` cannot be resolved (a nonexistent ref, a too-shallow git history, git's all-zeros SHA sentinel, or an empty string), `apply` SHALL fail before creating, updating, or deleting anything. It SHALL NOT fall back to create/update-only. When `<ref>` is specifically git's all-zeros SHA sentinel (`0000000000000000000000000000000000000000`) or an empty string, the error message SHALL name the specific condition in CI-agnostic terms — the sentinel: "the ref is git's all-zeros SHA, meaning there's no prior commit to compare against — common on a branch's first push"; an empty string: "`--since` was passed an empty value, likely because this trigger has no prior-commit reference to supply" — and suggest skipping `--since` for that invocation or passing an explicit ref. The message names the git-level condition, not any specific CI provider's field name (e.g. GitHub's `github.event.before`), so it stays useful across CI providers; GitHub-specific framing belongs in `asset-synch`'s own documentation and preflight output (see `openspec/changes/add-asset-synch-action`), not in the core CLI's error text. The failure mode itself (whole command fails, no fallback) is otherwise identical to any other unresolvable ref.

#### Scenario: Ref cannot be resolved
- **GIVEN** a `--since <ref>` value that git cannot resolve to a commit
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the command exits with an error and no asset is created, updated, or deleted

#### Scenario: All-zeros SHA sentinel gets a specific, actionable error
- **GIVEN** `--since` is passed git's all-zeros SHA sentinel, e.g. from `github.event.before` on a branch's first push
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the command exits with an error identifying the sentinel and recommending the caller skip `--since` for this invocation or pass an explicit ref, and no asset is created, updated, or deleted

#### Scenario: Empty `--since` value gets a specific, actionable error
- **GIVEN** `--since` is passed an empty string, e.g. from a quoted `--since "${{ github.event.before }}"` on a trigger type that doesn't define `before`
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the command exits with an error identifying the empty value and recommending the caller skip `--since` for this invocation or pass an explicit ref, and no asset is created, updated, or deleted

### Requirement: A resolvable-but-non-ancestor `--since` ref requires confirmation, not a hard failure
When `<ref>` resolves to a real commit that is not an ancestor of the current commit (e.g. after a force-push or history rewrite on the tracked branch), `apply` SHALL NOT treat this the same as an unresolvable ref. It SHALL prompt for confirmation before computing the deletion plan, naming the likely cause (force-push or history rewrite) — skipped only when `--force` is passed or agent-mode is active, the same bypass every other destructive operation in this CLI already uses. When neither applies and no confirmation can be obtained (no terminal available), the command SHALL fail rather than proceed silently.

#### Scenario: Non-ancestor ref prompts for confirmation
- **GIVEN** `--since <ref>` resolves to a real commit that is not an ancestor of the current commit
- **AND** `--force` is not passed and agent-mode is not active
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the user is prompted to confirm before any deletion plan is computed, and the prompt names the likely cause (force-push or history rewrite)

#### Scenario: Non-ancestor ref proceeds with --force
- **GIVEN** `--since <ref>` resolves to a real commit that is not an ancestor of the current commit
- **AND** `--force` is passed (or agent-mode is active)
- **WHEN** `dash0 apply -f <dir> --since <ref>` runs
- **THEN** the command proceeds without prompting, computing the deletion plan against the resolved ref as usual

### Requirement: `apply --dry-run` is deprecated in favor of `dash0 diff`
`--dry-run` on `apply` SHALL be marked deprecated in favor of `dash0 diff`. It SHALL continue to function for backward compatibility — including combined with `--since`, where it previews the plan (creates, updates, and deletions) without writing anything, the same as before this change — but each invocation SHALL print a deprecation warning to stderr recommending `dash0 diff` instead, following the same pattern used for other deprecated flags in this CLI (e.g. `teams update --name`). `apply --dry-run`'s output format and exit-code behavior are not held to permanent parity with `dash0 diff`'s exit-code convention; `dash0 diff` is the actively-maintained, canonical way to preview a plan going forward.

#### Scenario: `--dry-run` still works but warns
- **GIVEN** any `dash0 apply -f <file|directory> --dry-run` invocation, with or without `--since`
- **WHEN** the command runs
- **THEN** it previews the plan as before (no writes), and a deprecation warning recommending `dash0 diff` is printed to stderr

#### Scenario: `--dry-run --since` still only previews, never deletes
- **GIVEN** `--since <ref>` detects one or more assets that would be deleted
- **WHEN** `dash0 apply -f <dir> --dry-run --since <ref>` runs
- **THEN** the deletions are reported as part of the preview, and nothing is deleted, created, or updated
