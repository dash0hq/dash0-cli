## Context

`add-diff-and-since-flag` puts all deletion-detection logic inside the `dash0` binary and deliberately rejected a companion GitHub Action for that logic (see "Git lives inside the `dash0` binary, not a separate layer" in that change's `design.md`). This change is not that: it doesn't touch git-diffing or deletion detection at all. It exists purely because correctly gating `--since` around GitHub's event-payload quirks (the all-zeros sentinel, `before` being unset on non-push triggers, quoting) is boilerplate every adopting workflow would otherwise have to reproduce by hand, and this project already has precedent (`setup`, `send-log-event`) for eliminating that kind of repeated CI wiring with a small composite action.

## Decisions

### A thin wrapper, not a second implementation of ref resolution

The action's only logic is: given the triggering event name and `github.event.before`, decide whether `--since <ref>` is safe to pass, and either pass it (quoted) or omit it. It does not implement git-diffing or deletion-detection logic — `dash0 apply --since` already does all of that, including its own specific errors for the all-zeros/empty cases. The action does run a couple of lightweight git preflight checks of its own (resolvability, ancestry — see below) to decide whether to pass `--since` at all, but that's deciding *whether to pass the flag*, not duplicating what `dash0` does once it has one.

### Gate on the event payload, not on `dash0`'s error output

Considered alternative: always pass `--since ${{ github.event.before }}` and let `dash0`'s new specific error messages guide the user. Rejected as the action's default behavior: it would mean every first push to a new branch, and every `workflow_dispatch`/`schedule`/`pull_request` run, fails the whole job by default — defeating the point of a "wraps it correctly" action. The entire reason to build this is to make those cases *not* fail, by omitting `--since` automatically, rather than surfacing a well-worded error on every such run.

### Fetch depth is still the caller's responsibility, but a too-shallow checkout gets its own specific error

This action cannot fix an already-too-shallow checkout — by the time it runs, `actions/checkout` has already completed. It documents the concrete requirement (`fetch-depth: 0`, i.e. full history — not just "increase it," since `github.event.before` needs to cover however many commits the triggering push contained, which is unbounded and unknown in advance, unlike a fixed relative ref such as `HEAD~1`) rather than attempting to deepen the checkout itself, which would add a second git network operation with its own failure modes for uncertain benefit.

What the action *does* do is validate, before invoking `dash0`, that a real-looking `before` value actually resolves within the checkout it's given (e.g. `git rev-parse --verify <before>^{commit}`). This three-way split matters:

- `before` is all-zeros, empty, or unset → omit `--since`, proceed with a plain `apply`. Nothing is wrong; there's genuinely no prior state to diff against.
- `before` is real-looking and resolves → pass `--since <before>`, quoted.
- `before` is real-looking but does *not* resolve (most commonly: `fetch-depth` too shallow to contain it) → fail the job with a specific error naming `fetch-depth` as the likely cause and `fetch-depth: 0` as the fix.

The third case must fail loudly, not fall into the first case's "omit `--since`" behavior: unlike the sentinel/empty/unset cases, a too-shallow checkout means a real prior state *does* exist to diff against — the CI configuration just failed to fetch it. Silently omitting `--since` here would silently skip deletion detection because of a misconfigured checkout, which is exactly the class of quiet failure this whole feature (`add-diff-and-since-flag`) exists to close. `dash0 apply --since` itself would also produce an error for this case (a generic "unresolvable ref"), but performing the check in the action first lets the error carry CI-specific context — "this is your checkout's `fetch-depth`" — that the CLI has no way to know about.

### A non-ancestor `before` fails the job too, for the same reason a too-shallow one does

Since this action always passes `--force` to its `apply` invocation (see the `--force` decision in `add-diff-and-since-flag/design.md`), the CLI's own confirmation-bypass would silently let a non-ancestor `before` through — recomputing a deletion plan between two unrelated trees, exactly the mass-deletion risk `add-diff-and-since-flag/design.md`'s ancestor-check decision exists to guard against. The action therefore checks ancestry itself, alongside the existing resolvability check (`git rev-parse --verify` plus `git merge-base --is-ancestor <before> HEAD`), before ever invoking `dash0`. A `before` that resolves but isn't an ancestor — the signature of a force-push or history rewrite on the tracked branch — fails the job with a specific error naming the likely cause, the same posture as the too-shallow-checkout case: a real anomaly in the CI's git state, not something to silently bypass just because `--force` happens to be set for other reasons.

### Why a separate Action, not a CLI-side `--since auto` mode

Considered alternative: fold this action's event-gating logic into `dash0` itself, e.g. a `--since auto` flag that reads `github.event.before` directly. Rejected: it would require the CLI to understand every CI platform's specific event-payload conventions (GitHub's `github.event.before`, and whatever the equivalent is on GitLab CI, CircleCI, Buildkite, and so on), directly at odds with keeping the CLI's own error text and behavior CI-agnostic (see `add-diff-and-since-flag/design.md`'s note on why the CLI's ref-resolution errors describe the git-level condition, not any specific CI provider's field name).

A per-platform convenience package is the correct boundary instead: `asset-synch` is this project's GitHub Actions package; an equivalent package for another CI platform (a GitLab CI component, a CircleCI orb) would translate that platform's own event conventions the same way, without requiring `dash0` itself to know about any of them. This is the opposite arrangement from the git-diffing-in-an-Action alternative the sibling change rejected: that alternative would have put universally-needed logic (deletion detection, needed regardless of CI platform) in one platform-specific place. This action puts platform-specific logic (GitHub's event-payload translation) in a platform-specific place — the correct division of labor between a platform-agnostic core tool and platform-specific glue, not a repeat of the same mistake one layer up.

### The action calls `apply`, not `diff`, but exposes the resolved ref either way

The action's primary job is running `dash0 apply -f <dir>` with the correctly-resolved `--since` argument, since that's the actual sync operation a GitOps pipeline needs. But the resolution logic (event name + `before` → safe `--since` value or omission) is exactly what a workflow calling `dash0 diff` instead would also need, so that resolved value is exposed as an output rather than being locked inside the action's own `apply` invocation.

## Non-goals

- Auto-deepening a shallow `actions/checkout` from within this action.
- Any git-diffing or deletion-detection logic (stays inside `dash0`, per `add-diff-and-since-flag`).
- A GitLab CI or other-CI equivalent — GitHub Actions only, matching the existing two composite actions.
