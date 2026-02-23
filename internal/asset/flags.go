package asset

import "github.com/spf13/cobra"

// CommonFlags holds common flag values used across all asset commands
type CommonFlags struct {
	ApiUrl    string
	AuthToken string
	Dataset   string
	Output    string
}

// RegisterCommonFlags adds common flags to a command
func RegisterCommonFlags(cmd *cobra.Command, flags *CommonFlags) {
	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset to operate on")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "table", "Output format: table, wide, json, yaml, csv")
}

// ListFlags holds flags specific to list commands
type ListFlags struct {
	CommonFlags
	Limit      int
	All        bool
	SkipHeader bool
}

// RegisterListFlags adds list-specific flags to a command
func RegisterListFlags(cmd *cobra.Command, flags *ListFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().IntVarP(&flags.Limit, "limit", "l", 50, "Maximum number of results to return")
	cmd.Flags().BoolVarP(&flags.All, "all", "a", false, "Fetch all pages (ignore limit)")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table, wide, and csv output")
}

// GetFlags holds flags specific to get commands
type GetFlags struct {
	CommonFlags
}

// RegisterGetFlags adds get-specific flags to a command
func RegisterGetFlags(cmd *cobra.Command, flags *GetFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
}

// FileInputFlags holds flags for file-based input operations (create, update, apply)
type FileInputFlags struct {
	CommonFlags
	File   string
	DryRun bool
}

// RegisterFileInputFlags adds file input flags to a command and marks -f as required
func RegisterFileInputFlags(cmd *cobra.Command, flags *FileInputFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without creating/updating")
	cmd.MarkFlagRequired("file")
}

// DeleteFlags holds flags specific to delete commands
type DeleteFlags struct {
	CommonFlags
	Force bool
}

// RegisterDeleteFlags adds delete-specific flags to a command
func RegisterDeleteFlags(cmd *cobra.Command, flags *DeleteFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")
}
