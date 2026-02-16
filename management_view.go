package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ManagementModel represents the state of the management screen
type ManagementModel struct {
	services           []Service
	portForwards       []*PortForward
	proxyServices      []ProxyService
	proxyPodManager    *ProxyPodManager
	proxyForwards      map[string]*ProxyForward // Map of service name to forward
	proxySelectedState map[string]bool          // Staged selection state (what user wants)
	cursor             int
	proxyCursor        int    // Cursor position for proxy services pane
	config             *Config
	quitting           bool
	width              int
	height             int
	showOverrides      bool   // Toggle for showing context/namespace override details
	focusedPane        string // "port_forwards" or "proxy_services"
}

// tickMsg is sent periodically to update the view
type tickMsg time.Time

// proxyApplyCompleteMsg is sent when proxy selection is applied
type proxyApplyCompleteMsg struct {
	proxyForwards map[string]*ProxyForward
	err           error
}

// NewManagementModel creates a new management model with all services
func NewManagementModel(config *Config) ManagementModel {
	// Create port forwards for direct services
	services := config.Services
	portForwards := make([]*PortForward, len(services))
	for i, svc := range services {
		portForwards[i] = NewPortForward(svc, config.ClusterContext, config.Namespace, config.MaxRetries)
	}

	// Create proxy pod manager if proxy services exist
	var proxyPodManager *ProxyPodManager
	proxyServices := config.ProxyServices

	if len(proxyServices) > 0 {
		proxyPodManager = NewProxyPodManager(
			config.ProxyPodName,
			config.ProxyPodImage,
			config.ProxyPodNamespace,
			config.ProxyPodContext,
		)
	}

	// Initialize proxy selected state - all services start unselected
	proxySelectedState := make(map[string]bool)
	for _, pxSvc := range proxyServices {
		proxySelectedState[pxSvc.Name] = false
	}

	return ManagementModel{
		services:           services,
		portForwards:       portForwards,
		proxyServices:      proxyServices,
		proxyPodManager:    proxyPodManager,
		proxyForwards:      make(map[string]*ProxyForward),
		proxySelectedState: proxySelectedState,
		cursor:             0,
		proxyCursor:        0,
		config:             config,
		quitting:           false,
		focusedPane:        "port_forwards",
	}
}

func (m ManagementModel) Init() tea.Cmd {
	return tick()
}

