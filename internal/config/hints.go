package config

import (
	"fmt"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
)

// FlagFragmentIfNotActive returns " --profile X" when X is non-empty AND X
// is NOT the currently active profile (per ~/.dash0/profiles.json). It
// returns "" otherwise, including when X is empty. Use this in follow-up
// hints emitted by commands like `profiles create` and `config show` where
// the hint text might point at a profile that is already (or just became)
// active, in which case `--profile X` is redundant.
//
// Lookup errors bias toward eliding the fragment — better to print an
// ambiguous short hint than a confidently-wrong long one.
func FlagFragmentIfNotActive(profileName string) string {
	if profileName == "" {
		return ""
	}
	store, err := profiles.NewStore()
	if err != nil {
		return ""
	}
	active, err := store.GetActiveProfile()
	if err != nil || active == nil {
		return ""
	}
	if active.Name == profileName {
		return ""
	}
	return fmt.Sprintf(" --profile %s", profileName)
}

// ProfileDisplayName turns a profile-scoping hint into a noun phrase for
// error messages and prompts. Pass "" when the operation is implicitly
// acting on the active profile (no --profile was passed) to get "the
// active profile"; pass the actual profile name when the user is
// addressing a profile explicitly.
//
//	ProfileDisplayName("")      // "the active profile"
//	ProfileDisplayName("prod")  // `profile "prod"`
//
// The helper deliberately does not consult the on-disk active profile —
// the caller knows whether --profile was passed and is the right place to
// make that choice.
func ProfileDisplayName(profileName string) string {
	if profileName == "" {
		return "the active profile"
	}
	return fmt.Sprintf("profile %q", profileName)
}

// OAuthAuthenticateHint returns the follow-up hint printed after marking a
// profile as OAuth via `profiles create --oauth` or `profiles update
// --oauth`. In human mode it points at `dash0 login`. In agent mode it
// points at the escape hatches that an agent can actually execute
// (`DASH0_AUTH_TOKEN` or `profiles update --oauth=false --auth-token ...
// --force`), because `dash0 login` itself refuses to run in agent mode.
func OAuthAuthenticateHint(profileName string) string {
	if agentmode.Enabled {
		return fmt.Sprintf(
			"Hint: `dash0 login` cannot run in agent mode. Set DASH0_AUTH_TOKEN to a static `auth_*` token, or convert back to a static profile with `dash0 config profiles update %s --oauth=false --auth-token auth_<...> --force`.",
			profileName,
		)
	}
	return fmt.Sprintf("Hint: Run `dash0 login%s` to authenticate.", FlagFragmentIfNotActive(profileName))
}

// ProfileFlagFragment returns the " --profile X" fragment to splice into a
// command hint like `dash0 login`. Pass "" when the hint should NOT mention
// a profile (because the user did not pass --profile); pass the actual
// profile name to splice it in.
//
//	fmt.Sprintf("Run `dash0 login%s`.", ProfileFlagFragment(""))      // Run `dash0 login`.
//	fmt.Sprintf("Run `dash0 login%s`.", ProfileFlagFragment("prod"))  // Run `dash0 login --profile prod`.
//
// Like ProfileDisplayName, this helper trusts the caller's choice and does
// not consult the on-disk active profile.
func ProfileFlagFragment(profileName string) string {
	if profileName == "" {
		return ""
	}
	return fmt.Sprintf(" --profile %s", profileName)
}
