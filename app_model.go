package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ScreenType represents the current screen being displayed
type ScreenType int

const (
	ScreenManagement ScreenType = iota
	ScreenContextSelection
	ScreenConfirmation
	ScreenPresetSelection
)

// AppModel is the root model that manages screen transitions
type AppModel struct {
	screen              ScreenType
	config              *Config
	managementModel     ManagementModel
	contextModel        ContextSelectionModel
	confirmModel        ConfirmationModel
	presetModel         PresetSelectionModel
	targetContextOption ContextOption
}

// NewAppModel creates a new app model
func NewAppModel(config *Config) AppModel {
	return AppModel{
		screen:          ScreenManagement,
		config:          config,
		managementModel: NewManagementModel(config),
	}
}

func (m AppModel) Init() tea.Cmd {
	return m.managementModel.Init()
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenManagement:
		return m.updateManagement(msg)
	case ScreenContextSelection:
		return m.updateContextSelection(msg)
	case ScreenConfirmation:
		return m.updateConfirmation(msg)
	case ScreenPresetSelection:
		return m.updatePresetSelection(msg)
	}
	return m, nil
}

func (m AppModel) updateManagement(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Check for special keys
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "c" {
			// Only allow context change if there are alternative contexts
			if len(m.config.AlternativeContexts) > 0 {
				m.screen = ScreenContextSelection
				m.contextModel = NewContextSelectionModel(m.config)
				return m, m.contextModel.Init()
			}
		} else if keyMsg.String() == "p" {
			// Only allow preset selection if there are presets
			if len(m.config.Presets) > 0 {
				m.screen = ScreenPresetSelection
				m.presetModel = NewPresetSelectionModel(m.config)
				return m, m.presetModel.Init()
			}
		}
	}

	// Update management model
	updatedModel, cmd := m.managementModel.Update(msg)
	m.managementModel = updatedModel.(ManagementModel)
	return m, cmd
}

func (m AppModel) updateContextSelection(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.contextModel.Update(msg)
	m.contextModel = updatedModel.(ContextSelectionModel)

	if m.contextModel.cancelled {
		// Return to management
		m.screen = ScreenManagement
		return m, nil
	}

	if m.contextModel.selected {
		// Move to confirmation screen
		m.targetContextOption = m.contextModel.GetSelectedContext()
		m.screen = ScreenConfirmation
		m.confirmModel = NewConfirmationModel(m.targetContextOption.Name, m.targetContextOption.Context)
		return m, m.confirmModel.Init()
	}

	return m, cmd
}

func (m AppModel) updateConfirmation(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.confirmModel.Update(msg)
	m.confirmModel = updatedModel.(ConfirmationModel)

	if m.confirmModel.cancelled {
		// Return to context selection
		m.screen = ScreenContextSelection
		return m, nil
	}

	if m.confirmModel.confirmed {
		// Perform context switch
		return m.switchContext()
	}

	return m, cmd
}

func (m AppModel) switchContext() (tea.Model, tea.Cmd) {
	// Stop all running port forwards
	for _, pf := range m.managementModel.portForwards {
		pf.Stop()
	}

	// Update the config with new context and name
	m.config.ClusterContext = m.targetContextOption.Context
	m.config.ClusterName = m.targetContextOption.Name

	// Recreate management model with new context
	m.managementModel = NewManagementModel(m.config)
	m.screen = ScreenManagement

	return m, m.managementModel.Init()
}

func (m AppModel) updatePresetSelection(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.presetModel.Update(msg)
	m.presetModel = updatedModel.(PresetSelectionModel)

	if m.presetModel.cancelled {
		// Return to management
		m.screen = ScreenManagement
		return m, nil
	}

	if m.presetModel.selected {
		// Apply the preset
		return m.applyPreset()
	}

	return m, cmd
}

func (m AppModel) applyPreset() (tea.Model, tea.Cmd) {
	preset := m.presetModel.GetSelectedPreset()
	
	// Stop all running port forwards
	for _, pf := range m.managementModel.portForwards {
		if pf.IsRunning() {
			pf.Stop()
		}
	}

	// Start only the services in the preset
	presetServiceMap := make(map[string]bool)
	for _, serviceName := range preset.Services {
		presetServiceMap[serviceName] = true
	}

	// Find and start matching services
	for i, svc := range m.managementModel.services {
		if presetServiceMap[svc.Name] {
			m.managementModel.portForwards[i].Start()
		}
	}

	// Return to management screen
	m.screen = ScreenManagement
	return m, nil
}

func (m AppModel) View() string {
	switch m.screen {
	case ScreenManagement:
		return m.managementModel.View()
	case ScreenContextSelection:
		return m.contextModel.View()
	case ScreenConfirmation:
		return m.confirmModel.View()
	case ScreenPresetSelection:
		return m.presetModel.View()
	}
	return ""
}

