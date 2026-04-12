package agent0

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type networkOption struct {
	value       string
	label       string
	description string
}

var networkOptions = []networkOption{
	{
		value:       "no_network",
		label:       "No network access",
		description: "The agent can't browse the internet, but some integrations still work.",
	},
	{
		value:       "full",
		label:       "Full network access",
		description: "The agent can access any website or external service.",
	},
}

// networkSelectorModel is a bubbletea model that prompts the user to choose
// a network access level before starting a chat.
type networkSelectorModel struct {
	cursor   int
	selected string // Set when the user confirms
	aborted  bool
}

func newNetworkSelector() networkSelectorModel {
	return networkSelectorModel{}
}

func (m networkSelectorModel) Init() tea.Cmd {
	return nil
}

func (m networkSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if m.cursor < len(networkOptions)-1 {
				m.cursor++
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.selected = networkOptions[m.cursor].value
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d", "esc"))):
			m.aborted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	selectorTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				MarginBottom(1)

	selectorItemStyle = lipgloss.NewStyle().
				PaddingLeft(2)

	selectorSelectedStyle = lipgloss.NewStyle().
				PaddingLeft(0).
				Foreground(lipgloss.Color("39"))

	selectorDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				PaddingLeft(4)
)

func (m networkSelectorModel) View() string {
	s := selectorTitleStyle.Render("Network access") + "\n"
	s += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Choose the network access level for this session.") + "\n\n"

	for i, opt := range networkOptions {
		var line string
		if i == m.cursor {
			line = selectorSelectedStyle.Render(fmt.Sprintf("> %s", opt.label))
		} else {
			line = selectorItemStyle.Render(opt.label)
		}
		s += line + "\n"
		s += selectorDescStyle.Render(opt.description) + "\n"
		if i < len(networkOptions)-1 {
			s += "\n"
		}
	}

	s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("↑↓: navigate  enter: select  esc: cancel")

	return s
}

// runNetworkSelector runs the selector TUI and returns the chosen network level.
// Returns empty string if the user cancelled.
func runNetworkSelector() (string, error) {
	m := newNetworkSelector()
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return "", err
	}
	final := result.(networkSelectorModel)
	if final.aborted {
		return "", nil
	}
	return final.selected, nil
}
