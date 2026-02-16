package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusRunningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	statusStoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusStartingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	statusErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	statusRetryingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))
)

// ManagementModel represents the state of the management screen
type ManagementModel struct {
	services        []Service
	portForwards    []*PortForward
	proxyServices   []ProxyService
	proxyPodManager *ProxyPodManager
	proxyForwards   map[string]*ProxyForward // Map of service name to forward
	cursor          int
	config          *Config
	quitting        bool
	width           int
	height          int
	showOverrides   bool // Toggle for showing context/namespace override details
}

// tickMsg is sent periodically to update the view
type tickMsg time.Time

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

	return ManagementModel{
		services:        services,
		portForwards:    portForwards,
		proxyServices:   proxyServices,
		proxyPodManager: proxyPodManager,
		proxyForwards:   make(map[string]*ProxyForward),
		cursor:          0,
		config:          config,
		quitting:        false,
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
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Stop all port forwards and proxy forwards before quitting
			m.quitting = true
			for _, pf := range m.portForwards {
				pf.Stop()
			}
			for _, pxf := range m.proxyForwards {
				// Use Cleanup() to stop both port-forward and sql-tapd
				pxf.Cleanup()
			}
			// Delete the proxy pod if it exists
			if m.proxyPodManager != nil {
				m.proxyPodManager.DeletePod()
			}
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.services)-1 {
				m.cursor++
			}

		case "enter", "s":
			// Toggle the current service
			pf := m.portForwards[m.cursor]
			if pf.IsRunning() {
				pf.Stop()
			} else {
				pf.Start()
			}

		case "K":
			// Kill conflicting process on the selected service
			pf := m.portForwards[m.cursor]
			conflictInfo := pf.GetConflictInfo()
			if conflictInfo.HasConflict && conflictInfo.ProcessPID > 0 {
				if err := KillProcess(conflictInfo.ProcessPID); err == nil {
					// Give the OS a moment to release the port
					time.Sleep(100 * time.Millisecond)
					
					// Refresh conflict status for all services to detect if the port is now free
					for _, pf := range m.portForwards {
						pf.RefreshConflictStatus()
					}
					
					// Try to start the current service if its port is now available
					if !m.portForwards[m.cursor].HasPortConflict() {
						m.portForwards[m.cursor].Start()
					}
				} else {
					// Update error message if kill failed
					pf.mu.Lock()
					pf.ErrorMessage = fmt.Sprintf("Failed to kill PID %d: %v", conflictInfo.ProcessPID, err)
					pf.mu.Unlock()
				}
			}

		case "a":
			// Start all services
			for _, pf := range m.portForwards {
				if !pf.IsRunning() {
					pf.Start()
				}
			}

		case "d":
			// Start all default services
			for i, pf := range m.portForwards {
				if m.services[i].SelectedByDefault && !pf.IsRunning() {
					pf.Start()
				}
			}

		case "x":
			// Stop all services
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

	// Calculate pane widths
	leftWidth, rightWidth := m.calculatePaneWidths()

	// Build left pane (direct services)
	leftPane := m.renderLeftPane(leftWidth)

	// Build right pane (proxy services)
	rightPane := m.renderRightPane(rightWidth)

	// Use lipgloss to create split view with equal heights
	if rightPane != "" {
		// Make sure both panes have the same height
		leftHeight := lipgloss.Height(leftPane)
		rightHeight := lipgloss.Height(rightPane)
		maxHeight := leftHeight
		if rightHeight > maxHeight {
			maxHeight = rightHeight
		}

		// Set explicit heights for both panes
		leftStyle := lipgloss.NewStyle().Height(maxHeight)
		rightStyle := lipgloss.NewStyle().Height(maxHeight)

		return lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(leftPane), rightStyle.Render(rightPane))
	}

	return leftPane
}

// calculatePaneWidths returns the left and right pane widths based on terminal size
func (m ManagementModel) calculatePaneWidths() (int, int) {
	// Default minimum widths
	minLeftWidth := 60
	minRightWidth := 30

	// If no proxy services, use full width for left pane
	if len(m.proxyServices) == 0 {
		if m.width > 0 {
			return m.width - 4, 0 // Subtract padding
		}
		return 80, 0
	}

	// If terminal width is available, calculate 70/30 split
	if m.width > 0 {
		leftWidth := int(float64(m.width) * 0.7)
		rightWidth := int(float64(m.width) * 0.3)

		// Apply minimum constraints
		if leftWidth < minLeftWidth {
			leftWidth = minLeftWidth
		}
		if rightWidth < minRightWidth {
			rightWidth = minRightWidth
		}

		// Adjust if total exceeds available width
		total := leftWidth + rightWidth
		if total > m.width {
			leftWidth = m.width - rightWidth
		}

		return leftWidth, rightWidth
	}

	// Fallback to defaults
	return 80, 35
}

