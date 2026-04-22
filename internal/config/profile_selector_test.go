package config

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-api-client-go/profiles"
)

func TestResolveProfileSelector(t *testing.T) {
	// Clean up env var leakage between cases.
	original, originalSet := os.LookupEnv(EnvProfile)
	t.Cleanup(func() {
		if originalSet {
			os.Setenv(EnvProfile, original)
		} else {
			os.Unsetenv(EnvProfile)
		}
	})

	t.Run("flag takes precedence over env var", func(t *testing.T) {
		os.Setenv(EnvProfile, "from-env")
		defer os.Unsetenv(EnvProfile)

		sel := ResolveProfileSelector("from-flag")
		if !sel.IsSet() {
			t.Fatal("selector should be set")
		}
		if sel.Name != "from-flag" {
			t.Errorf("expected name 'from-flag', got %q", sel.Name)
		}
		if sel.Source != ProfileSourceFlag {
			t.Errorf("expected source flag, got %v", sel.Source)
		}
	})

	t.Run("env var used when flag is empty", func(t *testing.T) {
		os.Setenv(EnvProfile, "from-env")
		defer os.Unsetenv(EnvProfile)

		sel := ResolveProfileSelector("")
		if !sel.IsSet() {
			t.Fatal("selector should be set")
		}
		if sel.Name != "from-env" {
			t.Errorf("expected name 'from-env', got %q", sel.Name)
		}
		if sel.Source != ProfileSourceEnv {
			t.Errorf("expected source env, got %v", sel.Source)
		}
	})

	t.Run("empty flag and empty env var fall through", func(t *testing.T) {
		os.Unsetenv(EnvProfile)

		sel := ResolveProfileSelector("")
		if sel.IsSet() {
			t.Errorf("selector should not be set, got %+v", sel)
		}
		if sel.Source != ProfileSourceNone {
			t.Errorf("expected source none, got %v", sel.Source)
		}
	})

	t.Run("empty env var falls through", func(t *testing.T) {
		os.Setenv(EnvProfile, "")
		defer os.Unsetenv(EnvProfile)

		sel := ResolveProfileSelector("")
		if sel.IsSet() {
			t.Errorf("selector should not be set, got %+v", sel)
		}
	})
}

func TestProfileSelectorContextRoundtrip(t *testing.T) {
	sel := ProfileSelector{Name: "prod", Source: ProfileSourceFlag}
	ctx := WithProfileSelector(context.Background(), sel)

	got := ProfileSelectorFromContext(ctx)
	if got != sel {
		t.Errorf("roundtrip mismatch: want %+v, got %+v", sel, got)
	}

	empty := ProfileSelectorFromContext(context.Background())
	if empty.IsSet() {
		t.Errorf("empty context should return zero-value selector, got %+v", empty)
	}
}

func TestProfileSourceDescription(t *testing.T) {
	cases := []struct {
		source ProfileSource
		want   string
	}{
		{ProfileSourceNone, ""},
		{ProfileSourceFlag, "from --profile flag"},
		{ProfileSourceEnv, "from DASH0_PROFILE environment variable"},
	}
	for _, c := range cases {
		if got := c.source.Description(); got != c.want {
			t.Errorf("Description(%v) = %q, want %q", c.source, got, c.want)
		}
	}
}

func TestResolveConfigurationForProfile(t *testing.T) {
	configDir := setupTestConfigDir(t)

	testProfiles := []profiles.Profile{
		{
			Name: "dev",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://api-dev.example.com",
				AuthToken: "dev_auth_token",
				OtlpUrl:   "https://otlp-dev.example.com",
				Dataset:   "dev-ds",
			},
		},
		{
			Name: "prod",
			Configuration: profiles.Configuration{
				ApiUrl:    "https://api-prod.example.com",
				AuthToken: "prod_auth_token",
				Dataset:   "prod-ds",
			},
		},
	}
	createTestProfilesFile(t, configDir, testProfiles)
	setActiveProfile(t, configDir, "dev")

	t.Run("selected profile is used", func(t *testing.T) {
		cfg, err := ResolveConfigurationForProfile("prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ApiUrl != "https://api-prod.example.com" {
			t.Errorf("expected prod api url, got %q", cfg.ApiUrl)
		}
		if cfg.Dataset != "prod-ds" {
			t.Errorf("expected prod dataset, got %q", cfg.Dataset)
		}
	})

	t.Run("env vars override individual fields", func(t *testing.T) {
		os.Setenv(profiles.EnvDataset, "env-override-ds")
		defer os.Unsetenv(profiles.EnvDataset)

		cfg, err := ResolveConfigurationForProfile("prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Dataset != "env-override-ds" {
			t.Errorf("expected env override, got %q", cfg.Dataset)
		}
		// Other fields should still come from the profile.
		if cfg.ApiUrl != "https://api-prod.example.com" {
			t.Errorf("expected prod api url, got %q", cfg.ApiUrl)
		}
	})

	t.Run("unknown profile returns error listing available profiles", func(t *testing.T) {
		_, err := ResolveConfigurationForProfile("typo")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, `profile "typo" does not exist`) {
			t.Errorf("expected profile-not-exist error, got %q", msg)
		}
		if !strings.Contains(msg, "dev") || !strings.Contains(msg, "prod") {
			t.Errorf("expected available profiles in error, got %q", msg)
		}
	})

	t.Run("empty name is rejected", func(t *testing.T) {
		_, err := ResolveConfigurationForProfile("")
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})
}

func TestResolveConfigurationForProfile_NoProfilesAtAll(t *testing.T) {
	_ = setupTestConfigDir(t)

	_, err := ResolveConfigurationForProfile("any")
	if err == nil {
		t.Fatal("expected error when no profiles are configured")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no profiles are configured") {
		t.Errorf("expected 'no profiles are configured' in error, got %q", msg)
	}
	if !strings.Contains(msg, "dash0 config profiles create") {
		t.Errorf("expected hint in error, got %q", msg)
	}
}
