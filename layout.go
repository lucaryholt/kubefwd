package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// LayoutWithSidebar creates a two-column layout with sidebar and main content
type LayoutWithSidebar struct {
	SidebarContent string
	MainContent    string
	Width          int
	Height         int
	SidebarVisible bool
	SidebarWidth   int
}

// Render renders the layout with sidebar and main content
func (l LayoutWithSidebar) Render() string {
	if !l.SidebarVisible {
		// If sidebar is hidden, just show main content
		return l.MainContent
	}
	
	// Calculate dimensions
	sidebarWidth := l.SidebarWidth
	if sidebarWidth <= 0 {
		// Default to 25% of width, with min/max constraints
		sidebarWidth = l.Width / 4
		if sidebarWidth < 20 {
			sidebarWidth = 20
		}
		if sidebarWidth > 40 {
			sidebarWidth = 40
		}
	}
	
	// Style sidebar
	sidebarStyle := StyleSidebarContainer.Copy().
		Width(sidebarWidth).
		Height(l.Height)
	
	// Style main content area - just add padding, let content control width
	mainStyle := lipgloss.NewStyle().
		Height(l.Height).
		PaddingLeft(2).
		PaddingRight(2).
		PaddingTop(1)
	
	sidebar := sidebarStyle.Render(l.SidebarContent)
	main := mainStyle.Render(l.MainContent)
	
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
}

// LayoutSplit creates a split layout (left/right or top/bottom)
type LayoutSplit struct {
	LeftContent  string
	RightContent string
	Width        int
	Height       int
	SplitRatio   float64 // 0.0 to 1.0, percentage for left/top pane
	Vertical     bool    // true for left/right, false for top/bottom
}

