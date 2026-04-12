package agent0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderMarkdownBasic(t *testing.T) {
	renderer := newMarkdownRenderer(80)
	if renderer == nil {
		t.Skip("glamour renderer not available")
	}

	result := renderMarkdown(renderer, "Hello **world**")
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "world")
	// Glamour adds ANSI styling; exact output varies, but content is preserved.
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	renderer := newMarkdownRenderer(80)
	if renderer == nil {
		t.Skip("glamour renderer not available")
	}

	input := "```go\nfmt.Println(\"hello\")\n```"
	result := renderMarkdown(renderer, input)
	assert.Contains(t, result, "Println")
}

func TestRenderMarkdownNilRenderer(t *testing.T) {
	result := renderMarkdown(nil, "raw content")
	assert.Equal(t, "raw content", result)
}

func TestRenderMarkdownEmptyContent(t *testing.T) {
	renderer := newMarkdownRenderer(80)
	result := renderMarkdown(renderer, "")
	assert.Equal(t, "", result)
}

func TestNewMarkdownRendererZeroWidth(t *testing.T) {
	renderer := newMarkdownRenderer(0)
	assert.NotNil(t, renderer, "should default to 80 when width is 0")
}

func TestNewMarkdownRendererNegativeWidth(t *testing.T) {
	renderer := newMarkdownRenderer(-10)
	assert.NotNil(t, renderer, "should default to 80 when width is negative")
}

func TestRenderMarkdownPreservesAngleBracketTags(t *testing.T) {
	renderer := newMarkdownRenderer(80)
	if renderer == nil {
		t.Skip("glamour renderer not available")
	}

	input := `Here are the services:
<Service name='api' namespace='dash0'>
<Service name='ui' namespace='dash0'>`
	result := renderMarkdown(renderer, input)
	assert.Contains(t, result, "Service")
	assert.Contains(t, result, "api")
	assert.Contains(t, result, "namespace")
}

func TestEscapeAngleBracketsBasic(t *testing.T) {
	assert.Equal(t, "&lt;service&gt;", escapeAngleBrackets("<service>"))
}

func TestEscapeAngleBracketsPreservesCodeBlocks(t *testing.T) {
	input := "text <tag>\n```\n<inside-code>\n```\nmore <tag>"
	result := escapeAngleBrackets(input)
	assert.Contains(t, result, "&lt;tag&gt;")    // Outside code: escaped
	assert.Contains(t, result, "<inside-code>")   // Inside code block: preserved
}

func TestEscapeAngleBracketsPreservesInlineCode(t *testing.T) {
	input := "use `<div>` for that and <service> for this"
	result := escapeAngleBrackets(input)
	assert.Contains(t, result, "`<div>`")           // Inside code span: preserved
	assert.Contains(t, result, "&lt;service&gt;")   // Outside: escaped
}

func TestEscapeAngleBracketsNoTags(t *testing.T) {
	input := "plain text with no tags"
	assert.Equal(t, input, escapeAngleBrackets(input))
}
