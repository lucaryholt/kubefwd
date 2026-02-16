package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// ScreenType represents the current screen being displayed
type ScreenType int

const (
	ScreenManagement ScreenType = iota
	ScreenContextSelection
	ScreenConfirmation
	ScreenPresetSelection
	ScreenConfig
	ScreenPortChecker
	ScreenSqlTapSelection
	ScreenHelp
)

// AppModel is the root model that manages screen transitions
type AppModel struct {
	screen               ScreenType
	config               *Config
	configPath           string
	sidebar              SidebarModel
	managementModel      ManagementModel
	contextModel         ContextSelectionModel
	confirmModel         ConfirmationModel
	presetModel          PresetSelectionModel
	configModel          ConfigModel
	portCheckerModel     PortCheckerModel
	sqlTapSelectionModel SqlTapSelectionModel
	helpModel            HelpModel
	targetContextOption  ContextOption
	width                int
	height               int
	sidebarFocused       bool
}

// NewAppModel creates a new app model
func NewAppModel(config *Config, configPath string, autoStartDefault, autoStartDefaultProxy bool) AppModel {
	managementModel := NewManagementModel(config)
	sidebar := NewSidebarModel(config)

	// Auto-start services if flags are set
	if autoStartDefault {
		for i, svc := range managementModel.services {
			if svc.SelectedByDefault {
				managementModel.portForwards[i].Start()
			}
		}
	}

	if autoStartDefaultProxy && len(config.ProxyServices) > 0 {
		// Collect default proxy services
		var defaultProxyServices []ProxyService
		for _, pxSvc := range config.ProxyServices {
			if pxSvc.SelectedByDefault {
				defaultProxyServices = append(defaultProxyServices, pxSvc)
			}
		}

		// Apply proxy selection if there are default services
		if len(defaultProxyServices) > 0 {
			proxyForwards, err := ApplyProxySelection(
				managementModel.proxyPodManager,
				managementModel.proxyForwards,
				defaultProxyServices,
				config.ClusterContext,
				config.Namespace,
			)
			if err == nil {
				managementModel.proxyForwards = proxyForwards
				// Sync staged state with active services
				for _, pxSvc := range defaultProxyServices {
					managementModel.proxySelectedState[pxSvc.Name] = true
				}
			}
		}
	}

	return AppModel{
		screen:          ScreenManagement,
		config:          config,
		configPath:      configPath,
		sidebar:         sidebar,
		managementModel: managementModel,
		sidebarFocused:  false,
	}
}

func (m AppModel) Init() tea.Cmd {
	return m.managementModel.Init()
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window size changes globally
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = msg.Width
		m.height = msg.Height
		m.managementModel.width = msg.Width
		m.managementModel.height = msg.Height
		m.helpModel.width = msg.Width
		m.helpModel.height = msg.Height
	}

	// Handle global sidebar navigation (number keys 1-7) on ALL screens
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		key := keyMsg.String()
		
		// Check for number keys (1-7) for sidebar navigation - works globally
		if key >= "1" && key <= "7" {
			selected, section := m.sidebar.HandleKey(key)
			if selected {
				newScreen := GetSectionScreen(section)
				if newScreen != m.screen {
					return m.switchToScreen(newScreen)
				}
			}
			return m, nil
		}
		
		// Check for help key globally
		if key == "?" {
			m.screen = ScreenHelp
			m.helpModel = NewHelpModel()
			return m, m.helpModel.Init()
		}
	}

	switch m.screen {
	case ScreenManagement:
		return m.updateManagement(msg)
	case ScreenContextSelection:
		return m.updateContextSelection(msg)
	case ScreenConfirmation:
		return m.updateConfirmation(msg)
	case ScreenPresetSelection:
		return m.updatePresetSelection(msg)
	case ScreenConfig:
		return m.updateConfig(msg)
	case ScreenPortChecker:
		return m.updatePortChecker(msg)
	case ScreenSqlTapSelection:
		return m.updateSqlTapSelection(msg)
	case ScreenHelp:
		return m.updateHelp(msg)
	}
	return m, nil
}

