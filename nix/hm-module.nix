# Home Manager module for the Dash0 CLI.
#
# Declares Dash0 profiles per-user under ~/.dash0 (or programs.dash0.configDir).
# Because the CLI rewrites profiles.json at runtime (OAuth refresh/login), the
# file is assembled by an activation script that *merges* the declared profiles
# into the live file rather than symlinking a read-only copy from the Nix store.
# See nix/merge-profiles.jq for the merge semantics.
{ self }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.dash0;

  profileModule = lib.types.submodule (
    { name, ... }:
    {
      options = {
        apiUrl = lib.mkOption {
          type = lib.types.nullOr lib.types.str;
          default = null;
          example = "https://api.eu-west-1.aws.dash0.com";
          description = "Dash0 API URL. Required when {option}`auth` is `\"oauth\"`.";
        };
        otlpUrl = lib.mkOption {
          type = lib.types.nullOr lib.types.str;
          default = null;
          example = "https://ingress.eu-west-1.aws.dash0.com";
          description = "Dash0 OTLP/HTTP ingress URL (used by the send commands).";
        };
        dataset = lib.mkOption {
          type = lib.types.nullOr lib.types.str;
          default = null;
          example = "default";
          description = "Default dataset identifier for this profile.";
        };
        auth = lib.mkOption {
          type = lib.types.enum [
            "static"
            "oauth"
          ];
          default = "static";
          description = ''
            Authentication mode for the profile.

            `static` uses a long-lived `auth_*` token supplied via
            {option}`authTokenFile`. `oauth` seeds an OAuth-empty profile; run
            `dash0 login --profile ${name}` once to obtain tokens. The tokens
            are preserved across `home-manager switch` runs.
          '';
        };
        authTokenFile = lib.mkOption {
          # A runtime path (e.g. an agenix/sops-nix secret), kept as a string so
          # the secret is never copied into the world-readable Nix store. The
          # contents are read at activation time.
          type = lib.types.nullOr lib.types.str;
          default = null;
          example = "/run/secrets/dash0-token";
          description = ''
            Path to a file containing the static `auth_*` token, read at
            activation time. Only valid when {option}`auth` is `"static"`.
          '';
        };
      };
    }
  );

  # Metadata only — never contains secrets, safe to place in the Nix store.
  declaredList = lib.mapAttrsToList (name: p: {
    inherit name;
    inherit (p) apiUrl otlpUrl dataset auth;
    tokenFile = p.authTokenFile;
  }) cfg.profiles;

  declaredFile = pkgs.writeText "dash0-declared-profiles.json" (builtins.toJSON declaredList);
  mergeProgram = ./merge-profiles.jq;
in
{
  options.programs.dash0 = {
    enable = lib.mkEnableOption "the Dash0 CLI with declarative profiles";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.dash0;
      defaultText = lib.literalExpression "dash0-cli.packages.\${system}.dash0";
      description = "The Dash0 CLI package to install.";
    };

    configDir = lib.mkOption {
      type = lib.types.str;
      default = "${config.home.homeDirectory}/.dash0";
      defaultText = lib.literalExpression "\"\${config.home.homeDirectory}/.dash0\"";
      description = ''
        Directory holding the CLI's configuration. When set to a non-default
        path, `DASH0_CONFIG_DIR` is exported so the CLI finds it.
      '';
    };

    activeProfile = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "prod";
      description = "Name of the profile to mark active (writes the activeProfile pointer).";
    };

    pruneUndeclared = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        When enabled, profiles present on disk but not declared here are removed
        on activation, making this module the sole authority over profiles.json.
        Leave disabled to keep profiles created imperatively (e.g. via
        `dash0 login` on an undeclared profile).
      '';
    };

    profiles = lib.mkOption {
      type = lib.types.attrsOf profileModule;
      default = { };
      description = "Dash0 profiles to manage declaratively, keyed by profile name.";
      example = lib.literalExpression ''
        {
          prod = {
            apiUrl  = "https://api.eu-west-1.aws.dash0.com";
            otlpUrl = "https://ingress.eu-west-1.aws.dash0.com";
            dataset = "default";
            auth    = "oauth";
          };
        }
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = import ./assertions.nix lib cfg;

    home.packages = [ cfg.package ];

    home.sessionVariables.DASH0_CONFIG_DIR = cfg.configDir;

    home.activation.dash0Profiles = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
      PATH="${lib.makeBinPath [ pkgs.jq pkgs.coreutils ]}:$PATH"

      configDir=${lib.escapeShellArg cfg.configDir}
      profilesFile="$configDir/profiles.json"
      declaredFile=${declaredFile}
      mergeProgram=${mergeProgram}

      $DRY_RUN_CMD mkdir -p "$configDir"
      $DRY_RUN_CMD chmod 700 "$configDir"

      # Load the live file; if it is missing or corrupt, start from empty and
      # back up the corrupt copy rather than discarding it silently.
      live='{"profiles":[]}'
      if [ -e "$profilesFile" ]; then
        if jq -e . "$profilesFile" >/dev/null 2>&1; then
          live=$(cat "$profilesFile")
        else
          $DRY_RUN_CMD cp "$profilesFile" "$profilesFile.corrupt.$(date +%s)" || true
          $VERBOSE_ECHO "dash0: existing profiles.json was not valid JSON; backed it up"
        fi
      fi

      # Read each static profile's token from its file at activation time so the
      # secret never enters the Nix store. Build a name -> token JSON object.
      tokens='{}'
      nprof=$(jq 'length' "$declaredFile")
      i=0
      while [ "$i" -lt "$nprof" ]; do
        pname=$(jq -r ".[$i].name" "$declaredFile")
        ptf=$(jq -r ".[$i].tokenFile // empty" "$declaredFile")
        if [ -n "$ptf" ]; then
          if [ -r "$ptf" ]; then
            ptok=$(tr -d '\n' < "$ptf")
            tokens=$(jq --arg n "$pname" --arg t "$ptok" '.[$n]=$t' <<<"$tokens")
          else
            echo "dash0: warning: authTokenFile for profile '$pname' is not readable: $ptf" >&2
          fi
        fi
        i=$((i + 1))
      done

      merged=$(jq -n \
        --argjson declared "$(cat "$declaredFile")" \
        --argjson tokens "$tokens" \
        --argjson live "$live" \
        --argjson prune ${if cfg.pruneUndeclared then "true" else "false"} \
        -f "$mergeProgram")

      # Atomic, owner-only write.
      tmp=$(mktemp "$configDir/.profiles.json.XXXXXX")
      printf '%s\n' "$merged" > "$tmp"
      chmod 600 "$tmp"
      $DRY_RUN_CMD mv "$tmp" "$profilesFile"

      ${lib.optionalString (cfg.activeProfile != null) ''
        printf '%s' ${lib.escapeShellArg cfg.activeProfile} > "$configDir/activeProfile"
        chmod 600 "$configDir/activeProfile"
      ''}
    '';
  };
}
