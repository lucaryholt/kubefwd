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
	services     []Service
	portForwards []*PortForward
	cursor       int
	config       *Config
	quitting     bool
}

// tickMsg is sent periodically to update the view
type tickMsg time.Time

// NewManagementModel creates a new management model with all services
func NewManagementModel(config *Config) ManagementModel {
	services := config.Services
	portForwards := make([]*PortForward, len(services))
	for i, svc := range services {
		portForwards[i] = NewPortForward(svc, config.ClusterContext, config.Namespace, config.MaxRetries)
	}

	return ManagementModel{
		services:     services,
		portForwards: portForwards,
		cursor:       0,
		config:       config,
		quitting:     false,
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
			// Stop all port forwards before quitting
			m.quitting = true
			for _, pf := range m.portForwards {
				pf.Stop()
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

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("kubefwd"))
	b.WriteString("\n\n")

	// Cluster info
	clusterDisplay := m.config.ClusterContext
	if m.config.ClusterName != "" {
		clusterDisplay = m.config.ClusterName + " (" + m.config.ClusterContext + ")"
	}
	b.WriteString(fmt.Sprintf("Cluster: %s | Namespace: %s\n\n",
		clusterDisplay, m.config.Namespace))

	// Service list with status
	for i, pf := range m.portForwards {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render("▶ ")
		}

		status, errMsg := pf.GetStatus()
		retrying, retryAttempt, maxRetries := pf.GetRetryInfo()
		statusText := m.formatStatus(status, retrying, retryAttempt, maxRetries)
		
		svc := pf.Service
		
		// Add default indicator
		defaultIndicator := " "
		if svc.SelectedByDefault {
			defaultIndicator = "★"
		}
		
		line := fmt.Sprintf("%s%s %-20s %s :%d -> %s:%d",
			cursor, defaultIndicator, svc.Name, statusText, svc.LocalPort, svc.ServiceName, svc.RemotePort)
		
		// Show context/namespace if different from global
		overrides := ""
		if svc.Context != "" && svc.Context != m.config.ClusterContext {
			overrides += fmt.Sprintf(" [ctx: %s]", svc.Context)
		}
		if svc.Namespace != "" && svc.Namespace != m.config.Namespace {
			overrides += fmt.Sprintf(" [ns: %s]", svc.Namespace)
		}
		if overrides != "" {
			line += helpStyle.Render(overrides)
		}
		
		b.WriteString(line)
		
		// Show error message if present
		if status == StatusError && errMsg != "" {
			b.WriteString("\n")
			// Wrap long error messages
			errorLines := wrapText(errMsg, 100)
			for _, errorLine := range errorLines {
				b.WriteString(fmt.Sprintf("     %s", statusErrorStyle.Render(errorLine)))
				b.WriteString("\n")
			}
		}
		
		b.WriteString("\n")
	}

	// Help text
	b.WriteString("\n")
	
	// Build help text dynamically based on available features
	helpText := "↑/↓: navigate • enter/s: toggle • d: start defaults • a: start all • x: stop all"
	if len(m.config.Presets) > 0 {
		helpText += " • p: presets"
	}
	if len(m.config.AlternativeContexts) > 0 {
		helpText += " • c: change context"
	}
	helpText += " • q: quit"
	
	b.WriteString(helpStyle.Render(helpText))

	return b.String()
}

func (m ManagementModel) formatStatus(status PortForwardStatus, retrying bool, retryAttempt int, maxRetries int) string {
	if retrying {
		retryText := fmt.Sprintf("[RETRYING %d", retryAttempt)
		if maxRetries == -1 {
			retryText += "]"
		} else {
			retryText += fmt.Sprintf("/%d]", maxRetries)
		}
		return statusRetryingStyle.Render(retryText)
	}
	
	switch status {
	case StatusRunning:
		return statusRunningStyle.Render("[RUNNING]")
	case StatusStarting:
		return statusStartingStyle.Render("[STARTING]")
	case StatusError:
		return statusErrorStyle.Render("[ERROR]")
	case StatusStopped:
		return statusStoppedStyle.Render("[STOPPED]")
	default:
		return statusStoppedStyle.Render("[UNKNOWN]")
	}
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

