package init

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	promptStyle = lipgloss.NewStyle().Bold(true)
	defaultHint = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type textInputModel struct {
	label        string
	defaultValue string
	value        string
	done         bool
	aborted      bool
}

func newTextInputModel(label, defaultValue string) textInputModel {
	return textInputModel{
		label:        label,
		defaultValue: defaultValue,
		value:        defaultValue,
	}
}

func (m textInputModel) Init() tea.Cmd { return nil }

func (m textInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.aborted = true
			m.done = true
			return m, tea.Quit
		case "backspace":
			if len(m.value) > 0 {
				m.value = m.value[:len(m.value)-1]
			}
		case "ctrl+u":
			m.value = ""
		default:
			if len(msg.String()) == 1 {
				m.value += msg.String()
			}
		}
	}
	return m, nil
}

func (m textInputModel) View() string {
	if m.done {
		return ""
	}
	cursor := cursorStyle.Render("▸ ")
	label := promptStyle.Render(m.label + ": ")
	hint := ""
	if m.defaultValue != "" && m.value == m.defaultValue {
		hint = defaultHint.Render("  (enter to accept)")
	}
	return fmt.Sprintf("%s%s%s%s\n", cursor, label, m.value, hint)
}

// RunTextInput presents a single-line text input with a default value.
// Returns the entered text, or the default if unchanged.
func RunTextInput(label, defaultValue string) (string, error) {
	m := newTextInputModel(label, defaultValue)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("text input: %w", err)
	}
	result := finalModel.(textInputModel)
	if result.aborted {
		return "", fmt.Errorf("aborted")
	}
	return result.value, nil
}
