package help

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// commandHelp is the JSON-serializable representation of a cobra command's
// help metadata.
type commandHelp struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Usage       string        `json:"usage"`
	Example     string        `json:"example,omitempty"`
	Aliases     []string      `json:"aliases,omitempty"`
	Flags       []flagHelp    `json:"flags,omitempty"`
	Subcommands []commandHelp `json:"subcommands,omitempty"`
}

// flagHelp describes a single CLI flag.
type flagHelp struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

// PrintJSONHelp writes the command's help information as structured JSON to w.
func PrintJSONHelp(w io.Writer, cmd *cobra.Command) error {
	h := buildHelp(cmd)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(h); err != nil {
		return fmt.Errorf("failed to encode help as JSON: %w", err)
	}
	return nil
}

func buildHelp(cmd *cobra.Command) commandHelp {
	h := commandHelp{
		Name:        cmd.Name(),
		Description: cmd.Short,
		Usage:       cmd.UseLine(),
		Example:     cmd.Example,
	}
	if len(cmd.Aliases) > 0 {
		h.Aliases = cmd.Aliases
	}

	// Collect flags.
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		h.Flags = append(h.Flags, flagHelp{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
		})
	})

	// Collect subcommands (excluding built-in help/completion).
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() && sub.Name() != "help" {
			continue
		}
		if sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		h.Subcommands = append(h.Subcommands, buildHelp(sub))
	}

	return h
}
