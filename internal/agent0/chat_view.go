package agent0

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")) // Cyan

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")) // Grey

	userPrefixStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")) // Dim

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")) // Red

	statusMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)
)

func (m chatModel) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "Initializing..."
	}

	header := m.renderHeader()
	messageArea := m.viewport.View()
	inputArea := m.renderInputArea()
	status := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		messageArea,
		inputArea,
		status,
	)
}

func (m chatModel) renderHeader() string {
	title := headerStyle.Render("Agent0 Chat")

	if m.threadID != "" {
		threadInfo := statusBarStyle.Render(fmt.Sprintf("Thread: %s", m.threadID))
		padding := m.width - lipgloss.Width(title) - lipgloss.Width(threadInfo)
		if padding < 1 {
			padding = 1
		}
		return title + strings.Repeat(" ", padding) + threadInfo
	}

	return title
}

func (m chatModel) renderInputArea() string {
	if m.streaming {
		return statusMsgStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), m.statusText))
	}
	return m.textarea.View()
}

func (m chatModel) renderStatusBar() string {
	// Left side: network mode + profile
	var infoParts []string
	if m.cfg.networkLevel != "" {
		infoParts = append(infoParts, networkLevelLabel(m.cfg.networkLevel))
	}
	if m.cfg.profileName != "" {
		infoParts = append(infoParts, "profile: "+m.cfg.profileName)
	}
	left := statusBarStyle.Render(strings.Join(infoParts, "  │  "))

	right := statusBarStyle.Render("↑↓/pgup/pgdn: scroll  ctrl+c: cancel  ctrl+d: quit")
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
}

func networkLevelLabel(level string) string {
	switch level {
	case "no_network":
		return "🔒 no network"
	case "full":
		return "🌐 full network"
	default:
		return level
	}
}

// Message prefixes
const (
	prefixUser      = "👤 "
	prefixAssistant = "🤖 "
	prefixError     = "❌ "
)

func styleUserMessage(content string) string {
	var sb strings.Builder
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i == 0 {
			sb.WriteString(userPrefixStyle.Render(prefixUser))
		} else {
			sb.WriteString("\n   ") // Indent continuation lines to align with prefix
		}
		sb.WriteString(line)
	}
	return sb.String()
}

func styleAssistantMessage(content string) string {
	return prefixAssistant + content
}

func styleError(content string) string {
	return errorStyle.Render(prefixError + content)
}

func styleToolStep(step toolStep) string {
	icon := "⚙"
	if step.done {
		icon = "✓"
	}
	line := statusMsgStyle.Render(fmt.Sprintf("  %s %s", icon, step.why))
	if step.content != "" {
		line += "\n" + statusMsgStyle.Render(fmt.Sprintf("    → %s", step.content))
	}
	return line
}

