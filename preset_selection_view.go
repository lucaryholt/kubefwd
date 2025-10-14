package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	presetStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170"))
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

	b.WriteString(titleStyle.Render("kubefwd - Select Preset"))
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render("Note: This will stop all running services and start only the preset services."))
	b.WriteString("\n\n")

	// Preset list
	for i, preset := range m.presets {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render("▶ ")
		}

		line := fmt.Sprintf("%s%s (%d services)", cursor, preset.Name, len(preset.Services))
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: apply preset • q/esc: back"))

	return b.String()
}

// GetSelectedPreset returns the selected preset
func (m PresetSelectionModel) GetSelectedPreset() Preset {
	return m.presets[m.cursor]
}

