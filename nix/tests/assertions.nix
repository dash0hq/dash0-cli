# Unit tests for nix/assertions.nix, runnable as a flake check:
#   nix build .#checks.<system>.hm-assertions
{ lib, runCommand }:
let
  mkAssertions = import ../assertions.nix lib;

  mkCfg =
    {
      profiles ? { },
      activeProfile ? null,
      pruneUndeclared ? false,
    }:
    { inherit profiles activeProfile pruneUndeclared; };

  firstFailure = cfg: lib.findFirst (a: !a.assertion) null (mkAssertions cfg);
  allPass = cfg: firstFailure cfg == null;
  failMsg =
    cfg:
    let
      f = firstFailure cfg;
    in
    if f == null then "" else f.message;

  static = {
    apiUrl = "https://api.example";
    otlpUrl = null;
    dataset = null;
    auth = "static";
    authTokenFile = null;
  };
  oauth = static // {
    auth = "oauth";
  };
  oauthNoApi = oauth // {
    apiUrl = null;
  };
  oauthWithToken = oauth // {
    authTokenFile = "/run/secrets/x";
  };

  cases = [
    # activeProfile validation (the new assertion)
    {
      name = "active-declared-with-prune-passes";
      ok = allPass (mkCfg {
        profiles.prod = static;
        activeProfile = "prod";
        pruneUndeclared = true;
      });
    }
    {
      name = "active-undeclared-with-prune-fails";
      ok = !allPass (mkCfg {
        profiles.prod = static;
        activeProfile = "typo";
        pruneUndeclared = true;
      });
    }
    {
      name = "active-undeclared-without-prune-passes";
      ok = allPass (mkCfg {
        profiles.prod = static;
        activeProfile = "imperative";
        pruneUndeclared = false;
      });
    }
    {
      name = "active-null-with-prune-passes";
      ok = allPass (mkCfg {
        profiles.prod = static;
        activeProfile = null;
        pruneUndeclared = true;
      });
    }
    {
      name = "active-undeclared-message-names-pruneUndeclared";
      ok = lib.hasInfix "pruneUndeclared" (failMsg (mkCfg {
        profiles.prod = static;
        activeProfile = "typo";
        pruneUndeclared = true;
      }));
    }
    {
      name = "active-multiprofile-second-declared-passes";
      ok = allPass (mkCfg {
        profiles.prod = static;
        profiles.staging = static;
        activeProfile = "staging";
        pruneUndeclared = true;
      });
    }

    # pre-existing assertions still hold
    {
      name = "oauth-requires-apiurl-fails";
      ok = !allPass (mkCfg { profiles.x = oauthNoApi; });
    }
    {
      name = "oauth-with-tokenfile-fails";
      ok = !allPass (mkCfg { profiles.x = oauthWithToken; });
    }
    {
      name = "oauth-with-apiurl-passes";
      ok = allPass (mkCfg { profiles.x = oauth; });
    }
    {
      name = "static-passes";
      ok = allPass (mkCfg { profiles.x = static; });
    }
    {
      name = "empty-config-passes";
      ok = allPass (mkCfg { });
    }
  ];

  failures = builtins.filter (c: !c.ok) cases;
  report = lib.concatMapStringsSep "\n" (c: "${if c.ok then "PASS" else "FAIL"}: ${c.name}") cases;
in
runCommand "dash0-hm-assertions-test" { } ''
  echo "${report}"
  ${
    if failures == [ ] then
      ''
        echo "All ${toString (builtins.length cases)} assertion test cases passed."
        touch $out
      ''
    else
      ''
        echo "FAILED: ${lib.concatMapStringsSep ", " (c: c.name) failures}" >&2
        exit 1
      ''
  }
''
