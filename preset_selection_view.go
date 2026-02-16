package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PresetSelectionModel represents the preset selection screen state
type PresetSelectionModel struct {
	presets  []Preset
	cursor   int
	selected bool
	cancelled bool
	config   *Config
}

// NewPresetSelectionModel creates a new preset selection model
func NewPresetSelectionModel(config *Config) PresetSelectionModel {
	return PresetSelectionModel{
		presets:   config.Presets,
		cursor:    0,
		selected:  false,
		cancelled: false,
		config:    config,
	}
}

func (m PresetSelectionModel) Init() tea.Cmd {
	return nil
}

func (m PresetSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.presets)-1 {
				m.cursor++
			}

		case "enter":
			m.selected = true
			return m, nil
		}
	}

	return m, nil
}

func (m PresetSelectionModel) View() string {
	var b strings.Builder

	b.WriteString(StyleH1.Render("Select Preset"))
	b.WriteString("\n\n")

	b.WriteString(WarningMessage("This will stop all running services and start only the preset services."))
	b.WriteString("\n\n")

	b.WriteString(Divider(60))
	b.WriteString("\n\n")

	// Preset list
	for i, preset := range m.presets {
		cursor := "  "
		if m.cursor == i {
			cursor = StyleCursor.Render("▶ ")
		}

		presetName := StyleBodyPrimary.Copy().Bold(true).Render(preset.Name)
		serviceCount := StyleBodySecondary.Render(fmt.Sprintf("(%d services)", len(preset.Services)))
		line := fmt.Sprintf("%s%s %s", cursor, presetName, serviceCount)
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	helpShortcuts := []string{"↑/↓: navigate", "enter: apply preset", "q/esc: back"}
	b.WriteString(HelpText(helpShortcuts))

	return b.String()
}

// GetSelectedPreset returns the selected preset
func (m PresetSelectionModel) GetSelectedPreset() Preset {
	return m.presets[m.cursor]
}

