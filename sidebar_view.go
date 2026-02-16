package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// SidebarSection represents a navigation section in the sidebar
type SidebarSection int

const (
	SectionPortForwards SidebarSection = iota
	SectionPresets
	SectionSqlTap
	SectionPortChecker
	SectionConfiguration
	SectionContextSwitcher
)

// SidebarItem represents an item in the sidebar
type SidebarItem struct {
	Section     SidebarSection
	Label       string
	Hotkey      string
	Enabled     bool
}

// SidebarModel represents the sidebar state
type SidebarModel struct {
	items          []SidebarItem
	sections       []string
	cursor         int
	activeSection  SidebarSection
	config         *Config
}

// NewSidebarModel creates a new sidebar model
func NewSidebarModel(config *Config) SidebarModel {
	// Build items based on config
	items := []SidebarItem{
		{Section: SectionPortForwards, Label: "Services", Hotkey: "1", Enabled: true},
	}
	
	// Conditionally add items based on config
	if len(config.Presets) > 0 {
		items = append(items, SidebarItem{Section: SectionPresets, Label: "Presets", Hotkey: "2", Enabled: true})
	}
	
	items = append(items, SidebarItem{Section: SectionSqlTap, Label: "SQL Tap", Hotkey: "3", Enabled: true})
	items = append(items, SidebarItem{Section: SectionPortChecker, Label: "Port Checker", Hotkey: "4", Enabled: true})
	items = append(items, SidebarItem{Section: SectionConfiguration, Label: "Configuration", Hotkey: "5", Enabled: true})
	
	if len(config.AlternativeContexts) > 0 {
		items = append(items, SidebarItem{Section: SectionContextSwitcher, Label: "Context Switcher", Hotkey: "6", Enabled: true})
	}
	
	sections := []string{"SERVICES", "TOOLS", "SETTINGS"}
	
	return SidebarModel{
		items:         items,
		sections:      sections,
		cursor:        0,
		activeSection: SectionPortForwards,
		config:        config,
	}
}

func (m SidebarModel) Init() tea.Cmd {
	return nil
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	return m, nil
}

// GetActiveSection returns the currently active section
func (m SidebarModel) GetActiveSection() SidebarSection {
	return m.activeSection
}

// SetActiveSection sets the active section
func (m *SidebarModel) SetActiveSection(section SidebarSection) {
	m.activeSection = section
	
	// Update cursor to match active section
	for i, item := range m.items {
		if item.Section == section {
			m.cursor = i
			break
		}
	}
}

// IsVisible returns whether the sidebar is visible (always true now)
func (m SidebarModel) IsVisible() bool {
	return true
}

// MoveCursor moves the cursor up or down
func (m *SidebarModel) MoveCursor(delta int) {
	m.cursor += delta
	
	// Wrap around
	if m.cursor < 0 {
		m.cursor = len(m.items) - 1
	} else if m.cursor >= len(m.items) {
		m.cursor = 0
	}
	
	// Skip disabled items
	maxAttempts := len(m.items)
	for attempts := 0; attempts < maxAttempts; attempts++ {
		if m.items[m.cursor].Enabled {
			break
		}
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = len(m.items) - 1
		} else if m.cursor >= len(m.items) {
			m.cursor = 0
		}
	}
}

// SelectCurrentItem selects the current item under the cursor
func (m *SidebarModel) SelectCurrentItem() SidebarSection {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		m.activeSection = m.items[m.cursor].Section
		return m.activeSection
	}
	return m.activeSection
}

// HandleKey handles keyboard input for the sidebar
func (m *SidebarModel) HandleKey(key string) (bool, SidebarSection) {
	switch key {
	case "up", "k":
		m.MoveCursor(-1)
		return false, m.activeSection
	case "down", "j":
		m.MoveCursor(1)
		return false, m.activeSection
	case "enter":
		section := m.SelectCurrentItem()
		return true, section
	default:
		// Check hotkeys (number keys 1-7)
		for _, item := range m.items {
			if item.Enabled && item.Hotkey == key {
				m.activeSection = item.Section
				return true, item.Section
			}
		}
	}
	
	return false, m.activeSection
}

// View renders the sidebar
func (m SidebarModel) View() string {
	var b strings.Builder
	
	// Title
	b.WriteString(StyleH2.Render("kubefwd"))
	b.WriteString("\n")
	
	// Determine which section each item belongs to
	sectionGroups := map[string][]SidebarItem{
		"SERVICES":  {},
		"TOOLS":     {},
		"SETTINGS":  {},
	}
	
	for _, item := range m.items {
		switch item.Section {
		case SectionPortForwards, SectionPresets:
			sectionGroups["SERVICES"] = append(sectionGroups["SERVICES"], item)
		case SectionSqlTap, SectionPortChecker:
			sectionGroups["TOOLS"] = append(sectionGroups["TOOLS"], item)
		case SectionConfiguration, SectionContextSwitcher:
			sectionGroups["SETTINGS"] = append(sectionGroups["SETTINGS"], item)
		}
	}
	
	// Render sections
	firstSection := true
	for _, sectionName := range []string{"SERVICES", "TOOLS", "SETTINGS"} {
		items := sectionGroups[sectionName]
		if len(items) == 0 {
			continue
		}
		
		if !firstSection {
			b.WriteString("\n")
		}
		firstSection = false
		
		// Section header
		b.WriteString(StyleSidebarSection.Render(sectionName))
		b.WriteString("\n")
		
		// Section items
		for _, item := range items {
			if !item.Enabled {
				continue
			}
			
			// Check if this item is active
			isActive := item.Section == m.activeSection
			
			// Build line with consistent spacing (no cursor)
			line := "  "
			
			// Item label with hotkey
			itemLabel := item.Label
			if item.Hotkey != "" {
				hotkeyStyle := StyleDim
				if isActive {
					hotkeyStyle = StyleHighlight
				}
				itemLabel = fmt.Sprintf("%s %s", hotkeyStyle.Render("["+item.Hotkey+"]"), item.Label)
			}
			
			if isActive {
				line += StyleSidebarItemActive.Render(itemLabel)
			} else {
				line += StyleSidebarItem.Render(itemLabel)
			}
			
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	
	return b.String()
}

// GetSectionName returns the display name for a section
func GetSectionName(section SidebarSection) string {
	switch section {
	case SectionPortForwards:
		return "Services"
	case SectionPresets:
		return "Presets"
	case SectionSqlTap:
		return "SQL Tap"
	case SectionPortChecker:
		return "Port Checker"
	case SectionConfiguration:
		return "Configuration"
	case SectionContextSwitcher:
		return "Context Switcher"
	default:
		return "Unknown"
	}
}

// GetSectionScreen returns the corresponding screen type for a section
func GetSectionScreen(section SidebarSection) ScreenType {
	switch section {
	case SectionPortForwards:
		return ScreenManagement
	case SectionPresets:
		return ScreenPresetSelection
	case SectionSqlTap:
		return ScreenSqlTapSelection
	case SectionPortChecker:
		return ScreenPortChecker
	case SectionConfiguration:
		return ScreenConfig
	case SectionContextSwitcher:
		return ScreenContextSelection
	default:
		return ScreenManagement
	}
}
