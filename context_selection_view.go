package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ContextOption represents a context option in the selection list
type ContextOption struct {
	Name      string
	Context   string
	IsCurrent bool
}

// ContextSelectionModel represents the context selection screen state
type ContextSelectionModel struct {
	options       []ContextOption
	cursor        int
	selected      bool
	cancelled     bool
	currentConfig *Config
}

// NewContextSelectionModel creates a new context selection model
func NewContextSelectionModel(config *Config) ContextSelectionModel {
	// Use cluster_name if provided, otherwise use "Current"
	currentName := "Current"
	if config.ClusterName != "" {
		currentName = config.ClusterName
	}
	
	options := []ContextOption{
		{
			Name:      currentName,
			Context:   config.ClusterContext,
			IsCurrent: true,
		},
	}

	// Add alternative contexts
	for _, alt := range config.AlternativeContexts {
		options = append(options, ContextOption{
			Name:      alt.Name,
			Context:   alt.Context,
			IsCurrent: false,
		})
	}

	return ContextSelectionModel{
		options:       options,
		cursor:        0,
		selected:      false,
		cancelled:     false,
		currentConfig: config,
	}
}

func (m ContextSelectionModel) Init() tea.Cmd {
	return nil
}

func (m ContextSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, nil

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}

		case "enter":
			// Don't allow selecting the current context
			if !m.options[m.cursor].IsCurrent {
				m.selected = true
			}
			return m, nil
		}
	}

	return m, nil
}

func (m ContextSelectionModel) View() string {
	var b strings.Builder

	b.WriteString(StyleH1.Render("Select Cluster Context"))
	b.WriteString("\n\n")

	b.WriteString(InfoRow("Current", m.currentConfig.ClusterContext))
	b.WriteString("\n\n")

	b.WriteString(Divider(60))
	b.WriteString("\n\n")

	// Context list
	for i, opt := range m.options {
		cursor := "  "
		if m.cursor == i {
			cursor = StyleCursor.Render("▶ ")
		}

		var line string
		if opt.IsCurrent {
			badge := Badge("CURRENT", "success")
			line = fmt.Sprintf("%s%s %s", cursor, badge, StyleHighlight.Render(opt.Name+" ("+opt.Context+")"))
		} else {
			line = fmt.Sprintf("%s  %s", cursor, StyleBodyPrimary.Render(opt.Name+" ("+opt.Context+")"))
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	helpShortcuts := []string{"↑/↓: navigate", "enter: select", "q/esc: back"}
	b.WriteString(HelpText(helpShortcuts))

	return b.String()
}

// GetSelectedContext returns the selected context option
func (m ContextSelectionModel) GetSelectedContext() ContextOption {
	return m.options[m.cursor]
}

