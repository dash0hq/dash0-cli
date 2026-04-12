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
	// Replace agent0 custom tags (e.g. <Service>, <TimeRange>) with formatted text
	// before markdown rendering. Must run before escapeAngleBrackets.
	content = formatTags(content)
	// Escape remaining angle brackets that glamour would strip as HTML.
	content = escapeAngleBrackets(content)
	out, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(out)
}


// escapeAngleBrackets replaces < and > with HTML entities outside of markdown
// code spans and code blocks, so glamour doesn't strip them as HTML tags.
// Preserves > at the start of a line (markdown blockquote syntax).
func escapeAngleBrackets(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	inCodeBlock := false
	inCodeSpan := false
	startOfLine := true
	i := 0

	for i < len(s) {
		// Track fenced code blocks (```)
		if !inCodeSpan && i+2 < len(s) && s[i] == '`' && s[i+1] == '`' && s[i+2] == '`' {
			inCodeBlock = !inCodeBlock
			result.WriteString("```")
			i += 3
			startOfLine = false
			continue
		}

		// Track inline code spans (`)
		if !inCodeBlock && s[i] == '`' {
			inCodeSpan = !inCodeSpan
			result.WriteByte('`')
			i++
			startOfLine = false
			continue
		}

		if s[i] == '\n' {
			result.WriteByte('\n')
			i++
			startOfLine = true
			continue
		}

		// Only escape outside code
		if !inCodeBlock && !inCodeSpan {
			if s[i] == '<' {
				result.WriteString("&lt;")
				i++
				startOfLine = false
				continue
			}
			// Preserve > at start of line (blockquote) or after > (nested blockquote)
			if s[i] == '>' && !startOfLine {
				result.WriteString("&gt;")
				i++
				continue
			}
		}

		if s[i] != ' ' && s[i] != '>' {
			startOfLine = false
		}

		result.WriteByte(s[i])
		i++
	}

	return result.String()
}
