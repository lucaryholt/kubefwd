package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Badge creates a styled badge with text
func Badge(text string, badgeType string) string {
	style := GetBadgeStyle(badgeType)
	return style.Render(text)
}

// Button creates a styled button with text and hotkey
func Button(text string, hotkey string, buttonType string) string {
	style := GetButtonStyle(buttonType)
	
	hotkeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Underline(true)
	
	// Format hotkey in brackets
	if hotkey != "" {
		formattedText := fmt.Sprintf("%s %s", hotkeyStyle.Render(hotkey), text)
		return style.Render(formattedText)
	}
	
	return style.Render(text)
}

// Card creates a styled card container with optional title
func Card(title string, content string, width int) string {
	var b strings.Builder
	
	if title != "" {
		titleStyle := StyleH3.Copy().MarginBottom(1)
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n")
	}
	
	b.WriteString(content)
	
	cardStyle := StyleCard.Copy()
	if width > 0 {
		cardStyle = cardStyle.Width(width)
	}
	
	return cardStyle.Render(b.String())
}

// StatusIndicator creates a status indicator symbol with optional text
func StatusIndicator(status PortForwardStatus, retrying bool, retryAttempt int, maxRetries int) string {
	if retrying {
		retryText := fmt.Sprintf("↻ %d", retryAttempt)
		if maxRetries != -1 {
			retryText += fmt.Sprintf("/%d", maxRetries)
		}
		return StyleStatusRetrying.Render(retryText)
	}
	
	switch status {
	case StatusRunning:
		return StyleStatusRunning.Render("●")
	case StatusStarting:
		return StyleStatusStarting.Render("◐")
	case StatusError:
		return StyleStatusError.Render("✗")
	case StatusStopped:
		return StyleStatusStopped.Render("○")
	default:
		return StyleStatusStopped.Render("?")
	}
}

// StatusBadge creates a pill-shaped status badge with text
func StatusBadge(status PortForwardStatus, retrying bool) string {
	var text string
	var badgeType string
	
	if retrying {
		text = "RETRYING"
		badgeType = "warning"
	} else {
		switch status {
		case StatusRunning:
			text = "RUNNING"
			badgeType = "success"
		case StatusStarting:
			text = "STARTING"
			badgeType = "warning"
		case StatusError:
			text = "ERROR"
			badgeType = "error"
		case StatusStopped:
			text = "STOPPED"
			badgeType = "default"
		default:
			text = "UNKNOWN"
			badgeType = "default"
		}
	}
	
	return Badge(text, badgeType)
}

// ServiceLine creates a formatted service line with cursor, status, and info
type ServiceLineOptions struct {
	Cursor          string
	IsDefault       bool
	HasOverrides    bool
	Name            string
	Status          PortForwardStatus
	Retrying        bool
	RetryAttempt    int
	MaxRetries      int
	LocalPort       int
	ServiceName     string
	RemotePort      int
	ShowBadge       bool  // Show badge instead of symbol
}

func ServiceLine(opts ServiceLineOptions) string {
	var parts []string
	
	// Cursor
	if opts.Cursor != "" {
		parts = append(parts, StyleCursor.Render(opts.Cursor))
	} else {
		parts = append(parts, "  ")
	}
	
	// Default indicator
	if opts.IsDefault {
		parts = append(parts, StyleHighlight.Render("★"))
	} else {
		parts = append(parts, " ")
	}
	
	// Override indicator
	if opts.HasOverrides {
		parts = append(parts, StyleBodySecondary.Render("⚙"))
	} else {
		parts = append(parts, " ")
	}
	
	// Service name (left-aligned, fixed width)
	nameStyle := StyleBodyPrimary.Copy().Width(20).Align(lipgloss.Left)
	parts = append(parts, nameStyle.Render(opts.Name))
	
	// Status indicator or badge
	if opts.ShowBadge {
		parts = append(parts, StatusBadge(opts.Status, opts.Retrying))
	} else {
		parts = append(parts, StatusIndicator(opts.Status, opts.Retrying, opts.RetryAttempt, opts.MaxRetries))
	}
	
	// Port mapping
	portStyle := StyleBodyCode
	portInfo := fmt.Sprintf(":%d → %s:%d", opts.LocalPort, opts.ServiceName, opts.RemotePort)
	parts = append(parts, portStyle.Render(portInfo))
	
	return strings.Join(parts, " ")
}

// QuickActionBar creates a bar with quick action buttons
func QuickActionBar(buttons []struct{ Label, Hotkey, Type string }) string {
	var buttonStrs []string
	
	for _, btn := range buttons {
		buttonStrs = append(buttonStrs, Button(btn.Label, btn.Hotkey, btn.Type))
	}
	
	return strings.Join(buttonStrs, " ")
}

