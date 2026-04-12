package agent0

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Submit key.Binding
	Cancel key.Binding
	Quit   key.Binding
}

var chatKeys = keyMap{
	Submit: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "cancel/quit"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "quit"),
	),
}
