package resource

import "github.com/spf13/cobra"

// CommonFlags holds common flag values used across all resource commands
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
	cmd.Flags().StringVarP(&flags.Dataset, "dataset", "d", "", "Dataset to operate on")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "table", "Output format: table, json, yaml, wide")
}

// ListFlags holds flags specific to list commands
type ListFlags struct {
	CommonFlags
	Limit int
	All   bool
}

// RegisterListFlags adds list-specific flags to a command
func RegisterListFlags(cmd *cobra.Command, flags *ListFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().IntVarP(&flags.Limit, "limit", "l", 50, "Maximum number of results to return")
	cmd.Flags().BoolVarP(&flags.All, "all", "a", false, "Fetch all pages (ignore limit)")
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

// RegisterFileInputFlags adds file input flags to a command
func RegisterFileInputFlags(cmd *cobra.Command, flags *FileInputFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file")
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

// ExportFlags holds flags specific to export commands
type ExportFlags struct {
	CommonFlags
	File string
}

// RegisterExportFlags adds export-specific flags to a command
func RegisterExportFlags(cmd *cobra.Command, flags *ExportFlags) {
	RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Output file path (stdout if not specified)")
}
