{
  description = "Dash0 CLI — manage Dash0 assets and telemetry from the terminal, CI/CD, and AI agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    let
      # Keep this in sync with the latest release tag. It feeds the `-X
      # main.version` ldflag so `dash0 version` reports the right number.
      version = "1.15.0";

      # A reproducible build date derived from the flake's last-modified time
      # (when built from a clean tree) so the output is deterministic.
      date =
        let
          t = self.lastModifiedDate or "19700101000000";
        in
        "${builtins.substring 0 4 t}-${builtins.substring 4 2 t}-${builtins.substring 6 2 t}T${builtins.substring 8 2 t}:${builtins.substring 10 2 t}:${builtins.substring 12 2 t}Z";
    in
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        dash0 = pkgs.callPackage ./nix/package.nix {
          inherit version date;
        };
      in
      {
        packages = {
          default = dash0;
          dash0 = dash0;
        };

        apps.default = {
          type = "app";
          program = "${dash0}/bin/dash0";
        };

        # `nix flake check` — unit tests for the Home Manager module's pure
        # logic (profile assertions and the seed-merge jq program).
        checks = {
          hm-assertions = pkgs.callPackage ./nix/tests/assertions.nix { };
          hm-merge = pkgs.callPackage ./nix/tests/merge.nix { };
        };

        # `nix develop` — a shell with the Go toolchain and the project's
        # lint/changelog tooling, matching what the Makefile expects.
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go_1_25
            pkgs.gopls
            pkgs.gotools
            pkgs.golangci-lint
            pkgs.shellcheck
          ];
        };

        formatter = pkgs.nixfmt-rfc-style;
      }
    )
    // {
      # Overlay so downstreams can pull `dash0` into their own nixpkgs instance:
      #   nixpkgs.overlays = [ dash0-cli.overlays.default ];
      overlays.default = final: _prev: {
        dash0 = final.callPackage ./nix/package.nix { };
      };

      # Home Manager module for declarative per-user profiles:
      #   imports = [ dash0-cli.homeManagerModules.default ];
      homeManagerModules.default = import ./nix/hm-module.nix { inherit self; };
      homeManagerModules.dash0 = self.homeManagerModules.default;
    };
}
