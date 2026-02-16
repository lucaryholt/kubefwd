package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	portStatusFreeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	portStatusKubefwdStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true)

	portStatusExternalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	portTableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true).
				Underline(true)
)

// PortInfo represents information about a single port
type PortInfo struct {
	Port        int
	ServiceName string
	Type        string // "Direct" or "Proxy"
	Status      PortStatus
	PID         int
	ProcessInfo string
	Error       string
}

// portCheckerTickMsg is sent periodically to update the view
type portCheckerTickMsg time.Time

// PortCheckerModel represents the port checker screen
type PortCheckerModel struct {
	config        *Config
	portForwards  []*PortForward
	proxyForwards map[string]*ProxyForward
	ports         []PortInfo
	cursor        int
	loading       bool
	errorMessage  string
	cancelled     bool
	showingKill   bool
	killPID       int
	killService   string
	killStatus    PortStatus
	width         int
	height        int
}

// NewPortCheckerModel creates a new port checker model
func NewPortCheckerModel(config *Config, portForwards []*PortForward, proxyForwards map[string]*ProxyForward) PortCheckerModel {
	return PortCheckerModel{
		config:        config,
		portForwards:  portForwards,
		proxyForwards: proxyForwards,
		ports:         []PortInfo{},
		cursor:        0,
		loading:       true,
		cancelled:     false,
		showingKill:   false,
	}
}

func (m PortCheckerModel) Init() tea.Cmd {
	return tea.Batch(
		m.refreshPorts(),
		portCheckerTick(),
	)
}

// portCheckerTick returns a command that sends a portCheckerTickMsg after a short delay
func portCheckerTick() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return portCheckerTickMsg(t)
	})
}

// refreshPortsMsg is sent when port refresh is complete
type refreshPortsMsg struct {
	ports []PortInfo
	err   error
}

// refreshPorts checks the status of all ports and returns a command
func (m PortCheckerModel) refreshPorts() tea.Cmd {
	return func() tea.Msg {
		// Get all ports from config
		configPorts := GetAllPortsFromConfig(m.config)

		// Check each port
		var ports []PortInfo
		for _, cp := range configPorts {
			info := PortInfo{
				Port:        cp.Port,
				ServiceName: cp.ServiceName,
				Type:        cp.Type,
				Status:      PortStatusFree,
				PID:         0,
				ProcessInfo: "-",
			}

			// Check port usage
			usage, err := GetPortUsage(cp.Port)
			if err != nil {
				info.Error = err.Error()
			} else if usage.InUse {
				info.PID = usage.PID
				info.ProcessInfo = usage.ProcessInfo
				
				// Determine if it's a kubefwd process
				if IsKubefwdProcess(usage.PID, m.portForwards, m.proxyForwards) {
					info.Status = PortStatusKubefwd
				} else {
					info.Status = PortStatusExternal
				}
			}

			ports = append(ports, info)
		}

		// Sort ports by port number
		sort.Slice(ports, func(i, j int) bool {
			return ports[i].Port < ports[j].Port
		})

		return refreshPortsMsg{ports: ports, err: nil}
	}
}

func (m PortCheckerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle kill confirmation mode
	if m.showingKill {
		return m.updateKillConfirmation(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case refreshPortsMsg:
		m.ports = msg.ports
		m.loading = false
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
		}

	case portCheckerTickMsg:
		// Auto-refresh ports periodically
		return m, tea.Batch(
			m.refreshPorts(),
			portCheckerTick(),
		)

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
			if m.cursor < len(m.ports)-1 {
				m.cursor++
			}

		case "r":
			// Manual refresh
			m.loading = true
			return m, m.refreshPorts()

		case "K":
			// Kill process (capital K to avoid accidents)
			if m.cursor < len(m.ports) {
				port := m.ports[m.cursor]
				if port.Status != PortStatusFree {
					m.showingKill = true
					m.killPID = port.PID
					m.killService = port.ServiceName
					m.killStatus = port.Status
				}
			}
		}
	}

	return m, nil
}

func (m PortCheckerModel) updateKillConfirmation(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			// Confirm kill
			m.showingKill = false
			err := KillProcess(m.killPID)
			if err != nil {
				m.errorMessage = fmt.Sprintf("Failed to kill process: %v", err)
			} else {
				m.errorMessage = fmt.Sprintf("Process %d killed successfully", m.killPID)
			}
			// Refresh ports after killing
			m.loading = true
			return m, m.refreshPorts()

		case "n", "N", "esc":
			// Cancel kill
			m.showingKill = false
			return m, nil
		}
	}

	return m, nil
}

