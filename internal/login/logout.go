package login

import (
	"context"
	"errors"
	"fmt"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/oauth"
)

type logoutOptions struct {
	ProfileName string
	Force       bool
}

// runLogout revokes the refresh token of an OAuth profile and clears its
// auth state. The profile itself stays around so subsequent `dash0 login`
// can re-fill it.
//
// Refuses to operate on a static-token profile (the user should use
// `dash0 config profiles delete` for that). Refuses if there is no profile
// to act on.
func runLogout(ctx context.Context, opts logoutOptions) error {
	// Agent mode normally treats ConfirmDestructiveOperation as auto-confirm
	// (documented "agent mode == --force" for asset deletes). Logout is a
	// special case: it irreversibly revokes a refresh token server-side
	// and rips the active session out from under the user, mirroring what
	// `dash0 login` exists to set up. To keep agent-driven invocations from
	// silently destroying sessions just because an AI-agent env var
	// (CLAUDE_CODE, CURSOR_AGENT, etc.) is set, require an explicit --force
	// when running under agent mode. This mirrors `dash0 login`'s blanket
	// refusal to run in agent mode at all: both directions of the OAuth
	// session transition now require deliberate user intent.
	if agentmode.Enabled && !opts.Force {
		displayName := ""
		if opts.ProfileName != "" {
			displayName = opts.ProfileName
		}
		return fmt.Errorf(
			"dash0 logout requires --force when run in agent mode\n"+
				"Hint: re-run with `dash0 logout%s --force` to confirm revocation of the OAuth refresh token.",
			config.ProfileFlagFragment(displayName),
		)
	}

	store, err := profiles.NewStore()
	if err != nil {
		return fmt.Errorf("failed to open profile store: %w", err)
	}

	// Resolve the target profile.
	var (
		name           string
		passedExplicit = opts.ProfileName != ""
	)
	if passedExplicit {
		name = opts.ProfileName
	} else {
		active, err := store.GetActiveProfile()
		if err != nil {
			if errors.Is(err, profiles.ErrNoActiveProfile) {
				return errors.New("no active profile to log out of; pass --profile <name>")
			}
			return fmt.Errorf("failed to determine active profile: %w", err)
		}
		if active == nil {
			// Defensive: GetActiveProfile is not contracted to return (nil,
			// nil), but the alternative is a nil-pointer panic if the SDK
			// ever changes behavior. Surface the same message as the
			// documented ErrNoActiveProfile path.
			return errors.New("no active profile to log out of; pass --profile <name>")
		}
		name = active.Name
	}

	// Inspect the profile.
	cfg, err := loadProfileForLogout(store, name, passedExplicit)
	if err != nil {
		return err
	}

	switch {
	case cfg.OAuth == nil:
		// Static-token profile: refuse and steer toward `profiles delete`.
		displayName := ""
		if passedExplicit {
			displayName = name
		}
		return fmt.Errorf(
			"%s is not an OAuth profile; nothing to log out of.\nHint: Use `dash0 config profiles delete %s` to remove it.",
			config.ProfileDisplayName(displayName),
			name,
		)
	case cfg.OAuth.RefreshToken == "":
		// Already-logged-out OAuth-empty profile. Some prior state may have
		// left a stale access token on disk (interrupted refresh, manual
		// edit); clear it so the branch is idempotent.
		clearedStale := false
		if cfg.AuthToken != "" {
			if err := store.UpdateProfile(name, func(c *profiles.Configuration) {
				c.AuthToken = ""
			}); err != nil {
				return fmt.Errorf("failed to clear stale access token on profile %q: %w", name, err)
			}
			clearedStale = true
		}
		if !passedExplicit {
			fmt.Println("You are already logged out.")
		} else {
			fmt.Printf("Profile %q is already logged out.\n", name)
		}
		if clearedStale {
			// The stale access token was orphaned (no refresh token to
			// pair with) so it was almost certainly already invalid, but
			// be explicit about what changed instead of silently mutating
			// the profile.
			fmt.Println("Note: cleared a stale access token from the profile (no corresponding refresh token was present, so no server-side revocation was attempted).")
		}
		return nil
	}

	prompt := fmt.Sprintf("Log out of %s (revoking its OAuth refresh token)? [y/N]: ", subjectPhraseFor(name, passedExplicit))
	ok, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, opts.Force)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("aborted by user")
	}

	// Best-effort revocation before we clear the in-memory copy on disk.
	revoked := oauth.Revoke(cfg.ApiUrl, cfg.OAuth.RefreshToken)

	if err := store.UpdateProfile(name, func(c *profiles.Configuration) {
		c.AuthToken = ""
		// Keep the OAuth marker so the profile stays OAuth-typed; clear the
		// token-bearing fields.
		c.OAuth = &profiles.OAuthState{}
	}); err != nil {
		return fmt.Errorf("failed to clear OAuth state on profile %q: %w", name, err)
	}

	if passedExplicit {
		fmt.Printf("Logged out of profile %q.\n", name)
	} else {
		fmt.Println("Logged out.")
	}
	if !revoked {
		// Local state is cleared either way, but be explicit: the
		// authorization server still sees the refresh token as active.
		fmt.Println("Note: server-side refresh-token revocation failed; the token will remain valid on the authorization server until natural expiry.")
	}
	return nil
}

// loadProfileForLogout returns a copy of the named profile's configuration.
// It produces a clear error referencing the profile name when missing.
func loadProfileForLogout(store *profiles.Store, name string, passedExplicit bool) (profiles.Configuration, error) {
	all, err := store.GetProfiles()
	if err != nil {
		return profiles.Configuration{}, fmt.Errorf("failed to read profiles: %w", err)
	}
	for _, p := range all {
		if p.Name == name {
			return p.Configuration, nil
		}
	}
	if passedExplicit {
		return profiles.Configuration{}, fmt.Errorf("profile %q does not exist", name)
	}
	return profiles.Configuration{}, fmt.Errorf("the active profile %q no longer exists", name)
}

