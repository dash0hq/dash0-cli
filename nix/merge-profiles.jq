# Merge declared Dash0 profiles into the live profiles.json.
#
# The Dash0 CLI rewrites profiles.json at runtime (OAuth token refresh, login,
# logout), so Home Manager cannot own the file outright. This program upserts
# the declaratively managed profiles while preserving runtime-acquired OAuth
# state (client id, refresh token, access token, expiry) for profiles the user
# has logged into, so `home-manager switch` never logs them out.
#
# Parameters (all via --argjson):
#   $declared : array of { name, apiUrl, otlpUrl, dataset, auth, tokenFile }
#               (auth is "static" or "oauth"; tokenFile is informational only)
#   $tokens   : object mapping profile name -> static auth token (read from
#               authTokenFile at activation time; never stored in the Nix store)
#   $live     : the current profiles.json contents ({ profiles: [...] })
#   $prune    : bool; when true, drop live profiles not present in $declared
#
# Field presence mirrors the Go struct tags in dash0-api-client-go/profiles:
# apiUrl and authToken are always emitted; otlpUrl, dataset, and oauth are
# omitempty.

def cleanConf(c):
  { apiUrl: (c.apiUrl // ""), authToken: (c.authToken // "") }
  + (if (c.otlpUrl // "") != "" then { otlpUrl: c.otlpUrl } else {} end)
  + (if (c.dataset // "") != "" then { dataset: c.dataset } else {} end)
  + (if c.oauth != null then { oauth: c.oauth } else {} end);

($live.profiles // []) as $livePs
| reduce $declared[] as $d (
    { profiles: $livePs };
    ($d.name) as $name
    | ( .profiles | map(select(.name == $name)) | .[0] ) as $cur
    | (
        if $d.auth == "oauth"
        then
          { apiUrl: ($d.apiUrl // ""), otlpUrl: $d.otlpUrl, dataset: $d.dataset }
          | if ($cur != null and ($cur.configuration.oauth != null))
            then
              # Preserve the tokens the CLI obtained via `dash0 login`.
              .oauth = $cur.configuration.oauth
              | .authToken = ($cur.configuration.authToken // "")
            else
              # Seed the OAuth-empty state; `dash0 login` fills it in later.
              .oauth = {}
              | .authToken = ""
            end
        else
          { apiUrl: ($d.apiUrl // ""),
            otlpUrl: $d.otlpUrl,
            dataset: $d.dataset,
            authToken: ($tokens[$name] // "") }
        end
      ) as $conf
    | .profiles = ( .profiles | map(select(.name != $name)) )
                  + [ { name: $name, configuration: cleanConf($conf) } ]
  )
| if $prune
  then .profiles |= map(select(.name as $n | ($declared | map(.name) | index($n)) != null))
  else .
  end
