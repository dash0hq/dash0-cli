package config

import "context"

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// configContextKey is the context key for storing configuration
	configContextKey contextKey = "dash0-config"
)

// WithConfiguration returns a new context with the configuration stored
func WithConfiguration(ctx context.Context, cfg *Configuration) context.Context {
	return context.WithValue(ctx, configContextKey, cfg)
}

// FromContext retrieves the configuration from context, or nil if not present
func FromContext(ctx context.Context) *Configuration {
	cfg, _ := ctx.Value(configContextKey).(*Configuration)
	return cfg
}
