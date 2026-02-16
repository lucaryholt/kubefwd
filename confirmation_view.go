package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

	b.WriteString(StyleH1.Render("⚠️  Confirm Context Change"))
	b.WriteString("\n\n")

	b.WriteString(WarningMessage("This will stop all running port forwards!"))
	b.WriteString("\n\n")

	b.WriteString("You are about to switch to:\n")
	b.WriteString(StyleHighlight.Render("  → " + m.targetContextName + " (" + m.targetContext + ")"))
	b.WriteString("\n\n")

	b.WriteString("Type ")
	b.WriteString(StyleHighlight.Render("cluster_change"))
	b.WriteString(" to confirm:\n\n")

	// Show input box
	inputDisplay := m.input
	if len(inputDisplay) == 0 {
		inputDisplay = StyleDim.Render("(type here)")
	} else {
		inputDisplay = StyleHighlight.Render(inputDisplay)
	}
	b.WriteString("  > " + inputDisplay + "█")
	b.WriteString("\n\n")

	// Show result or help
	if m.input != "" && m.input != "cluster_change" {
		if strings.HasPrefix("cluster_change", m.input) {
			b.WriteString(StyleDim.Render("Keep typing..."))
		} else {
			b.WriteString(ErrorMessage("✗ Must type exactly: cluster_change", 60))
		}
	} else if m.input == "cluster_change" {
		b.WriteString(SuccessMessage("✓ Press Enter to confirm"))
	}

	b.WriteString("\n\n")
	helpShortcuts := []string{"esc: cancel"}
	b.WriteString(HelpText(helpShortcuts))

	return b.String()
}