// Render renders the split layout
func (l LayoutSplit) Render() string {
	// If Width is 0, just join content side by side without strict width constraints
	// This allows the parent layout to control sizing
	if l.Width <= 0 {
		// Simple side-by-side without width constraints
		if l.Vertical {
			// Add a border between them
			borderStyle := lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(ColorBorderGray).
				PaddingLeft(2)
			
			right := borderStyle.Render(l.RightContent)
			return lipgloss.JoinHorizontal(lipgloss.Top, l.LeftContent, right)
		} else {
			// Top/bottom without height constraints
			borderStyle := lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(true).
				BorderForeground(ColorBorderGray).
				PaddingTop(1)
			
			bottom := borderStyle.Render(l.RightContent)
			return lipgloss.JoinVertical(lipgloss.Left, l.LeftContent, bottom)
		}
	}
	
	if l.SplitRatio <= 0 || l.SplitRatio >= 1 {
		l.SplitRatio = 0.5 // Default to 50/50
	}
	
	if l.Vertical {
		// Left/Right split
		leftWidth := int(float64(l.Width) * l.SplitRatio)
		rightWidth := l.Width - leftWidth
		
		leftStyle := lipgloss.NewStyle().
			Width(leftWidth).
			Height(l.Height)
		
		rightStyle := lipgloss.NewStyle().
			Width(rightWidth).
			Height(l.Height).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(ColorBorderGray).
			PaddingLeft(2)
		
		left := leftStyle.Render(l.LeftContent)
		right := rightStyle.Render(l.RightContent)
		
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	
	// Top/Bottom split
	topHeight := int(float64(l.Height) * l.SplitRatio)
	bottomHeight := l.Height - topHeight
	
	topStyle := lipgloss.NewStyle().
		Width(l.Width).
		Height(topHeight)
	
	bottomStyle := lipgloss.NewStyle().
		Width(l.Width).
		Height(bottomHeight).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(ColorBorderGray).
		PaddingTop(1)
	
	top := topStyle.Render(l.LeftContent)
	bottom := bottomStyle.Render(l.RightContent)
	
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

// ContentSection represents a content section with optional header and footer
type ContentSection struct {
	Header  string
	Content string
	Footer  string
	Width   int
}

// Render renders the content section
func (cs ContentSection) Render() string {
	var b strings.Builder
	
	if cs.Header != "" {
		b.WriteString(cs.Header)
		b.WriteString("\n\n")
	}
	
	b.WriteString(cs.Content)
	
	if cs.Footer != "" {
		b.WriteString("\n\n")
		b.WriteString(cs.Footer)
	}
	
	if cs.Width > 0 {
		style := lipgloss.NewStyle().Width(cs.Width)
		return style.Render(b.String())
	}
	
	return b.String()
}

// GridLayout creates a grid layout for items
type GridLayout struct {
	Items      []string
	Columns    int
	Width      int
	ItemWidth  int
	Spacing    int
}

// Render renders the grid layout
func (g GridLayout) Render() string {
	if len(g.Items) == 0 {
		return ""
	}
	
	if g.Columns <= 0 {
		g.Columns = 1
	}
	
	if g.Spacing < 0 {
		g.Spacing = 0
	}
	
	// Calculate item width if not specified
	itemWidth := g.ItemWidth
	if itemWidth <= 0 && g.Width > 0 {
		itemWidth = (g.Width - (g.Spacing * (g.Columns - 1))) / g.Columns
	}
	
	var rows []string
	var currentRow []string
	
	for i, item := range g.Items {
		// Style the item with fixed width
		if itemWidth > 0 {
			itemStyle := lipgloss.NewStyle().Width(itemWidth)
			item = itemStyle.Render(item)
		}
		
		currentRow = append(currentRow, item)
		
		// Check if we need to start a new row
		if (i+1)%g.Columns == 0 || i == len(g.Items)-1 {
			// Join items in row with spacing
			spacer := strings.Repeat(" ", g.Spacing)
			rowStr := strings.Join(currentRow, spacer)
			rows = append(rows, rowStr)
			currentRow = []string{}
		}
	}
	
	return strings.Join(rows, "\n")
}

// PanelLayout creates a panel with title and content
type PanelLayout struct {
	Title      string
	Content    string
	Width      int
	Height     int
	Active     bool
	Scrollable bool
}

// Render renders the panel
func (p PanelLayout) Render() string {
	var b strings.Builder
	
	// Title with separator
	if p.Title != "" {
		titleStyle := StyleH3.Copy()
		if p.Active {
			titleStyle = titleStyle.Foreground(ColorPrimaryBlue)
		}
		b.WriteString(titleStyle.Render(p.Title))
		b.WriteString("\n")
		
		// Add divider
		if p.Width > 0 {
			b.WriteString(Divider(p.Width - 4)) // Account for padding
		} else {
			b.WriteString(Divider(60))
		}
		b.WriteString("\n\n")
	}
	
	// Content
	b.WriteString(p.Content)
	
	// Apply panel style
	var panelStyle lipgloss.Style
	if p.Active {
		panelStyle = StylePanelActive.Copy()
	} else {
		panelStyle = StylePanel.Copy()
	}
	
	if p.Width > 0 {
		panelStyle = panelStyle.Width(p.Width)
	}
	if p.Height > 0 {
		panelStyle = panelStyle.Height(p.Height)
	}
	
	return panelStyle.Render(b.String())
}

// StatusBarLayout creates a status bar at the bottom
type StatusBarLayout struct {
	LeftContent   string
	CenterContent string
	RightContent  string
	Width         int
}

// Render renders the status bar
func (sb StatusBarLayout) Render() string {
	if sb.Width <= 0 {
		sb.Width = 80
	}
	
	leftWidth := len(sb.LeftContent)
	rightWidth := len(sb.RightContent)
	centerWidth := len(sb.CenterContent)
	
	// Calculate spacing
	availableSpace := sb.Width - leftWidth - rightWidth - centerWidth
	if availableSpace < 0 {
		availableSpace = 0
	}
	
	leftSpacing := availableSpace / 2
	rightSpacing := availableSpace - leftSpacing
	
	var parts []string
	
	if sb.LeftContent != "" {
		parts = append(parts, sb.LeftContent)
	}
	
	if sb.CenterContent != "" {
		if leftSpacing > 0 {
			parts = append(parts, strings.Repeat(" ", leftSpacing))
		}
		parts = append(parts, sb.CenterContent)
	}
	
	if sb.RightContent != "" {
		if rightSpacing > 0 {
			parts = append(parts, strings.Repeat(" ", rightSpacing))
		}
		parts = append(parts, sb.RightContent)
	}
	
	statusStyle := lipgloss.NewStyle().
		Width(sb.Width).
		Foreground(ColorDimGray).
		Background(ColorBackgroundDark).
		Padding(0, 1)
	
	return statusStyle.Render(strings.Join(parts, ""))
}

// TableLayout creates a simple table layout
type TableLayout struct {
	Headers     []string
	Rows        [][]string
	ColumnWidth []int
	Width       int
}

// Render renders the table
func (t TableLayout) Render() string {
	if len(t.Headers) == 0 || len(t.Rows) == 0 {
		return ""
	}
	
	// Calculate column widths if not specified
	if len(t.ColumnWidth) != len(t.Headers) {
		t.ColumnWidth = make([]int, len(t.Headers))
		
		if t.Width > 0 {
			// Distribute width evenly
			colWidth := t.Width / len(t.Headers)
			for i := range t.ColumnWidth {
				t.ColumnWidth[i] = colWidth
			}
		} else {
			// Use content-based width
			for i, header := range t.Headers {
				maxWidth := len(header)
				for _, row := range t.Rows {
					if i < len(row) && len(row[i]) > maxWidth {
						maxWidth = len(row[i])
					}
				}
				t.ColumnWidth[i] = maxWidth + 2 // Add padding
			}
		}
	}
	
	var b strings.Builder
	
	// Render headers
	var headerCells []string
	for i, header := range t.Headers {
		cellStyle := StyleTableHeader.Copy().
			Width(t.ColumnWidth[i]).
			Align(lipgloss.Left)
		headerCells = append(headerCells, cellStyle.Render(header))
	}
	b.WriteString(strings.Join(headerCells, " "))
	b.WriteString("\n")
	
	// Render rows
	for _, row := range t.Rows {
		var rowCells []string
		for i, cell := range row {
			if i >= len(t.ColumnWidth) {
				break
			}
			cellStyle := StyleTableCell.Copy().
				Width(t.ColumnWidth[i]).
				Align(lipgloss.Left)
			rowCells = append(rowCells, cellStyle.Render(cell))
		}
		b.WriteString(strings.Join(rowCells, " "))
		b.WriteString("\n")
	}
	
	return b.String()
}

// OverlayLayout creates an overlay (modal on top of content)
type OverlayLayout struct {
	BackgroundContent string
	OverlayContent    string
	Width             int
	Height            int
	OverlayWidth      int
	OverlayHeight     int
}

// Render renders the overlay
func (o OverlayLayout) Render() string {
	if o.Width <= 0 || o.Height <= 0 {
		return o.OverlayContent
	}
	
	// Center overlay on top of background
	overlay := CenterContent(o.OverlayContent, o.Width, o.Height)
	
	return overlay
}

// HelpLayout creates a help panel layout
type HelpLayout struct {
	Sections []HelpSection
	Width    int
}

type HelpSection struct {
	Title    string
	Shortcuts []HelpShortcut
}

type HelpShortcut struct {
	Key         string
	Description string
}

// Render renders the help layout
func (h HelpLayout) Render() string {
	var b strings.Builder
	
	for i, section := range h.Sections {
		if i > 0 {
			b.WriteString("\n\n")
		}
		
		// Section title
		sectionStyle := StyleH3.Copy().MarginBottom(1)
		b.WriteString(sectionStyle.Render(section.Title))
		b.WriteString("\n")
		
		// Shortcuts
		for _, shortcut := range section.Shortcuts {
			keyStyle := StyleHighlight.Copy().Width(15).Align(lipgloss.Right)
			descStyle := StyleBodyPrimary
			
			line := keyStyle.Render(shortcut.Key) + "  " + descStyle.Render(shortcut.Description)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	
	return b.String()
}