// InfoRow creates a key-value info row
func InfoRow(key string, value string) string {
	keyStyle := StyleBodySecondary.Copy().Width(15).Align(lipgloss.Right)
	valueStyle := StyleBodyPrimary
	
	return keyStyle.Render(key+":") + " " + valueStyle.Render(value)
}

// ClusterInfo creates a formatted cluster information section
func ClusterInfo(clusterContext string, clusterName string, namespace string) string {
	var lines []string
	
	clusterDisplay := clusterContext
	if clusterName != "" {
		clusterDisplay = clusterName + " " + StyleDim.Render("("+clusterContext+")")
	}
	
	lines = append(lines, InfoRow("Cluster", clusterDisplay))
	lines = append(lines, InfoRow("Namespace", namespace))
	
	return strings.Join(lines, "\n")
}

// Header creates a section header with optional count
func Header(title string, count int, total int) string {
	var parts []string
	
	parts = append(parts, StyleH2.Render(title))
	
	if count > 0 || total > 0 {
		countText := fmt.Sprintf("[%d/%d Running]", count, total)
		countStyle := StyleBodySecondary
		if count > 0 {
			countStyle = StyleStatusRunning
		}
		parts = append(parts, countStyle.Render(countText))
	}
	
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

// HelpText creates formatted help text with shortcuts
func HelpText(shortcuts []string) string {
	return StyleHelp.Render(strings.Join(shortcuts, " • "))
}

// ErrorMessage creates a styled error message with wrapping
func ErrorMessage(message string, width int) string {
	if width <= 0 {
		width = 60
	}
	
	style := StyleStatusError.Copy().Width(width)
	return style.Render(message)
}

// SuccessMessage creates a styled success message
func SuccessMessage(message string) string {
	return StyleStatusRunning.Render(message)
}

// WarningMessage creates a styled warning message
func WarningMessage(message string) string {
	return StyleStatusStarting.Render(message)
}

// List creates a simple bullet list
func List(items []string) string {
	var lines []string
	
	bulletStyle := StyleBodySecondary
	itemStyle := StyleBodyPrimary
	
	for _, item := range items {
		line := bulletStyle.Render("  • ") + itemStyle.Render(item)
		lines = append(lines, line)
	}
	
	return strings.Join(lines, "\n")
}

// KeyValueList creates a list of key-value pairs
func KeyValueList(items map[string]string) string {
	var lines []string
	
	// Calculate max key length for alignment
	maxKeyLen := 0
	for key := range items {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}
	}
	
	keyStyle := StyleBodySecondary.Copy().Width(maxKeyLen + 2).Align(lipgloss.Right)
	valueStyle := StyleBodyPrimary
	
	for key, value := range items {
		line := keyStyle.Render(key+":") + " " + valueStyle.Render(value)
		lines = append(lines, line)
	}
	
	return strings.Join(lines, "\n")
}

// EmptyState creates a centered empty state message
func EmptyState(message string, hint string) string {
	var b strings.Builder
	
	messageStyle := StyleBodySecondary.Copy().Bold(true)
	b.WriteString(messageStyle.Render(message))
	b.WriteString("\n\n")
	
	if hint != "" {
		hintStyle := StyleBodySecondary
		b.WriteString(hintStyle.Render(hint))
	}
	
	return b.String()
}

// LoadingSpinner creates a simple loading indicator
func LoadingSpinner(text string) string {
	spinnerStyle := StyleStatusStarting
	textStyle := StyleBodySecondary
	
	return spinnerStyle.Render("◐") + " " + textStyle.Render(text)
}

// Checkbox creates a checkbox indicator
func Checkbox(checked bool, label string) string {
	var checkbox string
	if checked {
		checkbox = StyleCheckbox.Render("[✓]")
	} else {
		checkbox = StyleBodySecondary.Render("[ ]")
	}
	
	return checkbox + " " + StyleBodyPrimary.Render(label)
}

// Modal creates a modal dialog container
func Modal(title string, content string, width int, height int) string {
	var b strings.Builder
	
	// Title
	if title != "" {
		titleStyle := StyleH2.Copy().Align(lipgloss.Center)
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n\n")
	}
	
	// Content
	b.WriteString(content)
	
	// Apply modal styling
	modalStyle := StyleModal.Copy()
	if width > 0 {
		modalStyle = modalStyle.Width(width)
	}
	if height > 0 {
		modalStyle = modalStyle.Height(height)
	}
	
	return modalStyle.Render(b.String())
}

// CenterContent centers content in the available space
func CenterContent(content string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return content
	}
	
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

// TruncateString truncates a string to the specified length with ellipsis
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}

// WrapText wraps text to a maximum width, breaking at spaces
func WrapText(text string, width int) []string {
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
