package config

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dash0hq/dash0-api-client-go/profiles"
)

// EnvProfile is the environment variable that selects a named profile for the
// current invocation, overriding the active profile recorded on disk.
const EnvProfile = "DASH0_PROFILE"

// ProfileSource identifies where an explicit profile selection came from.
type ProfileSource int

const (
	// ProfileSourceNone indicates no explicit selection (the active profile is used).
	ProfileSourceNone ProfileSource = iota
	// ProfileSourceFlag indicates the selection came from the --profile flag.
	ProfileSourceFlag
	// ProfileSourceEnv indicates the selection came from the DASH0_PROFILE env var.
	ProfileSourceEnv
)

// Description returns a human-readable source description suitable for
// displaying in `config show` alongside a profile name.
func (s ProfileSource) Description() string {
	switch s {
	case ProfileSourceFlag:
		return "from --profile flag"
	case ProfileSourceEnv:
		return "from " + EnvProfile + " environment variable"
	default:
		return ""
	}
}

// ProfileSelector records an explicit profile selection and its source.
type ProfileSelector struct {
	// Name is the explicit profile name. An empty string means the selector
	// is absent and callers should use the active profile.
	Name string
	// Source identifies where the Name came from.
	Source ProfileSource
}

// IsSet reports whether the selector has a non-empty explicit name.
func (s ProfileSelector) IsSet() bool {
	return s.Name != ""
}

// ResolveProfileSelector returns the explicit profile selector derived from
// the --profile flag value and the DASH0_PROFILE environment variable.
// Empty strings are treated as "not set" and fall through to the next
// precedence step, which means the caller should use the active profile.
func ResolveProfileSelector(flagValue string) ProfileSelector {
	if flagValue != "" {
		return ProfileSelector{Name: flagValue, Source: ProfileSourceFlag}
	}
	if envValue := os.Getenv(EnvProfile); envValue != "" {
		return ProfileSelector{Name: envValue, Source: ProfileSourceEnv}
	}
	return ProfileSelector{}
}

type profileSelectorContextKey struct{}

// WithProfileSelector returns a new context carrying the given selector.
func WithProfileSelector(ctx context.Context, sel ProfileSelector) context.Context {
	return context.WithValue(ctx, profileSelectorContextKey{}, sel)
}

// ProfileSelectorFromContext returns the selector stored in ctx, or an empty
// selector if none is present.
func ProfileSelectorFromContext(ctx context.Context) ProfileSelector {
	sel, _ := ctx.Value(profileSelectorContextKey{}).(ProfileSelector)
	return sel
}

// ResolveConfigurationForProfile loads the named profile from the store and
// applies environment variable overrides on top.
// It returns a "profile not found" error that lists the available profiles
// when no profile with the given name exists.
// When profileName is empty, callers should use the normal active-profile
// resolution chain; this function only handles explicit selections.
func ResolveConfigurationForProfile(profileName string) (*profiles.Configuration, error) {
	if profileName == "" {
		return nil, fmt.Errorf("profile name must not be empty")
	}

	store, err := profiles.NewStore()
	if err != nil {
		return nil, err
	}

	all, err := store.GetProfiles()
	if err != nil {
		return nil, err
	}

	if len(all) == 0 {
		return nil, fmt.Errorf(
			"profile %q does not exist; no profiles are configured.\nHint: create one with 'dash0 config profiles create'",
			profileName,
		)
	}

	var matched *profiles.Profile
	names := make([]string, 0, len(all))
	for i := range all {
		names = append(names, all[i].Name)
		if all[i].Name == profileName {
			matched = &all[i]
		}
	}

	if matched == nil {
		sort.Strings(names)
		return nil, fmt.Errorf(
			"profile %q does not exist. Available profiles: %s",
			profileName,
			strings.Join(names, ", "),
		)
	}

	cfg := matched.Configuration

	if v := os.Getenv(profiles.EnvApiUrl); v != "" {
		cfg.ApiUrl = v
	}
	if v := os.Getenv(profiles.EnvAuthToken); v != "" {
		cfg.AuthToken = v
	}
	if v := os.Getenv(profiles.EnvOtlpUrl); v != "" {
		cfg.OtlpUrl = v
	}
	if v := os.Getenv(profiles.EnvDataset); v != "" {
		cfg.Dataset = v
	}

	return &cfg, nil
}
