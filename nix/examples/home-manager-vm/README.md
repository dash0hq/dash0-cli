# Home Manager module example for the Dash0 CLI

This example exercises the [`programs.dash0`](../../hm-module.nix) Home Manager module so you can verify, on a real activation, that it generates `~/.dash0/profiles.json` and that the CLI reads it back.

It declares three profiles for one user: two OAuth profiles (`prod`, `staging`) and one static profile (`ci`) whose token is read from a file.

Pick the approach that matches where you are:

- **Already inside a NixOS VM, or on any NixOS machine** → use [standalone Home Manager](#option-a-standalone-home-manager-recommended-inside-a-vm). It writes `~/.dash0` for your user with no root and no system rebuild — no nested VM required.
- **On any host, want a disposable guest** → [build a throwaway NixOS VM](#option-b-throwaway-nixos-vm).

## Prerequisites

Nix with flakes enabled.
If flakes are not on globally:

```bash
export NIX_CONFIG="experimental-features = nix-command flakes"
```

## Option A: standalone Home Manager (recommended inside a VM)

This activates the module for your current user only.
It does not touch the system configuration, so it is the right choice when you are already inside a NixOS VM.

If your username is not `alice`, edit `homeConfigurations.alice` in `flake.nix` to set `home.username` and `home.homeDirectory` to your user (or copy the block under your own name).

Activate it:

```bash
nix run home-manager/master -- switch --flake .#alice
```

Check that the module generated the profiles:

```bash
dash0 config profiles list
```

You should see `prod` marked active and in the `oauth (not logged in)` state, `staging` likewise, and `ci` seeded with an empty token (the demo token file does not exist under standalone activation — a harmless warning is printed; see [Notes](#notes)).

Inspect the resolved active profile:

```bash
dash0 config show
```

Roll back the activation when you are done:

```bash
nix run home-manager/master -- generations
```

Then activate an earlier generation's `activate` script, or simply delete `~/.dash0` since this is a throwaway VM.

## Option B: throwaway NixOS VM

Use this from any host (including non-NixOS) when you want a disposable guest rather than activating into your current machine.

Build the VM runner and boot it:

```bash
nixos-rebuild build-vm --flake .#dash0vm
```

```bash
./result/bin/run-dash0vm-vm
```

The console auto-logs in as `alice`, and the static `ci` profile resolves a (dummy) token from `/etc/dash0/demo-token`.
Quit the VM with `Ctrl-a x`.

> [!NOTE]
> Do not run `nixos-rebuild build-vm` from *inside* a NixOS VM just to test this — that nests a VM in a VM. Use Option A instead.

## Integrate into the machine's system config

To test the NixOS-integrated path (the `home-manager.nixosModules.home-manager` route), add the module under `home-manager.users.<you>` in the machine's own configuration, importing `dash0-cli.homeManagerModules.default`, then apply it ephemerally:

```bash
sudo nixos-rebuild test --flake /etc/nixos#<host>
```

`test` activates without changing the boot default, so it reverts on the next reboot — convenient in a throwaway VM.

## Test against a branch or a local checkout

Both options accept `--override-input` to swap the CLI source.
Point it at a branch:

```bash
nix run home-manager/master -- switch --flake .#alice --override-input dash0-cli github:dash0hq/dash0-cli/<branch>
```

Or at a local checkout (the path is the repo root):

```bash
nix run home-manager/master -- switch --flake .#alice --override-input dash0-cli path:/path/to/dash0-cli
```

The same flag works on the `nixos-rebuild build-vm` command for Option B.

## Verify the activeProfile guard

Set `pruneUndeclared = true` and point `activeProfile` at a name you did not declare (for example `"typo"`) in `flake.nix`, then re-activate (Option A) or rebuild (Option B).
Evaluation should fail with the `programs.dash0.activeProfile … no such profile is declared` assertion, before anything is built.

## Notes

- OAuth `dash0 login` opens a browser and listens on a localhost callback, which is awkward in a headless VM — perform a real login on a desktop NixOS box.
  The token-preservation behavior across rebuilds is covered by the `hm-merge` flake check (`nix flake check github:dash0hq/dash0-cli`).
- The static `ci` profile points `authTokenFile` at `/etc/dash0/demo-token`, which only exists in the Option B VM.
  Under standalone activation (Option A) the file is absent, so the module seeds an empty token and prints a warning — expected for the example.
  For real deployments, set `authTokenFile` to a secret path managed by [sops-nix](https://github.com/Mic92/sops-nix) or [agenix](https://github.com/ryantm/agenix) so the token never lands in the world-readable Nix store.
