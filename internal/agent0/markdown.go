package agent0

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// newMarkdownRenderer creates a glamour terminal markdown renderer.
// Returns nil if renderer creation fails; callers should fall back to raw text.
func newMarkdownRenderer(width int) *glamour.TermRenderer {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	return r
}

// renderMarkdown renders markdown content for terminal display.
// Falls back to raw content if the renderer is nil or rendering fails.
func renderMarkdown(renderer *glamour.TermRenderer, content string) string {
	if renderer == nil || content == "" {
		return content
	}
	out, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(out)
}
