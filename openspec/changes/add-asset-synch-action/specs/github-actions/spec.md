## ADDED Requirements

### Requirement: New `asset-synch` composite GitHub Action
A new composite action at `.github/actions/asset-synch/` SHALL wrap `dash0 apply -f <dir>`, resolving whether `--since` is safe to pass from the triggering event, rather than requiring the calling workflow to hand-write that gating logic. Since `--since` is gated behind `--experimental`/`-X` (a transitional requirement — see `add-diff-and-since-flag/design.md`), the action SHALL pass `-X` whenever it passes `--since`, and SHALL NOT pass `-X` on the branches where `--since` is omitted (no gate applies there).

#### Scenario: Ordinary push with a valid `before`
- **GIVEN** a `push` event where `github.event.before` resolves to a real commit
- **WHEN** the `asset-synch` action runs
- **THEN** it invokes `dash0 apply -f <dir> --since <before> --force --experimental`, `<before>` correctly quoted — `--force` is always passed since this action is designed exclusively for unattended CI, and `--experimental` is passed because `--since` requires it for now

#### Scenario: First push to a new branch
- **GIVEN** a `push` event where `github.event.before` is git's all-zeros SHA sentinel
- **WHEN** the `asset-synch` action runs
- **THEN** it invokes `dash0 apply -f <dir> --force` without `--since`, and the job does not fail

#### Scenario: Non-push trigger with no `before`
- **GIVEN** a trigger event (e.g. `workflow_dispatch`, `schedule`, `pull_request`) that does not define `github.event.before`
- **WHEN** the `asset-synch` action runs
- **THEN** it invokes `dash0 apply -f <dir> --force` without `--since`, and the job does not fail

### Requirement: A real but unresolvable `before` fails the job with a specific error
When `github.event.before` is a real-looking value (not all-zeros, not empty, not unset) but does not resolve within the current checkout, the `asset-synch` action SHALL fail the job before invoking `dash0`, with an error identifying `fetch-depth` as the likely cause and recommending `fetch-depth: 0` in the workflow's checkout step. It SHALL NOT omit `--since` and proceed with a plain apply in this case — unlike the all-zeros/empty/unset cases, a too-shallow checkout means a real prior state exists to diff against, and silently skipping `--since` here would silently skip deletion detection because of a misconfigured checkout.

#### Scenario: Checkout too shallow to resolve `before`
- **GIVEN** a `push` event where `github.event.before` names a real commit, but the workflow's checkout step fetched too little history to contain it
- **WHEN** the `asset-synch` action runs
- **THEN** the job fails before `dash0` is invoked, with an error naming `fetch-depth` as the likely cause and recommending `fetch-depth: 0`

### Requirement: A non-ancestor `before` fails the job with a specific error, even though `--force` is always passed
Since the action always passes `--force` to `apply` (which would otherwise silently proceed past a non-ancestor ref), the `asset-synch` action SHALL check ancestry itself before invoking `dash0`: when `before` resolves to a real commit that is not an ancestor of the current commit, the action SHALL fail the job before invoking `dash0`, with an error identifying the likely cause (a force-push or history rewrite on the tracked branch) — the same posture as the too-shallow-checkout case.

#### Scenario: Force-pushed branch produces a non-ancestor `before`
- **GIVEN** a `push` event where `github.event.before` resolves to a real commit that is not an ancestor of the current commit (e.g. after a force-push)
- **WHEN** the `asset-synch` action runs
- **THEN** the job fails before `dash0` is invoked, with an error naming the likely cause (force-push or history rewrite)

### Requirement: Resolved `since` value is exposed as an output
The action SHALL expose the value it decided to use for `--since` (or an empty value when omitted) as an action output, so a workflow that wants to call `dash0 diff` directly — instead of, or in addition to, this action's own `apply` call — can reuse the same resolution.

#### Scenario: Consuming the output for a `diff` step
- **GIVEN** a workflow with an `asset-synch` step followed by a separate step calling `dash0 diff`
- **WHEN** the workflow references the `asset-synch` step's `since` output
- **THEN** the value is the same one `asset-synch` used (or empty, in the omitted cases), so the `diff` step and the `asset-synch` step agree on whether `--since` applies

### Requirement: No git-diffing logic is duplicated in the action
The action SHALL NOT implement its own git history comparison, deletion detection, or asset-identifier parsing — all of that SHALL remain inside `dash0 apply --since` / `dash0 diff --since`. The action's only responsibility is deciding whether `--since` is safe to pass and invoking the CLI accordingly.

#### Scenario: Action delegates deletion detection to the CLI
- **GIVEN** a `push` event with a valid `before`
- **WHEN** the `asset-synch` action invokes `dash0 apply -f <dir> --since <before> --force --experimental`
- **THEN** all deletion detection, kind dispatch, and confirmation-bypass behavior is exactly what `dash0 apply --since --force --experimental` already does on its own — the action adds no additional logic beyond flag construction
