package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	serviceTypeDirectStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("33")).
		Bold(true)
	
	serviceTypeProxyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("170")).
		Bold(true)
)

// SqlTapItem represents a service with sql-tap enabled
type SqlTapItem struct {
	Name        string
	ServiceType string // "DIRECT" or "PROXY"
	GrpcPort    int
	Status      PortForwardStatus
}

// SqlTapSelectionModel represents the sql-tap selection screen state
type SqlTapSelectionModel struct {
	items     []SqlTapItem
	cursor    int
	launched  bool
	cancelled bool
}

// NewSqlTapSelectionModel creates a new sql-tap selection model
func NewSqlTapSelectionModel(portForwards []*PortForward, proxyForwards map[string]*ProxyForward) SqlTapSelectionModel {
	var items []SqlTapItem

	// Collect direct services with running sql-tap
	for _, pf := range portForwards {
		sqlTapMgr := pf.GetSqlTapManager()
		if sqlTapMgr.IsEnabled() {
			status, _ := sqlTapMgr.GetStatus()
			if status == StatusRunning {
				items = append(items, SqlTapItem{
					Name:        pf.Service.Name,
					ServiceType: "DIRECT",
					GrpcPort:    sqlTapMgr.GetGrpcPort(),
					Status:      status,
				})
			}
		}
	}

	// Collect proxy services with running sql-tap
	for _, pxf := range proxyForwards {
		sqlTapMgr := pxf.GetSqlTapManager()
		if sqlTapMgr.IsEnabled() {
			status, _ := sqlTapMgr.GetStatus()
			if status == StatusRunning {
				items = append(items, SqlTapItem{
					Name:        pxf.ProxyService.Name,
					ServiceType: "PROXY",
					GrpcPort:    sqlTapMgr.GetGrpcPort(),
					Status:      status,
				})
			}
		}
	}

	return SqlTapSelectionModel{
		items:     items,
		cursor:    0,
		launched:  false,
		cancelled: false,
	}
}

func (m SqlTapSelectionModel) Init() tea.Cmd {
	return nil
}

func (m SqlTapSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.items) > 0 {
				m.launched = true
				return m, nil
			}
		}
	}

	return m, nil
}

func (m SqlTapSelectionModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("kubefwd - Launch SQL-Tap"))
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("No sql-tap services are currently running."))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Start a service with sql-tap enabled first."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: back"))
		return b.String()
	}

	// Service list
	for i, item := range m.items {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render("▶ ")
		}

		// Format service type with color
		var serviceTypeText string
		if item.ServiceType == "DIRECT" {
			serviceTypeText = serviceTypeDirectStyle.Render("[DIRECT]")
		} else {
			serviceTypeText = serviceTypeProxyStyle.Render("[PROXY]")
		}

		// Status indicator
		statusText := statusRunningStyle.Render("●")

		line := fmt.Sprintf("%s%-25s %s %s gRPC:%d",
			cursor,
			item.Name,
			serviceTypeText,
			statusText,
			item.GrpcPort,
		)

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: launch • esc: back"))

	return b.String()
}

// GetSelectedItem returns the selected sql-tap item
func (m SqlTapSelectionModel) GetSelectedItem() SqlTapItem {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor]
	}
	return SqlTapItem{}
}
