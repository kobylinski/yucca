package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	btnNormal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("238")).
			Padding(0, 2).
			Margin(0, 1)

	btnFocused = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorPrimary).
			Padding(0, 2).
			Margin(0, 1).
			Bold(true)

	btnDanger = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorDanger).
			Padding(0, 2).
			Margin(0, 1).
			Bold(true)
)

type buttonKind int

const (
	btnKindNormal buttonKind = iota
	btnKindDanger
)

type buttonDef struct {
	label string
	kind  buttonKind
}

type buttonBar struct {
	buttons []buttonDef
	active  int
	focused bool
}

func newButtonBar(buttons ...buttonDef) buttonBar {
	return buttonBar{buttons: buttons}
}

func (b *buttonBar) focus() {
	b.focused = true
	b.active = 0
}

func (b *buttonBar) blur() {
	b.focused = false
}

// update handles key input when the button bar is focused.
// Returns the label of the activated button, or "".
func (b *buttonBar) update(msg tea.KeyMsg) string {
	if !b.focused {
		return ""
	}
	switch msg.String() {
	case "left", "h":
		if b.active > 0 {
			b.active--
		}
	case "right", "l", "tab":
		if b.active < len(b.buttons)-1 {
			b.active++
		}
	case "enter":
		if b.active < len(b.buttons) {
			return b.buttons[b.active].label
		}
	}
	return ""
}

func (b buttonBar) view() string {
	var parts []string
	for i, btn := range b.buttons {
		style := btnNormal
		if b.focused && i == b.active {
			if btn.kind == btnKindDanger {
				style = btnDanger
			} else {
				style = btnFocused
			}
		}
		parts = append(parts, style.Render(btn.label))
	}
	return strings.Join(parts, "")
}
