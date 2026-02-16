package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// HelpModel represents the help panel screen
type HelpModel struct {
	width     int
	height    int
	cancelled bool
}

// NewHelpModel creates a new help model
func NewHelpModel() HelpModel {
	return HelpModel{
		cancelled: false,
	}
}

func (m HelpModel) Init() tea.Cmd {
	return nil
}

func (m HelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "?":
			m.cancelled = true
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m HelpModel) View() string {
	var b strings.Builder

	b.WriteString(StyleH1.Render("Help"))
	b.WriteString("\n\n")

	// Create help layout with sections
	helpLayout := HelpLayout{
		Sections: []HelpSection{
			{
				Title: "Navigation",
				Shortcuts: []HelpShortcut{
					{Key: "↑/k", Description: "Move cursor up"},
					{Key: "↓/j", Description: "Move cursor down"},
					{Key: "Enter", Description: "Select / Toggle service"},
					{Key: "Tab", Description: "Switch between port forwards and proxy panes"},
				},
			},
			{
				Title: "Service Management",
				Shortcuts: []HelpShortcut{
					{Key: "s", Description: "Toggle selected service"},
					{Key: "d", Description: "Start all default services"},
					{Key: "a", Description: "Start all services"},
					{Key: "x", Description: "Stop all services"},
					{Key: "o", Description: "Toggle override info"},
				},
			},
			{
				Title: "Sidebar Navigation (Number Keys)",
				Shortcuts: []HelpShortcut{
					{Key: "1", Description: "Port Forwards (main view)"},
					{Key: "2", Description: "Proxy Services"},
					{Key: "3", Description: "Presets"},
					{Key: "4", Description: "SQL Tap"},
					{Key: "5", Description: "Port Checker"},
					{Key: "6", Description: "Configuration"},
					{Key: "7", Description: "Context Switcher"},
				},
			},
			{
				Title: "Legacy Shortcuts",
				Shortcuts: []HelpShortcut{
					{Key: "p", Description: "Open presets"},
					{Key: "r", Description: "Manage proxy services"},
					{Key: "c", Description: "Switch context"},
					{Key: "g", Description: "Edit configuration"},
					{Key: "l", Description: "Check ports"},
					{Key: "t", Description: "Open SQL Tap"},
				},
			},
			{
				Title: "General",
				Shortcuts: []HelpShortcut{
					{Key: "?", Description: "Show/hide this help"},
					{Key: "q/Ctrl+C", Description: "Quit application"},
				},
			},
		},
		Width: m.width,
	}

	b.WriteString(helpLayout.Render())
	b.WriteString("\n\n")

	// Footer
	helpShortcuts := []string{"Press ? or esc to close"}
	b.WriteString(HelpText(helpShortcuts))

	// Center content if dimensions available
	content := b.String()
	if m.width > 0 && m.height > 0 {
		return CenterContent(content, m.width, m.height)
	}

	return content
}
