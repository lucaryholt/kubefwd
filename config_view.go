package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfigModel represents the state of the config management screen
type ConfigModel struct {
	configPath string
	width      int
	height     int
	message    string
	cancelled  bool
	reloadCmd  tea.Cmd
}

// configReloadMsg is sent when config reload is requested
type configReloadMsg struct {
	err error
}

// NewConfigModel creates a new config model
func NewConfigModel(configPath string) ConfigModel {
	return ConfigModel{
		configPath: configPath,
		message:    "",
		cancelled:  false,
	}
}

func (m ConfigModel) Init() tea.Cmd {
	return nil
}

func (m ConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.cancelled = true
			return m, nil

		case "e":
			// Open config file in editor
			return m, m.openInEditor()

		case "r":
			// Request config reload
			m.message = "Reloading configuration..."
			return m, func() tea.Msg {
				return configReloadMsg{}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m ConfigModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Configuration Management"))
	b.WriteString("\n\n")

	// Config file path
	b.WriteString("Config file: ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(m.configPath))
	b.WriteString("\n\n")

	// Status message if present
	if m.message != "" {
		messageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		b.WriteString(messageStyle.Render(m.message))
		b.WriteString("\n\n")
	}

	// Available actions
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Actions:"))
	b.WriteString("\n")
	b.WriteString("  e - Open config file in editor\n")
	b.WriteString("  r - Reload configuration from file\n")
	b.WriteString("\n")

	// Help
	b.WriteString(helpStyle.Render("Press Esc or q to return to main view"))

	// Center content
	content := b.String()
	if m.width > 0 && m.height > 0 {
		contentStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Padding(2, 4)
		return contentStyle.Render(content)
	}

	return content
}

// openInEditor opens the config file in the user's preferred editor
func (m ConfigModel) openInEditor() tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// Check for common editors in order of preference
		for _, candidate := range []string{"vi", "nano", "vim"} {
			if _, err := exec.LookPath(candidate); err == nil {
				editor = candidate
				break
			}
		}
	}

	if editor == "" {
		return func() tea.Msg {
			return editorErrorMsg{err: fmt.Errorf("no editor found. Please set $EDITOR environment variable")}
		}
	}

	c := exec.Command(editor, m.configPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return editorErrorMsg{err: err}
		}
		return editorClosedMsg{}
	})
}

// editorClosedMsg is sent when the editor is closed successfully
type editorClosedMsg struct{}

// editorErrorMsg is sent when there's an error opening the editor
type editorErrorMsg struct {
	err error
}

