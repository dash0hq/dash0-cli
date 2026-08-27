**Depends on `openspec/changes/add-diff-and-since-flag`** — this action wraps `apply --since`/`--force`, which must exist first (Sections 1, 3, and 4 there — including the experimental gate, since the action's own invocation must pass `-X` while it's in effect). The action's YAML/docs/test-workflow scaffolding (below) can be drafted in parallel, but its preflight logic can't be verified end-to-end until the flag work lands.

## 1. Composite action

- [ ] 1.1 New `.github/actions/asset-synch/action.yaml`, following `.github/actions/setup/action.yaml` and `.github/actions/send-log-event/action.yaml`'s conventions (composite action, versioned, referenced by SHA from consumers).
- [ ] 1.2 Inputs: at minimum the directory to sync (mirroring `apply -f <dir>`); reuse whatever the `setup` action already establishes for profile/auth rather than re-accepting `api-url`/`auth-token` directly, if `setup` is expected to run first in the same job.
- [ ] 1.3 Read the triggering event name (`github.event_name`) and `github.event.before` inside the action's own script step.
- [ ] 1.4 Classify `before`: unset, empty, or git's all-zeros SHA sentinel → omit `--since` entirely, proceed with a plain `dash0 apply -f <dir> --force` (no `-X` needed here — no `--since` means no gate to satisfy), job does not fail.
- [ ] 1.5 Otherwise, run the resolvability preflight (`git rev-parse --verify "$BEFORE^{commit}"` or equivalent) — if it fails, fail the job with an error naming `fetch-depth` as the likely cause and `fetch-depth: 0` as the fix (do not fall back to a plain apply; a real prior state exists to diff against, silently proceeding would silently skip deletion detection).
- [ ] 1.6 Then run the ancestry preflight (`git merge-base --is-ancestor "$BEFORE" HEAD`) — if it fails (resolves but isn't an ancestor), fail the job with an error naming the likely cause (force-push or history rewrite on the tracked branch). This check is independent of `--force` — the action always passes `--force` to `apply`, so this preflight is the only thing standing between a force-pushed branch and a silently-computed deletion plan between two unrelated trees.
- [ ] 1.7 Otherwise, invoke `dash0 apply -f <dir> --since "$BEFORE" --force --experimental`, `$BEFORE` correctly quoted. `--experimental`/`-X` is required for now because `--since` is gated behind it (see `add-diff-and-since-flag/design.md`) — passed only on this branch, never on the omitted-`--since` branches above. This is transitional: drop it once `--since` is promoted to stable, with no other change to the action needed.
- [ ] 1.8 Expose the resolved `since` value (or empty, in the omitted cases) as an action output, so a workflow that wants to call `dash0 diff` itself can reuse the same resolution.
- [ ] 1.9 No git-diffing or deletion-detection logic of its own beyond the two preflight checks above — everything else stays inside `dash0`.

## 2. Documentation

- [ ] 2.1 `.github/actions/asset-synch/README.md`, following `.github/actions/setup/README.md`/`.github/actions/send-log-event/README.md`'s structure — inputs, outputs, a minimal usage example, and the `fetch-depth: 0` requirement called out explicitly (link to `add-diff-and-since-flag`'s fetch-depth guidance).
- [ ] 2.2 `docs/github-actions.md`: add an `asset-synch` section alongside the existing `setup`/`send-log-event` sections.
- [ ] 2.3 `docs/github-actions-maintenance.md`: add `asset-synch` to the "keeping the actions in sync with CLI changes" guidance — specifically, any future change to `apply --since`'s ref-resolution error semantics (all-zeros/empty/non-ancestor/too-shallow) needs a corresponding check in this action's preflight logic.
- [ ] 2.4 Changelog entry (`make chlog-new`) for the new `asset-synch` action.

## 3. Testing

- [ ] 3.1 New workflow `.github/workflows/test-asset-synch-action.yml`, following `.github/workflows/test-setup-action.yml`'s pattern (runs on every PR and push to `main`, plus `workflow_dispatch`) — since this action's correctness depends on `dash0 apply`'s ref-resolution behavior, the same rationale for testing on every push applies.
- [ ] 3.2 Test scenarios, each as a separate job: ordinary push with a valid `before` (since gets passed, quoted, `--force` and `--experimental` present); first push to a new branch (all-zeros sentinel, `--since`/`--experimental` omitted, job succeeds); `workflow_dispatch` (no `before`, `--since`/`--experimental` omitted, job succeeds); too-shallow checkout (job fails with the `fetch-depth` message); non-ancestor `before` via a simulated force-push (job fails with the force-push/history-rewrite message).
- [ ] 3.3 Verify the `since` output is set correctly (or empty) in each scenario, and that a downstream step can consume it.

## 4. Verification

- [ ] 4.1 All scenarios in `test-asset-synch-action.yml` pass.
- [ ] 4.2 `make lint` passes (`shellcheck` on the action's script steps, if inline bash is used).
- [ ] 4.3 Manual smoke test: adopt `asset-synch` in a real (or scratch) workflow and confirm it behaves correctly on an ordinary push, a first push to a new branch, and a `workflow_dispatch` run.
