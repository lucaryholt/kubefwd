package main

import (
	"github.com/charmbracelet/lipgloss"
)

// Professional color palette - refined blues and grays
var (
	// Primary colors
	ColorPrimaryBlue    = lipgloss.AdaptiveColor{Light: "#5B9BD5", Dark: "#5B9BD5"}
	ColorSecondaryBlue  = lipgloss.AdaptiveColor{Light: "#4A7FAD", Dark: "#6BA8D9"}
	ColorAccentGreen    = lipgloss.AdaptiveColor{Light: "#70AD47", Dark: "#85C95C"}
	ColorAccentAmber    = lipgloss.AdaptiveColor{Light: "#F4B942", Dark: "#F5C564"}
	ColorAccentRed      = lipgloss.AdaptiveColor{Light: "#E74C3C", Dark: "#EC6A5E"}
	
	// Neutral colors
	ColorNeutralGray    = lipgloss.AdaptiveColor{Light: "#6C757D", Dark: "#A8B2BD"}
	ColorDimGray        = lipgloss.AdaptiveColor{Light: "#ADB5BD", Dark: "#6C757D"}
	ColorBorderGray     = lipgloss.AdaptiveColor{Light: "#DEE2E6", Dark: "#495057"}
	
	// Background colors
	ColorBackgroundDark  = lipgloss.AdaptiveColor{Light: "#E9ECEF", Dark: "#2C3E50"}
	ColorBackgroundLight = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1A252F"}
	ColorBackgroundPanel = lipgloss.AdaptiveColor{Light: "#F8F9FA", Dark: "#212B36"}
	
	// Status colors (consistent with current behavior)
	ColorStatusRunning  = ColorAccentGreen
	ColorStatusStopped  = ColorDimGray
	ColorStatusStarting = ColorAccentAmber
	ColorStatusError    = ColorAccentRed
	ColorStatusRetrying = ColorAccentAmber
)

// Typography styles - clear hierarchy
var (
	// Heading styles
	StyleH1 = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimaryBlue).
		MarginBottom(1)
	
	StyleH2 = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondaryBlue).
		MarginBottom(1)
	
	StyleH3 = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorNeutralGray)
	
	// Body text styles
	StyleBodyPrimary = lipgloss.NewStyle().
		Foreground(ColorNeutralGray)
	
	StyleBodySecondary = lipgloss.NewStyle().
		Foreground(ColorDimGray)
	
	StyleBodyCode = lipgloss.NewStyle().
		Foreground(ColorSecondaryBlue)
	
	// Status styles
	StyleStatusRunning = lipgloss.NewStyle().
		Foreground(ColorStatusRunning).
		Bold(true)
	
	StyleStatusStopped = lipgloss.NewStyle().
		Foreground(ColorStatusStopped)
	
	StyleStatusStarting = lipgloss.NewStyle().
		Foreground(ColorStatusStarting)
	
	StyleStatusError = lipgloss.NewStyle().
		Foreground(ColorStatusError).
		Bold(true)
	
	StyleStatusRetrying = lipgloss.NewStyle().
		Foreground(ColorStatusRetrying).
		Bold(true)
	
	// UI element styles
	StyleCursor = lipgloss.NewStyle().
		Foreground(ColorPrimaryBlue).
		Bold(true)
	
	StyleHelp = lipgloss.NewStyle().
		Foreground(ColorDimGray)
	
	StyleDim = lipgloss.NewStyle().
		Foreground(ColorDimGray)
	
	StyleHighlight = lipgloss.NewStyle().
		Foreground(ColorPrimaryBlue).
		Bold(true)
	
	StyleCheckbox = lipgloss.NewStyle().
		Foreground(ColorAccentGreen)
	
	// Border styles
	StyleBorder = lipgloss.NewStyle().
		BorderForeground(ColorBorderGray)
	
	StyleBorderActive = lipgloss.NewStyle().
		BorderForeground(ColorPrimaryBlue)
)

// Sidebar styles
var (
	StyleSidebarSection = lipgloss.NewStyle().
		Foreground(ColorDimGray).
		Bold(true).
		MarginTop(1)
	
	StyleSidebarItem = lipgloss.NewStyle().
		Foreground(ColorNeutralGray).
		PaddingLeft(2)
	
	StyleSidebarItemActive = lipgloss.NewStyle().
		Foreground(ColorPrimaryBlue).
		Bold(true).
		PaddingLeft(2)
	
	StyleSidebarContainer = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderForeground(ColorBorderGray).
		PaddingTop(1).
		PaddingBottom(1).
		PaddingLeft(1).
		PaddingRight(1)
)

