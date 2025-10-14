package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyleConfirm = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

// ConfirmationModel represents the confirmation screen state
type ConfirmationModel struct {
	targetContextName string
	targetContext     string
	input             string
	confirmed         bool
	cancelled         bool
}

// NewConfirmationModel creates a new confirmation model
func NewConfirmationModel(contextName, context string) ConfirmationModel {
	return ConfirmationModel{
		targetContextName: contextName,
		targetContext:     context,
		input:             "",
		confirmed:         false,
		cancelled:         false,
	}
}

func (m ConfirmationModel) Init() tea.Cmd {
	return nil
}

func (m ConfirmationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.cancelled = true
			return m, nil

		case tea.KeyEnter:
			if strings.TrimSpace(m.input) == "cluster_change" {
				m.confirmed = true
			}
			return m, nil

		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}

		case tea.KeyRunes:
			m.input += string(msg.Runes)
		}
	}

	return m, nil
}

func (m ConfirmationModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("kubefwd - ⚠️  Confirm Context Change"))
	b.WriteString("\n\n")

	b.WriteString(warningStyle.Render("WARNING: This will change the default cluster context andq will stop all running port forwards!"))
	b.WriteString("\n\n")

	b.WriteString("You are about to switch to:\n")
	b.WriteString(inputStyle.Render("  → " + m.targetContextName + " (" + m.targetContext + ")"))
	b.WriteString("\n\n")

	b.WriteString("Type ")
	b.WriteString(inputStyle.Render("cluster_change"))
	b.WriteString(" to confirm:\n\n")

	// Show input box
	inputDisplay := m.input
	if len(inputDisplay) == 0 {
		inputDisplay = dimStyle.Render("(type here)")
	} else {
		inputDisplay = inputStyle.Render(inputDisplay)
	}
	b.WriteString("  > " + inputDisplay + "█")
	b.WriteString("\n\n")

	// Show result or help
	if m.input != "" && m.input != "cluster_change" {
		if strings.HasPrefix("cluster_change", m.input) {
			b.WriteString(dimStyle.Render("Keep typing..."))
		} else {
			b.WriteString(errorStyleConfirm.Render("✗ Must type exactly: cluster_change"))
		}
	} else if m.input == "cluster_change" {
		b.WriteString(inputStyle.Render("✓ Press Enter to confirm"))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("esc: cancel"))

	return b.String()
}