func (m ManagementModel) renderLeftPane(width int) string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("kubefwd"))
	b.WriteString("\n\n")

	// Cluster info
	clusterDisplay := m.config.ClusterContext
	if m.config.ClusterName != "" {
		clusterDisplay = m.config.ClusterName + " (" + m.config.ClusterContext + ")"
	}
	b.WriteString(fmt.Sprintf("Cluster: %s\n", clusterDisplay))
	b.WriteString(fmt.Sprintf("Namespace: %s\n\n", m.config.Namespace))

	// Service list
	for i, pf := range m.portForwards {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render("▶ ")
		}

		status, errMsg := pf.GetStatus()
		retrying, retryAttempt, maxRetries := pf.GetRetryInfo()
		conflictInfo := pf.GetConflictInfo()
		statusText := m.formatStatus(status, retrying, retryAttempt, maxRetries)

		svc := pf.Service

		// Default indicator
		defaultIndicator := " "
		if svc.SelectedByDefault {
			defaultIndicator = "★"
		}

		// Conflict indicator
		conflictIndicator := " "
		if conflictInfo.HasConflict {
			if conflictInfo.IsKubectl {
				conflictIndicator = "⚠"
			} else {
				conflictIndicator = "⚠"
			}
		}

		// Truncate service name if too long
		displayName := svc.Name
		if len(displayName) > 18 {
			displayName = displayName[:17] + "…"
		}

		// Check if service has overrides
		hasOverrides := (svc.Context != "" && svc.Context != m.config.ClusterContext) ||
			(svc.Namespace != "" && svc.Namespace != m.config.Namespace)

		// Show override indicator
		overrideIndicator := " "
		if hasOverrides {
			overrideIndicator = "⚙"
		}

		line := fmt.Sprintf("%s%s%s%s %-18s %s :%d → %s:%d",
			cursor, conflictIndicator, defaultIndicator, overrideIndicator, displayName, statusText, svc.LocalPort, svc.ServiceName, svc.RemotePort)

		// Show detailed context/namespace info only if toggle is on
		if m.showOverrides && hasOverrides {
			overrides := ""
			if svc.Context != "" && svc.Context != m.config.ClusterContext {
				overrides += fmt.Sprintf(" [ctx: %s]", svc.Context)
			}
			if svc.Namespace != "" && svc.Namespace != m.config.Namespace {
				overrides += fmt.Sprintf(" [ns: %s]", svc.Namespace)
			}
			line += helpStyle.Render(overrides)
		}

		b.WriteString(line)

		// Show error message if present
		if status == StatusError && errMsg != "" {
			b.WriteString("\n")
			// Calculate wrap width based on pane width
			wrapWidth := width - 10 // Account for indentation and padding
			if wrapWidth < 40 {
				wrapWidth = 40
			}
			errorLines := wrapText(errMsg, wrapWidth)
			for _, errorLine := range errorLines {
				b.WriteString(fmt.Sprintf("     %s", statusErrorStyle.Render(errorLine)))
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
	}

	// Help text - split into two rows
	b.WriteString("\n")
	
	// Row 1: Navigation and service controls
	helpRow1 := "↑↓/jk:nav • s:toggle • K:kill • d:def • a:all • x:stop • o:overrides"
	b.WriteString(helpStyle.Render(helpRow1))
	b.WriteString("\n")
	
	// Row 2: Mode switches and quit
	helpRow2 := ""
	if len(m.config.Presets) > 0 {
		helpRow2 += "p:presets • "
	}
	if len(m.config.AlternativeContexts) > 0 {
		helpRow2 += "c:context • "
	}
	if len(m.proxyServices) > 0 {
		helpRow2 += "r:proxy • "
	}
	helpRow2 += "g:config • q:quit"
	b.WriteString(helpStyle.Render(helpRow2))
	
	// SQL-Tap info if any service has it enabled
	hasSqlTap := false
	for _, pxf := range m.proxyForwards {
		if pxf.IsSqlTapEnabled() {
			hasSqlTap = true
			break
		}
	}
	if hasSqlTap {
		b.WriteString("\n\n")
		sqlTapHelp := "📊 SQL-Tap enabled services can be inspected with: sql-tap localhost:<grpc_port>"
		b.WriteString(helpStyle.Render(sqlTapHelp))
	}

	// Render in a styled box with dynamic width
	leftStyle := lipgloss.NewStyle().
		Width(width).
		PaddingLeft(2).
		PaddingRight(2)

	return leftStyle.Render(b.String())
}