// tick returns a command that sends a tickMsg after a short delay
func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m ManagementModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case proxyApplyCompleteMsg:
		// Proxy apply completed (success or error)
		if msg.err != nil {
			debugLog("Proxy apply failed: %v", msg.err)
		} else {
			// Update the model with the created proxy forwards
			m.proxyForwards = msg.proxyForwards
			
			// Sync the staged state with actual active services
			if m.proxyPodManager != nil {
				activeServiceNames := m.proxyPodManager.GetActiveServiceNames()
				activeMap := make(map[string]bool)
				for _, name := range activeServiceNames {
					activeMap[name] = true
				}
				
				// Update staged state to match active state
				for _, pxSvc := range m.proxyServices {
					m.proxySelectedState[pxSvc.Name] = activeMap[pxSvc.Name]
				}
			}
		}
		return m, nil
		
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Stop all port forwards and proxy forwards before quitting
			m.quitting = true
			for _, pf := range m.portForwards {
				pf.Stop()
			}
			for _, pxf := range m.proxyForwards {
				pxf.Stop()
			}
			// Delete the proxy pod if it exists
			if m.proxyPodManager != nil {
				m.proxyPodManager.DeletePod()
			}
			return m, tea.Quit

		case "tab":
			// Switch focus between panes
			if len(m.proxyServices) > 0 {
				if m.focusedPane == "port_forwards" {
					m.focusedPane = "proxy_services"
				} else {
					m.focusedPane = "port_forwards"
				}
			}

		case "up", "k":
			// Navigate based on focused pane
			if m.focusedPane == "port_forwards" {
				if m.cursor > 0 {
					m.cursor--
				}
			} else if m.focusedPane == "proxy_services" {
				if m.proxyCursor > 0 {
					m.proxyCursor--
				}
			}

		case "down", "j":
			// Navigate based on focused pane
			if m.focusedPane == "port_forwards" {
				if m.cursor < len(m.services)-1 {
					m.cursor++
				}
			} else if m.focusedPane == "proxy_services" {
				if m.proxyCursor < len(m.proxyServices)-1 {
					m.proxyCursor++
				}
			}

		case " ":
			// Space bar - toggle based on focused pane
			if m.focusedPane == "port_forwards" {
				// Toggle port forward service immediately
				if m.cursor >= 0 && m.cursor < len(m.portForwards) {
					pf := m.portForwards[m.cursor]
					if pf.IsRunning() {
						pf.Stop()
					} else {
						pf.Start()
					}
				}
			} else if m.focusedPane == "proxy_services" {
				// Toggle staged selection (don't apply yet)
				if m.proxyCursor >= 0 && m.proxyCursor < len(m.proxyServices) {
					selectedSvc := m.proxyServices[m.proxyCursor]
					m.proxySelectedState[selectedSvc.Name] = !m.proxySelectedState[selectedSvc.Name]
				}
			}

		case "enter", "s":
			// Enter - toggle or apply based on focused pane
			if m.focusedPane == "port_forwards" {
				// Toggle port forward service immediately
				if m.cursor >= 0 && m.cursor < len(m.portForwards) {
					pf := m.portForwards[m.cursor]
					if pf.IsRunning() {
						pf.Stop()
					} else {
						pf.Start()
					}
				}
			} else if m.focusedPane == "proxy_services" {
				// Apply the staged selections asynchronously
				var selectedServices []ProxyService
				for _, pxSvc := range m.proxyServices {
					if m.proxySelectedState[pxSvc.Name] {
						selectedServices = append(selectedServices, pxSvc)
					}
				}
				return m, m.applyProxySelectionAsync(selectedServices)
			}

		case "a":
			// Start all port forward services
			for _, pf := range m.portForwards {
				if !pf.IsRunning() {
					pf.Start()
				}
			}

		case "d":
			// Start all default services based on focused pane
			if m.focusedPane == "port_forwards" {
				// Start all default port forward services
				for i, pf := range m.portForwards {
					if m.services[i].SelectedByDefault && !pf.IsRunning() {
						pf.Start()
					}
				}
			} else if m.focusedPane == "proxy_services" {
				// Set staged state for default proxy services
				for _, pxSvc := range m.proxyServices {
					if pxSvc.SelectedByDefault {
						m.proxySelectedState[pxSvc.Name] = true
					}
				}
			}

		case "x":
			// Stop all port forward services
			for _, pf := range m.portForwards {
				if pf.IsRunning() {
					pf.Stop()
				}
			}

		case "o":
			// Toggle override info display
			m.showOverrides = !m.showOverrides
		}

	case tickMsg:
		// Continue ticking
		return m, tick()
	}

	return m, nil
}

func (m ManagementModel) View() string {
	if m.quitting {
		return "Stopping all port forwards...\n"
	}

	// If we have proxy services, render split panes
	if len(m.proxyServices) > 0 {
		// Calculate available width for content
		// The full terminal width has sidebar (25% with 20-40 constraints) + padding
		totalWidth := m.width
		if totalWidth <= 0 {
			totalWidth = 100
		}
		
		sidebarWidth := totalWidth / 4
		if sidebarWidth < 20 {
			sidebarWidth = 20
		}
		if sidebarWidth > 40 {
			sidebarWidth = 40
		}
		
		// Available width = total - sidebar - padding (2 left + 2 right)
		availableWidth := totalWidth - sidebarWidth - 4
		if availableWidth < 60 {
			availableWidth = 60
		}
		
		// Split into 70/30
		leftWidth := int(float64(availableWidth) * 0.7)
		rightWidth := availableWidth - leftWidth - 3 // Account for border
		
		// Build panes with proper widths
		leftPane := m.renderPortForwardsPane(leftWidth)
		rightPane := m.renderProxyServicesPane(rightWidth)

		layout := LayoutSplit{
			LeftContent:  leftPane,
			RightContent: rightPane,
			Width:        availableWidth,
			Height:       m.height,
			SplitRatio:   0.7,
			Vertical:     true,
		}
		return layout.Render()
	}

	// No proxy services - just show port forwards
	return m.renderPortForwardsPane(0)
}

