# NixOS VM example for the Dash0 Home Manager module

This example boots a disposable NixOS virtual machine with the [`programs.dash0`](../../hm-module.nix) Home Manager module enabled.
It lets you verify, on a real activation, that the module generates `~/.dash0/profiles.json` and that the CLI reads it back — without changing your host.

The VM declares three profiles for one user: two OAuth profiles (`prod`, `staging`) and one static profile (`ci`) whose token is read from a file.

## Prerequisites

A Linux machine with Nix and flakes enabled.
If flakes are not on globally:

```bash
export NIX_CONFIG="experimental-features = nix-command flakes"
```

## Run the VM

Build the VM runner script:

```bash
nixos-rebuild build-vm --flake .#dash0vm
```

Boot it:

```bash
./result/bin/run-dash0vm-vm
```

The console auto-logs in as `alice`.
Quit the VM with `Ctrl-a x`.

## What to check inside the VM

List the profiles the module generated:

```bash
dash0 config profiles list
```

You should see `prod` marked active and in the `oauth (not logged in)` state, `staging` likewise, and `ci` showing a masked `static` token read from `/etc/dash0/demo-token`.

Inspect the resolved active profile:

```bash
dash0 config show
```

If the profiles are not present yet, the Home Manager activation service may still be settling; check it with:

```bash
systemctl status home-manager-alice.service
```

## Test against a branch or a local checkout

By default the example pulls the CLI from the published repository.
Point it at a branch:

```bash
nixos-rebuild build-vm --flake .#dash0vm --override-input dash0-cli github:dash0hq/dash0-cli/<branch>
```

Or at a local checkout (run from this directory; the path is the repo root):

```bash
nixos-rebuild build-vm --flake .#dash0vm --override-input dash0-cli path:../../..
```

## Verify the activeProfile guard

Set `pruneUndeclared = true` and point `activeProfile` at a name you did not declare (for example `"typo"`) in `flake.nix`, then rebuild:

```bash
nixos-rebuild build-vm --flake .#dash0vm
```

Evaluation should fail with the `programs.dash0.activeProfile … no such profile is declared` assertion, before anything is built.

## Notes

- OAuth `dash0 login` opens a browser and listens on a localhost callback, which is awkward in a headless VM — perform a real login on a desktop NixOS box.
  The token-preservation behavior across rebuilds is covered by the `hm-merge` flake check (`nix flake check github:dash0hq/dash0-cli`).
- The demo static token lives in a world-readable `/etc/dash0/demo-token` purely for illustration.
  For real deployments, set `authTokenFile` to a secret path managed by [sops-nix](https://github.com/Mic92/sops-nix) or [agenix](https://github.com/ryantm/agenix) so the token never lands in the world-readable Nix store.