func (m ManagementModel) renderRightPane(width int) string {
	if len(m.proxyServices) == 0 {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Proxy Services"))
	b.WriteString("\n\n")

	// Pod status
	if m.proxyPodManager != nil {
		podStatus, podErr, activeCount := m.proxyPodManager.GetStatus()
		podStatusText := m.formatProxyPodStatus(podStatus)
		b.WriteString(fmt.Sprintf("Pod: %s", podStatusText))
		if activeCount > 0 {
			b.WriteString(fmt.Sprintf(" (%d)", activeCount))
		}
		b.WriteString("\n\n")
		
		// Show full error message wrapped to fit pane
		if podStatus == ProxyPodStatusError && podErr != "" {
			// Calculate wrap width based on pane width
			wrapWidth := width - 8 // Account for padding and border
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			errorLines := wrapText(podErr, wrapWidth)
			for _, line := range errorLines {
				b.WriteString(statusErrorStyle.Render(line))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	// Proxy service list
	for _, pxSvc := range m.proxyServices {
		isActive := m.proxyPodManager != nil && m.proxyPodManager.IsServiceActive(pxSvc.Name)

		checkbox := "[ ]"
		if isActive {
			checkbox = checkboxStyle.Render("[✓]")
		}

		// Show port forward status if active
		statusText := ""
		sqlTapIndicator := ""
		if isActive {
			if pxf, exists := m.proxyForwards[pxSvc.Name]; exists {
				status, _ := pxf.GetStatus()
				switch status {
				case StatusRunning:
					statusText = statusRunningStyle.Render("●")
				case StatusError:
					statusText = statusErrorStyle.Render("✗")
				}
				
				// Add SQL-Tap indicator if enabled
				if pxf.IsSqlTapEnabled() {
					sqlTapIndicator = " 📊"
				}
			}
		}

		// Truncate service name if too long for right pane
		displayName := pxSvc.Name
		maxNameWidth := width - 16 // Account for checkbox, padding, status, and sql-tap indicator
		if maxNameWidth < 10 {
			maxNameWidth = 10
		}
		if len(displayName) > maxNameWidth {
			displayName = displayName[:maxNameWidth-1] + "…"
		}

		line := fmt.Sprintf("%s %s", checkbox, displayName)
		if statusText != "" {
			line += " " + statusText
		}
		if sqlTapIndicator != "" {
			line += sqlTapIndicator
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press 'r' to manage"))

	// Render in a styled box with dynamic width
	rightStyle := lipgloss.NewStyle().
		Width(width).
		PaddingLeft(2).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	return rightStyle.Render(b.String())
}

func (m ManagementModel) formatStatus(status PortForwardStatus, retrying bool, retryAttempt int, maxRetries int) string {
	if retrying {
		retryText := fmt.Sprintf("↻ %d", retryAttempt)
		if maxRetries != -1 {
			retryText += fmt.Sprintf("/%d", maxRetries)
		}
		return statusRetryingStyle.Render(retryText)
	}
	
	switch status {
	case StatusRunning:
		return statusRunningStyle.Render("●")
	case StatusStarting:
		return statusStartingStyle.Render("◐")
	case StatusError:
		return statusErrorStyle.Render("✗")
	case StatusStopped:
		return statusStoppedStyle.Render("○")
	default:
		return statusStoppedStyle.Render("?")
	}
}

func (m ManagementModel) formatProxyPodStatus(status ProxyPodStatus) string {
	switch status {
	case ProxyPodStatusReady:
		return statusRunningStyle.Render("● Ready")
	case ProxyPodStatusCreating:
		return statusStartingStyle.Render("◐ Creating")
	case ProxyPodStatusError:
		return statusErrorStyle.Render("✗ Error")
	case ProxyPodStatusNotCreated:
		return statusStoppedStyle.Render("○ Not Created")
	default:
		return statusStoppedStyle.Render("? Unknown")
	}
}

// ApplyProxySelection updates the proxy pod and port forwards based on selected services
func (m *ManagementModel) ApplyProxySelection(selectedServices []ProxyService) error {
	// Stop all existing proxy forwards
	for _, pxf := range m.proxyForwards {
		pxf.Stop()
	}
	m.proxyForwards = make(map[string]*ProxyForward)

	// Create pod with selected services
	if err := m.proxyPodManager.CreatePodWithServices(selectedServices); err != nil {
		return err
	}

	// Start port forwards for selected services
	for _, pxSvc := range selectedServices {
		pxf := NewProxyForward(pxSvc, m.proxyPodManager, m.config.ClusterContext, m.config.Namespace)
		if err := pxf.Start(); err != nil {
			// Log error but continue with other services
			debugLog("Failed to start proxy forward for %s: %v", pxSvc.Name, err)
		}
		m.proxyForwards[pxSvc.Name] = pxf
	}

	return nil
}

// wrapText wraps text to a maximum width
func wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}

	var lines []string
	for len(text) > width {
		// Try to break at a space
		breakPoint := width
		for breakPoint > 0 && text[breakPoint] != ' ' && text[breakPoint] != '|' {
			breakPoint--
		}
		if breakPoint == 0 {
			breakPoint = width
		}

		lines = append(lines, strings.TrimSpace(text[:breakPoint]))
		text = strings.TrimSpace(text[breakPoint:])
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}

	return lines
}