// calculatePaneWidths returns the left and right pane widths
func (m ManagementModel) calculatePaneWidths() (int, int) {
	// The width passed to ManagementModel is the full terminal width
	// We need to account for the sidebar which takes ~25% of the width
	
	totalWidth := m.width
	if totalWidth <= 0 {
		totalWidth = 100
	}
	
	// Sidebar takes about 25% of total width (minimum 20, maximum 40)
	sidebarWidth := totalWidth / 4
	if sidebarWidth < 20 {
		sidebarWidth = 20
	}
	if sidebarWidth > 40 {
		sidebarWidth = 40
	}
	
	// Available width for content after sidebar
	availableWidth := totalWidth - sidebarWidth - 4 // Account for borders and padding
	
	if len(m.proxyServices) == 0 {
		return availableWidth, 0
	}

	// Split content area 70/30
	leftWidth := int(float64(availableWidth) * 0.7)
	rightWidth := availableWidth - leftWidth
	
	return leftWidth, rightWidth
}

// renderPortForwardsPane renders the port forwards pane
func (m ManagementModel) renderPortForwardsPane(width int) string {
	var b strings.Builder

	// Pane indicator - more visible with border
	var paneIndicator string
	if m.focusedPane == "port_forwards" {
		activeStyle := lipgloss.NewStyle().
			Foreground(ColorPrimaryBlue).
			Background(lipgloss.AdaptiveColor{Light: "#D1ECF1", Dark: "#1C2F3A"}).
			Bold(true).
			Padding(0, 1)
		paneIndicator = activeStyle.Render("● ACTIVE")
	} else {
		inactiveStyle := lipgloss.NewStyle().
			Foreground(ColorDimGray).
			Padding(0, 1)
		paneIndicator = inactiveStyle.Render("○ Press Tab")
	}

	// Header with running count
	runningCount := 0
	for _, pf := range m.portForwards {
		if pf.IsRunning() {
			runningCount++
		}
	}

	header := Header("Port Forwards", runningCount, len(m.services))
	b.WriteString(header)
	b.WriteString(" ")
	b.WriteString(paneIndicator)
	b.WriteString("\n\n")

	// Cluster info
	clusterInfo := ClusterInfo(m.config.ClusterContext, m.config.ClusterName, m.config.Namespace)
	b.WriteString(clusterInfo)
	b.WriteString("\n\n")

	// Divider - use a reasonable default width
	contentWidth := 50 // Fixed width for content
	if width > 4 {
		contentWidth = width - 4
	}
	b.WriteString(Divider(contentWidth))
	b.WriteString("\n\n")

	// Service list
	for i, pf := range m.portForwards {
		cursor := ""
		if m.focusedPane == "port_forwards" && m.cursor == i {
			cursor = "▶"
		}

		status, errMsg := pf.GetStatus()
		retrying, retryAttempt, maxRetries := pf.GetRetryInfo()
		svc := pf.Service

		// Check if service has overrides
		hasOverrides := (svc.Context != "" && svc.Context != m.config.ClusterContext) ||
			(svc.Namespace != "" && svc.Namespace != m.config.Namespace)

		// Build service line
		serviceLine := ServiceLine(ServiceLineOptions{
			Cursor:       cursor,
			IsDefault:    svc.SelectedByDefault,
			HasOverrides: hasOverrides,
			Name:         TruncateString(svc.Name, 20),
			Status:       status,
			Retrying:     retrying,
			RetryAttempt: retryAttempt,
			MaxRetries:   maxRetries,
			LocalPort:    svc.LocalPort,
			ServiceName:  svc.ServiceName,
			RemotePort:   svc.RemotePort,
			ShowBadge:    false,
		})

		b.WriteString(serviceLine)

		// Add sql-tap status if enabled
		sqlTapMgr := pf.GetSqlTapManager()
		if sqlTapMgr.IsEnabled() {
			sqlTapStatus, _ := sqlTapMgr.GetStatus()
			sqlTapStatusSymbol := StatusIndicator(sqlTapStatus, false, 0, 0)
			sqlTapInfo := fmt.Sprintf(" %s SQL-TAP :%d (gRPC:%d)",
				sqlTapStatusSymbol, sqlTapMgr.GetListenPort(), sqlTapMgr.GetGrpcPort())
			b.WriteString(StyleBodySecondary.Render(sqlTapInfo))
		}

		// Show detailed context/namespace info only if toggle is on
		if m.showOverrides && hasOverrides {
			overrides := ""
			if svc.Context != "" && svc.Context != m.config.ClusterContext {
				overrides += fmt.Sprintf(" [ctx: %s]", svc.Context)
			}
			if svc.Namespace != "" && svc.Namespace != m.config.Namespace {
				overrides += fmt.Sprintf(" [ns: %s]", svc.Namespace)
			}
			b.WriteString(StyleHelp.Render(overrides))
		}

		b.WriteString("\n")

		// Show error message if present
		if status == StatusError && errMsg != "" {
			wrapWidth := contentWidth - 10
			if wrapWidth < 40 {
				wrapWidth = 40
			}
			errorLines := WrapText(errMsg, wrapWidth)
			for _, errorLine := range errorLines {
				b.WriteString(fmt.Sprintf("     %s", ErrorMessage(errorLine, wrapWidth)))
				b.WriteString("\n")
			}
		}
	}

	// Quick Actions Bar
	b.WriteString("\n")
	b.WriteString(SectionDivider("QUICK ACTIONS", contentWidth))
	b.WriteString("\n\n")

	quickActions := QuickActionBar([]struct{ Label, Hotkey, Type string }{
		{Label: "Start Defaults", Hotkey: "D", Type: "primary"},
		{Label: "Start All", Hotkey: "A", Type: "success"},
		{Label: "Stop All", Hotkey: "X", Type: "danger"},
	})
	b.WriteString(quickActions)
	b.WriteString("\n\n")

	// Help text
	helpShortcuts := []string{
		"↑↓/jk: navigate",
		"Enter: toggle",
		"Tab: switch pane",
		"?: help",
	}
	b.WriteString(HelpText(helpShortcuts))

	return b.String()
}