// Panel and container styles
var (
	StylePanel = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorderGray).
		Padding(1, 2)
	
	StylePanelActive = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryBlue).
		Padding(1, 2)
	
	StyleCard = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorderGray).
		Background(ColorBackgroundPanel).
		Padding(1, 2).
		MarginBottom(1)
	
	StyleModal = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryBlue).
		Background(ColorBackgroundPanel).
		Padding(1, 2)
)

// Table styles
var (
	StyleTableHeader = lipgloss.NewStyle().
		Foreground(ColorPrimaryBlue).
		Bold(true).
		Underline(true)
	
	StyleTableCell = lipgloss.NewStyle().
		Foreground(ColorNeutralGray)
	
	StyleTableCellDim = lipgloss.NewStyle().
		Foreground(ColorDimGray)
)

// Badge styles (for status indicators, tags, etc.)
func GetBadgeStyle(badgeType string) lipgloss.Style {
	base := lipgloss.NewStyle().
		Padding(0, 1).
		MarginRight(1)
	
	switch badgeType {
	case "success", "running":
		return base.
			Foreground(ColorAccentGreen).
			Background(lipgloss.AdaptiveColor{Light: "#D4EDDA", Dark: "#1B3A2A"})
	case "warning", "starting":
		return base.
			Foreground(ColorAccentAmber).
			Background(lipgloss.AdaptiveColor{Light: "#FFF3CD", Dark: "#3D3522"})
	case "error":
		return base.
			Foreground(ColorAccentRed).
			Background(lipgloss.AdaptiveColor{Light: "#F8D7DA", Dark: "#3A2022"})
	case "info":
		return base.
			Foreground(ColorPrimaryBlue).
			Background(lipgloss.AdaptiveColor{Light: "#D1ECF1", Dark: "#1C2F3A"})
	case "default":
		return base.
			Foreground(ColorDimGray).
			Background(lipgloss.AdaptiveColor{Light: "#E9ECEF", Dark: "#343A40"})
	default:
		return base.
			Foreground(ColorNeutralGray).
			Background(ColorBackgroundPanel)
	}
}

// Button styles
func GetButtonStyle(buttonType string) lipgloss.Style {
	base := lipgloss.NewStyle().
		Padding(0, 2).
		MarginRight(1)
	
	switch buttonType {
	case "primary":
		return base.
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorPrimaryBlue).
			Bold(true)
	case "success":
		return base.
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorAccentGreen).
			Bold(true)
	case "danger":
		return base.
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorAccentRed).
			Bold(true)
	case "secondary":
		return base.
			Foreground(ColorNeutralGray).
			Background(ColorBackgroundDark)
	default:
		return base.
			Foreground(ColorNeutralGray).
			Background(ColorBackgroundPanel)
	}
}

// Divider creates a horizontal divider line
func Divider(width int) string {
	if width <= 0 {
		width = 80
	}
	style := lipgloss.NewStyle().
		Foreground(ColorBorderGray)
	return style.Render(lipgloss.NewStyle().Width(width).Render("─"))
}

// SectionDivider creates a labeled section divider
func SectionDivider(label string, width int) string {
	if width <= 0 {
		width = 80
	}
	
	if label == "" {
		return Divider(width)
	}
	
	labelStyle := lipgloss.NewStyle().
		Foreground(ColorDimGray).
		Bold(true).
		Padding(0, 1)
	
	labelWidth := lipgloss.Width(labelStyle.Render(label))
	lineWidth := (width - labelWidth) / 2
	
	if lineWidth < 0 {
		lineWidth = 0
	}
	
	leftLine := lipgloss.NewStyle().
		Foreground(ColorBorderGray).
		Render(lipgloss.NewStyle().Width(lineWidth).Render("─"))
	
	rightLine := lipgloss.NewStyle().
		Foreground(ColorBorderGray).
		Render(lipgloss.NewStyle().Width(width - labelWidth - lineWidth).Render("─"))
	
	return leftLine + labelStyle.Render(label) + rightLine
}
