## ADDED Requirements

### Requirement: `dash0 diff` requires `--experimental`
`diff` SHALL be gated behind `--experimental`/`-X` using the existing whole-command mechanism (`experimental.RequireExperimental`), the same as every other experimental command in this CLI — `[experimental]` prefixed on its `Short` description, `-X` shown in its `Example` lines. Every other scenario in this document assumes `--experimental`/`-X` is already passed; the gate is this requirement's concern alone and is not repeated in every `WHEN` clause below.

#### Scenario: `diff` without `--experimental` fails
- **GIVEN** `--experimental`/`-X` is not passed
- **WHEN** `dash0 diff -f <file>` runs
- **THEN** the command fails before any API operation, naming `diff` and pointing at `--experimental`/`-X`

### Requirement: New `dash0 diff` command
`dash0` SHALL provide a new top-level `diff` command accepting `-f <file|directory>`, the same input `apply` accepts. `diff` SHALL query Dash0 for each document's current state and report what `apply` would do, without writing anything. It SHALL accurately distinguish a create from an update, unlike the existing local-only `apply --dry-run`.

#### Scenario: Preview a create
- **GIVEN** a document describing an asset that does not yet exist in Dash0
- **WHEN** `dash0 diff -f <file>` runs
- **THEN** the asset is reported as would-be-created, and nothing is created in Dash0

#### Scenario: Preview an update
- **GIVEN** a document describing an asset that already exists in Dash0 with different content
- **WHEN** `dash0 diff -f <file>` runs
- **THEN** the difference between the current and proposed state is shown, and nothing is changed in Dash0

### Requirement: `diff --since <ref>` previews deletions too
`dash0 diff -f <dir> --since <ref>` SHALL preview the full plan `dash0 apply -f <dir> --since <ref>` would execute, including which assets would be deleted, following the same identifier-based detection, kind coverage, identity-matching, and ref-resolution rules defined for `apply`'s `--since` flag. It SHALL NOT delete, create, or update anything. `--since` is accepted with `-f <file>` as well as `-f <dir>`: a surviving multi-document YAML file can still have an asset reported as would-be-deleted if one of its documents was removed, and a surviving `PrometheusRule` CRD can have an individual alerting rule reported as would-be-deleted (resolved by name) even though the CRD's shared identifier persists — per `apply`'s identifier-based and name-based detection.

#### Scenario: Preview a deletion
- **GIVEN** a file whose content yields one or more asset identifiers was present under the directory at `<ref>` and has since been deleted
- **WHEN** `dash0 diff -f <dir> --since <ref>` runs
- **THEN** every asset whose identifier came from that file is reported as would-be-deleted, and nothing is deleted in Dash0

#### Scenario: Unresolvable ref fails diff the same way it fails apply
- **GIVEN** a `--since <ref>` value that git cannot resolve to a commit
- **WHEN** `dash0 diff -f <dir> --since <ref>` runs
- **THEN** the command exits with code `2` and reports no plan

#### Scenario: All-zeros SHA sentinel gets the same specific error as apply
- **GIVEN** `--since` is passed git's all-zeros SHA sentinel, e.g. from `github.event.before` on a branch's first push
- **WHEN** `dash0 diff -f <dir> --since <ref>` runs
- **THEN** the command exits with code `2`, identifying the sentinel and recommending the caller skip `--since` for this invocation or pass an explicit ref, and reports no plan

#### Scenario: Empty `--since` value gets the same specific error as apply
- **GIVEN** `--since` is passed an empty string, e.g. from a quoted `--since "${{ github.event.before }}"` on a trigger type that doesn't define `before`
- **WHEN** `dash0 diff -f <dir> --since <ref>` runs
- **THEN** the command exits with code `2`, identifying the empty value and recommending the caller skip `--since` for this invocation or pass an explicit ref, and reports no plan

### Requirement: A resolvable-but-non-ancestor `--since` ref surfaces a warning, not a blocking confirmation
Since `diff` never mutates anything, a non-ancestor ref does not need the confirmation `apply --since` requires. `diff` SHALL print a warning identifying the likely cause (force-push or history rewrite) and noting the resulting preview may be misleading, then proceed to compute and show the plan.

#### Scenario: Non-ancestor ref warns but still previews
- **GIVEN** `--since <ref>` resolves to a real commit that is not an ancestor of the current commit
- **WHEN** `dash0 diff -f <dir> --since <ref>` runs
- **THEN** a warning is printed identifying the likely cause, and the plan is still computed and shown

### Requirement: A document fetch failure aborts the whole plan, not just that document
If querying Dash0 for any single document's current state fails (e.g. a transient network error, an authentication failure, a 5xx response), `diff` SHALL abort computing the plan entirely and report the failure as an error. It SHALL NOT report a partial plan covering only the documents whose fetch succeeded — consistent with `apply`'s existing all-or-nothing validation gate, where all documents are validated before any are applied.

#### Scenario: A fetch fails partway through a multi-document diff
- **GIVEN** a directory with multiple documents, where Dash0 returns a transient error while fetching the current state of one of them after others have already succeeded
- **WHEN** `dash0 diff -f <dir>` runs
- **THEN** the command exits with code `2`, and no partial plan covering only the successfully-fetched documents is reported

### Requirement: A deleted asset with no identifier fails diff too
`dash0 diff --since <ref>` SHALL apply the same rule as `apply --since`: when a deleted document's content at `<ref>` carries no stable identifier, `diff` SHALL fail with an error before reporting any plan, rather than reporting a partial preview that omits that asset.

#### Scenario: No-identifier deletion fails diff the same way it fails apply
- **GIVEN** a deleted document whose content at `<ref>` has no stable identifier (id or origin, depending on kind)
- **WHEN** `dash0 diff -f <dir> --since <ref>` runs
- **THEN** the command exits with code `2` before reporting any plan, naming the offending document and the missing identifier

### Requirement: Exit code distinguishes differences from errors, matching `kubectl diff`
`dash0 diff` SHALL use a three-way exit code, a deliberate exception to this CLI's usual uniform 0/1 convention, scoped to `diff` alone: `0` when there is nothing to report (the proposed state matches Dash0's current state exactly — nothing would be created, updated, or deleted), `1` when at least one difference is found (a create, update, or deletion pending), and `2` for a genuine error (e.g. an unresolvable `--since` ref, a deleted asset with no stable identifier, an authentication failure, a document fetch failure). This mirrors `kubectl diff`'s own exit-code convention, the command `diff` is explicitly modeled on, so a CI script can branch on "review pending changes" versus "something is broken" from the exit code alone. `apply` (including its deprecated `--dry-run` flag) is unaffected and keeps this CLI's uniform 0/1 convention.

#### Scenario: No differences
- **GIVEN** every document matches its corresponding asset in Dash0 exactly and no deletions are pending
- **WHEN** `dash0 diff -f <dir>` runs
- **THEN** it exits with code `0`

#### Scenario: At least one difference
- **GIVEN** at least one document would be created, updated, or (with `--since`) deleted
- **WHEN** `dash0 diff` runs
- **THEN** it exits with code `1`

#### Scenario: A genuine error exits with a distinct code
- **GIVEN** a genuine error condition (e.g. an unresolvable `--since` ref, a deleted asset with no stable identifier, an authentication failure, a document fetch failure)
- **WHEN** `dash0 diff` runs
- **THEN** it exits with code `2`, distinct from both the clean (`0`) and differences-pending (`1`) cases
