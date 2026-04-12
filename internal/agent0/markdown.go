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
	// Agent0 responses may contain custom XML-like tags (e.g. <Service>, <metric>)
	// that are not HTML. Glamour's HTML sanitizer (bluemonday) strips unknown tags.
	// Escape angle brackets so they render as literal text.
	content = escapeAngleBrackets(content)
	out, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(out)
}

// escapeAngleBrackets replaces < and > with HTML entities outside of markdown
// code spans and code blocks, so glamour doesn't strip them as HTML tags.
func escapeAngleBrackets(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	inCodeBlock := false
	inCodeSpan := false
	i := 0

	for i < len(s) {
		// Track fenced code blocks (```)
		if !inCodeSpan && i+2 < len(s) && s[i] == '`' && s[i+1] == '`' && s[i+2] == '`' {
			inCodeBlock = !inCodeBlock
			result.WriteString("```")
			i += 3
			continue
		}

		// Track inline code spans (`)
		if !inCodeBlock && s[i] == '`' {
			inCodeSpan = !inCodeSpan
			result.WriteByte('`')
			i++
			continue
		}

		// Only escape outside code
		if !inCodeBlock && !inCodeSpan {
			if s[i] == '<' {
				result.WriteString("&lt;")
				i++
				continue
			}
			if s[i] == '>' {
				result.WriteString("&gt;")
				i++
				continue
			}
		}

		result.WriteByte(s[i])
		i++
	}

	return result.String()
}
