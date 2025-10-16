package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	checkboxStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))
)

// proxySelectionTickMsg is sent periodically to update the view
type proxySelectionTickMsg time.Time

// ProxySelectionModel represents the proxy service selection screen
type ProxySelectionModel struct {
	proxyServices   []ProxyService
	selected        map[int]bool // Track which services are selected
	activeServices  map[string]bool // Track which services are currently active
	cursor          int
	confirmed       bool
	cancelled       bool
	podManager      *ProxyPodManager
}

// NewProxySelectionModel creates a new proxy selection model
func NewProxySelectionModel(proxyServices []ProxyService, podManager *ProxyPodManager) ProxySelectionModel {
	selected := make(map[int]bool)
	activeServices := make(map[string]bool)
	
	// Mark currently active services
	if podManager != nil {
		for _, name := range podManager.GetActiveServiceNames() {
			activeServices[name] = true
		}
		
		// Pre-select active services
		for i, svc := range proxyServices {
			if activeServices[svc.Name] {
				selected[i] = true
			}
		}
	}
	
	return ProxySelectionModel{
		proxyServices:  proxyServices,
		selected:       selected,
		activeServices: activeServices,
		cursor:         0,
		confirmed:      false,
		cancelled:      false,
		podManager:     podManager,
	}
}

func (m ProxySelectionModel) Init() tea.Cmd {
	return proxySelectionTick()
}

// proxySelectionTick returns a command that sends a proxySelectionTickMsg after a short delay
func proxySelectionTick() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return proxySelectionTickMsg(t)
	})
}

func (m ProxySelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.proxyServices)-1 {
				m.cursor++
			}

		case " ":
			// Toggle selection
			m.selected[m.cursor] = !m.selected[m.cursor]

		case "enter":
			m.confirmed = true
			return m, nil
		}

	case proxySelectionTickMsg:
		// Continue ticking
		return m, proxySelectionTick()
	}

	return m, nil
}

func (m ProxySelectionModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Select Proxy Services"))
	b.WriteString("\n\n")

	// Pod status
	if m.podManager != nil {
		podStatus, podErr, activeCount := m.podManager.GetStatus()
		podStatusText := m.formatProxyPodStatus(podStatus)
		b.WriteString(fmt.Sprintf("Pod Status: %s", podStatusText))
		if activeCount > 0 {
			b.WriteString(fmt.Sprintf(" (%d active)", activeCount))
		}
		b.WriteString("\n")
		
		// Show error message if present
		if podStatus == ProxyPodStatusError && podErr != "" {
			errorLines := wrapText(podErr, 70)
			for _, line := range errorLines {
				b.WriteString("  ")
				b.WriteString(statusErrorStyle.Render(line))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Service list with checkboxes
	for i, svc := range m.proxyServices {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render("▶ ")
		}

		// Checkbox
		checkbox := "[ ]"
		if m.selected[i] {
			checkbox = checkboxStyle.Render("[✓]")
		}

		// Connection info
		connStr := fmt.Sprintf(":%d -> %s:%d", svc.LocalPort, svc.TargetHost, svc.TargetPort)

		line := fmt.Sprintf("%s%s %-25s %s", cursor, checkbox, svc.Name, connStr)
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Show currently active services
	b.WriteString("\n")
	activeNames := []string{}
	for name := range m.activeServices {
		activeNames = append(activeNames, name)
	}
	if len(activeNames) > 0 {
		b.WriteString(helpStyle.Render(fmt.Sprintf("Currently active: %s", strings.Join(activeNames, ", "))))
	} else {
		b.WriteString(helpStyle.Render("No proxy services currently active"))
	}

	// Help text
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("space: toggle • enter: apply changes • esc/q: cancel"))

	return b.String()
}

// GetSelectedServices returns the list of selected proxy services
func (m ProxySelectionModel) GetSelectedServices() []ProxyService {
	var selected []ProxyService
	for i, svc := range m.proxyServices {
		if m.selected[i] {
			selected = append(selected, svc)
		}
	}
	return selected
}

// formatProxyPodStatus formats the proxy pod status with colors
func (m ProxySelectionModel) formatProxyPodStatus(status ProxyPodStatus) string {
	switch status {
	case ProxyPodStatusReady:
		return statusRunningStyle.Render("[READY]")
	case ProxyPodStatusCreating:
		return statusStartingStyle.Render("[CREATING]")
	case ProxyPodStatusError:
		return statusErrorStyle.Render("[ERROR]")
	case ProxyPodStatusNotCreated:
		return statusStoppedStyle.Render("[NOT CREATED]")
	default:
		return statusStoppedStyle.Render("[UNKNOWN]")
	}
}

