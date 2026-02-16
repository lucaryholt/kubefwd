package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	b.WriteString(StyleH1.Render("Port Status Checker"))
	b.WriteString("\n\n")
	
	// Add description
	descText := StyleBodySecondary.Render("Check which ports are in use and identify processes that may be blocking your services.")
	b.WriteString(descText)
	b.WriteString("\n")
	descText2 := StyleBodySecondary.Render("You can kill conflicting processes directly from this screen.")
	b.WriteString(descText2)
	b.WriteString("\n\n")

	// Cluster info
	clusterInfo := ClusterInfo(m.config.ClusterContext, m.config.ClusterName, m.config.Namespace)
	b.WriteString(clusterInfo)
	b.WriteString("\n\n")

	// Error message
	if m.errorMessage != "" {
		b.WriteString(ErrorMessage(m.errorMessage, 80))
		b.WriteString("\n\n")
	}

	// Loading indicator
	if m.loading {
		b.WriteString(LoadingSpinner("Loading port information..."))
		b.WriteString("\n")
	} else {
		// Table header
		header := fmt.Sprintf("%-7s %-25s %-8s %-12s %-8s %s",
			"Port", "Service", "Type", "Status", "PID", "Process")
		b.WriteString(StyleTableHeader.Render(header))
		b.WriteString("\n")

		// Separator
		b.WriteString(Divider(100))
		b.WriteString("\n")

		// Port list
		if len(m.ports) == 0 {
			b.WriteString(EmptyState("No ports configured", ""))
			b.WriteString("\n")
		} else {
			for i, port := range m.ports {
				cursor := "  "
				if m.cursor == i {
					cursor = StyleCursor.Render("▶ ")
				}

				// Format status with color
				statusText := m.formatStatus(port.Status)

				// Format PID
				pidText := "-"
				if port.PID > 0 {
					pidText = fmt.Sprintf("%d", port.PID)
				}

				// Truncate service name and process info if too long
				serviceName := TruncateString(port.ServiceName, 24)
				processInfo := TruncateString(port.ProcessInfo, 40)

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
					b.WriteString(fmt.Sprintf("     %s", ErrorMessage("Error: "+port.Error, 80)))
					b.WriteString("\n")
				}
			}
		}
	}

	// Help text
	b.WriteString("\n")
	helpShortcuts := []string{"↑↓/jk: nav", "K: kill", "r: refresh", "esc: back"}
	b.WriteString(HelpText(helpShortcuts))

	return b.String()
}

func (m PortCheckerModel) renderKillConfirmation() string {
	var b strings.Builder

	b.WriteString(StyleH2.Render("Confirm Kill Process"))
	b.WriteString("\n\n")

	if m.killStatus == PortStatusKubefwd {
		b.WriteString(WarningMessage(fmt.Sprintf(
			"⚠ Kill kubefwd process (PID %d)?\n\nService: %s\n\nThis will stop the port forward.",
			m.killPID, m.killService)))
	} else {
		b.WriteString(WarningMessage(fmt.Sprintf(
			"⚠ Kill external process (PID %d)?\n\nService: %s\n\nThis process is not managed by kubefwd.",
			m.killPID, m.killService)))
	}

	b.WriteString("\n\n")
	helpShortcuts := []string{"y: confirm", "n: cancel"}
	b.WriteString(HelpText(helpShortcuts))

	// Use new modal component
	modal := Modal("Confirm Kill Process", b.String(), 60, 0)

	if m.width > 0 && m.height > 0 {
		return CenterContent(modal, m.width, m.height)
	}

	return modal
}

func (m PortCheckerModel) formatStatus(status PortStatus) string {
	switch status {
	case PortStatusFree:
		return Badge("FREE", "default")
	case PortStatusKubefwd:
		return Badge("KUBEFWD", "success")
	case PortStatusExternal:
		return Badge("EXTERNAL", "warning")
	default:
		return Badge("UNKNOWN", "default")
	}
}
