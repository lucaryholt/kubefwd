package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	b.WriteString(StyleH1.Render("Configuration Management"))
	b.WriteString("\n\n")

	// Config file path in a card
	cardContent := InfoRow("Config File", m.configPath)
	configCard := Card("", cardContent, 60)
	b.WriteString(configCard)
	b.WriteString("\n")

	// Status message if present
	if m.message != "" {
		if strings.Contains(m.message, "Error") || strings.Contains(m.message, "error") {
			b.WriteString(ErrorMessage(m.message, 60))
		} else if strings.Contains(m.message, "success") {
			b.WriteString(SuccessMessage(m.message))
		} else {
			b.WriteString(WarningMessage(m.message))
		}
		b.WriteString("\n\n")
	}

	// Available actions
	b.WriteString(StyleH3.Render("Actions"))
	b.WriteString("\n\n")
	
	actions := []string{
		"Edit config file",
		"Reload configuration",
	}
	b.WriteString(List(actions))
	b.WriteString("\n\n")

	// Help
	helpShortcuts := []string{"e: edit", "r: reload", "esc/q: back"}
	b.WriteString(HelpText(helpShortcuts))

	// Center content if dimensions available
	content := b.String()
	if m.width > 0 && m.height > 0 {
		return CenterContent(content, m.width, m.height)
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

