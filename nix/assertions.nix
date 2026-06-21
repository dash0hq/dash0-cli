# Pure assertion list for the Dash0 Home Manager module.
#
# Factored out of nix/hm-module.nix so the validation logic can be unit-tested
# without standing up the whole Home Manager module system. See nix/tests/.
#
# Signature: lib -> cfg -> [ { assertion = <bool>; message = <string>; } ]
# where cfg has the fields { profiles, activeProfile, pruneUndeclared } and each
# profile has { apiUrl, auth, authTokenFile, ... }.
lib: cfg:
(lib.mapAttrsToList (name: p: {
  assertion = !(p.auth == "oauth" && p.authTokenFile != null);
  message = "programs.dash0.profiles.${name}: authTokenFile cannot be set when auth = \"oauth\".";
}) cfg.profiles)
++ (lib.mapAttrsToList (name: p: {
  assertion = (p.auth != "oauth") || (p.apiUrl != null && p.apiUrl != "");
  message = "programs.dash0.profiles.${name}: apiUrl is required when auth = \"oauth\" (needed by `dash0 login`).";
}) cfg.profiles)
++ [
  {
    # With pruning on, the module owns profiles.json outright, so an
    # activeProfile that is not declared here would be removed on activation,
    # leaving the CLI pointed at a profile that does not exist. With pruning
    # off the active profile may legitimately be one created imperatively via
    # `dash0 login`, so the check only applies when pruneUndeclared is set.
    assertion =
      !(cfg.pruneUndeclared && cfg.activeProfile != null)
      || builtins.hasAttr cfg.activeProfile cfg.profiles;
    message = "programs.dash0.activeProfile is set to \"${toString cfg.activeProfile}\" but no such profile is declared under programs.dash0.profiles; with pruneUndeclared = true it would be removed on activation. Declare it, or set pruneUndeclared = false.";
  }
]
