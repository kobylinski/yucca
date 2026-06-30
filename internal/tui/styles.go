package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("12")  // blue
	colorMuted     = lipgloss.Color("240") // gray
	colorDanger    = lipgloss.Color("9")   // red
	colorHighlight = lipgloss.Color("11")  // yellow
	colorBorder    = lipgloss.Color("237") // dark gray

	// Pane styles
	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	paneFocusedStyle = paneStyle.
				BorderForeground(colorPrimary)

	paneDimmedStyle = paneStyle.
			Foreground(colorMuted)

	// Text styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	valueStyle = lipgloss.NewStyle().
			Bold(true)

	aliasStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	// List item styles — no backgrounds, just bold for selected
	selectedItemStyle = lipgloss.NewStyle().Bold(true)
	normalItemStyle   = lipgloss.NewStyle()

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), true, false, false, false).
			BorderForeground(colorBorder).
			Padding(0, 1)

	// Help text
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
)