func (m PortCheckerModel) View() string {
	if m.showingKill {
		return m.renderKillConfirmation()
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Port Status Checker"))
	b.WriteString("\n\n")

	// Cluster info
	clusterDisplay := m.config.ClusterContext
	if m.config.ClusterName != "" {
		clusterDisplay = m.config.ClusterName + " (" + m.config.ClusterContext + ")"
	}
	b.WriteString(fmt.Sprintf("Cluster: %s | Namespace: %s\n\n", clusterDisplay, m.config.Namespace))

	// Error message
	if m.errorMessage != "" {
		b.WriteString(statusErrorStyle.Render(m.errorMessage))
		b.WriteString("\n\n")
	}

	// Loading indicator
	if m.loading {
		b.WriteString(statusStartingStyle.Render("Loading port information..."))
		b.WriteString("\n")
	} else {
		// Table header
		header := fmt.Sprintf("%-7s %-25s %-8s %-12s %-8s %s",
			"Port", "Service", "Type", "Status", "PID", "Process")
		b.WriteString(portTableHeaderStyle.Render(header))
		b.WriteString("\n")

		// Separator
		b.WriteString(strings.Repeat("─", 100))
		b.WriteString("\n")

		// Port list
		if len(m.ports) == 0 {
			b.WriteString(helpStyle.Render("No ports configured"))
			b.WriteString("\n")
		} else {
			for i, port := range m.ports {
				cursor := "  "
				if m.cursor == i {
					cursor = cursorStyle.Render("▶ ")
				}

				// Format status with color
				statusText := m.formatStatus(port.Status)

				// Format PID
				pidText := "-"
				if port.PID > 0 {
					pidText = fmt.Sprintf("%d", port.PID)
				}

				// Truncate service name and process info if too long
				serviceName := port.ServiceName
				if len(serviceName) > 24 {
					serviceName = serviceName[:21] + "..."
				}

				processInfo := port.ProcessInfo
				if len(processInfo) > 40 {
					processInfo = processInfo[:37] + "..."
				}

				line := fmt.Sprintf("%s%-7d %-25s %-8s %-12s %-8s %s",
					cursor,
					port.Port,
					serviceName,
					port.Type,
					statusText,
					pidText,
					processInfo,
				)

				b.WriteString(line)
				b.WriteString("\n")

				// Show error if present
				if port.Error != "" {
					b.WriteString(fmt.Sprintf("     %s", statusErrorStyle.Render("Error: "+port.Error)))
					b.WriteString("\n")
				}
			}
		}
	}

	// Help text
	b.WriteString("\n")
	helpText := "↑↓/jk:nav • K:kill • r:refresh • esc:back"
	b.WriteString(helpStyle.Render(helpText))

	return b.String()
}

func (m PortCheckerModel) renderKillConfirmation() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Confirm Kill Process"))
	b.WriteString("\n\n")

	if m.killStatus == PortStatusKubefwd {
		b.WriteString(statusErrorStyle.Render(fmt.Sprintf(
			"⚠ Kill kubefwd process (PID %d)?\n\nService: %s\n\nThis will stop the port forward.",
			m.killPID, m.killService)))
	} else {
		b.WriteString(statusErrorStyle.Render(fmt.Sprintf(
			"⚠ Kill external process (PID %d)?\n\nService: %s\n\nThis process is not managed by kubefwd.",
			m.killPID, m.killService)))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("y:confirm • n:cancel"))

	// Center the modal
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Background(lipgloss.Color("235")).
		MaxWidth(60)

	modal := modalStyle.Render(b.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			modal,
		)
	}

	return modal
}

func (m PortCheckerModel) formatStatus(status PortStatus) string {
	switch status {
	case PortStatusFree:
		return portStatusFreeStyle.Render("✓ FREE")
	case PortStatusKubefwd:
		return portStatusKubefwdStyle.Render("● KUBEFWD")
	case PortStatusExternal:
		return portStatusExternalStyle.Render("⚠ EXTERNAL")
	default:
		return portStatusFreeStyle.Render("? UNKNOWN")
	}
}
