# Unit tests for nix/merge-profiles.jq, runnable as a flake check:
#   nix build .#checks.<system>.hm-merge
{ runCommand, jq }:
runCommand "dash0-hm-merge-test" { nativeBuildInputs = [ jq ]; } ''
  set -eu
  prog=${../merge-profiles.jq}
  pass=0; fail=0
  check() { # name expected actual
    if [ "$2" = "$3" ]; then
      echo "PASS: $1"; pass=$((pass + 1))
    else
      echo "FAIL: $1 (expected '$2', got '$3')" >&2; fail=$((fail + 1))
    fi
  }
  merge() { # declared tokens live prune
    jq -n --argjson declared "$1" --argjson tokens "$2" --argjson live "$3" --argjson prune "$4" -f "$prog"
  }

  # 1: static profile, token injected from file
  res=$(merge \
    '[{"name":"prod","apiUrl":"https://api.eu","otlpUrl":"https://ing.eu","dataset":"default","auth":"static","tokenFile":"/x"}]' \
    '{"prod":"auth_SECRET"}' '{"profiles":[]}' false)
  check "static.authToken"  "auth_SECRET"   "$(jq -r '.profiles[0].configuration.authToken' <<<"$res")"
  check "static.otlpUrl"    "https://ing.eu" "$(jq -r '.profiles[0].configuration.otlpUrl'   <<<"$res")"
  check "static.no-oauth"   "null"          "$(jq -c '.profiles[0].configuration.oauth'      <<<"$res")"

  # 2: oauth profile, first activation seeds oauth:{}
  res=$(merge \
    '[{"name":"prod","apiUrl":"https://api.eu","otlpUrl":null,"dataset":null,"auth":"oauth","tokenFile":null}]' \
    '{}' '{"profiles":[]}' false)
  check "oauth.seed.oauth-empty" "{}" "$(jq -c '.profiles[0].configuration.oauth'     <<<"$res")"
  check "oauth.seed.authToken"   ""   "$(jq -r '.profiles[0].configuration.authToken' <<<"$res")"

  # 3: oauth re-activation PRESERVES tokens minted by `dash0 login`
  live='{"profiles":[{"name":"prod","configuration":{"apiUrl":"https://api.eu","authToken":"dash0_at_LIVE","oauth":{"clientId":"cid","refreshToken":"rt_LIVE","expiresAt":"2026-01-01T00:00:00Z"}}}]}'
  res=$(merge \
    '[{"name":"prod","apiUrl":"https://api.eu-NEW","otlpUrl":null,"dataset":"ds2","auth":"oauth","tokenFile":null}]' \
    '{}' "$live" false)
  check "oauth.preserve.refreshToken" "rt_LIVE"       "$(jq -r '.profiles[0].configuration.oauth.refreshToken' <<<"$res")"
  check "oauth.preserve.accessToken"  "dash0_at_LIVE" "$(jq -r '.profiles[0].configuration.authToken'          <<<"$res")"
  check "oauth.update.apiUrl"         "https://api.eu-NEW" "$(jq -r '.profiles[0].configuration.apiUrl'        <<<"$res")"
  check "oauth.update.dataset"        "ds2"           "$(jq -r '.profiles[0].configuration.dataset'            <<<"$res")"

  # 4: undeclared live profile kept (prune=false) / dropped (prune=true)
  declared='[{"name":"prod","apiUrl":"https://api.eu","otlpUrl":null,"dataset":null,"auth":"oauth","tokenFile":null}]'
  live='{"profiles":[{"name":"prod","configuration":{"apiUrl":"x","authToken":"","oauth":{}}},{"name":"scratch","configuration":{"apiUrl":"y","authToken":"auth_z"}}]}'
  res=$(merge "$declared" '{}' "$live" false)
  check "prune-false.keeps-scratch" "1" "$(jq '[.profiles[]|select(.name=="scratch")]|length' <<<"$res")"
  res=$(merge "$declared" '{}' "$live" true)
  check "prune-true.drops-scratch"  "0" "$(jq '[.profiles[]|select(.name=="scratch")]|length' <<<"$res")"
  check "prune-true.keeps-prod"     "1" "$(jq '[.profiles[]|select(.name=="prod")]|length'    <<<"$res")"

  # 5: switching static->oauth drops stale oauth and uses the file token
  live='{"profiles":[{"name":"prod","configuration":{"apiUrl":"x","authToken":"old","oauth":{"clientId":"c","refreshToken":"r"}}}]}'
  res=$(merge \
    '[{"name":"prod","apiUrl":"https://api.eu","otlpUrl":null,"dataset":null,"auth":"static","tokenFile":"/x"}]' \
    '{"prod":"auth_NEW"}' "$live" false)
  check "switch.static-drops-oauth"  "null"     "$(jq -c '.profiles[0].configuration.oauth'      <<<"$res")"
  check "switch.static-uses-filetoken" "auth_NEW" "$(jq -r '.profiles[0].configuration.authToken' <<<"$res")"

  # 6: multiple profiles in one home all land, each independent
  res=$(merge \
    '[{"name":"prod","apiUrl":"https://api.eu","otlpUrl":null,"dataset":null,"auth":"static","tokenFile":"/p"},{"name":"staging","apiUrl":"https://api.us","otlpUrl":null,"dataset":"staging","auth":"static","tokenFile":"/s"},{"name":"eu","apiUrl":"https://api.eu","otlpUrl":null,"dataset":null,"auth":"oauth","tokenFile":null}]' \
    '{"prod":"auth_P","staging":"auth_S"}' '{"profiles":[]}' false)
  check "multi.count"          "3"      "$(jq '.profiles|length' <<<"$res")"
  check "multi.prod-token"     "auth_P" "$(jq -r '.profiles[]|select(.name=="prod").configuration.authToken'    <<<"$res")"
  check "multi.staging-token"  "auth_S" "$(jq -r '.profiles[]|select(.name=="staging").configuration.authToken' <<<"$res")"
  check "multi.eu-oauth-seed"  "{}"     "$(jq -c '.profiles[]|select(.name=="eu").configuration.oauth'          <<<"$res")"

  echo "----"
  echo "merge: $pass passed, $fail failed"
  [ "$fail" -eq 0 ] || exit 1
  touch "$out"
''
