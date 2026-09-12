# Maintaining the GitHub Actions

Repository-side notes on the three composite actions under `.github/actions/`.
For the user-facing action documentation, see [github-actions.md](github-actions.md) and the sibling [setup/README.md](../.github/actions/setup/README.md), [send-log-event/README.md](../.github/actions/send-log-event/README.md), and [sync-assets/README.md](../.github/actions/sync-assets/README.md).

## Keeping the actions in sync with CLI changes

When modifying the logic of `dash0 config`, ensure that the [setup](../.github/actions/setup/action.yaml) GitHub Action is not affected negatively.
Ensure that the constraints of `dash0 config profiles create` are enforced in the input validation of the setup GitHub action.

When modifying the flags of `dash0 logs send`, ensure that the [send-log-event](../.github/actions/send-log-event/action.yaml) GitHub Action inputs stay in sync.

`send-log-event` and `sync-assets` both reuse-or-create a Dash0 CLI profile via the shared `.github/actions/lib/ensure-profile.sh` script, rather than duplicating that logic inline.
It detects an existing active profile via `dash0 config show -o json`'s `.profile.value` field — a change to that field's name or shape (see `docs/commands.md`'s `config show` section) needs the equivalent change there.
`send-log-event` still supports CLI versions back to 1.1.0 (unlike `sync-assets`, floored at 1.17.0), which predate `config show -o json` (introduced alongside agent mode in v1.8.0) — the script falls back to parsing `config show`'s human-readable output when `-o json` itself fails, so this compatibility must be preserved if the fallback is ever touched.
`REQUIRE_PROFILE` controls whether "no active profile and no connection inputs" is a hard error (`send-log-event`, which always needs credentials) or a soft warning (`sync-assets`, which has a credential-free dry-run path).

When modifying `apply --since`'s ref-resolution error semantics (the all-zeros SHA sentinel, empty-string handling, non-ancestor warning, or the `--dry-run --agent-mode` JSON shape), ensure the [sync-assets](../.github/actions/sync-assets/action.yaml) GitHub Action's own preflight logic stays in sync:

- The `Resolve --since comparison ref` step's all-zeros sentinel constant and its resolvability/ancestry `git` checks mirror `dash0 apply --since`'s own classification (`internal/git/ref.go`'s `ClassifyRef`) — a change to which values `dash0` treats as "no prior state" needs the equivalent change here.
- The `Compute pending plan` step parses `dash0 apply --dry-run --agent-mode`'s JSON output (`[{path, changes: [{op, kind, name, originOrId, since}]}]`) with `jq`. A change to that JSON shape (see `internal/apply/dryrun.go`'s `dryRunChangeJSON`) needs the equivalent change to this step's `jq` filters, and to the `deletions`/`modifications` output shape documented in the action's README.

## Testing the setup action

The workflow `.github/workflows/test-setup-action.yml` runs on every pull request and on every push to `main` (not just changes to the action) because CLI changes — especially to `dash0 config profiles create` — can break the action's profile-creation step.
Direct pushes to feature branches without an open PR do not trigger the workflow; open a (draft) PR to run it.
The workflow can also be triggered manually via `workflow_dispatch`.

The profile-creation tests mirror the parameter combinations tested in `TestCreateProfileCmdPartialFields` in `internal/config/config_cmd_test.go`.
Each combination is a separate job that asserts the correct fields are set and the omitted fields show `(not set)` (or `default` for dataset).
When adding or removing flags from `dash0 config profiles create`, update both the unit test and the workflow.

## Testing the sync-assets action

The workflow `.github/workflows/test-sync-assets-action.yml` runs on every pull request and on every push to `main`, mirroring the setup action's testing rationale: changes to `apply --since`'s ref-resolution behavior can silently break this action's preflight logic.
Most jobs need no real Dash0 credentials — dummy `api-url`/`auth-token` values are enough, since either the job intercepts every `dash0 apply` call with a wrapper script (mirroring `send-log-event`'s `test-argument-mapping` job) to verify argument construction, or it exercises `dry-run: true`, which makes no API calls at all.
Only `test-end-to-end-sync` performs a real create-then-delete cycle against the live Dash0 API and is gated behind `if: github.repository == 'dash0hq/dash0-cli' && github.actor != 'dependabot[bot]'`, using the same `secrets.DASH0_API_URL`/`secrets.DASH0_AUTH_TOKEN`/`secrets.DASH0_DATASET` secrets `ci.yml`'s roundtrip tests use (a different, more sensitive credential set than `send-log-event`'s `vars.DASH0_OTLP_URL`/`secrets.DASH0_AUTH_TOKEN`, since this action talks to the API, not OTLP ingest).

The non-ancestor-ref scenarios construct a real orphan commit in the checkout (`git hash-object -w -t tree /dev/null` plus `git commit-tree`) rather than trying to simulate `github.event.before`, since a test workflow cannot fabricate an arbitrary value for that context field — testing the `since` input directly (an explicit ref, not relying on the triggering event) is the practical equivalent, and is what the action's own resolvability/ancestry preflights operate on either way.

### Why `since: auto` handles each GitHub event corner case the way it does

- **First push to a new branch / no `before` at all.** `github.event.before` is git's all-zeros SHA sentinel on a branch's first push, and is unset entirely on `workflow_dispatch`/`schedule`. Neither names a real commit, so there is no prior state to compare against — treating either as "no comparison ref" (rather than failing the job) is the only sane default, since a first push or a manually dispatched run is not a misconfiguration.
- **`pull_request` events are excluded even though `synchronize` sets `before`.** That `before` is the PR branch's own prior push, not a state a force-applied deletion should ever run against automatically: this action always passes `--force` internally (no TTY to answer a confirmation prompt in CI), so if `auto` derived from a PR's `before`, a PR that temporarily removes an asset file would delete the live asset the moment the workflow runs with real credentials — turning a preview surface into a destructive one. A caller that genuinely wants deletion detection on `pull_request` must pass an explicit `since` ref.
- **Too-shallow checkout.** `actions/checkout` only fetches history reachable from the ref(s) it checks out; a shallow clone (the default) may not include the comparison ref at all, so `git rev-parse --verify` fails and the job fails before `dash0` is ever invoked. The same failure can also mean the ref was orphaned by a force-push or rebase — no checkout depth fetches commits unreachable from any branch or tag — so the error message names both possibilities rather than only `fetch-depth`.
- **A force-pushed or rewritten branch.** Once a resolvable comparison ref is not an ancestor of `HEAD`, the CLI's own non-ancestor handling degrades to a stderr-only warning once `--force` is set (which this action always sets) — nothing is left to stop it. The action's own `git merge-base --is-ancestor` preflight, run before `dash0` is ever invoked, restores a real backstop. `is-ancestor` exits `1` for a genuine non-ancestor and something else (commonly `128`) for an actual git error (e.g. a corrupt object store); only exit `1` is treated as "maybe a force-push" and gated behind `accept-non-ancestor-ref` — any other exit code is always a hard failure, since `accept-non-ancestor-ref` should never mask an unrelated git problem as an accepted force-push.
- **A broken expression.** An explicit `since` value that resolves to an empty string or the all-zeros sentinel has no routine cause (unlike `auto` finding no prior state) — it almost always means a workflow expression evaluated to nothing. Failing immediately, rather than silently falling into the same "nothing to do" path as `auto`, surfaces the broken expression instead of masking it as a no-op run.
- **Unsafe interpolation.** Hand-writing `--since "${{ github.event.before }}"` in a workflow is prone to unquoted or ungated interpolation mistakes (a bare `--since` with nothing after it on an event with no `before`, or one that silently swallows a neighboring flag). This action reads `github.event.before` itself so no workflow ever needs to interpolate it into a flag by hand.

## Keeping the action READMEs in sync with the website

The three `README.md` files under `.github/actions/*/` are synced to `dash0.com/docs` as `Setup Dash0 CLI`, `Send Log Event`, and `Sync Dash0 Assets` subpages under the `GitHub Actions` group.
Edits to any README also affect the website on the next release; conversely, treat the README as the source of truth and update it whenever the action's inputs, outputs, or behavior change.
See [`.github/workflows/sync-docs/transformations.yaml`](../.github/workflows/sync-docs/transformations.yaml) for the sync declarations.
