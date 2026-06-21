# Example: exercise the Dash0 Home Manager module.
#
# Two ways to use it (see ./README.md):
#   - homeConfigurations.alice — activate the module for your user with
#     standalone Home Manager. Best when you are already on a NixOS machine or
#     inside a NixOS VM: it writes ~/.dash0 with no root and no system rebuild.
#   - nixosConfigurations.dash0vm — boot a fresh throwaway NixOS guest with the
#     module active, when you want a disposable machine from any host.
#
# By default this pulls the CLI from the published repository. To test a branch
# or a local checkout, override the input, e.g.:
#   --override-input dash0-cli github:dash0hq/dash0-cli/<branch>
#   --override-input dash0-cli path:/path/to/dash0-cli
{
  description = "Example exercising the Dash0 Home Manager module";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager.url = "github:nix-community/home-manager";
    home-manager.inputs.nixpkgs.follows = "nixpkgs";
    dash0-cli.url = "github:dash0hq/dash0-cli";
  };

  outputs =
    {
      nixpkgs,
      home-manager,
      dash0-cli,
      ...
    }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};

      # Shared module: the part that is identical whether the module is loaded
      # standalone (homeConfigurations) or via the NixOS integration
      # (nixosConfigurations). It deliberately omits home.username /
      # home.homeDirectory, which only the standalone path needs to set.
      dash0HomeModule = {
        imports = [ dash0-cli.homeManagerModules.default ];
        home.stateVersion = "24.11";

        programs.dash0 = {
          enable = true;
          activeProfile = "prod";

          # Two OAuth profiles (seeded; run `dash0 login` to fill them) and one
          # static profile reading its token from a file. The static token file
          # only exists in the nixosConfigurations variant below; under
          # standalone Home Manager it is absent, so the `ci` profile is seeded
          # with an empty token (a harmless warning is printed).
          profiles.prod = {
            apiUrl = "https://api.eu-west-1.aws.dash0.com";
            otlpUrl = "https://ingress.eu-west-1.aws.dash0.com";
            dataset = "default";
            auth = "oauth";
          };
          profiles.staging = {
            apiUrl = "https://api.us-west-2.aws.dash0.com";
            auth = "oauth";
          };
          profiles.ci = {
            apiUrl = "https://api.us-west-2.aws.dash0.com";
            auth = "static";
            authTokenFile = "/etc/dash0/demo-token";
          };
        };
      };
    in
    {
      # Standalone Home Manager. Change "alice" and the username/homeDirectory
      # below to match the user you are testing as, then:
      #   nix run home-manager/master -- switch --flake .#alice
      homeConfigurations.alice = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [
          dash0HomeModule
          {
            home.username = "alice";
            home.homeDirectory = "/home/alice";
          }
        ];
      };

      # Fresh throwaway NixOS guest:
      #   nixos-rebuild build-vm --flake .#dash0vm && ./result/bin/run-dash0vm-vm
      nixosConfigurations.dash0vm = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          home-manager.nixosModules.home-manager
          (
            { ... }:
            {
              system.stateVersion = "24.11";

              # Auto-login on the VM console so you land in a shell that can run
              # the CLI immediately.
              services.getty.autologinUser = "alice";
              users.users.alice = {
                isNormalUser = true;
                initialPassword = "test";
              };

              # A demo static token. This is intentionally a world-readable file
              # for the example — do NOT do this for real secrets; use a
              # sops-nix/agenix secret path for authTokenFile instead.
              environment.etc."dash0/demo-token".text = "auth_demo_not_a_real_token";

              home-manager.users.alice = dash0HomeModule;
            }
          )
        ];
      };
    };
}
