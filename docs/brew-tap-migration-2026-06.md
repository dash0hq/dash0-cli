# Homebrew tap migration — June 2026

The Dash0 CLI's Homebrew distribution is moving in two ways:

1. From `Formula/dash0.rb` inside the [main repository](https://github.com/dash0hq/dash0-cli) to a dedicated tap repository at [`dash0hq/homebrew-dash0-cli`](https://github.com/dash0hq/homebrew-dash0-cli).
2. From a Homebrew **formula** to a Homebrew **cask**, which is the standard mechanism for pre-built CLI binaries.

For you as a user, the practical impact is one new install command.

If you install the CLI with Homebrew, please read on.

## TL;DR

After the migration release ships, install or reinstall with this single command:

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

That is the **whole** install path on Homebrew 6.0 and later.
No `brew tap` step, no `brew trust` step, no extra ceremony.

If you have already tapped `dash0hq/dash0-cli` in the past, run these once after the migration release:

```bash
brew uninstall dash0
```

```bash
brew untap dash0hq/dash0-cli
```

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

After that, `brew upgrade --cask dash0` (or plain `brew upgrade`) picks up future updates from the new tap.

## Why this is happening

Two unrelated forces converged on the same change.

**Homebrew 6.0 enforces tap trust.** [Homebrew 6.0](https://brew.sh/2026/06/11/homebrew-6.0.0/), released on 11 June 2026, requires explicit trust for any non-official tap.
Before 6.0, you could `brew tap dash0hq/dash0-cli <URL>` and `brew install dash0` would just work.
After 6.0, the same workflow requires `brew trust dash0hq/dash0-cli` first.
Homebrew exempts fully qualified installs (`brew install owner/tap/name`) from the trust prompt — but only when the tap repository follows the `homebrew-<name>` naming convention.
Our previous setup did not qualify: the formula lived in `dash0hq/dash0-cli`, a repository that is not named `homebrew-*` because it is the CLI's source tree.
Moving the formula to a dedicated `dash0hq/homebrew-dash0-cli` repository fixes this.

**Goreleaser is phasing out formula generation for binary CLIs.** Homebrew formulae are designed for software that gets built from source.
A Go CLI like `dash0` ships as a pre-built binary, and the right Homebrew mechanism for distributing pre-built binaries is a [cask](https://docs.brew.sh/Cask-Cookbook), not a formula.
Historically, [goreleaser](https://goreleaser.com) generated formulae that pretended to be source-based but actually just dropped a pre-compiled tarball into place — a workaround from when Linuxbrew did not support casks.
That workaround is no longer necessary, and goreleaser [has deprecated the `brews:` configuration](https://goreleaser.com/deprecations#brews) in favor of `homebrew_casks:`.
Since we already had to move repositories for the trust reason, doing the formula-to-cask switch in the same migration avoids a second user-facing transition later.

As a happy coincidence, the tap shortname Homebrew uses (`dash0hq/dash0-cli`, with the `homebrew-` prefix stripped) is unchanged.
The qualified install path keeps the same name you may already have in muscle memory; only the `--cask` flag is new.

## What is changing

| | Before | After |
|---|---|---|
| Repository | `dash0hq/dash0-cli` (with the CLI source tree) | `dash0hq/homebrew-dash0-cli` (dedicated) |
| Tap shortname | `dash0hq/dash0-cli` | `dash0hq/dash0-cli` (unchanged) |
| Homebrew artifact type | Formula (`Formula/dash0.rb`) | Cask (`Casks/dash0.rb`) |
| Install command | `brew tap dash0hq/dash0-cli <URL>` + `brew trust dash0hq/dash0-cli` + `brew install dash0` | `brew install --cask dash0hq/dash0-cli/dash0` |
| Upgrade command | `brew upgrade dash0` | `brew upgrade --cask dash0` (or just `brew upgrade`) |
| Where new releases land | Manual formula update in main repo | Auto-generated PR or push on the dedicated tap by [goreleaser](https://goreleaser.com) |

The `dash0` binary lands in the same place it always has (`$(brew --prefix)/bin/dash0`), and the shell completions install identically.
Only the install command and the underlying repository change.

## What you need to do

### New users

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

That is the only command.

### Existing users (you already installed `dash0` via the old tap)

After the migration release ships, `brew upgrade dash0` will print a deprecation warning that looks like this:

```text
Warning: dash0 is deprecated because it has moved to https://github.com/dash0hq/homebrew-dash0-cli!
It will be disabled on 2026-12-31.
Replacement:
  brew install --cask dash0hq/dash0-cli/dash0
```

The migration is not automatic.
Homebrew tracks formulae and casks separately, so the upgrade message tells you what to run; it does not run it for you.
You need to uninstall the formula and install the cask explicitly:

```bash
brew uninstall dash0
```

```bash
brew untap dash0hq/dash0-cli
```

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

The `brew uninstall` step removes the formula version of `dash0` and the `brew untap` step removes the stale pointer to the old repository.
The qualified `brew install --cask` step taps the new repository on first use, trusts it automatically (per the Homebrew 6.0 qualified-install exemption), and installs the cask.
From then on, future updates flow through the new tap.

### Users who set `HOMEBREW_NO_AUTO_UPDATE=1`

Run `brew update` once after the migration release ships so Homebrew sees the deprecation warning and the migration message.
Otherwise the migration is invisible to you and you stay frozen on the cutover release.

## Timeline

| Phase | Date | What happens |
|---|---|---|
| Cutover release | June 2026 | The new tap publishes its first cask version. The old `Formula/dash0.rb` is frozen and tagged with `deprecate!`. `brew upgrade dash0` warns but still completes. |
| Hard cutoff | 31 December 2026 (target) | The old formula is tagged with `disable!`. `brew install dash0` and `brew upgrade dash0` against the old tap stop working with a clear error message pointing at the new install command. |
| Removal | 2027 (estimated) | The old `Formula/dash0.rb` is removed entirely. By this point traffic to the old tap should be near zero. |

The deprecation timeline is generous on purpose — you do not need to do anything urgently.
Migrating in the same week you see the first warning is fine; so is migrating six months later.
The only date that matters is the hard cutoff at the end of 2026, after which old-tap users see an error rather than a warning.

## Troubleshooting

### "Why a cask and not just a formula in the new location?"

Homebrew formulae are designed for software you build from source; casks are designed for pre-built binaries.
The Dash0 CLI is a Go binary that ships pre-compiled, so a cask is the right tool for the job.
Goreleaser, the release pipeline we use, [agrees](https://goreleaser.com/deprecations#brews) and has phased out its formula generator.
Doing the formula-to-cask switch in the same migration as the repository move avoids putting users through two transitions.

### "Why do I need the `--cask` flag in the install command?"

When you run `brew install owner/tap/name`, Homebrew looks for a formula at that path first.
If only a cask exists at the qualified path (as is the case here), the bare `brew install` errors with `FormulaUnavailableError`.
The `--cask` flag tells Homebrew to resolve as a cask.
This is a known Homebrew quirk — there is no auto-detection for qualified paths.

### "I see `brew trust dash0hq/dash0-cli` prompted somewhere — do I still need to run it?"

No.
The trust step was a workaround for the period between Homebrew 6.0's release and this migration.
Once you install via the qualified path (`brew install --cask dash0hq/dash0-cli/dash0`), Homebrew trusts the new tap automatically.
If you ran `brew trust dash0hq/dash0-cli` before this migration, it is harmless and there is nothing to undo.

### "I followed the migration but I still have the old `dash0` formula installed alongside the new cask. What now?"

Run `brew uninstall dash0` (which uninstalls the formula, since the cask has a different identity) to remove the old binary.
The cask install remains in place.
If you skipped the `brew uninstall dash0` step in the migration above, this is the cleanup.

### "Will my installed `dash0` binary keep working in the meantime?"

Yes.
Nothing about the binary itself changes.
The migration only affects how Homebrew finds and updates it.
A `dash0` you installed last month will keep running until you update it; the warnings only appear when you `brew upgrade`.

### "What about CI?"

If you install `dash0` in CI via Homebrew, switch the install command to the qualified cask form:

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

Avoid the `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` escape hatch — Homebrew documents it as a temporary measure that will be removed.

For dash0-specific CI use cases, prefer the [`setup` GitHub Action](../.github/actions/setup/README.md), which installs the CLI directly from GitHub Releases and does not depend on Homebrew at all.

## Questions or problems?

Please [open an issue](https://github.com/dash0hq/dash0-cli/issues) on the main repository, not on the tap repository.
The tap repository's cask is auto-generated and accepts no direct edits — all changes flow through the main repository's release pipeline.