func (m AppModel) updateManagement(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Check for special keys
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		key := keyMsg.String()
		
		// Legacy hotkey support (backward compatibility)
		if key == "c" {
			// Only allow context change if there are alternative contexts
			if len(m.config.AlternativeContexts) > 0 {
				m.screen = ScreenContextSelection
				m.contextModel = NewContextSelectionModel(m.config)
				return m, m.contextModel.Init()
			}
		} else if key == "p" {
			// Only allow preset selection if there are presets
			if len(m.config.Presets) > 0 {
				m.screen = ScreenPresetSelection
				m.presetModel = NewPresetSelectionModel(m.config)
				return m, m.presetModel.Init()
			}
		} else if key == "g" {
			// Open config management screen
			m.screen = ScreenConfig
			m.configModel = NewConfigModel(m.configPath)
			return m, m.configModel.Init()
		} else if key == "l" {
			// Open port checker screen
			m.screen = ScreenPortChecker
			m.portCheckerModel = NewPortCheckerModel(m.config, m.managementModel.portForwards, m.managementModel.proxyForwards)
			return m, m.portCheckerModel.Init()
		} else if key == "t" {
			// Open sql-tap selection screen
			m.screen = ScreenSqlTapSelection
			m.sqlTapSelectionModel = NewSqlTapSelectionModel(
				m.managementModel.portForwards,
				m.managementModel.proxyForwards,
			)
			return m, m.sqlTapSelectionModel.Init()
		}
	}

	// Update management model (handles Tab for pane switching and other keys)
	updatedModel, cmd := m.managementModel.Update(msg)
	m.managementModel = updatedModel.(ManagementModel)
	return m, cmd
}

// switchToScreen switches to a different screen based on sidebar selection
func (m AppModel) switchToScreen(screen ScreenType) (tea.Model, tea.Cmd) {
	m.screen = screen
	
	switch screen {
	case ScreenManagement:
		return m, nil
	case ScreenContextSelection:
		if len(m.config.AlternativeContexts) > 0 {
			m.contextModel = NewContextSelectionModel(m.config)
			return m, m.contextModel.Init()
		}
	case ScreenPresetSelection:
		if len(m.config.Presets) > 0 {
			m.presetModel = NewPresetSelectionModel(m.config)
			return m, m.presetModel.Init()
		}
	case ScreenConfig:
		m.configModel = NewConfigModel(m.configPath)
		return m, m.configModel.Init()
	case ScreenPortChecker:
		m.portCheckerModel = NewPortCheckerModel(m.config, m.managementModel.portForwards, m.managementModel.proxyForwards)
		return m, m.portCheckerModel.Init()
	case ScreenSqlTapSelection:
		m.sqlTapSelectionModel = NewSqlTapSelectionModel(
			m.managementModel.portForwards,
			m.managementModel.proxyForwards,
		)
		return m, m.sqlTapSelectionModel.Init()
	}
	
	// Default to management
	m.screen = ScreenManagement
	return m, nil
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
	// Stop all running port forwards and proxy forwards
	for _, pf := range m.managementModel.portForwards {
		pf.Stop()
	}
	for _, pxf := range m.managementModel.proxyForwards {
		pxf.Stop()
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

	// Find and start matching direct services
	for i, svc := range m.managementModel.services {
		if presetServiceMap[svc.Name] {
			m.managementModel.portForwards[i].Start()
		}
	}

	// Return to management screen
	m.screen = ScreenManagement
	return m, nil
}

func (m AppModel) updatePortChecker(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.portCheckerModel.Update(msg)
	m.portCheckerModel = updatedModel.(PortCheckerModel)

	if m.portCheckerModel.cancelled {
		// Return to management
		m.screen = ScreenManagement
		return m, nil
	}

	return m, cmd
}

func (m AppModel) updateSqlTapSelection(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.sqlTapSelectionModel.Update(msg)
	m.sqlTapSelectionModel = updatedModel.(SqlTapSelectionModel)

	if m.sqlTapSelectionModel.cancelled {
		// Return to management
		m.screen = ScreenManagement
		return m, nil
	}

	if m.sqlTapSelectionModel.launched {
		// Launch sql-tap and return to management
		item := m.sqlTapSelectionModel.GetSelectedItem()
		if err := LaunchSqlTapInNewTab(item.GrpcPort); err != nil {
			debugLog("Failed to launch sql-tap: %v", err)
			debugLog("Run manually: %s", GetSqlTapLaunchCommand(item.GrpcPort))
		} else {
			debugLog("Successfully launched sql-tap for %s on gRPC port %d", item.Name, item.GrpcPort)
		}
		m.screen = ScreenManagement
		return m, nil
	}

	return m, cmd
}

func (m AppModel) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.helpModel.Update(msg)
	m.helpModel = updatedModel.(HelpModel)

	if m.helpModel.cancelled {
		// Return to management
		m.screen = ScreenManagement
		return m, nil
	}

	return m, cmd
}