// renderProxyServicesPane renders the proxy services pane
func (m ManagementModel) renderProxyServicesPane(width int) string {
	if len(m.proxyServices) == 0 {
		return ""
	}

	var b strings.Builder

	// Pane indicator - more visible with border
	var paneIndicator string
	if m.focusedPane == "proxy_services" {
		activeStyle := lipgloss.NewStyle().
			Foreground(ColorPrimaryBlue).
			Background(lipgloss.AdaptiveColor{Light: "#D1ECF1", Dark: "#1C2F3A"}).
			Bold(true).
			Padding(0, 1)
		paneIndicator = activeStyle.Render("● ACTIVE")
	} else {
		inactiveStyle := lipgloss.NewStyle().
			Foreground(ColorDimGray).
			Padding(0, 1)
		paneIndicator = inactiveStyle.Render("○ Press Tab")
	}

	// Title
	b.WriteString(StyleH2.Render("Proxy Services"))
	b.WriteString(" ")
	b.WriteString(paneIndicator)
	b.WriteString("\n\n")

	// Pod status
	if m.proxyPodManager != nil {
		podStatus, podErr, activeCount := m.proxyPodManager.GetStatus()
		podStatusText := m.formatProxyPodStatus(podStatus)
		b.WriteString(fmt.Sprintf("Pod: %s", podStatusText))
		if activeCount > 0 {
			b.WriteString(fmt.Sprintf(" (%d)", activeCount))
		}
		b.WriteString("\n")
		
		// Show helpful message when pod is creating
		if podStatus == ProxyPodStatusCreating {
			creatingStyle := lipgloss.NewStyle().
				Foreground(ColorAccentAmber).
				Bold(true)
			creatingMsg := creatingStyle.Render("⏳ Creating proxy pod... This may take a moment.")
			b.WriteString(creatingMsg)
			b.WriteString("\n")
		}
		
		b.WriteString("\n")

		// Show error if present
		if podStatus == ProxyPodStatusError && podErr != "" {
			wrapWidth := width - 8
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			errorLines := WrapText(podErr, wrapWidth)
			for _, line := range errorLines {
				b.WriteString(ErrorMessage(line, wrapWidth))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	// Check if there are pending changes
	hasChanges := false
	for _, pxSvc := range m.proxyServices {
		isStaged := m.proxySelectedState[pxSvc.Name]
		isActive := m.proxyPodManager != nil && m.proxyPodManager.IsServiceActive(pxSvc.Name)
		if isStaged != isActive {
			hasChanges = true
			break
		}
	}

	// Show pending changes banner
	if hasChanges {
		warningStyle := lipgloss.NewStyle().Foreground(ColorAccentAmber)
		b.WriteString(warningStyle.Render("⚠ Changes pending - Press Enter to apply"))
		b.WriteString("\n\n")
	}

	// Proxy service list
	for i, pxSvc := range m.proxyServices {
		cursor := "  "
		if m.focusedPane == "proxy_services" && m.proxyCursor == i {
			cursor = StyleCursor.Render("▶ ")
		}

		// Get staged and active states
		isStaged := m.proxySelectedState[pxSvc.Name]
		isActive := m.proxyPodManager != nil && m.proxyPodManager.IsServiceActive(pxSvc.Name)
		isSynced := isStaged == isActive

		// Checkbox based on staged state
		checkboxText := Checkbox(isStaged, pxSvc.Name)

		// Add sync indicator
		syncIndicator := ""
		if !isSynced {
			syncStyle := lipgloss.NewStyle().Foreground(ColorAccentAmber)
			syncIndicator = " " + syncStyle.Render("●")
		}

		// Show port forward status if active
		statusText := ""
		if isActive {
			if pxf, exists := m.proxyForwards[pxSvc.Name]; exists {
				status, _ := pxf.GetStatus()
				statusText = " " + StatusIndicator(status, false, 0, 0)

				// Add sql-tap status if enabled
				sqlTapMgr := pxf.GetSqlTapManager()
				if sqlTapMgr.IsEnabled() {
					sqlTapStatus, _ := sqlTapMgr.GetStatus()
					sqlTapStatusSymbol := StatusIndicator(sqlTapStatus, false, 0, 0)
					statusText += fmt.Sprintf(" [ST %s:%d/%d]",
						sqlTapStatusSymbol, sqlTapMgr.GetListenPort(), sqlTapMgr.GetGrpcPort())
				}
			}
		}

		line := fmt.Sprintf("%s%s%s%s", cursor, checkboxText, syncIndicator, statusText)
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	helpShortcuts := []string{"Space: toggle", "D: select defaults", "Enter: apply", "Tab: switch pane"}
	b.WriteString(HelpText(helpShortcuts))

	return b.String()
}

func (m ManagementModel) formatProxyPodStatus(status ProxyPodStatus) string {
	switch status {
	case ProxyPodStatusReady:
		return StyleStatusRunning.Render("● Ready")
	case ProxyPodStatusCreating:
		return StyleStatusStarting.Render("◐ Creating")
	case ProxyPodStatusError:
		return StyleStatusError.Render("✗ Error")
	case ProxyPodStatusNotCreated:
		return StyleStatusStopped.Render("○ Not Created")
	default:
		return StyleStatusStopped.Render("? Unknown")
	}
}

// ApplyProxySelection updates the proxy pod and port forwards based on selected services
// Returns the created proxy forwards map
func ApplyProxySelection(proxyPodManager *ProxyPodManager, proxyForwards map[string]*ProxyForward, selectedServices []ProxyService, clusterContext, namespace string) (map[string]*ProxyForward, error) {
	// Stop all existing proxy forwards
	for _, pxf := range proxyForwards {
		pxf.Stop()
	}
	newProxyForwards := make(map[string]*ProxyForward)

	// Create pod with selected services
	if err := proxyPodManager.CreatePodWithServices(selectedServices); err != nil {
		return newProxyForwards, err
	}

	// Start port forwards for selected services
	for _, pxSvc := range selectedServices {
		pxf := NewProxyForward(pxSvc, proxyPodManager, clusterContext, namespace)
		if err := pxf.Start(); err != nil {
			// Log error but continue with other services
			debugLog("Failed to start proxy forward for %s: %v", pxSvc.Name, err)
		}
		newProxyForwards[pxSvc.Name] = pxf
	}

	return newProxyForwards, nil
}

// applyProxySelectionAsync returns a command that applies proxy selection in the background
func (m ManagementModel) applyProxySelectionAsync(selectedServices []ProxyService) tea.Cmd {
	return func() tea.Msg {
		proxyForwards, err := ApplyProxySelection(m.proxyPodManager, m.proxyForwards, selectedServices, m.config.ClusterContext, m.config.Namespace)
		return proxyApplyCompleteMsg{
			proxyForwards: proxyForwards,
			err:           err,
		}
	}
}