func (m AppModel) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle config reload message
	if _, ok := msg.(configReloadMsg); ok {
		return m.reloadConfig()
	}

	// Handle editor messages
	if _, ok := msg.(editorClosedMsg); ok {
		m.configModel.message = "Editor closed. Press 'r' to reload configuration."
	}
	if errMsg, ok := msg.(editorErrorMsg); ok {
		m.configModel.message = fmt.Sprintf("Error: %v", errMsg.err)
	}

	updatedModel, cmd := m.configModel.Update(msg)
	m.configModel = updatedModel.(ConfigModel)

	if m.configModel.cancelled {
		// Return to management
		m.screen = ScreenManagement
		return m, nil
	}

	return m, cmd
}

func (m AppModel) reloadConfig() (tea.Model, tea.Cmd) {
	// Track which services are currently running
	runningServices := make(map[string]bool)
	for i, pf := range m.managementModel.portForwards {
		if pf.IsRunning() {
			runningServices[m.managementModel.services[i].Name] = true
		}
	}

	// Track which proxy services are currently running
	runningProxyServices := make(map[string]bool)
	for name, pxf := range m.managementModel.proxyForwards {
		status, _ := pxf.GetStatus()
		if status == StatusRunning {
			runningProxyServices[name] = true
		}
	}

	// Load new configuration
	newConfig, err := LoadConfig(m.configPath)
	if err != nil {
		m.configModel.message = fmt.Sprintf("Error loading config: %v", err)
		return m, nil
	}

	// Stop all running port forwards
	for _, pf := range m.managementModel.portForwards {
		if pf.IsRunning() {
			pf.Stop()
		}
	}

	// Stop all proxy forwards
	for _, pxf := range m.managementModel.proxyForwards {
		pxf.Stop()
	}

	// Delete the proxy pod if it exists
	if m.managementModel.proxyPodManager != nil {
		m.managementModel.proxyPodManager.DeletePod()
	}

	// Update config and recreate management model
	m.config = newConfig
	m.managementModel = NewManagementModel(newConfig)

	// Restart services that were running and still exist
	for i, svc := range m.managementModel.services {
		if runningServices[svc.Name] {
			m.managementModel.portForwards[i].Start()
		}
	}

	// Restart proxy services that were running and still exist
	if len(runningProxyServices) > 0 && len(m.config.ProxyServices) > 0 {
		var servicesToRestart []ProxyService
		for _, pxSvc := range m.config.ProxyServices {
			if runningProxyServices[pxSvc.Name] {
				servicesToRestart = append(servicesToRestart, pxSvc)
			}
		}
		if len(servicesToRestart) > 0 {
			proxyForwards, err := ApplyProxySelection(
				m.managementModel.proxyPodManager,
				m.managementModel.proxyForwards,
				servicesToRestart,
				m.config.ClusterContext,
				m.config.Namespace,
			)
			if err != nil {
				m.configModel.message = fmt.Sprintf("Config reloaded, but proxy services failed: %v", err)
				return m, nil
			}
			m.managementModel.proxyForwards = proxyForwards
			// Sync staged state
			for _, pxSvc := range servicesToRestart {
				m.managementModel.proxySelectedState[pxSvc.Name] = true
			}
		}
	}

	m.configModel.message = "Configuration reloaded successfully!"
	return m, nil
}

func (m AppModel) View() string {
	var mainContent string
	
	switch m.screen {
	case ScreenManagement:
		mainContent = m.managementModel.View()
	case ScreenContextSelection:
		mainContent = m.contextModel.View()
	case ScreenConfirmation:
		mainContent = m.confirmModel.View()
	case ScreenPresetSelection:
		mainContent = m.presetModel.View()
	case ScreenConfig:
		mainContent = m.configModel.View()
	case ScreenPortChecker:
		mainContent = m.portCheckerModel.View()
	case ScreenSqlTapSelection:
		mainContent = m.sqlTapSelectionModel.View()
	case ScreenHelp:
		mainContent = m.helpModel.View()
	default:
		mainContent = ""
	}
	
	// Show sidebar on all screens
	if m.sidebar.IsVisible() {
		layout := LayoutWithSidebar{
			SidebarContent: m.sidebar.View(),
			MainContent:    mainContent,
			Width:          m.width,
			Height:         m.height,
			SidebarVisible: true,
		}
		return layout.Render()
	}
	
	return mainContent
}

